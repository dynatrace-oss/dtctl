package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// update controls whether golden files are regenerated.
// Run with -update to regenerate: go test ./... -update
var update = flag.Bool("update", false, "update golden files")

// ansiRegex matches ANSI escape sequences (color codes, cursor control, etc.)
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes all ANSI escape sequences from a string.
// Use this for comparing output that may contain color codes.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// goldenDir returns the path to the testdata/golden directory relative to the
// calling test file. It walks up the call stack to find the test's source
// directory, then resolves testdata/golden from there.
func goldenDir() string {
	_, filename, _, ok := runtime.Caller(2) // caller of the public function
	if !ok {
		// Fallback: use CWD-relative path (works when running from cmd/)
		return filepath.Join("testdata", "golden")
	}
	return filepath.Join(filepath.Dir(filename), "testdata", "golden")
}

// normalizeLineEndings replaces \r\n with \n to ensure consistent comparison
// across platforms (Windows checks out files with CRLF by default).
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// AssertGolden compares actual output against a golden file identified by name.
// The golden file is stored at testdata/golden/<name>.golden relative to the
// calling test file's directory.
//
// When run with -update, the golden file is created or overwritten with actual.
// When run without -update, the test fails if the file does not exist or if
// the content does not match.
func AssertGolden(t *testing.T, name string, actual string) {
	t.Helper()

	goldenPath := filepath.Join(goldenDir(), name+".golden")

	if *update {
		dir := filepath.Dir(goldenPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found: %s\nRun with -update to create:\n  go test ./... -update\n\nActual output:\n%s", goldenPath, actual)
	}

	expectedStr := normalizeLineEndings(string(expected))
	actualStr := normalizeLineEndings(actual)

	if expectedStr != actualStr {
		t.Errorf("output does not match golden file %s\n\n--- expected ---\n%s\n--- actual ---\n%s\n--- diff hint ---\nRun with -update to accept the new output:\n  go test ./pkg/output/ -update",
			goldenPath, expectedStr, actualStr)
	}
}

// AssertGoldenStripped is like AssertGolden but strips ANSI escape sequences
// from the actual output before comparing. Use this for output that contains
// terminal color codes (e.g., charts, sparklines, braille graphs).
func AssertGoldenStripped(t *testing.T, name string, actual string) {
	t.Helper()
	AssertGolden(t, name, StripANSI(actual))
}
