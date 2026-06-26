package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/inspect"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// emitInspectResult renders an inspect result. Row access (kind:records) and the
// re-derived summary (kind:file-summary) reuse the same envelope contract and
// printers as the rest of dtctl, so an agent learns nothing new (IN5). In agent
// mode an oversized row window re-spills to a new managed file instead of
// flooding context (IN8).
func emitInspectResult(cmd *cobra.Command, cfg *config.Config, req inspect.Request, res *inspect.Result) error {
	if res.Kind == output.KindFileSummary {
		return emitInspectSummary(req, res)
	}
	return emitInspectRecords(cmd, cfg, req, res)
}

// emitInspectRecords renders a row-access result. In agent mode it first checks
// whether the rows are themselves a context hazard and re-spills if so (IN8),
// otherwise emits a self-describing kind:records envelope (or, when a --jq /
// non-JSON encoding owns the shape, the standard agent printer).
func emitInspectRecords(cmd *cobra.Command, cfg *config.Config, req inspect.Request, res *inspect.Result) error {
	records := res.Records

	if agentMode {
		spilled, err := maybeRespill(cmd, cfg, req, res)
		if err != nil {
			return err
		}
		if spilled != nil {
			return output.EncodeEnvelope(os.Stdout, *spilled)
		}

		// --jq or a non-JSON result encoding owns the output shape; defer to the
		// standard agent printer (mirrors `query`'s inline path).
		if jqFilter != "" || output.NormalizeMeasureEncoding(outputFormat) != "json" {
			ctx := inspectContext(req, res, len(records))
			ap := output.NewAgentPrinter(os.Stdout, ctx)
			ap.SetResultFormat(outputFormat)
			ap.SetJQFilter(jqFilter)
			return ap.PrintList(records)
		}

		ctx := inspectContext(req, res, len(records))
		resp := output.Response{
			OK:              true,
			EnvelopeVersion: output.EnvelopeVersion,
			Result:          &output.InlineRecords{Kind: output.KindRecords, Records: records},
			Context:         ctx,
		}
		return output.EncodeEnvelope(os.Stdout, resp)
	}

	// Human / scripted output: print the rows in the chosen format, warnings to
	// stderr so they never corrupt a piped result.
	printInspectWarnings(res.Warnings)
	format := outputFormat
	if jqFilter != "" {
		format = output.NormalizeJQOutputFormat(format)
	}
	p := output.NewPrinterWithOpts(output.PrinterOptions{Format: format, Writer: os.Stdout, JQFilter: jqFilter})
	return p.PrintList(records)
}

// emitInspectSummary renders a re-derived file-summary (--schema/--stats/--sample).
func emitInspectSummary(req inspect.Request, res *inspect.Result) error {
	total := 0
	if res.Summary != nil {
		total = res.Summary.Rows
	}

	if agentMode {
		ctx := inspectContext(req, res, total)
		if jqFilter != "" {
			ap := output.NewAgentPrinter(os.Stdout, ctx)
			ap.SetResultFormat(outputFormat)
			ap.SetJQFilter(jqFilter)
			return ap.Print(res.Summary)
		}
		resp := output.Response{
			OK:              true,
			EnvelopeVersion: output.EnvelopeVersion,
			Result:          res.Summary,
			Context:         ctx,
		}
		return output.EncodeEnvelope(os.Stdout, resp)
	}

	// Human output: a struct does not table well, so default tabular formats to
	// pretty JSON; honour an explicit structured format otherwise.
	printInspectWarnings(res.Warnings)
	format := outputFormat
	if jqFilter != "" {
		format = output.NormalizeJQOutputFormat(format)
	}
	switch format {
	case "", "table", "wide":
		format = "json"
	}
	p := output.NewPrinterWithOpts(output.PrinterOptions{Format: format, Writer: os.Stdout, JQFilter: jqFilter})
	return p.Print(res.Summary)
}

// inspectContext builds the agent envelope context shared by both result kinds.
func inspectContext(req inspect.Request, res *inspect.Result, total int) *output.ResponseContext {
	t := total
	ctx := &output.ResponseContext{
		Verb:        "inspect",
		Resource:    inspectResourceFromSidecar(res.Sidecar),
		Total:       &t,
		Warnings:    res.Warnings,
		Suggestions: inspectSuggestions(req, res),
	}
	return ctx
}

// inspectSuggestions returns tool-agnostic follow-up hints (D28: no third-party
// project is ever named). A re-derived summary leads with row access (IN4) since
// that is the call an agent cannot satisfy from a manifest it already holds.
func inspectSuggestions(req inspect.Request, res *inspect.Result) []string {
	if res.Kind != output.KindFileSummary {
		return nil
	}
	return []string{
		"# this is a re-derived summary; for the rows themselves use row access, e.g. dtctl inspect " + req.Path + " --head 20",
		"# for aggregate questions, prefer pushing the work into DQL and re-querying ('… | summarize …')",
	}
}

// printInspectWarnings prints engine warnings to stderr for a human reader.
func printInspectWarnings(warnings []string) {
	for _, w := range warnings {
		output.PrintWarning("%s", w)
	}
}
