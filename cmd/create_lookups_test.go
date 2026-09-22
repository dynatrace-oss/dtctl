package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setCreateLookupFlags points the shared command at a file and resets the
// flags afterwards, since the cobra command is a package-level singleton.
func setCreateLookupFlags(t *testing.T, file string) {
	t.Helper()

	flags := map[string]string{
		"file":         file,
		"path":         "/lookups/test/t",
		"lookup-field": "id",
	}
	for name, value := range flags {
		if err := createLookupCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	t.Cleanup(func() { resetFlagSet(createLookupCmd.Flags()) })
}

// TestCreateLookupDryRun_ReportsAutoDetectedPattern covers the dry-run
// preview: it used to print "(auto-detect from CSV)" without saying what the
// pattern would be, which is exactly the information needed to spot the
// mismatch behind #471.
func TestCreateLookupDryRun_ReportsAutoDetectedPattern(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t.csv")
	// A row with an empty cell plus a quoted cell containing the delimiter.
	if err := os.WriteFile(file, []byte("id,name,owner\n1,alpha,\n3,\"gamma, inc\",team-c\n"), 0o600); err != nil {
		t.Fatalf("write CSV: %v", err)
	}
	setCreateLookupFlags(t, file)

	// The assertions below are about the human rendering, so the mode is pinned:
	// dry-run output is enveloped in agent mode, and another test in this package
	// leaves agentMode set.
	withAgentMode(t, false)

	originalDryRun := dryRun
	t.Cleanup(func() { dryRun = originalDryRun })
	dryRun = true

	out := captureStdout(t, func() {
		if err := createLookupCmd.RunE(createLookupCmd, nil); err != nil {
			t.Fatalf("RunE() error = %v", err)
		}
	})

	wantPattern := "Parse Pattern: LD*:id '\\t' LD*:name '\\t' LD*:owner (auto-detected)"
	if !strings.Contains(out, wantPattern) {
		t.Errorf("output does not contain %q:\n%s", wantPattern, out)
	}
	for _, want := range []string{"Records: 2", "re-emitted with tab separators"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

// TestCreateLookupDryRun_RejectsUnparseableCSV makes sure the dry-run reports
// the input problem instead of a pattern that could never match.
func TestCreateLookupDryRun_RejectsUnparseableCSV(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t.csv")
	if err := os.WriteFile(file, []byte("id,name\n1,alpha,extra\n"), 0o600); err != nil {
		t.Fatalf("write CSV: %v", err)
	}
	setCreateLookupFlags(t, file)

	originalDryRun := dryRun
	t.Cleanup(func() { dryRun = originalDryRun })
	dryRun = true

	var err error
	_ = captureStdout(t, func() {
		err = createLookupCmd.RunE(createLookupCmd, nil)
	})

	if err == nil {
		t.Fatal("RunE() error = nil, want an error for a row with extra fields")
	}
	if !strings.Contains(err.Error(), "line 2 has 3 fields") {
		t.Errorf("error = %q, want it to mention the offending line", err)
	}
}

// TestCreateLookupDryRun_AgentModeEmitsEnvelope covers the same call site in
// agent mode: the plan must arrive as JSON on stdout, because that is the stream
// an agent decodes and an error from this command has always been enveloped.
func TestCreateLookupDryRun_AgentModeEmitsEnvelope(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t.csv")
	if err := os.WriteFile(file, []byte("id,name\n1,alpha\n"), 0o600); err != nil {
		t.Fatalf("write CSV: %v", err)
	}
	setCreateLookupFlags(t, file)
	withAgentMode(t, true)

	originalDryRun := dryRun
	t.Cleanup(func() { dryRun = originalDryRun })
	dryRun = true

	out := captureStdout(t, func() {
		if err := createLookupCmd.RunE(createLookupCmd, nil); err != nil {
			t.Fatalf("RunE() error = %v", err)
		}
	})

	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			DryRun  bool              `json:"dry_run"`
			Verb    string            `json:"verb"`
			Details map[string]string `json:"details"`
			Message string            `json:"message"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("agent-mode dry run is not valid JSON: %v\n%s", err, out)
	}
	if !resp.OK || !resp.Result.DryRun || resp.Result.Verb != "create" {
		t.Errorf("unexpected envelope: %+v", resp.Result)
	}
	if got := resp.Result.Details["lookup_field"]; got != "id" {
		t.Errorf("details[lookup_field] = %q, want \"id\"", got)
	}
	if !strings.Contains(resp.Result.Message, "Dry run: would create lookup table") {
		t.Errorf("message lost the human text: %q", resp.Result.Message)
	}
}
