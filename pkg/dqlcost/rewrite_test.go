package dqlcost

import (
	"strings"
	"testing"
)

func TestRewrite_AddsDefaultFrom(t *testing.T) {
	in := `fetch logs | filter loglevel == "ERROR"`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if !strings.Contains(out, "from:now()-2h") {
		t.Fatalf("missing default from: %s", out)
	}
	if !containsRule(changes, "COST001") {
		t.Fatalf("expected COST001 change, got %+v", changes)
	}
}

func TestRewrite_PreservesExistingFrom(t *testing.T) {
	in := `fetch logs, from:now()-1h | filter loglevel == "ERROR"`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if containsRule(changes, "COST001") {
		t.Fatalf("COST001 rewrite should not fire when from: is present: %+v", changes)
	}
	if strings.Count(out, "from:") != 1 {
		t.Fatalf("duplicate from: in %s", out)
	}
}

func TestRewrite_AddsScanLimit(t *testing.T) {
	in := `fetch logs, from:now()-1h | filter loglevel == "ERROR"`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if !strings.Contains(out, "scanLimitGBytes:500") {
		t.Fatalf("missing scanLimit: %s", out)
	}
	if !containsRule(changes, "COST008") {
		t.Fatalf("expected COST008 change, got %+v", changes)
	}
}

func TestRewrite_PreservesExistingScanLimit(t *testing.T) {
	in := `fetch logs, from:now()-1h, scanLimitGBytes:50 | filter loglevel == "ERROR"`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if containsRule(changes, "COST008") {
		t.Fatalf("COST008 rewrite should not fire when scanLimitGBytes is present: %+v", changes)
	}
	if strings.Count(out, "scanLimitGBytes:") != 1 {
		t.Fatalf("duplicate scanLimitGBytes in %s", out)
	}
}

func TestRewrite_DoesNotMoveLimitByDefault(t *testing.T) {
	// Tenant measurement showed moving limit after summarize increases scan
	// by ~40× because Grail short-circuits when limit appears early.
	in := `fetch logs, from:now()-1h, scanLimitGBytes:50 | filter x == "y" | limit 100 | summarize c = count(), by:{loglevel}`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if containsRule(changes, "COST005") {
		t.Fatalf("COST005 should not fire by default; got %+v", changes)
	}
	if out != in {
		t.Fatalf("limit/summarize order should be preserved by default:\nin:  %s\nout: %s", in, out)
	}
}

func TestRewrite_OptInMovesLimit(t *testing.T) {
	// Still supported under explicit opt-in for edge cases where the caller
	// genuinely wants an aggregate over the full dataset rather than a sample.
	opts := DefaultRewriteOptions()
	opts.MoveLimitAfter = true
	in := `fetch logs, from:now()-1h, scanLimitGBytes:50 | filter x == "y" | limit 100 | summarize c = count(), by:{loglevel}`
	out, changes := Rewrite(in, opts)
	if !containsRule(changes, "COST005") {
		t.Fatalf("expected COST005 change under opt-in, got %+v", changes)
	}
	sumIdx := strings.Index(out, "summarize")
	limIdx := strings.Index(out, "limit 100")
	if sumIdx > limIdx {
		t.Fatalf("summarize should precede limit after rewrite: %s", out)
	}
}

func TestRewrite_LeavesCanonicalQueryUnchanged(t *testing.T) {
	in := `fetch logs, from:now()-1h, scanLimitGBytes:50 | filter loglevel == "ERROR" | summarize c = count() | limit 10`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if out != in {
		t.Fatalf("clean query should not be rewritten:\nin:  %s\nout: %s", in, out)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %+v", changes)
	}
}

func TestRewrite_TimeseriesUntouched(t *testing.T) {
	// timeseries is free + already time-bound; rewriter should not touch it.
	in := `timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}, from:now()-1h`
	out, changes := Rewrite(in, DefaultRewriteOptions())
	if out != in {
		t.Fatalf("timeseries query should be left alone:\nin:  %s\nout: %s", in, out)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %+v", changes)
	}
}

func TestRewrite_OptInIndividualRules(t *testing.T) {
	in := `fetch logs | filter loglevel == "ERROR"`
	// Only AddDefaultFrom enabled.
	opts := RewriteOptions{AddDefaultFrom: true, DefaultFrom: "now()-1h"}
	out, changes := Rewrite(in, opts)
	if !strings.Contains(out, "from:now()-1h") {
		t.Fatalf("missing from: %s", out)
	}
	if strings.Contains(out, "scanLimitGBytes") {
		t.Fatalf("scan limit should not have been added: %s", out)
	}
	if len(changes) != 1 || changes[0].Rule != "COST001" {
		t.Fatalf("expected exactly one COST001 change, got %+v", changes)
	}
}

func containsRule(changes []Change, rule string) bool {
	for _, c := range changes {
		if c.Rule == rule {
			return true
		}
	}
	return false
}
