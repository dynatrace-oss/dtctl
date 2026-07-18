package inventory

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockResponse struct {
	match   string
	records []map[string]interface{}
	err     error
}

type mockRunner struct {
	responses []mockResponse
	calls     []string
}

func (m *mockRunner) RunQuery(_ context.Context, dql string) (*RunResult, error) {
	m.calls = append(m.calls, dql)
	for _, r := range m.responses {
		if strings.Contains(dql, r.match) {
			if r.err != nil {
				return nil, r.err
			}
			return &RunResult{Records: r.records, Seconds: 0.1}, nil
		}
	}
	return &RunResult{Seconds: 0.1}, nil // default: empty result
}

func rec(kv ...interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func testDefs() map[string]*CapabilityDef {
	return map[string]*CapabilityDef{
		"spans":       {DataObject: "spans"},
		"rum":         {DataObject: "user.events"},
		"hosts":       {EntityTypes: []string{"HOST"}},
		"azure":       {EntityTypes: []string{"AZURE_*"}},
		"k8s-metrics": {MetricKey: "dt.kubernetes.*"},
		"genai":       {Probe: "fetch spans GENAIPROBE | limit 1", Window: "24h"},
		"rap":         {Probe: "fetch security.events RAPPROBE | limit 1", Window: "24h"},
	}
}

func testRunner() *mockRunner {
	return &mockRunner{responses: []mockResponse{
		{match: "dt.system.data_objects", records: []map[string]interface{}{
			rec("name", "logs", "fetchable", true), rec("name", "spans", "fetchable", true),
			rec("name", "dt.davis.events", "fetchable", true), rec("name", "metrics", "fetchable", false),
		}},
		{match: "dt.system.buckets", records: []map[string]interface{}{rec("name", "default_logs")}},
		{match: `smartscapeNodes "*"`, records: []map[string]interface{}{
			rec("type", "HOST", "c", float64(5)),
			rec("type", "K8S_POD", "c", float64(10)),
		}},
		{match: "metrics from:", records: []map[string]interface{}{rec("metric.key", "dt.kubernetes.container.cpu")}},
		{match: "GENAIPROBE", records: []map[string]interface{}{rec("gen_ai.system", "x")}},
		// RAPPROBE deliberately unmatched: empty result → absent
	}}
}

func TestDiscoverInventory(t *testing.T) {
	runner := testRunner()
	inv, err := Discover(context.Background(), runner, testDefs(), DiscoverOptions{
		ContextName: "test",
		Segments:    []SegmentInfo{{UID: "s1", Name: "prod"}},
	})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	report := inv.Discovery

	if want := []string{"logs", "spans", "dt.davis.events"}; strings.Join(inv.DataObjects, ",") != strings.Join(want, ",") {
		t.Errorf("DataObjects = %v, want %v", inv.DataObjects, want)
	}
	if len(inv.Unfetchable) != 1 || inv.Unfetchable[0] != "metrics" {
		t.Errorf("Unfetchable = %v, want [metrics]", inv.Unfetchable)
	}
	if len(inv.Buckets) != 1 || inv.Buckets[0] != "default_logs" {
		t.Errorf("Buckets = %v", inv.Buckets)
	}
	if inv.EntityTypes["HOST"] != 5 || inv.EntityTypes["K8S_POD"] != 10 {
		t.Errorf("EntityTypes = %v", inv.EntityTypes)
	}

	wantPresent := []string{"genai", "hosts", "k8s-metrics", "spans"}
	if strings.Join(inv.Capabilities, ",") != strings.Join(wantPresent, ",") {
		t.Errorf("Capabilities = %v, want %v", inv.Capabilities, wantPresent)
	}
	// Every absent entry carries its evidence.
	wantAbsent := map[string]string{
		"azure": "no AZURE_* entities in the live census",
		"rap":   "discovery probe returned no rows in 24h",
		"rum":   "no user.events in the data-object catalog",
	}
	if len(inv.Absent) != len(wantAbsent) {
		t.Fatalf("Absent = %v, want %d entries", inv.Absent, len(wantAbsent))
	}
	for _, a := range inv.Absent {
		name, evidence, _ := strings.Cut(a, " (")
		if want, ok := wantAbsent[name]; !ok || !strings.Contains(evidence, want) {
			t.Errorf("absent entry %q, want evidence %q", a, wantAbsent[name])
		}
	}

	if len(inv.Segments) != 1 || inv.Segments[0].UID != "s1" {
		t.Errorf("Segments = %v", inv.Segments)
	}
	// Notes: unfetchable + davis canonical stream + census lookback.
	if len(inv.Notes) != 3 {
		t.Errorf("Notes = %v, want 3", inv.Notes)
	}
	// Battery: catalog, buckets, census, metric catalog, 2 probes.
	if report.Queries != 6 {
		t.Errorf("report.Queries = %d, want 6 (calls: %v)", report.Queries, runner.calls)
	}
}

func TestDiscoverSkipsMetricCatalogWithoutMetricDefs(t *testing.T) {
	runner := testRunner()
	defs := map[string]*CapabilityDef{"spans": {DataObject: "spans"}}
	if _, err := Discover(context.Background(), runner, defs, DiscoverOptions{}); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	for _, c := range runner.calls {
		if strings.HasPrefix(c, "metrics ") {
			t.Errorf("metric catalog queried without any metricKey definition: %v", runner.calls)
		}
	}
}

func TestDiscoverBudgetStopsProbes(t *testing.T) {
	runner := testRunner()
	// 3 queries: catalog, buckets, census — no budget left for probes.
	inv, err := Discover(context.Background(), runner, testDefs(), DiscoverOptions{BudgetQueries: 3})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	report := inv.Discovery
	if report.Queries != 3 {
		t.Errorf("report.Queries = %d, want 3", report.Queries)
	}
	// Probe-shaped capabilities degrade to absent; the partial state is noted.
	found := false
	for _, n := range inv.Notes {
		if strings.Contains(n, "budget exhausted") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing budget-exhausted note: %v", inv.Notes)
	}
}

func TestDiscoverCatalogErrorAborts(t *testing.T) {
	runner := &mockRunner{responses: []mockResponse{
		{match: "dt.system.data_objects", err: fmt.Errorf("boom")},
	}}
	if _, err := Discover(context.Background(), runner, testDefs(), DiscoverOptions{}); err == nil {
		t.Fatal("Discover() should abort when the data-object catalog is unavailable")
	}
}

func TestDiscoverCatalogFallbackWithoutUsableWith(t *testing.T) {
	// The partitioned catalog query errors (no usable_with column); the flat
	// list must be used instead.
	runner := &mockRunner{responses: []mockResponse{
		{match: "usable_with", err: fmt.Errorf("FIELD_DOES_NOT_EXIST")},
		{match: "dt.system.data_objects | fields name", records: []map[string]interface{}{rec("name", "logs")}},
	}}
	inv, err := Discover(context.Background(), runner, map[string]*CapabilityDef{"logs": {DataObject: "logs"}}, DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(inv.DataObjects) != 1 || inv.DataObjects[0] != "logs" {
		t.Errorf("DataObjects = %v, want [logs]", inv.DataObjects)
	}
	if len(inv.Capabilities) != 1 || inv.Capabilities[0] != "logs" {
		t.Errorf("Capabilities = %v, want [logs]", inv.Capabilities)
	}
}
