package inventory

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RunResult is one probe/query outcome as discovery sees it.
type RunResult struct {
	Records []map[string]interface{}
	Seconds float64
}

// Runner executes DQL on the live environment. The cmd layer implements it
// over the existing DQL executor (with scan caps); tests implement it over
// fixtures.
type Runner interface {
	RunQuery(ctx context.Context, dql string) (*RunResult, error)
}

// DiscoverOptions parameterizes a discovery run.
type DiscoverOptions struct {
	ContextName string
	Now         func() time.Time
	// Segments are pre-fetched by the caller (segments come from the API, not
	// DQL) and pass through into the inventory.
	Segments []SegmentInfo
	// Budget is mandatory, not advisory: discovery stops with a partial
	// inventory rather than overrunning.
	BudgetQueries int     // 0 = 100
	BudgetSeconds float64 // 0 = 300
}

type budgetRunner struct {
	runner  Runner
	report  *Report
	queries int
	seconds float64
}

var errBudgetExhausted = fmt.Errorf("discovery budget exhausted")

func (b *budgetRunner) run(ctx context.Context, dql string) (*RunResult, error) {
	if b.queries <= 0 || b.seconds <= 0 {
		return nil, errBudgetExhausted
	}
	res, err := b.runner.RunQuery(ctx, dql)
	b.report.Queries++
	b.queries--
	if res != nil {
		b.report.Seconds += res.Seconds
		b.seconds -= res.Seconds
	}
	return res, err
}

// Discover runs the discovery battery against a live environment and returns
// the inventory, its consumption receipt attached. Probes run cheapest-first:
// data-object catalog → buckets → entity census → metric catalog (only when a
// metricKey definition needs it) → probe-shaped capability definitions.
func Discover(ctx context.Context, runner Runner, defs map[string]*CapabilityDef, opts DiscoverOptions) (*Inventory, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	report := &Report{}
	br := &budgetRunner{
		runner:  runner,
		report:  report,
		queries: valueOr(opts.BudgetQueries, 100),
		seconds: valueOrF(opts.BudgetSeconds, 300),
	}

	inv := &Inventory{
		Context:     opts.ContextName,
		GeneratedAt: now().UTC().Format(time.RFC3339),
		Segments:    opts.Segments,
		Discovery:   report,
	}

	// A hard error on the catalog aborts: without it nothing below is
	// meaningful. Everything after degrades to a report note.
	fetchable, unfetchable, err := br.dataObjectCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("data-object discovery failed: %w", err)
	}
	inv.DataObjects = fetchable
	inv.Unfetchable = unfetchable
	if len(unfetchable) > 0 {
		inv.Notes = append(inv.Notes, fmt.Sprintf(
			"catalog objects without fetch support: %s — metric data is queried via the timeseries/metrics commands, smartscape via smartscapeNodes/smartscapeEdges",
			strings.Join(unfetchable, ", ")))
	}
	// Capability shapes check catalog membership, not fetchability.
	objects := stringSet(append(append([]string{}, fetchable...), unfetchable...))

	if buckets, err := br.stringColumn(ctx, "fetch dt.system.buckets | fields name | sort name asc | limit 1000", "name"); err == nil {
		inv.Buckets = buckets
	} else if err != errBudgetExhausted {
		report.Notes = append(report.Notes, "bucket discovery failed: "+firstLine(err.Error()))
	}

	census := map[string]int64{}
	if res, err := br.run(ctx, `smartscapeNodes "*" | summarize c = count(), by:{type} | sort c desc | limit 1000`); err == nil {
		for _, rec := range res.Records {
			if t, ok := rec["type"].(string); ok {
				census[t] = asInt64(rec["c"])
			}
		}
		inv.EntityTypes = census
	} else if err != errBudgetExhausted {
		report.Notes = append(report.Notes, "entity census failed: "+firstLine(err.Error()))
	}

	var metricKeys []string
	if anyMetricDef(defs) {
		if keys, err := br.stringColumn(ctx, "metrics from:now()-2h | summarize c = count(), by:{metric.key} | limit 10000", "metric.key"); err == nil {
			metricKeys = keys
		} else if err != errBudgetExhausted {
			report.Notes = append(report.Notes, "metric catalog failed: "+firstLine(err.Error()))
		}
	}

	inv.Capabilities, inv.Absent = br.evaluateCapabilities(ctx, defs, objects, census, metricKeys)

	// Canonical-stream notes: recurring mistakes where a plausible query
	// silently measures the wrong thing — stated as facts so consumers can
	// steer before the mistake, not after.
	if objects["dt.davis.events"] {
		inv.Notes = append(inv.Notes,
			"Davis problem/event analytics: fetch dt.davis.events — the generic `events` stream mixes other event kinds and its counts diverge from Davis")
	}
	if len(census) > 0 {
		inv.Notes = append(inv.Notes,
			"current-state entity census: smartscapeNodes \"<TYPE>\" — `fetch dt.entity.*` is a lookback view over the query window and diverges from the live topology")
	}
	if br.queries <= 0 || br.seconds <= 0 {
		note := fmt.Sprintf("discovery budget exhausted after %d queries / %.0fs — the inventory is partial", report.Queries, report.Seconds)
		inv.Notes = append(inv.Notes, note)
		report.Notes = append(report.Notes, note)
	}
	return inv, nil
}

// evaluateCapabilities evaluates every definition: structural shapes against
// the discovered facts, probe shapes against the live environment. A probe
// hard error is evidence of absence (e.g. inner-map access on a missing
// field), not of breakage.
func (b *budgetRunner) evaluateCapabilities(ctx context.Context, defs map[string]*CapabilityDef, objects map[string]bool, census map[string]int64, metricKeys []string) (present, absent []string) {
	names := make([]string, 0, len(defs))
	for n := range defs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		def := defs[name]
		ok := false
		switch {
		case def.DataObject != "":
			ok = objects[def.DataObject]
		case len(def.EntityTypes) > 0:
			for _, pattern := range def.EntityTypes {
				for t, c := range census {
					if c > 0 && globMatch(pattern, t) {
						ok = true
						break
					}
				}
			}
		case def.MetricKey != "":
			for _, k := range metricKeys {
				if globMatch(def.MetricKey, k) {
					ok = true
					break
				}
			}
		case def.Probe != "":
			if res, err := b.run(ctx, def.Probe); err == nil && len(res.Records) > 0 {
				ok = true
			}
		}
		if ok {
			present = append(present, name)
		} else {
			// Carry the evidence with the verdict, so the negative is citable
			// without anyone re-deriving it with fresh probes.
			absent = append(absent, name+" ("+absenceEvidence(def)+")")
		}
	}
	return present, absent
}

// absenceEvidence says what was checked when a capability came up absent.
func absenceEvidence(def *CapabilityDef) string {
	switch {
	case def.DataObject != "":
		return "no " + def.DataObject + " in the data-object catalog"
	case len(def.EntityTypes) > 0:
		return "no " + strings.Join(def.EntityTypes, "|") + " entities in the live census"
	case def.MetricKey != "":
		return "no metric keys matching " + def.MetricKey + " in the live metric catalog"
	case def.Probe != "":
		return "discovery probe returned no rows in " + def.Window
	default:
		return "no discovery definition matched"
	}
}

// dataObjectCatalog returns the queryable-object catalog partitioned by fetch
// support (the catalog's usable_with column). The catalog lists objects that
// only work through other commands — advertising those as fetch targets bakes
// in DATA_OBJECT_NOT_SUPPORTED failures for consumers.
func (b *budgetRunner) dataObjectCatalog(ctx context.Context) (fetchable, unfetchable []string, err error) {
	res, err := b.run(ctx,
		`fetch dt.system.data_objects | fieldsAdd fetchable = in("fetch", usable_with) | fields name, fetchable | sort name asc | limit 5000`)
	if err != nil {
		// Environments without usable_with fall back to the flat list.
		names, ferr := b.stringColumn(ctx, "fetch dt.system.data_objects | fields name | sort name asc | limit 5000", "name")
		return names, nil, ferr
	}
	for _, rec := range res.Records {
		name, _ := rec["name"].(string)
		if name == "" {
			continue
		}
		if f, ok := rec["fetchable"].(bool); ok && !f {
			unfetchable = append(unfetchable, name)
		} else {
			fetchable = append(fetchable, name)
		}
	}
	return fetchable, unfetchable, nil
}

func (b *budgetRunner) stringColumn(ctx context.Context, dql, column string) ([]string, error) {
	res, err := b.run(ctx, dql)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, rec := range res.Records {
		if s, ok := rec[column].(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func anyMetricDef(defs map[string]*CapabilityDef) bool {
	for _, d := range defs {
		if d.MetricKey != "" {
			return true
		}
	}
	return false
}

func globMatch(pattern, s string) bool {
	ok, err := path.Match(pattern, s)
	return err == nil && ok
}

func stringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, s := range items {
		set[s] = true
	}
	return set
}

func asInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 220 {
		s = s[:220] + "…"
	}
	return s
}

func valueOr(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func valueOrF(v, def float64) float64 {
	if v > 0 {
		return v
	}
	return def
}
