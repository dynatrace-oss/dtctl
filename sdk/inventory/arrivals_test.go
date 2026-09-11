package inventory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 9, 11, 6, 25, 0, 0, time.UTC)

func at(minutes, seconds int) string {
	return fixedNow.Add(-(time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second)).Format(time.RFC3339)
}

func windowDefs() map[string]*CapabilityDef {
	return map[string]*CapabilityDef{
		"logs":      {DataObject: "logs"},
		"spans":     {DataObject: "spans", TimeField: "start_time"},
		"bizevents": {DataObject: "bizevents"},
		"rum":       {DataObject: "user.events"},
		// security.events is in the catalog and busy, but carries nothing the
		// scope names — the n/a case.
		"security":  {DataObject: "security.events"},
		"synthetic": {DataObject: "dt.synthetic.events"},
		"k8smetric": {MetricKey: "dt.kubernetes.*"},
		// Non-signal shapes must be ignored entirely by windowed mode.
		"hosts": {EntityTypes: []string{"HOST"}},
		"genai": {Probe: "fetch spans GENAIPROBE | limit 1", Window: "24h"},
	}
}

// windowRunner serves a tenant where each state is represented exactly once.
//
// Applicability probes share a prefix with the scoped probes they follow, so
// they are listed first: the mock returns the first response whose match is a
// substring of the query.
func windowRunner() *mockRunner {
	return &mockRunner{responses: []mockResponse{
		// user.events does carry the scope's field, so its zero match is a
		// real "this scope is not producing into it".
		{match: "fetch user.events, from:now()-15m | filter isNotNull",
			records: []map[string]interface{}{rec("present", "1")}},
		// security.events carries no such field...
		{match: "fetch security.events, from:now()-15m | filter isNotNull",
			records: []map[string]interface{}{rec("present", "0")}},
		// ...and is demonstrably receiving records, so the miss is about the
		// field, not about a quiet window.
		{match: "fetch security.events, from:now()-15m | limit 1 | summarize any",
			records: []map[string]interface{}{rec("any", "9")}},

		{match: "dt.system.data_objects", records: []map[string]interface{}{
			rec("name", "logs", "fetchable", true, "type", "table"),
			rec("name", "spans", "fetchable", true, "type", "table"),
			rec("name", "bizevents", "fetchable", true, "type", "table"),
			rec("name", "user.events", "fetchable", true, "type", "table"),
			rec("name", "security.events", "fetchable", true, "type", "table"),
			rec("name", "metrics", "fetchable", false, "type", "table"),
		}},
		{match: "dt.system.buckets", records: []map[string]interface{}{
			rec("name", "default_logs", "dt.system.table", "logs", "records", float64(1000), "has_access", true),
			rec("name", "default_spans", "dt.system.table", "spans", "records", float64(500), "has_access", true),
			// bizevents is in the catalog but empty within retention → no-data.
			rec("name", "default_bizevents", "dt.system.table", "bizevents", "records", float64(0), "has_access", true),
			// user.events holds plenty tenant-wide but matches nothing → empty.
			rec("name", "default_rum", "dt.system.table", "user.events", "records", float64(2000), "has_access", true),
			rec("name", "default_security", "dt.system.table", "security.events", "records", float64(3000), "has_access", true),
		}},
		{match: "metrics from:", records: []map[string]interface{}{rec("metric.key", "dt.kubernetes.container.cpu")}},
		{match: "fetch logs, from:", records: []map[string]interface{}{
			rec("matched", "100", "last_seen", at(0, 30)),
		}},
		{match: "fetch spans, from:", records: []map[string]interface{}{
			rec("matched", "50", "last_seen", at(20, 0)),
		}},
		{match: "fetch bizevents, from:", records: []map[string]interface{}{rec("matched", "0")}},
		{match: "fetch user.events, from:", records: []map[string]interface{}{rec("matched", "0")}},
		{match: "fetch security.events, from:", records: []map[string]interface{}{rec("matched", "0")}},
		{match: "timeseries", records: []map[string]interface{}{
			rec("n", []interface{}{float64(5), float64(7)},
				"timeframe", map[string]interface{}{"start": at(3, 0)},
				"interval", "60000000000"),
		}},
	}}
}

func windowOpts() DiscoverOptions {
	return DiscoverOptions{
		Now:        func() time.Time { return fixedNow },
		Since:      "now()-15m",
		Scope:      `k8s.namespace.name == "payments"`,
		StaleAfter: 5 * time.Minute,
	}
}

func signalByName(t *testing.T, inv *Inventory, name string) Signal {
	t.Helper()
	for _, s := range inv.Signals {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("signal %q not in result (%d signals)", name, len(inv.Signals))
	return Signal{}
}

func TestWindowedSignalStates(t *testing.T) {
	runner := windowRunner()
	inv, err := Discover(context.Background(), runner, windowDefs(), windowOpts())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	for _, tc := range []struct {
		name  string
		state SignalState
	}{
		{"logs", SignalLive},
		{"spans", SignalStale},
		{"bizevents", SignalNoData},
		{"rum", SignalEmpty},
		{"security", SignalNotApplicable},
		{"synthetic", SignalAbsent},
		{"k8smetric", SignalLive},
	} {
		if got := signalByName(t, inv, tc.name).State; got != tc.state {
			t.Errorf("%s state = %q, want %q", tc.name, got, tc.state)
		}
	}

	// Non-signal shapes have no "since" reading and must not appear.
	for _, absent := range []string{"hosts", "genai"} {
		for _, s := range inv.Signals {
			if s.Name == absent {
				t.Errorf("%s must not be reported as a signal", absent)
			}
		}
	}

	if logs := signalByName(t, inv, "logs"); logs.Records != 100 || logs.AgeSeconds != 30 {
		t.Errorf("logs = %d records / age %ds, want 100 / 30", logs.Records, logs.AgeSeconds)
	}
	if m := signalByName(t, inv, "k8smetric"); m.Datapoints != 12 {
		t.Errorf("k8smetric datapoints = %d, want 12", m.Datapoints)
	}

	// The empty verdict must cite tenant-wide liveness, since that is the whole
	// reason it is distinguishable from no-data.
	if ev := signalByName(t, inv, "rum").Evidence; !strings.Contains(ev, "2000 records within retention") {
		t.Errorf("rum evidence should cite retention, got %q", ev)
	}

	// The n/a verdict must say it is about the question, not about the data.
	if ev := signalByName(t, inv, "security").Evidence; !strings.Contains(ev, "k8s.namespace.name") ||
		!strings.Contains(ev, "property of the question") {
		t.Errorf("n/a evidence should name the field and disclaim the data, got %q", ev)
	}

	want := StateSummary{Live: 2, Stale: 1, Empty: 1, NoData: 1, NotApplicable: 1, Absent: 1}
	if *inv.Summary != want {
		t.Errorf("summary = %+v, want %+v", *inv.Summary, want)
	}
	if inv.Window == nil || inv.Window.Since != "now()-15m" || inv.Window.Filter == "" {
		t.Errorf("window not recorded: %+v", inv.Window)
	}
}

func TestWindowedProbeShape(t *testing.T) {
	runner := windowRunner()
	if _, err := Discover(context.Background(), runner, windowDefs(), windowOpts()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	joined := strings.Join(runner.calls, "\n")

	// Filter-first is a cost requirement, not a style choice: countIf-shaped
	// probes measured ~19x the scan of the filtered form.
	for _, call := range runner.calls {
		if strings.Contains(call, "countIf(") {
			t.Errorf("probe must not use countIf (measured ~19x scan cost): %s", call)
		}
	}
	if !strings.Contains(joined, `fetch logs, from:now()-15m | filter k8s.namespace.name == "payments" | summarize matched = count(), last_seen = takeMax(timestamp)`) {
		t.Errorf("logs probe not filter-first with the scope pushed into the fetch:\n%s", joined)
	}
	// spans carries start_time and no timestamp at all.
	if !strings.Contains(joined, "fetch spans, from:now()-15m | filter") || !strings.Contains(joined, "takeMax(start_time)") {
		t.Errorf("spans probe must read start_time:\n%s", joined)
	}
	// A stream outside the catalog must not be probed: the query would fail and
	// the failure would masquerade as a probe error.
	if strings.Contains(joined, "fetch dt.synthetic.events") {
		t.Error("must not probe a stream that is absent from the catalog")
	}
	// The entity census has no windowed meaning and is pure cost here.
	if strings.Contains(joined, "smartscapeNodes") {
		t.Error("windowed mode must not run the entity census")
	}
}

func TestWindowedSuppressesEnvironmentListings(t *testing.T) {
	inv, err := Discover(context.Background(), windowRunner(), windowDefs(), windowOpts())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.DataObjects) > 0 || len(inv.Buckets) > 0 || len(inv.QueryOnly) > 0 || len(inv.EntityTypes) > 0 {
		t.Errorf("windowed mode must not carry environment-wide listings: objects=%d buckets=%d queryOnly=%d entities=%d",
			len(inv.DataObjects), len(inv.Buckets), len(inv.QueryOnly), len(inv.EntityTypes))
	}
	if len(inv.Capabilities) > 0 || len(inv.Absent) > 0 {
		t.Error("windowed mode must not emit the retention-scoped capability verdicts")
	}
}

func TestWindowedStaleBoundary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		age   time.Duration
		state SignalState
	}{
		{"just inside", 4*time.Minute + 59*time.Second, SignalLive},
		{"exactly at threshold", 5 * time.Minute, SignalLive},
		{"just past", 5*time.Minute + 1*time.Second, SignalStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := windowRunner()
			runner.responses = append([]mockResponse{{
				match:   "fetch logs, from:",
				records: []map[string]interface{}{rec("matched", "7", "last_seen", fixedNow.Add(-tc.age).Format(time.RFC3339))},
			}}, runner.responses...)
			inv, err := Discover(context.Background(), runner, windowDefs(), windowOpts())
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if got := signalByName(t, inv, "logs").State; got != tc.state {
				t.Errorf("age %s → %q, want %q", tc.age, got, tc.state)
			}
		})
	}
}

func TestWindowedTruncatedProbeIsUnknownNotAbsent(t *testing.T) {
	runner := windowRunner()
	runner.responses = append([]mockResponse{{
		match: "fetch logs, from:", truncated: true,
		records: []map[string]interface{}{rec("matched", "0")},
	}}, runner.responses...)
	inv, err := Discover(context.Background(), runner, windowDefs(), windowOpts())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	logs := signalByName(t, inv, "logs")
	if logs.State != SignalUnknown {
		t.Fatalf("truncated probe → %q, want unknown: a cut-short probe that found nothing proves nothing", logs.State)
	}
	if !strings.Contains(logs.Evidence, "cut short") {
		t.Errorf("evidence should say the probe was cut short, got %q", logs.Evidence)
	}
}

func TestWindowedProbeFailureIsUnknown(t *testing.T) {
	runner := windowRunner()
	runner.responses = append([]mockResponse{{
		match: "fetch logs, from:", err: fmt.Errorf("boom"),
	}}, runner.responses...)
	inv, err := Discover(context.Background(), runner, windowDefs(), windowOpts())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := signalByName(t, inv, "logs").State; got != SignalUnknown {
		t.Errorf("failed probe → %q, want unknown", got)
	}
}

func TestWindowedSignalsFilter(t *testing.T) {
	opts := windowOpts()
	opts.Signals = []string{"logs", "SPANS"}
	runner := windowRunner()
	inv, err := Discover(context.Background(), runner, windowDefs(), opts)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.Signals) != 2 {
		t.Fatalf("got %d signals, want 2", len(inv.Signals))
	}
	// Case-insensitive, so --signals SPANS works as typed.
	signalByName(t, inv, "spans")
	// With no metric signal selected the catalog read must be skipped.
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "metrics from:") {
			t.Error("metric catalog read despite no metric signal in scope")
		}
	}
}

func TestAllZeroTripwire(t *testing.T) {
	live := []Signal{{Name: "logs", State: SignalLive}, {Name: "spans", State: SignalEmpty}}
	if note := allZeroTripwire(live); note != "" {
		t.Errorf("tripwire must stay quiet when something is live, got %q", note)
	}

	allEmpty := []Signal{{Name: "logs", State: SignalEmpty}, {Name: "spans", State: SignalEmpty}}
	note := allZeroTripwire(allEmpty)
	if note == "" {
		t.Fatal("tripwire must fire when every signal is empty")
	}
	if !strings.Contains(note, "scope") {
		t.Errorf("tripwire should implicate the scope, got %q", note)
	}

	// One empty signal is not a pattern; absent/unknown are not "empty".
	if n := allZeroTripwire([]Signal{{Name: "logs", State: SignalEmpty}}); n != "" {
		t.Errorf("single empty signal must not trip the wire, got %q", n)
	}
	if n := allZeroTripwire([]Signal{{Name: "a", State: SignalAbsent}, {Name: "b", State: SignalUnknown}}); n != "" {
		t.Errorf("absent/unknown must not trip the wire, got %q", n)
	}

	// States that say nothing about whether the scope is valid must not weigh
	// against the wrong-scope explanation either. Counting no-data as
	// "evaluated but not empty" made the tripwire unfireable on any tenant
	// with one quiet stream, which is nearly all of them.
	withNoData := []Signal{
		{Name: "logs", State: SignalEmpty},
		{Name: "spans", State: SignalEmpty},
		{Name: "synthetic", State: SignalNoData},
	}
	if allZeroTripwire(withNoData) == "" {
		t.Error("a tenant-wide-empty stream must not suppress the tripwire")
	}
	withNA := append(withNoData, Signal{Name: "rum", State: SignalNotApplicable})
	if allZeroTripwire(withNA) == "" {
		t.Error("an inapplicable signal must not suppress the tripwire")
	}
}

func TestTimeFieldOnlyWithDataObject(t *testing.T) {
	err := ValidateDefinitions(map[string]*CapabilityDef{
		"bad": {MetricKey: "dt.*", TimeField: "timestamp"},
	})
	if err == nil {
		t.Fatal("timeField on a non-dataObject shape must be rejected")
	}
}

func TestBuiltinTimeFieldOverrides(t *testing.T) {
	defs := BuiltinDefinitions()
	// spans and user.events carry start_time and no timestamp at all;
	// takeMax(timestamp) there yields an undefined column rather than an
	// error, so the signal would report live with no age and could never go
	// stale. Verified against a live tenant for every built-in stream.
	for _, name := range []string{"spans", "rum"} {
		if got := defs[name].TimeField; got != "start_time" {
			t.Errorf("builtin %s timeField = %q, want start_time", name, got)
		}
	}
	for _, name := range []string{"logs", "bizevents", "security", "davis", "davis-events", "synthetic"} {
		if got := defs[name].TimeField; got != "" {
			t.Errorf("%s carries timestamp and should use the default time field, got %q", name, got)
		}
	}
}

func TestBuiltinDavisViewsDeclareTheirBuckets(t *testing.T) {
	// dt.system.buckets keys records by table, and the davis views are views
	// over `events` that declare no query_string in the catalog. Without an
	// explicit bucket list they lose the empty/no-data discrimination
	// entirely and can only ever report "coverage unknown".
	defs := BuiltinDefinitions()
	for _, name := range []string{"davis", "davis-events"} {
		if len(defs[name].BackingBuckets) == 0 {
			t.Errorf("%s is a view over events and must declare its backing buckets", name)
		}
	}
}

func TestBackingBucketsOnlyWithDataObject(t *testing.T) {
	err := ValidateDefinitions(map[string]*CapabilityDef{
		"bad": {MetricKey: "dt.*", BackingBuckets: []string{"x*"}},
	})
	if err == nil {
		t.Fatal("backingBuckets on a non-dataObject shape must be rejected")
	}
}

func TestScopeFields(t *testing.T) {
	for _, tc := range []struct {
		scope string
		want  []string
	}{
		{`k8s.namespace.name == "payments"`, []string{"k8s.namespace.name"}},
		// Values never contribute field names, even when they look like one.
		{`log.source == "service.name"`, []string{"log.source"}},
		{`a == "x" and b != "y"`, []string{"a", "b"}},
		// Functions are dropped by their trailing paren, not by a builtin list.
		{`matchesPhrase(content, "boom")`, []string{"content"}},
		{`isNotNull(k8s.pod.name)`, []string{"k8s.pod.name"}},
		// Backtick-quoted fields are taken verbatim.
		{"`my-weird field` == \"x\"", []string{"my-weird field"}},
		// Literals and operators are not fields.
		{`true`, nil},
		{`status == "ERROR" or status == "WARN"`, []string{"status"}},
		{``, nil},
	} {
		got := scopeFields(tc.scope)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("scopeFields(%q) = %v, want %v", tc.scope, got, tc.want)
		}
	}
}

func TestMetricDimensionApplicability(t *testing.T) {
	fields := []string{"k8s.namespace.name"}
	// "undefined" is Grail's way of saying the metric has no such dimension.
	if got := metricDimensionApplicability(map[string]string{"k8s.namespace.name": TypeUndefined}, fields); got != applicabilityNo {
		t.Errorf("an undefined dimension should read as not-applicable, got %v", got)
	}
	if got := metricDimensionApplicability(map[string]string{"k8s.namespace.name": "string"}, fields); got != applicabilityYes {
		t.Errorf("a typed dimension should read as applicable, got %v", got)
	}
	// One real dimension is enough to make the question askable.
	two := []string{"k8s.namespace.name", "host.name"}
	types := map[string]string{"k8s.namespace.name": TypeUndefined, "host.name": "string"}
	if got := metricDimensionApplicability(types, two); got != applicabilityYes {
		t.Errorf("one existing dimension should make the scope applicable, got %v", got)
	}
	// Missing type information is never an absence claim.
	if got := metricDimensionApplicability(nil, fields); got != applicabilityUnknown {
		t.Errorf("absent type metadata must yield unknown, got %v", got)
	}
	if got := metricDimensionApplicability(map[string]string{"other": "string"}, fields); got != applicabilityUnknown {
		t.Errorf("a field the probe did not report on must yield unknown, got %v", got)
	}
}

func TestMetricFamilyThatCannotCarryTheScope(t *testing.T) {
	defs := map[string]*CapabilityDef{"host-metrics": {MetricKey: "dt.host.*"}}
	runner := &mockRunner{responses: []mockResponse{
		{match: "dt.system.data_objects", records: []map[string]interface{}{
			rec("name", "metrics", "fetchable", false, "type", "table"),
		}},
		{match: "dt.system.buckets"},
		{match: "metrics from:", records: []map[string]interface{}{rec("metric.key", "dt.host.cpu.usage")}},
		// The grouping probe is the applicability test; it must precede the
		// scoped one in the fixture because both start with "timeseries".
		{match: "by:{`k8s.namespace.name`}", types: map[string]string{"k8s.namespace.name": TypeUndefined}},
		{match: "timeseries"}, // scoped probe: no rows, as a live tenant returns
	}}
	inv, err := Discover(context.Background(), runner, defs, windowOpts())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	sig := signalByName(t, inv, "host-metrics")
	if sig.State != SignalNotApplicable {
		t.Fatalf("dt.host.* has no k8s.namespace.name dimension, so the state should be n/a, got %q (%s)", sig.State, sig.Evidence)
	}
	// The grouping probe scans nothing, which is what makes it affordable.
	found := false
	for _, c := range runner.calls {
		if strings.Contains(c, "by:{`k8s.namespace.name`}") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a by:{} dimension probe, calls: %v", runner.calls)
	}
}

func TestQuietStreamIsNotCalledNotApplicable(t *testing.T) {
	// A stream that received nothing at all in the window cannot answer
	// whether it carries the scope's fields — the isNotNull probe misses for
	// the same reason the scoped probe did. Reporting n/a here would invent a
	// schema claim out of a quiet window.
	defs := map[string]*CapabilityDef{"logs": {DataObject: "logs"}}
	runner := &mockRunner{responses: []mockResponse{
		{match: "dt.system.data_objects", records: []map[string]interface{}{
			rec("name", "logs", "fetchable", true, "type", "table"),
		}},
		{match: "dt.system.buckets", records: []map[string]interface{}{
			rec("name", "default_logs", "dt.system.table", "logs", "records", float64(5000), "has_access", true),
		}},
		{match: "| filter isNotNull", records: []map[string]interface{}{rec("present", "0")}},
		{match: "| limit 1 | summarize any", records: []map[string]interface{}{rec("any", "0")}},
		{match: "fetch logs, from:", records: []map[string]interface{}{rec("matched", "0")}},
	}}
	inv, err := Discover(context.Background(), runner, defs, windowOpts())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := signalByName(t, inv, "logs").State; got != SignalEmpty {
		t.Errorf("a quiet window must stay empty, not become n/a: got %q", got)
	}
}

func TestNoDataSkipsTheApplicabilityProbe(t *testing.T) {
	// A stream with no records within retention is already fully explained,
	// and asking it about the scope's fields could only mislead.
	runner := windowRunner()
	if _, err := Discover(context.Background(), runner, windowDefs(), windowOpts()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, c := range runner.calls {
		if strings.Contains(c, "fetch bizevents") && strings.Contains(c, "isNotNull") {
			t.Errorf("no-data stream must not be probed for applicability: %s", c)
		}
	}
}

func TestViewRetentionResolvesThroughBuckets(t *testing.T) {
	// dt.davis.problems is a view over `events` with no table of its own, so
	// without bucket resolution its retention coverage is simply unknown and
	// empty/no-data collapses.
	facts := discoveredFacts{
		streamRows:  map[string]int64{"events": 900},
		bucketRows:  map[string]int64{"default_davis_problems": 40, "default_logs": 7},
		viewBuckets: map[string][]string{"dt.davis.problems": {"default_davis*"}},
		viewTable:   map[string]string{"dt.davis.problems": "events"},
	}
	cov := retentionFor("dt.davis.problems", facts)
	if !cov.known || cov.rows != 40 || !cov.exact {
		t.Fatalf("buckets should give an exact figure for the view: %+v", cov)
	}
	state, ev := emptyState("dt.davis.problems", cov)
	if state != SignalEmpty || !strings.Contains(ev, "the stream works") {
		t.Errorf("a live view should support the strong empty claim: %q / %q", state, ev)
	}

	// Without declared buckets the backing table is all there is, and it is a
	// superset — so the strong claim must not be made.
	noBuckets := facts
	noBuckets.viewBuckets = nil
	cov = retentionFor("dt.davis.problems", noBuckets)
	if !cov.known || cov.exact {
		t.Fatalf("the backing table is a superset, not an exact figure: %+v", cov)
	}
	_, ev = emptyState("dt.davis.problems", cov)
	if strings.Contains(ev, "the stream works") {
		t.Errorf("a superset count must not be presented as the view's own liveness: %q", ev)
	}
	if !strings.Contains(ev, "superset") {
		t.Errorf("the evidence should say the figure is a superset: %q", ev)
	}

	// An empty backing is conclusive in the other direction.
	empty := facts
	empty.bucketRows = map[string]int64{"default_davis_problems": 0}
	state, _ = emptyState("dt.davis.problems", retentionFor("dt.davis.problems", empty))
	if state != SignalNoData {
		t.Errorf("empty buckets mean the view holds nothing: got %q", state)
	}
}

func TestViewBacking(t *testing.T) {
	buckets, table := viewBacking(`// Bucket filters are added for performance
// Custom bucket filter: 'davis*'
fetch events, scanLimitGBytes:-1, bucket: {"default_davis*", "davis*"}
| filter event.kind == "DAVIS_PROBLEM"`)
	if strings.Join(buckets, ",") != "default_davis*,davis*" {
		t.Errorf("buckets = %v", buckets)
	}
	if table != "events" {
		t.Errorf("table = %q, want events", table)
	}

	// A view that names no buckets still resolves to its table.
	buckets, table = viewBacking(`fetch user.sessions`)
	if len(buckets) != 0 || table != "user.sessions" {
		t.Errorf("buckets = %v, table = %q", buckets, table)
	}
}

func TestScopeSyntaxErrorAbortsTheWholeRun(t *testing.T) {
	// A scope that does not parse fails identically for every signal. Marking
	// each one "unknown" would burn the budget, print the same parse error a
	// dozen times, and dress a typo up as an inconclusive environment.
	runner := windowRunner()
	runner.responses = append([]mockResponse{{
		match: "fetch logs, from:",
		err:   fmt.Errorf(`{"errorType":"PARSE_ERROR","errorMessage":"` + "`" + `(` + "`" + ` isn't allowed here"}`),
	}}, runner.responses...)
	_, err := Discover(context.Background(), runner, windowDefs(), windowOpts())
	if err == nil {
		t.Fatal("a scope that does not parse must abort the run, not yield unknowns")
	}
	if !strings.Contains(err.Error(), "--scope") {
		t.Errorf("the error should point at the scope the user wrote: %v", err)
	}
	for _, c := range runner.calls {
		if strings.Contains(c, "fetch user.events") {
			t.Errorf("probing must stop at the first parse failure, but continued: %v", runner.calls)
		}
	}
}

func TestSignalNames(t *testing.T) {
	names := SignalNames(windowDefs())
	if strings.Join(names, ",") != "bizevents,k8smetric,logs,rum,security,spans,synthetic" {
		t.Errorf("SignalNames = %v", names)
	}
	// Entity-census and probe shapes have no windowed reading and are not
	// signal names a caller may require.
	for _, n := range names {
		if n == "hosts" || n == "genai" {
			t.Errorf("%q is not a signal type", n)
		}
	}
}
