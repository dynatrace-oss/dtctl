package inspect

import (
	"io"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// Primitive is the single analytics operation an inspect call performs. Exactly
// one is chosen per call (IN1); the set is closed (IN6) — adding to it is a
// deliberate decision, not an open extension point, which is how "no second
// query language" stays true over time.
type Primitive string

const (
	// Row-access primitives — the reason inspect exists (D30). They return
	// arbitrary rows the Layer 1 manifest never carried.
	PrimHead Primitive = "head" // first N rows
	PrimTail Primitive = "tail" // last N rows
	PrimPage Primitive = "page" // a paginated window: offset O, limit L

	// Re-derivation primitives — conveniences for a file whose manifest is no
	// longer in context (a stale spill, a cross-session hand-off). They re-emit
	// the Layer 1 manifest shape from disk rather than re-querying Grail.
	PrimSchema Primitive = "schema" // columns + types + null counts
	PrimStats  Primitive = "stats"  // per-column profile
	PrimSample Primitive = "sample" // N representative (leading) rows
)

// DefaultRowCount is the row cap applied to head/tail/sample (and a bare
// --fields, which defaults to head) when the caller does not specify one. Kept
// small so a row-access call stays inline and agent-context-friendly by default.
const DefaultRowCount = 10

// Request is a fully-validated inspect invocation. The command layer parses
// flags, enforces one-primitive-per-call, and resolves the active context/tenant
// before constructing it; the engine assumes it is well-formed.
type Request struct {
	Path      string
	Primitive Primitive

	// N is the row count for head/tail/sample.
	N int
	// Offset/Limit are the page window for PrimPage.
	Offset, Limit int
	// Fields projects row-access output to these columns, in DQL `fields` style.
	// Empty means all columns.
	Fields []string
	// StatsColumns restricts PrimStats to these columns. Empty means all.
	StatsColumns []string

	// ActiveContext / ActiveTenant are the live dtctl context name and tenant id,
	// used for the structural cross-context/tenant refusal (D9/D32).
	ActiveContext string
	ActiveTenant  string
}

// Result is the engine output. Exactly one of Records / Summary is populated,
// matching Kind (output.KindRecords for row access, output.KindFileSummary for
// the re-derived manifest primitives).
type Result struct {
	Kind     string
	Records  []map[string]interface{}
	Summary  *output.ResultFileManifest
	Format   string
	Sidecar  *output.SidecarManifest
	Warnings []string
}

// Run executes a validated inspect request against the spilled file. It resolves
// the file format and provenance (sidecar), refuses a cross-context/tenant file
// structurally before opening it (D9/D32), then dispatches the chosen primitive.
// All failure paths return a *Error carrying a stable envelope code.
func Run(req Request) (*Result, error) {
	// Provenance: the sidecar carries what cannot be re-derived from raw rows —
	// sampling flag/ratio, tenant, context, original query (IN3). Absent for an
	// older/hand-copied file; present is the common fresh-spill case.
	sidecar, err := output.ReadSidecar(req.Path)
	if err != nil {
		return nil, errUnreadable(req.Path, "", err)
	}
	query := ""
	if sidecar != nil {
		query = sidecar.Query
	}

	// Refuse a file that belongs to another context/tenant — without opening it
	// (D9/D32). Structural for managed paths (the path's context segment); for
	// user-chosen paths (which opt out of partitioning, D25) fall back to the
	// sidecar's tenant id when both are known.
	if cerr := checkContext(req, sidecar); cerr != nil {
		return nil, cerr
	}

	format := ""
	if sidecar != nil {
		format = sidecar.Format
	}
	if format == "" {
		format = formatFromExtension(req.Path)
	}
	if format == "" {
		return nil, errBadFlags(
			"cannot determine the format of "+quote(req.Path)+" (no sidecar manifest and an unrecognised extension)",
			"name the file with a .jsonl, .json, .csv, or .parquet extension, or re-run the original query to spill a self-describing file",
		)
	}

	r, err := openReader(req.Path, format, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	res := &Result{Format: format, Sidecar: sidecar}

	switch req.Primitive {
	case PrimHead, PrimTail, PrimPage:
		return res, runRowAccess(r, req, sidecar, res)
	case PrimSchema, PrimStats, PrimSample:
		return res, runSummary(r, req, sidecar, res)
	default:
		return nil, errBadFlags("no primitive selected (choose one of --head, --tail, --page, --schema, --stats, --sample, or --fields)")
	}
}

// checkContext enforces the cross-context/tenant refusal (D9/D32).
func checkContext(req Request, sidecar *output.SidecarManifest) error {
	// Structural: a managed path embeds its context as a directory segment.
	if seg, ok := output.ManagedContextFor(req.Path); ok && req.ActiveContext != "" {
		if seg != output.SanitizeContextName(req.ActiveContext) {
			return errWrongContext(req.Path, seg, req.ActiveContext)
		}
	}
	// Cross-tenant guard for any path (covers user-chosen, non-partitioned paths)
	// when both tenant ids are known and disagree.
	if sidecar != nil && sidecar.TenantID != "" && req.ActiveTenant != "" && sidecar.TenantID != req.ActiveTenant {
		fileCtx := sidecar.ContextName
		if fileCtx == "" {
			fileCtx = sidecar.TenantID
		}
		return errWrongContext(req.Path, fileCtx, req.ActiveContext)
	}
	return nil
}

// runRowAccess executes head/tail/page, applying --fields projection, and fills
// res with the resulting records (output.KindRecords).
func runRowAccess(r Reader, req Request, sidecar *output.SidecarManifest, res *Result) error {
	if err := validateFields(r, sidecar, req.Fields); err != nil {
		return err
	}

	var (
		rows []map[string]interface{}
		err  error
	)
	switch req.Primitive {
	case PrimHead:
		rows, err = readHead(r, rowCount(req.N), req.Fields)
	case PrimTail:
		rows, err = readTail(r, rowCount(req.N), req.Fields)
	case PrimPage:
		rows, err = readPage(r, req.Offset, pageLimit(req.Limit), req.Fields)
	}
	if err != nil {
		return err
	}

	res.Kind = output.KindRecords
	res.Records = rows
	// When the schema was unknowable up front (NDJSON/JSON, no sidecar), surface
	// any requested field that never appeared rather than silently returning
	// empty projections.
	res.Warnings = append(res.Warnings, missingFieldWarnings(r, sidecar, req.Fields, rows)...)
	return nil
}

// runSummary executes schema/stats/sample, re-deriving the manifest shape from
// disk (output.KindFileSummary).
func runSummary(r Reader, req Request, sidecar *output.SidecarManifest, res *Result) error {
	m := &output.ResultFileManifest{
		Kind:   output.KindFileSummary,
		Path:   req.Path,
		Format: res.Format,
	}
	if sidecar != nil {
		m.ContextName = sidecar.ContextName
		m.TenantID = sidecar.TenantID
		m.Sampled = sidecar.Sampled
		m.SamplingRatio = sidecar.SamplingRatio
		m.Query = sidecar.Query
		m.Rows = sidecar.Rows
	}

	switch req.Primitive {
	case PrimSample:
		rows, err := readHead(r, rowCount(req.N), req.Fields)
		if err != nil {
			return err
		}
		m.SampleRows = output.SampleRows(rows, len(rows))

	case PrimSchema, PrimStats:
		acc := output.NewStatsAccumulator(output.DefaultStatsTopK, output.DefaultStatsMaxDistinct)
		for {
			rec, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return wrapReadError(err)
			}
			acc.Observe(rec)
		}
		cols := acc.Finalize(m.Sampled)
		m.Rows = acc.Rows() // authoritative: we scanned the whole file

		if req.Primitive == PrimSchema {
			m.Columns = schemaView(cols)
		} else { // PrimStats
			if len(req.StatsColumns) > 0 {
				var ferr error
				cols, ferr = selectColumns(cols, req.StatsColumns)
				if ferr != nil {
					return ferr
				}
			}
			m.SetStats(cols, m.Sampled)
			if sidecar == nil {
				res.Warnings = append(res.Warnings,
					"no manifest found next to this file — sampling is unknown; these stats may reflect a Grail sample, not the full population")
			}
		}
	}

	res.Kind = output.KindFileSummary
	res.Summary = m
	return nil
}

func rowCount(n int) int {
	if n <= 0 {
		return DefaultRowCount
	}
	return n
}

func pageLimit(l int) int {
	if l <= 0 {
		return DefaultRowCount
	}
	return l
}
