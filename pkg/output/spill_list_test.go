package output

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListSpillFiles_MissingDirIsEmpty(t *testing.T) {
	entries, err := ListSpillFiles(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("missing dir should yield no entries, got %d", len(entries))
	}
}

func TestListSpillFiles_ProvenanceAndSkips(t *testing.T) {
	dir := t.TempDir()

	// A spilled file WITH a sidecar.
	withSidecar := filepath.Join(dir, "q-aaaa.jsonl")
	if err := os.WriteFile(withSidecar, []byte("{\"a\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if err := WriteSidecar(withSidecar, &SidecarManifest{
		EnvelopeVersion: EnvelopeVersion,
		Format:          "jsonl",
		Query:           "fetch logs | limit 10",
		Rows:            10,
		Sampled:         true,
		SamplingRatio:   0.5,
		ContextName:     "prod",
		TenantID:        "abc12345",
		Bytes:           4096,
		Created:         created,
	}); err != nil {
		t.Fatal(err)
	}

	// A bare spilled file with NO sidecar.
	bare := filepath.Join(dir, "q-bbbb.csv")
	if err := os.WriteFile(bare, []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Noise that must be skipped: a stray temp file and the prune marker.
	if err := os.WriteFile(filepath.Join(dir, "q-cccc.jsonl.tmp"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, pruneMarkerName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := ListSpillFiles(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 data files (sidecar + bare), got %d: %+v", len(entries), entries)
	}

	byPath := map[string]SpillFileEntry{}
	for _, e := range entries {
		byPath[filepath.Base(e.Path)] = e
	}

	ws := byPath["q-aaaa.jsonl"]
	if ws.Query != "fetch logs | limit 10" || ws.Rows != 10 || !ws.Sampled || ws.TenantID != "abc12345" {
		t.Errorf("sidecar provenance not carried: %+v", ws)
	}
	if ws.Bytes != 4096 {
		t.Errorf("sidecar Bytes should win over file size, got %d", ws.Bytes)
	}
	if !ws.Created.Equal(created) {
		t.Errorf("sidecar Created should win, got %v want %v", ws.Created, created)
	}
	if ws.SidecarMissing {
		t.Errorf("file with sidecar must not be flagged SidecarMissing")
	}

	bareEntry := byPath["q-bbbb.csv"]
	if !bareEntry.SidecarMissing {
		t.Errorf("bare file must be flagged SidecarMissing: %+v", bareEntry)
	}
	if bareEntry.Format != "csv" {
		t.Errorf("bare file format should derive from extension, got %q", bareEntry.Format)
	}
	if bareEntry.Bytes == 0 {
		t.Errorf("bare file should fall back to on-disk size")
	}
}

func TestListSpillFiles_SortedMostRecentFirst(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = WriteSidecar(p, &SidecarManifest{Format: "jsonl", Created: time.Now().Add(-age).UTC()})
	}
	mk("q-old.jsonl", 3*time.Hour)
	mk("q-new.jsonl", 1*time.Minute)
	mk("q-mid.jsonl", 1*time.Hour)

	entries, err := ListSpillFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"q-new.jsonl", "q-mid.jsonl", "q-old.jsonl"}
	for i, w := range want {
		if got := filepath.Base(entries[i].Path); got != w {
			t.Errorf("position %d = %q, want %q (most-recent-first)", i, got, w)
		}
	}
}
