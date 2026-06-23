package output

import "time"

// EnvelopeVersion is the current agent-envelope contract version (D31). It is
// introduced at 1 with the spill feature and must be incremented on any future
// backward-incompatible shape change. Consumers MUST treat an unrecognised
// result.kind as opaque and fall back to context (the forward-compat rule).
const EnvelopeVersion = 1

// result.kind discriminator values (D2). A consumer branches once on this.
const (
	// KindRecords is today's inline behaviour: the rows are returned directly.
	KindRecords = "records"
	// KindResultFile means the result was spilled to disk; the payload is a
	// manifest (stats + sample + a file handle).
	KindResultFile = "result-file"
	// KindSummaryOnly means the result was large but there was no writable
	// filesystem; the manifest is returned without a path (stats + sample only).
	KindSummaryOnly = "summary-only"
)

// Stable spill-file error codes (D32). These are part of the versioned envelope
// contract and are consumed by Layer 2 (`dtctl inspect`) when it acts on a
// handed-out path that has since become mortal (TTL prune / overwrite / delete).
const (
	ErrCodeSpillFileNotFound     = "spill_file_not_found"
	ErrCodeSpillFileUnreadable   = "spill_file_unreadable"
	ErrCodeSpillFileWrongContext = "spill_file_wrong_context"
)

// SampleStats wraps per-column stats computed on a sampled result (D23). When a
// result is sampled the stats move out of the manifest's top-level `columns`
// into this block, each column additionally carrying basis:"sample", so an
// agent cannot read a sample-based figure as population truth.
type SampleStats struct {
	Basis   string        `json:"basis"` // always "sample"
	Columns []ColumnStats `json:"columns"`
}

// InlineRecords is the result payload for the KindRecords envelope (D2): the
// result was small enough to return inline, so the rows are carried directly,
// still behind the same self-describing discriminator so an agent branches on
// result.kind uniformly across inline and spilled results (D2/D31). Emitted only
// in agent mode on the spill-aware path; the non-spill output path is unchanged.
type InlineRecords struct {
	Kind     string                   `json:"kind"`
	Records  []map[string]interface{} `json:"records"`
	Metadata interface{}              `json:"metadata,omitempty"`
}

// ResultFileManifest is the result payload for the KindResultFile /
// KindSummaryOnly envelope shapes. It is the in-session contract; the on-disk
// SidecarManifest (D34) carries the same provenance for cross-session use.
type ResultFileManifest struct {
	Kind string `json:"kind"`
	// Path is the on-disk location of the spilled file. Omitted for
	// summary-only (no writable filesystem).
	Path string `json:"path,omitempty"`
	// Query is the original DQL text, recorded so a stale-file recovery can
	// suggest a concrete re-query (D32).
	Query  string `json:"query,omitempty"`
	Format string `json:"format"`
	Rows   int    `json:"rows"`
	// Bytes is the size of the spilled data file. Omitted for summary-only.
	Bytes       int64  `json:"bytes,omitempty"`
	ContextName string `json:"context_name,omitempty"`
	// TenantID is provenance kept in the manifest, not the path (D9).
	TenantID      string  `json:"tenant_id,omitempty"`
	Sampled       bool    `json:"sampled,omitempty"`
	SamplingRatio float64 `json:"sampling_ratio,omitempty"`

	// Exactly one of Columns / SampleStats is populated: Columns when the
	// result was not sampled, SampleStats (with per-column basis) when it was.
	Columns     []ColumnStats `json:"columns,omitempty"`
	SampleStats *SampleStats  `json:"sample_stats,omitempty"`

	// ColumnsOmitted lists, by name, the columns whose per-column profile was
	// dropped from this envelope to keep it compact for a wide result (the
	// least-populated columns; see CapColumnsForEnvelope). Full stats for every
	// column — including these — are in the on-disk sidecar manifest. Empty when
	// no columns were omitted.
	ColumnsOmitted []string `json:"columns_omitted,omitempty"`

	SampleRows []map[string]interface{} `json:"sample_rows,omitempty"`
}

// SetStats places the computed column stats in the correct location depending on
// whether the result was sampled (D23): population stats go under `columns`,
// sampled stats go under `sample_stats` with basis:"sample".
func (m *ResultFileManifest) SetStats(cols []ColumnStats, sampled bool) {
	if sampled {
		m.SampleStats = &SampleStats{Basis: "sample", Columns: cols}
		m.Columns = nil
		return
	}
	m.Columns = cols
	m.SampleStats = nil
}

// SidecarManifest is the tiny on-disk JSON written next to the spilled data file
// (D34): q-<hash>.manifest.json. A spilled file can outlive its session, and
// sampled/sampling_ratio/tenant_id/query cannot be re-derived from raw rows, so
// they are persisted here for Layer 2's inspect to (a) present sampled stats
// honestly, (b) refuse cross-context reads, and (c) suggest a concrete re-query
// when the file is gone.
type SidecarManifest struct {
	EnvelopeVersion int           `json:"envelope_version"`
	Format          string        `json:"format"`
	Sampled         bool          `json:"sampled"`
	SamplingRatio   float64       `json:"sampling_ratio,omitempty"`
	TenantID        string        `json:"tenant_id,omitempty"`
	ContextName     string        `json:"context_name,omitempty"`
	Query           string        `json:"query,omitempty"`
	Rows            int           `json:"rows"`
	Bytes           int64         `json:"bytes"`
	Created         time.Time     `json:"created"`
	Columns         []ColumnStats `json:"columns,omitempty"`
}
