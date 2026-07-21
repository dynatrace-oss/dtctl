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

// RunResult is one probe/query outcome as discovery sees it. Truncated means
// the result is incomplete — a record/byte cap, scan limit, or timeout cut it
// short — so "no matching row" in it is not evidence that none exists.
type RunResult struct {
	Records   []map[string]interface{}
	Seconds   float64
	Truncated bool
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
	// skipped records that at least one query was refused for lack of budget —
	// the condition under which the inventory is actually partial. Merely
	// ending a run with zero budget left is not.
	skipped bool
}

var errBudgetExhausted = fmt.Errorf("discovery budget exhausted")

func (b *budgetRunner) run(ctx context.Context, dql string) (*RunResult, error) {
	// A cancelled run must not keep issuing queries (each would round-trip to
	// the executor just to fail); surface the cancellation instead.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.queries <= 0 || b.seconds <= 0 {
		b.skipped = true
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

// discoveredFacts carries the structural facts capability evaluation runs
// against, with availability flags: a fact source that failed or was skipped
// must yield "unknown" verdicts, never fabricated absence evidence.
type discoveredFacts struct {
	objects    map[string]bool
	census     map[string]int64
	censusOK   bool
	metricKeys []string
	metricsOK  bool
	// streamRows sums bucket records per stream (dt.system.table → records
	// across its accessible buckets): a catalog object whose buckets are all
	// empty holds no data within retention. Streams without bucket coverage
	// are simply not in the map and keep their catalog verdict.
	streamRows map[string]int64
	// The *Truncated flags mark a fact source that loaded but incompletely
	// (result cap, scan limit): a hit in it still proves presence, but a miss
	// proves nothing and must degrade to unknown, not absent.
	objectsTruncated bool
	censusTruncated  bool
	metricsTruncated bool
}

// Discover runs the discovery battery against a live environment and returns
// the inventory, its consumption receipt attached. Probes run cheapest-first:
// data-object catalog → buckets → entity census → metric catalog (only when a
// metricKey definition needs it) → probe-shaped capability definitions.
// Cancellation aborts the whole run: a half-discovered inventory is never
// returned as if it were a verdict.
func Discover(ctx context.Context, runner Runner, defs map[string]*CapabilityDef, opts DiscoverOptions) (*Inventory, error) {
	// Fail fast on a malformed definition set: the CLI path arrives validated
	// (ParseDefinitions), but a set constructed in Go bypasses that — and a
	// def with no shape would otherwise get no verdict at all, silently.
	if err := ValidateDefinitions(defs); err != nil {
		return nil, fmt.Errorf("invalid capability definitions: %w", err)
	}
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
	fetchable, queryOnly, objectsTruncated, err := br.dataObjectCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("data-object discovery failed: %w", err)
	}
	// The legacy dt.entity.* lookback views are collapsed to a count: they can
	// number in the hundreds, and the census below is the canonical entity
	// surface. The full catalog still backs capability verdicts.
	for _, name := range fetchable {
		if strings.HasPrefix(name, "dt.entity.") {
			inv.EntityViews++
		} else {
			inv.DataObjects = append(inv.DataObjects, name)
		}
	}
	inv.QueryOnly = queryOnly
	if len(queryOnly) > 0 {
		inv.Notes = append(inv.Notes, fmt.Sprintf(
			"catalog objects without fetch support: %s — metric data is queried via the timeseries/metrics commands, smartscape via smartscapeNodes/smartscapeEdges",
			strings.Join(queryOnly, ", ")))
	}
	facts := discoveredFacts{
		// Capability shapes check catalog membership, not fetchability.
		objects:          stringSet(append(append([]string{}, fetchable...), queryOnly...)),
		objectsTruncated: objectsTruncated,
		census:           map[string]int64{},
	}
	if objectsTruncated {
		report.Notes = append(report.Notes, "data-object catalog truncated — the object list is incomplete")
	}

	if res, err := br.run(ctx, "fetch dt.system.buckets | fields name, dt.system.table, records, has_access | sort name asc | limit 1000"); err == nil {
		streamRows := map[string]int64{}
		for _, rec := range res.Records {
			if name, ok := rec["name"].(string); ok && name != "" {
				inv.Buckets = append(inv.Buckets, name)
			}
			// Inaccessible buckets report no usable record count — leaving them
			// out keeps an all-inaccessible stream on its catalog verdict
			// instead of a fabricated "empty".
			if access, ok := rec["has_access"].(bool); ok && !access {
				continue
			}
			if table, ok := rec["dt.system.table"].(string); ok && table != "" {
				streamRows[table] += asInt64(rec["records"])
			}
		}
		if res.Truncated {
			// Unseen buckets may hold the rows: liveness can't be judged.
			report.Notes = append(report.Notes, "bucket list truncated — buckets are missing")
		} else {
			facts.streamRows = streamRows
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	} else if err != errBudgetExhausted {
		report.Notes = append(report.Notes, "bucket discovery failed: "+firstLine(err.Error()))
	}

	if res, err := br.run(ctx, `smartscapeNodes "*" | summarize c = count(), by:{type} | sort c desc | limit 1000`); err == nil {
		for _, rec := range res.Records {
			if t, ok := rec["type"].(string); ok {
				facts.census[t] = asInt64(rec["c"])
			}
		}
		facts.censusOK = true
		facts.censusTruncated = res.Truncated
		inv.EntityTypes = facts.census
		if res.Truncated {
			report.Notes = append(report.Notes, "entity census truncated — rare entity types may be missing")
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	} else if err != errBudgetExhausted {
		report.Notes = append(report.Notes, "entity census failed: "+firstLine(err.Error()))
	}

	if anyMetricDef(defs) {
		if keys, truncated, err := br.stringColumn(ctx, "metrics from:now()-2h | summarize c = count(), by:{metric.key} | limit 10000", "metric.key"); err == nil {
			facts.metricKeys = keys
			facts.metricsOK = true
			facts.metricsTruncated = truncated
			if truncated {
				report.Notes = append(report.Notes, fmt.Sprintf("metric catalog truncated at %d keys — keys are missing", len(keys)))
			}
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else if err != errBudgetExhausted {
			report.Notes = append(report.Notes, "metric catalog failed: "+firstLine(err.Error()))
		}
	}

	inv.Capabilities, inv.Absent, inv.Unknown, err = br.evaluateCapabilities(ctx, defs, facts)
	if err != nil {
		return nil, err
	}

	// Canonical-stream notes: recurring mistakes where a plausible query
	// silently measures the wrong thing — stated as facts so consumers can
	// steer before the mistake, not after.
	if facts.objects["dt.davis.events"] {
		inv.Notes = append(inv.Notes,
			"Davis problem/event analytics: fetch dt.davis.events — the generic `events` stream mixes other event kinds and its counts diverge from Davis")
	}
	if len(facts.census) > 0 {
		inv.Notes = append(inv.Notes,
			"current-state entity census: smartscapeNodes \"<TYPE>\" — `fetch dt.entity.*` is a lookback view over the query window and diverges from the live topology")
	}
	if br.skipped {
		note := fmt.Sprintf("discovery budget exhausted after %d queries / %.0fs — the inventory is partial", report.Queries, report.Seconds)
		inv.Notes = append(inv.Notes, note)
		report.Notes = append(report.Notes, note)
	}
	return inv, nil
}

// evaluateCapabilities evaluates every definition: structural shapes against
// the discovered facts, probe shapes against the live environment. Verdicts
// are honest three-state: present, absent (with the evidence checked), or
// unknown when no check actually ran — a failed probe, an exhausted budget, or
// an unavailable fact source is not evidence of absence.
func (b *budgetRunner) evaluateCapabilities(ctx context.Context, defs map[string]*CapabilityDef, facts discoveredFacts) (present []string, absent, unknown []CapabilityStatus, err error) {
	names := make([]string, 0, len(defs))
	for n := range defs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		def := defs[name]
		// Carry the evidence with the verdict, so the negative is citable
		// without anyone re-deriving it with fresh probes.
		absentWith := func(evidence string) {
			absent = append(absent, CapabilityStatus{Name: name, Evidence: evidence})
		}
		verdict := func(ok bool) {
			if ok {
				present = append(present, name)
			} else {
				absentWith(absenceEvidence(def))
			}
		}
		noVerdict := func(reason string) {
			unknown = append(unknown, CapabilityStatus{Name: name, Evidence: reason})
		}
		// A miss in a truncated fact source proves nothing: the match may sit in
		// the rows that were cut off. A hit still proves presence.
		switch {
		case def.DataObject != "":
			switch {
			case facts.objects[def.DataObject]:
				// Catalog membership proves the stream exists, not that it holds
				// data: on a tenant that never ingested RUM, user.events is
				// still in the catalog. Bucket statistics close that gap for
				// free — when they cover the object and every accessible bucket
				// is empty, the capability is absent, with the emptiness cited.
				if rows, covered := facts.streamRows[def.DataObject]; covered && rows == 0 {
					absentWith(def.DataObject + " is in the catalog, but all its buckets are empty (0 records within retention)")
				} else {
					verdict(true)
				}
			case facts.objectsTruncated:
				noVerdict("not evaluated: data-object catalog truncated")
			default:
				verdict(false)
			}
		case len(def.EntityTypes) > 0:
			switch {
			case !facts.censusOK:
				noVerdict("not evaluated: entity census unavailable")
			case censusMatches(def.EntityTypes, facts.census):
				verdict(true)
			case facts.censusTruncated:
				noVerdict("not evaluated: entity census truncated")
			default:
				verdict(false)
			}
		case def.MetricKey != "":
			switch {
			case !facts.metricsOK:
				noVerdict("not evaluated: metric catalog unavailable")
			case anyGlobMatch(def.MetricKey, facts.metricKeys):
				verdict(true)
			case facts.metricsTruncated:
				noVerdict("not evaluated: metric catalog truncated")
			default:
				verdict(false)
			}
		case def.Probe != "":
			res, perr := b.run(ctx, def.Probe)
			switch {
			case perr == nil && len(res.Records) > 0:
				verdict(true)
			case perr == nil && res.Truncated:
				noVerdict("not evaluated: probe result was cut by a limit before any match")
			case perr == nil:
				verdict(false)
			case ctx.Err() != nil:
				return nil, nil, nil, ctx.Err()
			case perr == errBudgetExhausted:
				noVerdict("not evaluated: discovery budget exhausted")
			default:
				noVerdict("probe failed: " + firstLine(perr.Error()))
			}
		}
	}
	return present, absent, unknown, nil
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

// censusMatches reports whether any census type with a live count matches any
// of the glob patterns.
func censusMatches(patterns []string, census map[string]int64) bool {
	for _, pattern := range patterns {
		for t, c := range census {
			if c > 0 && globMatch(pattern, t) {
				return true
			}
		}
	}
	return false
}

func anyGlobMatch(pattern string, keys []string) bool {
	for _, k := range keys {
		if globMatch(pattern, k) {
			return true
		}
	}
	return false
}

// dataObjectCatalog returns the queryable-object catalog partitioned by fetch
// support (the catalog's usable_with column). The catalog lists objects that
// only work through other commands — advertising those as fetch targets bakes
// in DATA_OBJECT_NOT_SUPPORTED failures for consumers.
func (b *budgetRunner) dataObjectCatalog(ctx context.Context) (fetchable, queryOnly []string, truncated bool, err error) {
	res, err := b.run(ctx,
		`fetch dt.system.data_objects | fieldsAdd fetchable = in("fetch", usable_with) | fields name, fetchable | sort name asc | limit 5000`)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		// Environments without usable_with fall back to the flat list.
		names, truncated, ferr := b.stringColumn(ctx, "fetch dt.system.data_objects | fields name | sort name asc | limit 5000", "name")
		return names, nil, truncated, ferr
	}
	for _, rec := range res.Records {
		name, _ := rec["name"].(string)
		if name == "" {
			continue
		}
		if f, ok := rec["fetchable"].(bool); ok && !f {
			queryOnly = append(queryOnly, name)
		} else {
			fetchable = append(fetchable, name)
		}
	}
	return fetchable, queryOnly, res.Truncated, nil
}

func (b *budgetRunner) stringColumn(ctx context.Context, dql, column string) ([]string, bool, error) {
	res, err := b.run(ctx, dql)
	if err != nil {
		return nil, false, err
	}
	var out []string
	for _, rec := range res.Records {
		if s, ok := rec[column].(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out, res.Truncated, nil
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
