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
		"synthetic": {DataObject: "dt.synthetic.events"},
		"k8smetric": {MetricKey: "dt.kubernetes.*"},
		// Non-signal shapes must be ignored entirely by windowed mode.
		"hosts": {EntityTypes: []string{"HOST"}},
		"genai": {Probe: "fetch spans GENAIPROBE | limit 1", Window: "24h"},
	}
}

// windowRunner serves a tenant where each state is represented exactly once.
func windowRunner() *mockRunner {
	return &mockRunner{responses: []mockResponse{
		{match: "dt.system.data_objects", records: []map[string]interface{}{
			rec("name", "logs", "fetchable", true),
			rec("name", "spans", "fetchable", true),
			rec("name", "bizevents", "fetchable", true),
			rec("name", "user.events", "fetchable", true),
			rec("name", "metrics", "fetchable", false),
		}},
		{match: "dt.system.buckets", records: []map[string]interface{}{
			rec("name", "default_logs", "dt.system.table", "logs", "records", float64(1000), "has_access", true),
			rec("name", "default_spans", "dt.system.table", "spans", "records", float64(500), "has_access", true),
			// bizevents is in the catalog but empty within retention → no-data.
			rec("name", "default_bizevents", "dt.system.table", "bizevents", "records", float64(0), "has_access", true),
			// user.events holds plenty tenant-wide but matches nothing → empty.
			rec("name", "default_rum", "dt.system.table", "user.events", "records", float64(2000), "has_access", true),
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
		Where:      `k8s.namespace.name == "payments"`,
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

	want := StateSummary{Live: 2, Stale: 1, Empty: 1, NoData: 1, Absent: 1}
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
}

func TestTimeFieldOnlyWithDataObject(t *testing.T) {
	err := ValidateDefinitions(map[string]*CapabilityDef{
		"bad": {MetricKey: "dt.*", TimeField: "timestamp"},
	})
	if err == nil {
		t.Fatal("timeField on a non-dataObject shape must be rejected")
	}
}

func TestBuiltinSpansOverridesTimeField(t *testing.T) {
	if got := BuiltinDefinitions()["spans"].TimeField; got != "start_time" {
		t.Errorf("builtin spans timeField = %q, want start_time", got)
	}
	if got := BuiltinDefinitions()["logs"].TimeField; got != "" {
		t.Errorf("logs should use the default time field, got %q", got)
	}
}
