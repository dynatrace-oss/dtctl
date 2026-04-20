package dqlcost

import (
	"strings"
	"testing"
)

func TestLint_COST001_MissingFrom(t *testing.T) {
	findings := Lint(`fetch logs | filter loglevel == "ERROR" | limit 100`)
	if !hasRule(findings, "COST001") {
		t.Fatalf("expected COST001, got %v", findings)
	}
}

func TestLint_COST001_WithFromPasses(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | filter loglevel == "ERROR" | limit 100`)
	if hasRule(findings, "COST001") {
		t.Fatalf("COST001 should not fire when from: is present: %v", findings)
	}
}

func TestLint_COST002_LogMakeTimeseries(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | makeTimeseries cnt = count(), interval:5m`)
	if !hasRule(findings, "COST002") {
		t.Fatalf("expected COST002, got %v", findings)
	}
}

func TestLint_COST003_TransformedFilter(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | filter lower(k8s.namespace.name) == "payments"`)
	if !hasRule(findings, "COST003") {
		t.Fatalf("expected COST003, got %v", findings)
	}
}

func TestLint_COST004_SortAfterFetch(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | sort timestamp desc | filter loglevel == "ERROR"`)
	if !hasRule(findings, "COST004") {
		t.Fatalf("expected COST004, got %v", findings)
	}
}

func TestLint_COST005_LimitBeforeSummarize(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | limit 100 | summarize c = count(), by:{loglevel}`)
	if !hasRule(findings, "COST005") {
		t.Fatalf("expected COST005, got %v", findings)
	}
}

func TestLint_COST006_WildcardMatchesValue(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | filter matchesValue(content, "*error*")`)
	if !hasRule(findings, "COST006") {
		t.Fatalf("expected COST006, got %v", findings)
	}
}

func TestLint_COST008_MissingScanLimit(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h | filter loglevel == "ERROR"`)
	if !hasRule(findings, "COST008") {
		t.Fatalf("expected COST008, got %v", findings)
	}
}

func TestLint_COST008_PresentPasses(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-1h, scanLimitGBytes:50 | filter loglevel == "ERROR"`)
	if hasRule(findings, "COST008") {
		t.Fatalf("COST008 should not fire when scanLimitGBytes is present: %v", findings)
	}
}

func TestLint_COST009_LongWindowNoSampling(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-7d | filter loglevel == "ERROR"`)
	if !hasRule(findings, "COST009") {
		t.Fatalf("expected COST009, got %v", findings)
	}
}

func TestLint_COST009_With48h(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-48h | filter loglevel == "ERROR"`)
	if !hasRule(findings, "COST009") {
		t.Fatalf("expected COST009 for 48h window, got %v", findings)
	}
}

func TestLint_COST009_SamplingPresentPasses(t *testing.T) {
	findings := Lint(`fetch logs, from:now()-7d, samplingRatio:100 | filter loglevel == "ERROR"`)
	if hasRule(findings, "COST009") {
		t.Fatalf("COST009 should not fire when samplingRatio is present: %v", findings)
	}
}

func TestLint_TimeseriesIsFree(t *testing.T) {
	// timeseries on metrics should produce no COST001/COST002/COST008 findings.
	findings := Lint(`timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}, from:now()-1h`)
	for _, bad := range []string{"COST001", "COST002", "COST008"} {
		if hasRule(findings, bad) {
			t.Errorf("%s should not fire on timeseries query: %v", bad, findings)
		}
	}
}

func TestLint_GoldPath(t *testing.T) {
	// Canonical cost-optimized query should produce zero findings.
	q := `fetch logs, from:now()-1h, scanLimitGBytes:50
| filter dt.system.bucket == "default_logs"
| filter loglevel == "ERROR"
| fieldsKeep timestamp, content, k8s.pod.name
| makeTimeseries c = count(), by:{k8s.pod.name}, from:now()-1h`
	findings := Lint(q)
	// COST002 is acceptable here (we still use fetch logs | makeTimeseries).
	// All other rules should be silent.
	for _, f := range findings {
		if f.Rule != "COST002" {
			t.Errorf("unexpected finding on gold-path query: %+v", f)
		}
	}
}

func TestMaxSeverity(t *testing.T) {
	findings := []Finding{
		{Rule: "A", Severity: SeverityInfo},
		{Rule: "B", Severity: SeverityError},
		{Rule: "C", Severity: SeverityWarn},
	}
	if got := MaxSeverity(findings); got != SeverityError {
		t.Fatalf("MaxSeverity = %v, want error", got)
	}
	if got := MaxSeverity(nil); got != Severity(-1) {
		t.Fatalf("MaxSeverity(nil) = %v, want -1", got)
	}
}

func TestFormat(t *testing.T) {
	findings := []Finding{{Rule: "COST001", Severity: SeverityError, Message: "m", Suggestion: "s"}}
	got := Format(findings)
	if !strings.Contains(got, "COST001") || !strings.Contains(got, "error") {
		t.Fatalf("Format output missing expected fields: %q", got)
	}
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
