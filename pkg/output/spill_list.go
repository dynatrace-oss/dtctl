package output

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SpillFileEntry describes one spilled result data file discovered in a spill
// directory, enriched with its sidecar provenance (D34) when present. It is the
// per-file element of a KindFileList listing — enough for an agent to recognise
// a lost handle (the original query, row count, when it was spilled, whether it
// was sampled) and inspect it by path, without re-querying Grail.
type SpillFileEntry struct {
	Path    string    `json:"path"`
	Format  string    `json:"format"`
	Bytes   int64     `json:"bytes"`
	Created time.Time `json:"created"`
	// Sidecar-derived provenance — empty/zero when the file has no manifest.
	Query         string  `json:"query,omitempty"`
	Rows          int     `json:"rows,omitempty"`
	Sampled       bool    `json:"sampled,omitempty"`
	SamplingRatio float64 `json:"sampling_ratio,omitempty"`
	ContextName   string  `json:"context_name,omitempty"`
	TenantID      string  `json:"tenant_id,omitempty"`
	// SidecarMissing flags a bare data file with no manifest next to it: its
	// provenance (query, sampling) is unknown, so a consumer must not trust the
	// absence of `sampled` as proof the data is a full population.
	SidecarMissing bool `json:"sidecar_missing,omitempty"`
}

// SpillList is the result payload for the KindFileList envelope (D2): the
// spilled files visible in the directory that was listed. It carries the
// directory and whether it is the managed cache (vs. a user-chosen --spill-dir
// that opts out of the managed privacy guarantees, D25) so the listing is
// self-describing.
type SpillList struct {
	Kind    string           `json:"kind"`
	Dir     string           `json:"dir"`
	Managed bool             `json:"managed"`
	Files   []SpillFileEntry `json:"files"`
}

// spillDataExtensions is the closed set of data-file extensions a spill can
// carry (mirrors the spill format set). Sidecars (.manifest.json), temp files
// (.tmp), the prune marker (.last-prune) and probe files (.probe-*) are not in
// this set and so are skipped by the listing.
var spillDataExtensions = map[string]bool{
	".jsonl":   true,
	".json":    true,
	".csv":     true,
	".parquet": true,
}

// ListSpillFiles enumerates spilled result data files directly in dir (it does
// not recurse — a managed listing is scoped to a single context partition by the
// caller, D9), pairing each with its sidecar manifest when one is present. It
// skips sidecar, temp, probe and prune-marker files. A non-existent dir is not
// an error: it yields an empty list (nothing has been spilled in this context
// yet). Entries are sorted most-recent-first — the order most useful for
// recovering a handle that just aged out of context.
func ListSpillFiles(dir string) ([]SpillFileEntry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []SpillFileEntry
	for _, de := range ents {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		// A sidecar ends with .manifest.json — exclude it before the extension
		// check (its extension is .json, a data extension).
		if strings.HasSuffix(name, ".manifest.json") || strings.HasSuffix(name, tmpSuffix) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !spillDataExtensions[ext] {
			continue
		}

		path := filepath.Join(dir, name)
		entry := SpillFileEntry{Path: path, Format: strings.TrimPrefix(ext, ".")}

		if info, ierr := de.Info(); ierr == nil {
			entry.Bytes = info.Size()
			entry.Created = info.ModTime().UTC()
		}

		// Sidecar provenance wins over filesystem metadata where it exists: it is
		// authoritative for created time, byte count and row count, and is the only
		// source for query/sampling/tenant (D34).
		sc, scerr := ReadSidecar(path)
		switch {
		case scerr != nil || sc == nil:
			entry.SidecarMissing = true
		default:
			entry.Query = sc.Query
			entry.Rows = sc.Rows
			entry.Sampled = sc.Sampled
			entry.SamplingRatio = sc.SamplingRatio
			entry.ContextName = sc.ContextName
			entry.TenantID = sc.TenantID
			if sc.Format != "" {
				entry.Format = sc.Format
			}
			if sc.Bytes > 0 {
				entry.Bytes = sc.Bytes
			}
			if !sc.Created.IsZero() {
				entry.Created = sc.Created.UTC()
			}
		}
		out = append(out, entry)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Created.After(out[j].Created)
	})
	return out, nil
}
