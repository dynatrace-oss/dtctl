package inventory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DefaultTimeField is the record field windowed probes read event time from
// when a capability definition does not override it.
const DefaultTimeField = "timestamp"

// evaluateSignals answers, per signal type, whether data is arriving for the
// caller's scope inside the window.
//
// Only stream- and metric-shaped definitions are signal types; entity-census
// and probe shapes have no meaningful "since" reading and are left to the
// unwindowed path.
//
// Probes are filter-first by necessity, not style: on a live tenant, folding
// the predicate into `summarize ... countIf(...)` scanned 205 GB against 10.8
// GB for the same question asked as `| filter ... | summarize count()`, and an
// unfiltered `summarize count()` scanned 204 GB. So the scope filter is pushed
// into the fetch, and the "is this stream live at all" half of the answer is
// taken from bucket metadata that costs nothing rather than a second probe.
func (b *budgetRunner) evaluateSignals(ctx context.Context, defs map[string]*CapabilityDef, facts discoveredFacts, opts DiscoverOptions) ([]Signal, error) {
	names := make([]string, 0, len(defs))
	for n := range defs {
		if !isSignalDef(defs[n]) {
			continue
		}
		if len(opts.Signals) > 0 && !containsFold(opts.Signals, n) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	signals := make([]Signal, 0, len(names))
	for _, name := range names {
		def := defs[name]
		var (
			sig  Signal
			serr error
		)
		if def.MetricKey != "" {
			sig, serr = b.probeMetricSignal(ctx, name, def, facts, opts, now())
		} else {
			sig, serr = b.probeStreamSignal(ctx, name, def, facts, opts, now())
		}
		if serr != nil {
			return nil, serr
		}
		signals = append(signals, sig)
	}
	return signals, nil
}

// probeStreamSignal evaluates one fetch-backed stream.
func (b *budgetRunner) probeStreamSignal(ctx context.Context, name string, def *CapabilityDef, facts discoveredFacts, opts DiscoverOptions, now time.Time) (Signal, error) {
	sig := Signal{Name: name}

	// A stream that is not in the catalog cannot be probed, and must not be:
	// the query would fail and the failure would read as a probe error rather
	// than the plain fact that this environment does not carry the stream.
	if !facts.objects[def.DataObject] {
		if facts.objectsTruncated {
			sig.State = SignalUnknown
			sig.Evidence = "not evaluated: data-object catalog truncated, so the stream's absence is not established"
			return sig, nil
		}
		sig.State = SignalAbsent
		sig.Evidence = "no " + def.DataObject + " in the data-object catalog"
		return sig, nil
	}

	timeField := def.TimeField
	if timeField == "" {
		timeField = DefaultTimeField
	}
	dql := fmt.Sprintf("fetch %s, from:%s | filter %s | summarize matched = count(), last_seen = takeMax(%s)",
		def.DataObject, opts.Since, opts.Where, timeField)

	res, err := b.run(ctx, dql)
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return sig, ctx.Err()
	case err == errBudgetExhausted:
		sig.State = SignalUnknown
		sig.Evidence = "not evaluated: discovery budget exhausted"
		return sig, nil
	default:
		sig.State = SignalUnknown
		sig.Evidence = "probe failed: " + firstLine(err.Error())
		return sig, nil
	}
	if res.Truncated {
		// A cut-short probe that found nothing proves nothing: the match may
		// sit in the part that was never read.
		sig.State = SignalUnknown
		sig.Evidence = "not evaluated: probe was cut short by a limit (scan cap, result cap, or read timeout) — narrow --since or raise the cap"
		return sig, nil
	}

	var matched int64
	var lastSeen string
	if len(res.Records) > 0 {
		matched = asInt64(res.Records[0]["matched"])
		lastSeen, _ = res.Records[0]["last_seen"].(string)
	}
	sig.Records = matched

	if matched == 0 {
		sig.State, sig.Evidence = emptyState(def.DataObject, facts)
		return sig, nil
	}
	applyFreshness(&sig, lastSeen, now, opts.StaleAfter)
	return sig, nil
}

// metricKeySampleSize bounds how many keys of a family get probed. Each probe
// is free (0 scanned bytes, ~25 ms), but a wide family can hold thousands of
// keys and the budget is finite.
const metricKeySampleSize = 8

// probeMetricSignal evaluates one metric family.
//
// Metrics are not fetchable, so the probe is a timeseries over concrete keys —
// `count(*)` is not expressible, and the keys cannot be combined into one
// query: a multi-aggregation timeseries returns *no rows at all* when any one
// of its keys has no data, so a single quiet key would erase the evidence of
// the live ones. Verified on a live tenant.
//
// One key is not a family, either: picking a single representative produced a
// confident "empty" for a namespace whose k8s metrics were demonstrably
// flowing, because the arbitrary pick happened to be a key nobody emits. So a
// bounded sample is probed, and a miss across the sample only becomes a real
// absence verdict when the sample was the whole family.
func (b *budgetRunner) probeMetricSignal(ctx context.Context, name string, def *CapabilityDef, facts discoveredFacts, opts DiscoverOptions, now time.Time) (Signal, error) {
	sig := Signal{Name: name}
	if !facts.metricsOK {
		sig.State = SignalUnknown
		sig.Evidence = "not evaluated: metric catalog unavailable"
		return sig, nil
	}
	matching := globMatches(def.MetricKey, facts.metricKeys)
	if len(matching) == 0 {
		if facts.metricsTruncated {
			sig.State = SignalUnknown
			sig.Evidence = "not evaluated: metric catalog truncated, so the absence of a key matching " + def.MetricKey + " is not established"
			return sig, nil
		}
		sig.State = SignalNoData
		sig.Evidence = "no metric keys matching " + def.MetricKey + " in the window"
		return sig, nil
	}
	sample := matching
	if len(sample) > metricKeySampleSize {
		sample = sample[:metricKeySampleSize]
	}

	for _, key := range sample {
		dql := fmt.Sprintf("timeseries n = count(%s), from:%s, filter: %s", key, opts.Since, opts.Where)
		res, err := b.run(ctx, dql)
		switch {
		case err == nil:
		case ctx.Err() != nil:
			return sig, ctx.Err()
		case err == errBudgetExhausted:
			sig.State = SignalUnknown
			sig.Evidence = "not evaluated: discovery budget exhausted"
			return sig, nil
		default:
			sig.State = SignalUnknown
			sig.Evidence = "probe failed: " + firstLine(err.Error())
			return sig, nil
		}
		points, lastSeen := summarizeSeries(res.Records)
		if points > 0 {
			// A hit settles the family: at least one of its metrics is
			// arriving for this scope.
			sig.Datapoints = points
			applyFreshness(&sig, lastSeen, now, opts.StaleAfter)
			return sig, nil
		}
	}

	// Nothing in the sample reported. That is only an absence claim if the
	// sample was the whole family — and only if the catalog it came from was
	// complete.
	if len(sample) < len(matching) || facts.metricsTruncated {
		sig.State = SignalUnknown
		sig.Evidence = fmt.Sprintf("not evaluated: no datapoints from %d of %d keys matching %s, which is a sample, not the family — probe a specific key with --signals to settle it",
			len(sample), len(matching), def.MetricKey)
		return sig, nil
	}
	sig.State = SignalEmpty
	sig.Evidence = fmt.Sprintf("all %d keys matching %s exist but reported no datapoints for this scope in the window",
		len(matching), def.MetricKey)
	return sig, nil
}

// emptyState distinguishes the two zero-match cases that matter. A stream that
// holds records within retention but matched nothing for this scope is the
// onboarding-failure signal; a stream that is empty tenant-wide is not about
// this source at all.
func emptyState(object string, facts discoveredFacts) (SignalState, string) {
	rows, covered := facts.streamRows[object]
	switch {
	case covered && rows == 0:
		return SignalNoData, object + " is in the catalog, but all its buckets are empty (0 records within retention)"
	case covered:
		return SignalEmpty, fmt.Sprintf("0 records matched in the window, but %s holds %d records within retention: the stream works, this scope is not producing into it", object, rows)
	default:
		return SignalEmpty, "0 records matched in the window (retention coverage for " + object + " is unknown, so tenant-wide liveness was not established)"
	}
}

// applyFreshness turns a last-seen timestamp into live-or-stale. An
// unparseable timestamp downgrades to live-without-age rather than inventing a
// staleness verdict from a value we did not understand.
func applyFreshness(sig *Signal, lastSeen string, now time.Time, staleAfter time.Duration) {
	sig.State = SignalLive
	if lastSeen == "" {
		return
	}
	sig.LastSeen = lastSeen
	ts, err := parseGrailTime(lastSeen)
	if err != nil {
		return
	}
	// A timeseries bucket is stamped with its end, which for the current
	// bucket lies in the future. Reporting a last-seen after the run started
	// reads as a clock bug; clamp it to now.
	if ts.After(now) {
		ts = now
		sig.LastSeen = now.UTC().Format(time.RFC3339)
	}
	age := now.Sub(ts)
	sig.AgeSeconds = int64(age.Seconds())
	if staleAfter > 0 && age > staleAfter {
		sig.State = SignalStale
		sig.Evidence = fmt.Sprintf("data arrived in this window but stopped %s ago (stale after %s)",
			roundDuration(age), roundDuration(staleAfter))
	}
}

// summarizeSeries totals the datapoints of a timeseries result and derives the
// timestamp of the last non-empty interval.
func summarizeSeries(records []map[string]interface{}) (points int64, lastSeen string) {
	for _, rec := range records {
		values, ok := rec["n"].([]interface{})
		if !ok {
			continue
		}
		start, interval, haveFrame := seriesFrame(rec)
		for i, v := range values {
			if v == nil {
				continue
			}
			n := asInt64(v)
			if n <= 0 {
				continue
			}
			points += n
			if haveFrame {
				// A bucket covers [start+i*interval, start+(i+1)*interval); the
				// data it holds is no newer than its end.
				end := start.Add(time.Duration(i+1) * interval)
				if lastSeen == "" || end.Format(time.RFC3339) > lastSeen {
					lastSeen = end.Format(time.RFC3339)
				}
			}
		}
	}
	return points, lastSeen
}

// seriesFrame extracts the bucket grid from a timeseries record.
func seriesFrame(rec map[string]interface{}) (start time.Time, interval time.Duration, ok bool) {
	frame, isMap := rec["timeframe"].(map[string]interface{})
	if !isMap {
		return start, 0, false
	}
	s, _ := frame["start"].(string)
	if s == "" {
		return start, 0, false
	}
	start, err := parseGrailTime(s)
	if err != nil {
		return start, 0, false
	}
	// Grail reports the interval as a nanosecond count, sometimes as a string.
	ns := asInt64(rec["interval"])
	if ns <= 0 {
		return start, 0, false
	}
	return start, time.Duration(ns), true
}

// parseGrailTime parses the timestamp forms Grail returns, which carry up to
// nanosecond precision.
func parseGrailTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", s)
}

// roundDuration renders an age the way an operator reads it, not to the
// nanosecond.
func roundDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return d.Round(time.Second).String()
	case d < time.Hour:
		return d.Round(time.Minute).String()
	default:
		return d.Round(time.Minute).String()
	}
}

// Summarize counts signals by state.
func Summarize(signals []Signal) *StateSummary {
	sum := &StateSummary{}
	for _, s := range signals {
		switch s.State {
		case SignalLive:
			sum.Live++
		case SignalStale:
			sum.Stale++
		case SignalEmpty:
			sum.Empty++
		case SignalNoData:
			sum.NoData++
		case SignalAbsent:
			sum.Absent++
		case SignalUnknown:
			sum.Unknown++
		}
	}
	return sum
}

// isSignalDef reports whether a capability definition describes a signal type
// that can be asked "is data arriving": a fetchable stream or a metric family.
func isSignalDef(def *CapabilityDef) bool {
	return def != nil && (def.DataObject != "" || def.MetricKey != "")
}

// globMatches returns every key in the family, in a stable order so a run is
// reproducible.
func globMatches(pattern string, keys []string) []string {
	var out []string
	for _, k := range keys {
		if globMatch(pattern, k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), s) {
			return true
		}
	}
	return false
}
