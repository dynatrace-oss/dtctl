package exec

import (
	"fmt"
	"os"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// finishStreamedJSONL closes out a `-o jsonl` stream: the rows are already on
// stdout, so only the stderr tail the buffered path emits is left.
func (e *DQLExecutor) finishStreamedJSONL(query string, result *DQLQueryResponse, opts DQLExecuteOptions) error {
	notifications := result.GetNotifications()
	if len(notifications) > 0 {
		e.PrintNotifications(notifications)
		if advice := unsortedSummarizeAdvice(query, notifications); advice != "" {
			output.PrintHint("%s", advice)
		}
	}
	if len(opts.MetadataFields) > 0 {
		if meta := extractQueryMetadata(result); meta != nil {
			fields := resolveMetadataFields(query, meta, opts)
			fmt.Fprint(os.Stderr, output.FormatMetadataFooter(meta, fields))
		}
	}
	return nil
}

// finishStreamedSpill commits the streamed file and emits the summary envelope.
//
// It is buildSpillResponse for a result that was never held: the column stats,
// the constant/all-null columns and the sample rows come from the accumulators
// the collector folded each row into, and the row count from its counter.
// Nothing below re-reads the rows.
func (e *DQLExecutor) finishStreamedSpill(query string, result *DQLQueryResponse, c *streamCollector, opts DQLExecuteOptions) error {
	resp, err := e.buildStreamedSpillResponse(query, result, c, opts)
	if err != nil {
		return err
	}
	return output.EncodeEnvelope(os.Stdout, resp)
}

func (e *DQLExecutor) buildStreamedSpillResponse(query string, result *DQLQueryResponse, c *streamCollector, opts DQLExecuteOptions) (output.Response, error) {
	sampled, canonical, tfStart, tfEnd, samplingRatio := spillProvenanceOf(query, result, opts)

	format, targetPath, baseDir, managed, summaryOnly, warnings, err := e.resolveSpillTarget(canonical, tfStart, tfEnd, opts)
	if err != nil {
		return output.Response{}, err
	}
	summaryReason := ""
	if summaryOnly {
		summaryReason = summaryReasonNoLocation
	}

	cols := c.stats.Finalize(sampled)
	compaction := c.compaction()
	rows := c.rows

	summaryCols := cols
	sampleSource := c.sample
	if compaction != nil {
		summaryCols = compaction.FilterColumns(cols)
		sampleSource = compaction.Tabular(c.sample)
	}
	sampleRows := output.SampleRows(sampleSource, output.DefaultSampleRows)
	envCols, omittedCols := output.CapColumnsForEnvelope(summaryCols, output.DefaultMaxSummaryColumns)

	manifest := &output.ResultFileManifest{
		Query:         query,
		Format:        format,
		Rows:          rows,
		ContextName:   opts.ContextName,
		TenantID:      opts.TenantID,
		Sampled:       sampled,
		SamplingRatio: samplingRatio,
		SampleRows:    sampleRows,
	}
	manifest.SetStats(envCols, sampled)
	manifest.ColumnsOmitted = omittedCols
	manifest.Types = emittedTypes(result, opts)
	if compaction != nil {
		manifest.Constant = compaction.EnvelopeConstant()
		manifest.NullColumns = compaction.NullColumns
	}

	decided := "spilled"
	if summaryOnly {
		// No writable location: the rows went nowhere, so drop the temp file.
		c.abort()
	} else {
		written, cerr := c.writer.Commit(targetPath)
		if cerr != nil {
			if opts.Spill.ToPath != "" {
				return output.Response{}, fmt.Errorf("failed to write spill file %q: %w", targetPath, cerr)
			}
			summaryOnly = true
			summaryReason = summaryReasonWriteFailed
			warnings = append(warnings, "spill write failed; returning overview only")
		} else {
			manifest.Kind = output.KindResultFile
			manifest.Path = targetPath
			manifest.Bytes = written

			_ = output.WriteSidecar(targetPath, &output.SidecarManifest{
				EnvelopeVersion: output.EnvelopeVersion,
				Format:          format,
				Sampled:         sampled,
				SamplingRatio:   samplingRatio,
				TenantID:        opts.TenantID,
				ContextName:     opts.ContextName,
				Query:           query,
				Rows:            rows,
				Bytes:           written,
				Created:         time.Now().UTC(),
				Columns:         cols,
			})
			if managed && baseDir != "" {
				output.PruneOldSpills(baseDir, opts.Spill.TTL)
			}
		}
	}

	if summaryOnly {
		manifest.Kind = output.KindSummaryOnly
		decided = "summary-only"
	}

	if opts.JQFilter != "" {
		warnings = append(warnings, "--jq was not applied to the spilled result; the file holds the full untransformed rows — apply your filter to the file locally")
	}

	suggestions := spillSuggestions(query, manifest.Kind, summaryReason)
	if manifest.Kind == output.KindResultFile && manifest.Path != "" {
		suggestions = append(suggestions,
			"# for bounded row access without re-querying Grail: dtctl inspect "+manifest.Path+" --head 20 (also --tail, --page --offset N --limit M, --fields a,b)")
	}
	if n := len(omittedCols); n > 0 {
		if manifest.Kind == output.KindResultFile {
			suggestions = append(suggestions, fmt.Sprintf("# %d sparser columns were omitted from this summary to keep it compact; their names are in result.columns_omitted and full per-column stats are in the sidecar manifest next to the file", n))
		} else {
			suggestions = append(suggestions, fmt.Sprintf("# %d sparser columns were omitted from this summary to keep it compact; their names are in result.columns_omitted (the rows were not written to disk, so there is no sidecar manifest)", n))
		}
	}

	notifWarnings, notifSuggestions := queryNotificationAdvice(query, result.GetNotifications())
	warnings = append(warnings, notifWarnings...)
	suggestions = append(notifSuggestions, suggestions...)
	scanWarnings, scanSuggestions := heavyScanAdvice(result)
	warnings = append(warnings, scanWarnings...)
	suggestions = append(suggestions, scanSuggestions...)
	suggestions = append(suggestions, lookbackAdvice(query)...)
	suggestions = append(suggestions, metadataDefaultAdvice(query, extractQueryMetadata(result), opts, false)...)
	if compaction != nil && compaction.Changed("json") {
		suggestions = append(suggestions, compactSummarySuggestion)
	}
	suggestions = append(suggestions, seriesAdvice(opts)...)

	total := rows
	ctx := &output.ResponseContext{
		Verb:           "query",
		Resource:       resourceFromQuery(query),
		Total:          &total,
		Decided:        decided,
		ThresholdBytes: opts.Spill.Threshold,
		// No measured_bytes: the rows were never serialised for inline emission.
		Streamed:    true,
		Warnings:    warnings,
		Suggestions: suggestions,
	}

	return output.Response{
		OK:              true,
		EnvelopeVersion: output.EnvelopeVersion,
		Result:          manifest,
		Context:         ctx,
		Metadata:        envelopeMetadata(query, result, opts),
	}, nil
}

// compaction finalises the streamed compaction, or returns nil when the
// invocation did not ask for one (the same condition compactionFor applies to a
// buffered result).
func (c *streamCollector) compaction() *output.Compaction {
	if !c.opts.Compact || c.opts.JQFilter != "" || c.compact == nil {
		return nil
	}
	fin := c.compact.Finalize()
	return &fin
}
