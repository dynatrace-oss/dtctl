package output

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSpillWriter_CommitsAtomically(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSpillWriter(dir)
	if err != nil {
		t.Fatalf("NewSpillWriter: %v", err)
	}
	if _, err := io.WriteString(w.Writer(), "{\"a\":1}\n{\"a\":2}\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if w.Bytes() != 16 {
		t.Errorf("Bytes() = %d, want 16", w.Bytes())
	}

	target := filepath.Join(dir, "q-abc.jsonl")
	n, err := w.Commit(target)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if n != 16 {
		t.Errorf("Commit returned %d bytes, want 16", n)
	}

	got, err := os.ReadFile(target) //nolint:gosec // test-local temp path
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "{\"a\":1}\n{\"a\":2}\n" {
		t.Errorf("file holds %q", got)
	}

	// Windows does not carry Unix permission bits.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != spillFileMode {
			t.Errorf("mode = %v, want %v", perm, spillFileMode)
		}
	}
	assertNoTempLeftovers(t, dir)
}

func TestSpillWriter_AbortLeavesNothingCommitted(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSpillWriter(dir)
	if err != nil {
		t.Fatalf("NewSpillWriter: %v", err)
	}
	if _, err := io.WriteString(w.Writer(), "partial"); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Abort()
	w.Abort() // idempotent, so it works as a defer alongside Commit

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("abort left %d entries behind: %v", len(entries), entries)
	}
}

func TestSpillWriter_AbortAfterCommitKeepsTheFile(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSpillWriter(dir)
	if err != nil {
		t.Fatalf("NewSpillWriter: %v", err)
	}
	if _, err := io.WriteString(w.Writer(), "done"); err != nil {
		t.Fatalf("write: %v", err)
	}
	target := filepath.Join(dir, "q-abc.jsonl")
	if _, err := w.Commit(target); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	w.Abort()

	if _, err := os.Stat(target); err != nil {
		t.Errorf("Abort removed a committed file: %v", err)
	}
}

// rename(2) is only atomic within one filesystem, so a target outside the
// streaming directory has to be refused rather than silently copied.
func TestSpillWriter_RefusesATargetOutsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSpillWriter(dir)
	if err != nil {
		t.Fatalf("NewSpillWriter: %v", err)
	}
	defer w.Abort()

	if _, err := w.Commit(filepath.Join(t.TempDir(), "elsewhere.jsonl")); err == nil {
		t.Error("Commit to another directory should fail")
	}
}

func TestSpillWriter_CreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "results")
	w, err := NewSpillWriter(dir)
	if err != nil {
		t.Fatalf("NewSpillWriter: %v", err)
	}
	defer w.Abort()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != spillDirMode {
		t.Errorf("dir mode = %v, want %v", perm, spillDirMode)
	}
}

func assertNoTempLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), tmpSuffix) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
