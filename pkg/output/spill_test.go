package output

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSpillHash_StableAndSensitive(t *testing.T) {
	h1 := SpillHash("fetch logs", "start", "end")
	h2 := SpillHash("fetch logs", "start", "end")
	if h1 != h2 {
		t.Errorf("hash not stable: %q != %q", h1, h2)
	}
	if SpillHash("fetch logs", "start", "end") == SpillHash("fetch spans", "start", "end") {
		t.Errorf("hash should differ for different queries")
	}
	// guard against trivial concatenation collisions
	if SpillHash("ab", "c") == SpillHash("a", "bc") {
		t.Errorf("hash should domain-separate parts")
	}
}

func TestSanitizeContextName(t *testing.T) {
	cases := map[string]string{
		"prod":        "prod",
		"my prod/env": "my-prod-env",
		"../../etc":   "etc",
		"":            "default",
		"a.b_c-d":     "a.b_c-d",
		"///":         "default",
	}
	for in, want := range cases {
		if got := SanitizeContextName(in); got != want {
			t.Errorf("SanitizeContextName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteSpillFile_AtomicAndPerms(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "q-abc.json")

	n, err := WriteSpillFile(target, func(w io.Writer) error {
		_, e := io.WriteString(w, "hello world")
		return e
	})
	if err != nil {
		t.Fatalf("WriteSpillFile: %v", err)
	}
	if n != int64(len("hello world")) {
		t.Errorf("bytes = %d, want %d", n, len("hello world"))
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q", string(data))
	}

	// no leftover .tmp files
	entries, _ := os.ReadDir(filepath.Dir(target))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), tmpSuffix) {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}

	if runtime.GOOS != "windows" {
		info, _ := os.Stat(target)
		if perm := info.Mode().Perm(); perm != spillFileMode {
			t.Errorf("file perm = %o, want %o", perm, spillFileMode)
		}
	}
}

func TestWriteSpillFile_WriteErrorLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "q-err.json")
	_, err := WriteSpillFile(target, func(io.Writer) error {
		return io.ErrClosedPipe
	})
	if err == nil {
		t.Fatal("expected error from write callback")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("target should not exist after a failed write")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("no files (incl. tmp) should remain, found %d", len(entries))
	}
}

func TestWriteSidecarAndPath(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "q-7f3a.json")
	if got := SidecarPathFor(dataPath); got != filepath.Join(dir, "q-7f3a.manifest.json") {
		t.Errorf("SidecarPathFor = %q", got)
	}
	sc := &SidecarManifest{EnvelopeVersion: EnvelopeVersion, Format: "json", Query: "fetch logs", Rows: 3, Created: time.Now()}
	if err := WriteSidecar(dataPath, sc); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}
	if _, err := os.Stat(SidecarPathFor(dataPath)); err != nil {
		t.Errorf("sidecar not written: %v", err)
	}
}

func TestProbeWritable(t *testing.T) {
	if !ProbeWritable(filepath.Join(t.TempDir(), "newdir")) {
		t.Errorf("expected fresh temp subdir to be writable")
	}
	// A path whose parent is a regular file cannot be created.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ProbeWritable(filepath.Join(f, "sub")) {
		t.Errorf("expected path under a file to be non-writable")
	}
}

func TestPruneOldSpills(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "prod", "q-old.json")
	fresh := filepath.Join(base, "prod", "q-new.json")
	stray := filepath.Join(base, "prod", "q-old.123.tmp")
	for _, p := range []string{old, fresh, stray} {
		if _, err := WriteSpillFile(p, func(w io.Writer) error { _, e := io.WriteString(w, "x"); return e }); err != nil {
			t.Fatal(err)
		}
	}
	// Age out the old file and the stray tmp.
	past := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, past, past)
	_ = os.Chtimes(stray, past, past)

	PruneOldSpills(base, 24*time.Hour)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file should be pruned")
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("stray tmp should be pruned")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file should survive: %v", err)
	}
}

func TestPruneOldSpills_Throttled(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "q-old.json")
	if _, err := WriteSpillFile(old, func(w io.Writer) error { _, e := io.WriteString(w, "x"); return e }); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, past, past)

	// First prune writes the marker and removes the old file.
	PruneOldSpills(base, 24*time.Hour)
	// Recreate an old file; a second immediate prune must be throttled (skipped).
	if _, err := WriteSpillFile(old, func(w io.Writer) error { _, e := io.WriteString(w, "x"); return e }); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(old, past, past)
	PruneOldSpills(base, 24*time.Hour)
	if _, err := os.Stat(old); err != nil {
		t.Errorf("second prune within throttle window should have been skipped, but file was removed")
	}
}

func TestMeasureSerializedBytes(t *testing.T) {
	records := []map[string]interface{}{{"a": "x"}, {"a": "y"}}
	n, enc := MeasureSerializedBytes(records, "json")
	if n <= 0 {
		t.Errorf("measured bytes = %d, want > 0", n)
	}
	if enc != "json" {
		t.Errorf("encoding = %q, want json", enc)
	}
	// table is not emitted in agent mode -> measured as json
	if _, enc := MeasureSerializedBytes(records, "table"); enc != "json" {
		t.Errorf("table should be measured as json, got %q", enc)
	}
	if _, enc := MeasureSerializedBytes(records, "csv"); enc != "csv" {
		t.Errorf("csv encoding = %q, want csv", enc)
	}
}
