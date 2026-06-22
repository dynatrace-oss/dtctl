package output

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Spill privacy/perm defaults (D12). Directories are owner-only, files
// owner-only read/write — spilled results are raw telemetry at rest.
const (
	spillDirMode  os.FileMode = 0o700
	spillFileMode os.FileMode = 0o600
)

// DefaultSpillTTL bounds how long spilled telemetry sits on disk (D11). It
// doubles as a privacy exposure-window bound.
const DefaultSpillTTL = 24 * time.Hour

// pruneThrottle gates opportunistic TTL pruning so we sweep at most ~hourly
// (no stat-storm, no daemon) — D11.
const pruneThrottle = time.Hour

const (
	pruneMarkerName = ".last-prune"
	spillSubdir     = "results"
	tmpSuffix       = ".tmp"
)

// SpillBaseDir resolves the base directory for managed spills (D7). Resolution:
// an explicit preferred dir (config spill.dir / DTCTL_SPILL_DIR) if writable →
// the OS user cache dir → $TMPDIR. Returns an error only if none are usable, in
// which case the caller degrades to summary-only (D8). The returned bool
// reports whether the location is the managed cache (false ⇒ a user-chosen dir
// that opts out of the managed privacy guarantees, D25).
func SpillBaseDir(preferred string) (dir string, managed bool, err error) {
	if preferred != "" {
		full := filepath.Join(preferred, spillSubdir)
		if ProbeWritable(full) {
			return full, false, nil
		}
		return "", false, fmt.Errorf("spill dir %q is not writable", preferred)
	}

	if cache, cerr := os.UserCacheDir(); cerr == nil {
		full := filepath.Join(cache, "dtctl", spillSubdir)
		if ProbeWritable(full) {
			return full, true, nil
		}
	}

	tmp := filepath.Join(os.TempDir(), "dtctl", spillSubdir)
	if ProbeWritable(tmp) {
		return tmp, true, nil
	}

	return "", false, fmt.Errorf("no writable spill location")
}

var contextSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SanitizeContextName makes a context name safe to use as a single path
// segment (D9). Empty/degenerate names collapse to "default".
func SanitizeContextName(name string) string {
	s := contextSanitizer.ReplaceAllString(name, "-")
	s = strings.Trim(s, "-._")
	if s == "" {
		return "default"
	}
	return s
}

// SpillHash returns the short stable digest used in the spill filename
// q-<hash>.<ext> (D10). It is computed over the canonical query + timeframe +
// segments + sampling so re-running the same query overwrites in place rather
// than accumulating. It is deliberately NOT a cache key — re-runs always
// re-query Grail and overwrite (D10 Non-Goal).
func SpillHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = io.WriteString(h, p)
		_, _ = h.Write([]byte{0}) // domain separator between parts
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:4]) // 8 hex chars, e.g. "7f3a9c12"
}

// ProbeWritable reports whether dir can be created and written to. It does not
// trust config; it probes (D8) by ensuring the dir exists and creating then
// removing a marker file.
func ProbeWritable(dir string) bool {
	if err := os.MkdirAll(dir, spillDirMode); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// WriteSpillFile writes data atomically to targetPath: it writes to a unique
// temp file in the same directory and rename(2)s it onto the final name on
// success (D22). A crashed/partial spill leaves only a .tmp that the TTL prune
// sweeps; readers never observe a half-written file. Returns the number of
// bytes written. The parent directory is created with 0700 and the file ends up
// 0600.
func WriteSpillFile(targetPath string, write func(io.Writer) error) (int64, error) {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, spillDirMode); err != nil {
		return 0, err
	}

	base := filepath.Base(targetPath)
	tmp, err := os.CreateTemp(dir, base+".*"+tmpSuffix)
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we don't make it to the rename.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	counter := &countingWriter{w: tmp}
	if werr := write(counter); werr != nil {
		_ = tmp.Close()
		return 0, werr
	}
	if cerr := tmp.Close(); cerr != nil {
		return 0, cerr
	}
	if err := os.Chmod(tmpName, spillFileMode); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return 0, err
	}
	committed = true
	return counter.n, nil
}

// WriteSidecar writes the on-disk sidecar manifest next to a spilled data file
// (D34). For data file q-<hash>.<ext> the sidecar is q-<hash>.manifest.json. It
// uses the same atomic temp+rename and 0600 perms, and is intended to be written
// LAST so its presence implies a complete data file.
func WriteSidecar(dataPath string, sc *SidecarManifest) error {
	sidecarPath := SidecarPathFor(dataPath)
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	_, err = WriteSpillFile(sidecarPath, func(w io.Writer) error {
		_, werr := w.Write(data)
		return werr
	})
	return err
}

// SidecarPathFor returns the sidecar manifest path for a spilled data file.
func SidecarPathFor(dataPath string) string {
	dir := filepath.Dir(dataPath)
	base := filepath.Base(dataPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(dir, stem+".manifest.json")
}

// PruneOldSpills opportunistically deletes spilled files (and stray .tmp files,
// D22) older than ttl, throttled to ~hourly via a marker so we never stat-storm
// (D11). Errors are swallowed: pruning is best-effort and must never fail a
// query.
func PruneOldSpills(baseDir string, ttl time.Duration) {
	if baseDir == "" {
		return
	}
	if ttl <= 0 {
		ttl = DefaultSpillTTL
	}
	marker := filepath.Join(baseDir, pruneMarkerName)
	if info, err := os.Stat(marker); err == nil {
		if time.Since(info.ModTime()) < pruneThrottle {
			return // pruned recently; skip
		}
	}
	// Touch the marker first so concurrent runs don't all prune at once.
	if err := os.MkdirAll(baseDir, spillDirMode); err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)), spillFileMode)

	cutoff := time.Now().Add(-ttl)
	_ = filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) == pruneMarkerName {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
		return nil
	})
}

// MeasureSerializedBytes serialises records in the chosen display encoding and
// returns the byte count plus the normalised encoding name used (D24). The
// threshold is measured against the actual encoding the invocation will emit so
// the byte count is a faithful proxy for the tokens the agent would otherwise
// receive (50 KB of JSON ≠ 50 KB of toon). table/wide/chart-style formats are
// measured as json because that is what agent mode actually emits.
func MeasureSerializedBytes(records interface{}, format string) (int64, string) {
	enc := NormalizeMeasureEncoding(format)
	counter := &countingWriter{w: io.Discard}
	p := NewPrinterWithOpts(PrinterOptions{Format: enc, Writer: counter})
	_ = p.PrintList(records)
	return counter.n, enc
}

// NormalizeMeasureEncoding maps a display format to the encoding the bytes are
// actually measured (and emitted) as in agent mode. Human-readable layouts that
// agent mode never emits collapse to json.
func NormalizeMeasureEncoding(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "csv"
	case "toon":
		return "toon"
	case "yaml", "yml":
		return "yaml"
	case "json", "ndjson", "jsonl":
		return "json"
	default:
		// table/wide/chart/sparkline/etc. are not emitted in agent mode.
		return "json"
	}
}

// countingWriter counts bytes written through it so a spill can report the
// on-disk file size without a follow-up stat.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
