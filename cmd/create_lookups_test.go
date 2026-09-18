package cmd

import (
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
	t.Cleanup(func() {
		for name := range flags {
			_ = createLookupCmd.Flags().Set(name, "")
		}
	})
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
