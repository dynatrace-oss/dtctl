package exec

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

const userPathPrivacyWarning = "spill path is a user-chosen location and opts out of the managed privacy guarantees (no TTL pruning, no per-context partitioning, best-effort 0600 only); you own its lifetime"

// trySpill runs the spill decision and emits the appropriate agent envelope.
// It returns handled=true when it produced output (the caller returns
// immediately); handled=false means the caller should continue with the
// unchanged output path (a non-agent inline result, or a shape this path
// deliberately leaves alone — see buildSpillResponse).
func (e *DQLExecutor) trySpill(query string, result *DQLQueryResponse, records []map[string]interface{}, displayFormat string, opts DQLExecuteOptions) (bool, error) {
	resp, handled, err := e.buildSpillResponse(query, result, records, displayFormat, opts)
	if err != nil {
		return true, err
	}
	if !handled {
		return false, nil
	}
	return true, writeEnvelope(os.Stdout, resp)
}

// buildSpillResponse makes the inline-vs-spill decision (D5/D19-buffered),
// writes the spilled file + sidecar when applicable, and assembles the agent
// envelope. The handled return reports whether this path produced a Response to
// emit: true for a spilled/summary-only result, and for an inline result in
// agent mode (a self-describing kind:"records" envelope, D2/D31); false when the
// caller should fall through to today's unchanged output path. It is separated
// from trySpill so it can be unit-tested without capturing stdout: it returns
// the Response and leaves emission to the caller, while its only side effect
// (writing to disk) is fully controlled via opts.Spill (ToPath / Dir).
func (e *DQLExecutor) buildSpillResponse(query string, result *DQLQueryResponse, records []map[string]interface{}, displayFormat string, opts DQLExecuteOptions) (output.Response, bool, error) {
	// Measure serialised size against the chosen display encoding (D24).
	measured, encoding := output.MeasureSerializedBytes(records, displayFormat)

	switch opts.Spill.Mode {
	case SpillAuto:
		if measured <= opts.Spill.Threshold {
			return e.inlineRecordsResponse(query, result, records, measured, encoding, opts) // inline
		}
	case SpillAlways:
		// always spill
	default:
		return e.inlineRecordsResponse(query, result, records, measured, encoding, opts) // never / unknown -> inline
	}

	// Provenance from Grail metadata.
	g := result.GetMetadata()
	sampled := false
	canonical := query
	var tfStart, tfEnd string
	if g != nil {
		sampled = g.Sampled
		if g.CanonicalQuery != "" {
			canonical = g.CanonicalQuery
		}
		if g.AnalysisTimeframe != nil {
			tfStart, tfEnd = g.AnalysisTimeframe.Start, g.AnalysisTimeframe.End
		}
	}
	samplingRatio := 0.0
	if sampled {
		samplingRatio = opts.DefaultSamplingRatio
	}

	format, targetPath, baseDir, managed, summaryOnly, warnings, err := e.resolveSpillTarget(canonical, tfStart, tfEnd, opts)
	if err != nil {
		return output.Response{}, true, err
	}

	// A --jq transform is not applied to spilled rows (the file holds the full
	// untransformed result so stats/sample stay columnar). Surface it loudly
	// rather than silently dropping the filter.
	if opts.JQFilter != "" {
		warnings = append(warnings, "--jq was not applied to the spilled result; the file holds the full untransformed rows — apply your filter to the file locally")
	}

	cols := output.ComputeColumnStats(records, sampled, output.DefaultStatsTopK, output.DefaultStatsMaxDistinct)
	sampleRows := output.SampleRows(records, output.DefaultSampleRows)

	manifest := &output.ResultFileManifest{
		Query:         query,
		Format:        format,
		Rows:          len(records),
		ContextName:   opts.ContextName,
		TenantID:      opts.TenantID,
		Sampled:       sampled,
		SamplingRatio: samplingRatio,
		SampleRows:    sampleRows,
	}
	manifest.SetStats(cols, sampled)

	decided := "spilled"
	if !summaryOnly {
		written, werr := output.WriteSpillFile(targetPath, func(w io.Writer) error {
			p := output.NewPrinterWithOpts(output.PrinterOptions{Format: format, Writer: w})
			return p.PrintList(records)
		})
		if werr != nil {
			if opts.Spill.ToPath != "" {
				// The caller pinned an explicit destination; a failure there is a
				// real error, not a reason to silently degrade.
				return output.Response{}, true, fmt.Errorf("failed to write spill file %q: %w", targetPath, werr)
			}
			// Managed write failed unexpectedly -> degrade to summary-only rather
			// than dumping rows into context (D8: never dump on failure).
			summaryOnly = true
			warnings = append(warnings, "spill write failed; returning overview only")
		} else {
			manifest.Kind = output.KindResultFile
			manifest.Path = targetPath
			manifest.Bytes = written

			// Sidecar manifest (D34), written last so its presence implies a
			// complete data file. Best-effort: a sidecar failure must not fail
			// the query.
			_ = output.WriteSidecar(targetPath, &output.SidecarManifest{
				EnvelopeVersion: output.EnvelopeVersion,
				Format:          format,
				Sampled:         sampled,
				SamplingRatio:   samplingRatio,
				TenantID:        opts.TenantID,
				ContextName:     opts.ContextName,
				Query:           query,
				Rows:            len(records),
				Bytes:           written,
				Created:         time.Now().UTC(),
				Columns:         cols,
			})

			// Opportunistic, throttled TTL prune of the managed cache (D11).
			if managed && baseDir != "" {
				output.PruneOldSpills(baseDir, opts.Spill.TTL)
			}
		}
	}

	if summaryOnly {
		manifest.Kind = output.KindSummaryOnly
		decided = "summary-only"
	}

	total := len(records)
	ctx := &output.ResponseContext{
		Verb:             "query",
		Resource:         resourceFromQuery(query),
		Total:            &total,
		Decided:          decided,
		ThresholdBytes:   opts.Spill.Threshold,
		MeasuredBytes:    measured,
		MeasuredEncoding: encoding,
		Warnings:         warnings,
		Suggestions:      spillSuggestions(query, manifest.Kind),
	}

	resp := output.Response{
		OK:              true,
		EnvelopeVersion: output.EnvelopeVersion,
		Result:          manifest,
		Context:         ctx,
	}
	return resp, true, nil
}

// inlineRecordsResponse handles the inline (not-spilled) decision. In agent mode
// it returns a self-describing kind:"records" envelope so a consumer branches on
// result.kind uniformly across inline and spilled results (D2/D31); the rows are
// carried directly. It deliberately leaves two shapes alone — falling through to
// the caller's unchanged output path (handled=false) — so it never overrides an
// explicit, non-JSON choice the caller already made:
//   - a non-JSON display encoding (-o toon/csv/yaml/chart): the envelope is JSON;
//     wrapping would silently discard the requested format.
//   - a --jq transform: agent-mode jq already owns the output shape.
//
// Outside agent mode an inline result is always a fall-through (a human wants the
// table/CSV, not an envelope).
func (e *DQLExecutor) inlineRecordsResponse(query string, result *DQLQueryResponse, records []map[string]interface{}, measured int64, encoding string, opts DQLExecuteOptions) (output.Response, bool, error) {
	if !opts.AgentMode || encoding != "json" || opts.JQFilter != "" {
		return output.Response{}, false, nil
	}

	res := &output.InlineRecords{Kind: output.KindRecords, Records: records}
	// Agent mode defaults --metadata to "all"; preserve that provenance under the
	// result payload (it previously rode alongside the bare records map).
	if len(opts.MetadataFields) > 0 {
		if meta := extractQueryMetadata(result); meta != nil {
			res.Metadata = output.MetadataToMap(meta, opts.MetadataFields)
		}
	}

	total := len(records)
	ctx := &output.ResponseContext{
		Verb:             "query",
		Resource:         resourceFromQuery(query),
		Total:            &total,
		Decided:          "inline",
		ThresholdBytes:   opts.Spill.Threshold,
		MeasuredBytes:    measured,
		MeasuredEncoding: encoding,
	}
	return output.Response{
		OK:              true,
		EnvelopeVersion: output.EnvelopeVersion,
		Result:          res,
		Context:         ctx,
	}, true, nil
}

// resolveSpillTarget decides the format, destination path, and base dir for a
// spill, and whether it must degrade to summary-only (D7/D8/D25).
func (e *DQLExecutor) resolveSpillTarget(canonical, tfStart, tfEnd string, opts DQLExecuteOptions) (format, targetPath, baseDir string, managed, summaryOnly bool, warnings []string, err error) {
	// Explicit caller-chosen destination (--spill-to): write exactly there.
	if opts.Spill.ToPath != "" {
		f, ferr := spillFormatForPath(opts.Spill.ToPath, opts.Spill.Format)
		if ferr != nil {
			return "", "", "", false, false, nil, ferr
		}
		dir := filepath.Dir(opts.Spill.ToPath)
		if !output.ProbeWritable(dir) {
			return "", "", "", false, false, nil, fmt.Errorf("spill destination directory %q is not writable", dir)
		}
		return f, opts.Spill.ToPath, "", false, false, []string{userPathPrivacyWarning}, nil
	}

	format = opts.Spill.Format
	if format == "" {
		format = defaultSpillFormat
	}
	if verr := validateSpillFormat(format); verr != nil {
		return "", "", "", false, false, nil, verr
	}

	base, isManaged, berr := output.SpillBaseDir(opts.Spill.Dir)
	if berr != nil {
		// No writable location anywhere -> summary-only (D8). Stats are still
		// computable without disk.
		return format, "", "", false, true,
			[]string{"no writable spill location — overview only; local follow-up unavailable"}, nil
	}
	// The managed cache is partitioned by context (D9); a user-chosen dir opts
	// out of partitioning (and of TTL/perms guarantees) and is flagged (D25).
	dir := base
	if isManaged {
		dir = filepath.Join(base, output.SanitizeContextName(opts.ContextName))
	} else {
		warnings = append(warnings, userPathPrivacyWarning)
	}
	hash := output.SpillHash(canonical, tfStart, tfEnd, fmt.Sprintf("%v", opts.Segments), fmt.Sprintf("%g", opts.DefaultSamplingRatio))
	targetPath = filepath.Join(dir, "q-"+hash+"."+extForFormat(format))
	return format, targetPath, base, isManaged, false, warnings, nil
}

// writeEnvelope emits the agent envelope as indented JSON.
func writeEnvelope(w io.Writer, resp output.Response) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

// defaultSpillFormat is the spill format used when none is configured. JSON Lines
// is the default (D26): schema-less, append-friendly, one record per line, and
// read natively by common local tooling — it reuses the `-o jsonl` writer added
// alongside Parquet in the output-formats change (PR1).
const defaultSpillFormat = "jsonl"

// validateSpillFormat accepts the formats dtctl can spill to, all backed by the
// existing `-o` writers: JSON Lines (default), JSON, CSV, and Parquet.
func validateSpillFormat(format string) error {
	switch strings.ToLower(format) {
	case "jsonl", "json", "csv", "parquet":
		return nil
	default:
		return fmt.Errorf("unsupported spill format %q (use jsonl, json, csv, or parquet)", format)
	}
}

// spillFormatForPath infers the spill format from a destination file extension,
// falling back to the configured format (or the default) for an extension-less path.
func spillFormatForPath(path, fallback string) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "jsonl", "json", "csv", "parquet":
		return ext, nil
	case "":
		if fallback == "" {
			return defaultSpillFormat, nil
		}
		return fallback, validateSpillFormat(fallback)
	default:
		return "", fmt.Errorf("unsupported --spill-to file extension %q (use .jsonl, .json, .csv, or .parquet)", ext)
	}
}

func extForFormat(format string) string {
	switch strings.ToLower(format) {
	case "csv":
		return "csv"
	case "json":
		return "json"
	case "parquet":
		return "parquet"
	default:
		return "jsonl"
	}
}

// spillSuggestions builds the public, tool-agnostic follow-up hints (D27/D28).
// It nudges toward DQL aggregation for non-aggregating queries and points at
// generic local tooling — it never names a specific third-party project.
func spillSuggestions(query, kind string) []string {
	var s []string
	if kind == output.KindResultFile {
		s = append(s, "# the full result is on disk at the path above; read it with your file tooling for row-level follow-up")
	}
	if isNonAggregatingQuery(query) {
		s = append(s, "# for aggregate questions, prefer pushing the work into DQL, e.g. add '| summarize count(), by:{<field>}' and re-query")
	}
	if kind == output.KindResultFile {
		s = append(s, "# for complex local analysis, process the spilled file with your preferred local analytics tooling")
	} else {
		s = append(s, "# no writable spill location was available — re-run with --spill-to <path> to write the rows to a file you choose")
	}
	return s
}

// isNonAggregatingQuery is a cheap heuristic: a query that does not summarise or
// build a timeseries is "raw row" shaped, which is exactly when the DQL-aggregate
// nudge is worth showing (D27).
func isNonAggregatingQuery(query string) bool {
	l := strings.ToLower(query)
	for _, agg := range []string{"summarize", "maketimeseries", "makets"} {
		if strings.Contains(l, agg) {
			return false
		}
	}
	return true
}

// resourceFromQuery extracts the fetched resource (e.g. "logs") from a DQL query
// for the envelope's context.resource, best-effort.
func resourceFromQuery(query string) string {
	fields := strings.Fields(strings.ToLower(query))
	for i, f := range fields {
		if f == "fetch" && i+1 < len(fields) {
			return strings.TrimRight(fields[i+1], ",")
		}
	}
	return ""
}
