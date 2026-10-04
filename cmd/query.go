package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
)

// isTerminal checks if the given file is a terminal
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

// isStderrTerminal checks if stderr is a terminal (for color output)
func isStderrTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// formatRequiresIncludeTypes reports whether the output format needs DQL column
// type metadata to render correctly, so the query layer can request it even when
// the user did not pass --include-types. Parquet derives its columnar schema from
// these types (a "long" must become an INT64 column, not a value-inferred DOUBLE).
func formatRequiresIncludeTypes(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), "parquet")
}

func isSupportedQueryOutputFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "table", "wide", "json", "yaml", "yml", "csv", "jsonl", "parquet", "toon", "auto", "chart", "sparkline", "spark", "barchart", "bar", "braille", "br":
		return true
	default:
		return false
	}
}

// agentResultFormat returns the output format for `query` and whether it is the
// agent-mode default. In agent mode with no -o, `query` defaults to -o auto
// (the cheapest lossless encoding for the result's shape); any explicit -o
// wins, and outside agent mode the -o value is returned unchanged.
func agentResultFormat() (format string, byDefault bool) {
	outputFlag := rootCmd.PersistentFlags().Lookup("output")
	if agentMode && (outputFlag == nil || !outputFlag.Changed) {
		return output.FormatAuto, true
	}
	return outputFormat, false
}

// compactSince is the release that introduced --compact (#578). It is
// experimental because it reshapes the rows a stable command returns.
const compactSince = "0.40.0"

// resolveQueryCompact decides whether query output is compacted (nulls omitted,
// single-value columns hoisted into a `constant` map). Agent mode is
// token-optimal by default, so it is on there and off otherwise; an explicit
// --compact / --compact=false always wins. It applies only where the output has
// a place for the constant map: the agent envelope (json/toon/auto, and the
// table layouts agent mode renders as json) and plain json/yaml/toon/auto. An explicit
// --compact on any other format returns a warning; the agent-mode default stays
// silent there.
func resolveQueryCompact(cmd *cobra.Command, agentMode bool, format string) (bool, string) {
	compact := agentMode
	explicit := cmd.Flags().Changed("compact")
	if explicit {
		compact, _ = cmd.Flags().GetBool("compact")
	}
	if !compact {
		return false, ""
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json", "yaml", "yml", "toon", "auto":
		return true, ""
	case "", "table", "wide":
		if agentMode {
			return true, ""
		}
	}
	if explicit {
		return false, fmt.Sprintf("--compact is ignored with -o %s (it applies to json, yaml and toon output)", displayFormatName(format))
	}
	return false, ""
}

func displayFormatName(format string) string {
	if f := strings.TrimSpace(format); f != "" {
		return f
	}
	return "table"
}

// queryCmd represents the query command
var queryCmd = &cobra.Command{
	Use:     "query [dql-string]",
	Aliases: []string{"q"},
	Short:   "Execute a DQL query",
	Long: `Execute a DQL query against Grail storage.

DQL (Dynatrace Query Language) queries can be executed inline or from a file.
Template variables can be used with the --set flag for reusable queries.

Template Syntax:
  Use {{.variable}} to reference variables.
  Use {{.variable | default "value"}} for default values.

Examples:
  # Execute inline query
  dtctl query "fetch logs | limit 10"

  # Execute from file
  dtctl query -f query.dql

  # Read from stdin (avoids shell escaping issues)
  dtctl query -f - -o json <<'EOF'
  metrics | filter startsWith(metric.key, "dt") | limit 10
  EOF

  # PowerShell: pipe a here-string in -- as an argument, Windows PowerShell 5.1
  # strips the inner double quotes DQL needs (see docs/WINDOWS.md#quoting)
  @'
  fetch logs, bucket:{"custom-logs"} | filter contains(host.name, "api")
  '@ | dtctl query -o json

  # Pipe query from file
  cat query.dql | dtctl query -o json

  # Execute with template variables
  dtctl query -f query.dql --set host=h-123 --set timerange=1h

  # Output as JSON or CSV
  dtctl query "fetch logs" -o json
  dtctl query "fetch logs" -o csv

  # Output as JSON Lines (one JSON object per line) or Parquet for large exports
  dtctl query "fetch logs" -o jsonl
  dtctl query "fetch logs" --max-result-records 100000 -o parquet > logs.parquet

  # Download large datasets with custom limits
  dtctl query "fetch logs" --max-result-records 10000 -o csv > logs.csv

  # Query with specific timeframe
  dtctl query "fetch logs" --default-timeframe-start "2024-01-01T00:00:00Z" \
    --default-timeframe-end "2024-01-02T00:00:00Z" -o csv

  # Query with timezone and locale
  dtctl query "fetch logs" --timezone "Europe/Paris" --locale "fr_FR" -o json

  # Query with sampling for large datasets
  dtctl query "fetch logs" --default-sampling-ratio 10 --max-result-records 10000 -o csv

  # Display as chart with live updates (refresh every 10s)
  dtctl query "timeseries avg(dt.host.cpu.usage)" -o chart --live

  # Live mode with custom interval
  dtctl query "timeseries avg(dt.host.cpu.usage)" -o chart --live --interval 5s

  # Fullscreen chart (uses terminal dimensions)
  dtctl query "timeseries avg(dt.host.cpu.usage)" -o chart --fullscreen

  # Custom chart dimensions
  dtctl query "timeseries avg(dt.host.cpu.usage)" -o chart --width 150 --height 30

  # Compact timeseries: per-series summary + sparkline, or downsampling
  # that keeps each bucket's min and max
  dtctl query "timeseries avg(dt.host.cpu.usage), by:{host.name}" --series=summary
  dtctl query "timeseries avg(dt.host.cpu.usage)" --series=downsample:30 -o csv

  # Round numbers to 3 significant digits
  dtctl query "timeseries avg(dt.host.cpu.usage)" --precision 3 -o json

  # In agent mode --series=summary and --precision 4 are the defaults;
  # restore the raw datapoints with
  dtctl query "timeseries avg(dt.host.cpu.usage)" -A --series=full --precision 0

  # Include query metadata (execution time, scanned records, etc.)
  dtctl query "fetch logs | limit 10" --metadata
  dtctl query "fetch logs | limit 10" -M -o json

  # Include only selected metadata fields
  dtctl query "fetch logs | limit 10" --metadata=executionTimeMilliseconds,scannedRecords,scannedBytes

  # Include only the metadata worth acting on (cost, sampling, default window)
  dtctl query "fetch logs | limit 10" --metadata=minimal
  dtctl query "fetch logs | limit 10" -M=queryId,analysisTimeframe -o json

  # Apply a filter segment to narrow results
  dtctl query "fetch logs | limit 10" --segment my-segment-uid

  # Apply multiple segments (AND-combined)
  dtctl query "fetch logs | limit 10" -S seg-uid-1 -S seg-uid-2

  # Bind variables to a segment inline (URL-query style)
  dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001"

  # Multiple values for a variable (comma-separated)
  dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001,HOST-002"

  # Multiple variables on one segment
  dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001&ns=production"

  # Apply segments with variables from a YAML file
  dtctl query "fetch logs | limit 10" --segments-file segments.yaml
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isSupportedQueryOutputFormat(outputFormat) {
			return fmt.Errorf("unsupported output format %q for query", outputFormat)
		}

		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		queryFile, _ := cmd.Flags().GetString("file")
		setFlags, _ := cmd.Flags().GetStringArray("set")
		dqlFlag, _ := cmd.Flags().GetString("dql")
		if dqlFlag != "" && len(args) == 0 {
			args = []string{dqlFlag}
		}
		// Agents write `dtctl query dql <text>` / `query execute <text>`
		// (hallucinated subcommands) and shell-split queries into several
		// positional args. Until now args[0] was sent alone — the literal
		// string "dql", or a truncated query — producing an opaque
		// UNKNOWN_COMMAND or silently wrong results. Strip the marker token
		// and rejoin the fragments instead (no DQL statement starts with
		// these words).
		if len(args) > 1 {
			switch args[0] {
			case "dql", "execute", "exec", "run":
				args = args[1:]
			}
		}
		if len(args) > 1 {
			args = []string{strings.Join(args, " ")}
		}

		query, err := resolveQueryInput(queryFile, args, osStdin())
		if err != nil {
			return err
		}

		// Apply template rendering if --set flags are provided
		if len(setFlags) > 0 {
			vars, err := template.ParseSetFlags(setFlags)
			if err != nil {
				return fmt.Errorf("invalid --set flag: %w", err)
			}

			rendered, err := template.RenderTemplate(query, vars)
			if err != nil {
				return fmt.Errorf("template rendering failed: %w", err)
			}

			query = rendered
		}

		// The API ignores a default timeframe it cannot parse, so a relative
		// "-1h" silently ran over the server's default window instead. Reject
		// it and name the flag that takes a duration.
		for _, name := range []string{"default-timeframe-start", "default-timeframe-end"} {
			if v, _ := cmd.Flags().GetString(name); v != "" {
				if _, perr := time.Parse(time.RFC3339, v); perr != nil {
					return fmt.Errorf("--%s must be an RFC3339 timestamp, got %q (for a relative window use --from/--to, e.g. --from 1h)", name, v)
				}
			}
		}
		window, err := resolveQueryFromTo(cmd)
		if err != nil {
			return err
		}
		run := dqlRun{Query: query}
		if window != nil {
			run.TimeframeStart, run.TimeframeEnd = window.FromRFC3339(), window.ToRFC3339()
		}
		return runDQL(cmd, cfg, c, run)
	},
}

// maxSegmentsPerQuery is the maximum number of filter segments allowed per query (Dynatrace limit).
const maxSegmentsPerQuery = 10

// parseSegmentFlags parses --segment flag values into FilterSegmentRef entries.
// Each value can be a plain segment ID/name, or include inline variable bindings
// using URL-query-style syntax: "SEGMENT?var=val&var2=val1,val2"
func parseSegmentFlags(segmentIDs []string) ([]exec.FilterSegmentRef, error) {
	var refs []exec.FilterSegmentRef
	for _, raw := range segmentIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("--segment value must not be empty")
		}

		id := raw
		var variables []exec.FilterSegmentVariable

		// Split on first "?" to separate segment ID from inline variables
		if qIdx := strings.Index(raw, "?"); qIdx >= 0 {
			id = strings.TrimSpace(raw[:qIdx])
			queryStr := raw[qIdx+1:]

			if id == "" {
				return nil, fmt.Errorf("invalid --segment %q: segment ID must not be empty", raw)
			}
			if queryStr == "" {
				return nil, fmt.Errorf("invalid --segment %q: expected variables after '?'", raw)
			}

			// Parse "var=val&var2=val1,val2" pairs
			pairs := strings.Split(queryStr, "&")
			for _, pair := range pairs {
				pair = strings.TrimSpace(pair)
				if pair == "" {
					continue
				}
				eqIdx := strings.Index(pair, "=")
				if eqIdx < 0 {
					return nil, fmt.Errorf("invalid --segment %q: expected VARIABLE=VALUE in %q", raw, pair)
				}
				varName := strings.TrimSpace(pair[:eqIdx])
				valuesStr := strings.TrimSpace(pair[eqIdx+1:])
				if varName == "" {
					return nil, fmt.Errorf("invalid --segment %q: variable name must not be empty", raw)
				}
				if valuesStr == "" {
					return nil, fmt.Errorf("invalid --segment %q: variable %q value must not be empty", raw, varName)
				}
				values := strings.Split(valuesStr, ",")
				for i, v := range values {
					values[i] = strings.TrimSpace(v)
				}
				variables = append(variables, exec.FilterSegmentVariable{
					Name:   varName,
					Values: values,
				})
			}
		}

		refs = append(refs, exec.FilterSegmentRef{ID: id, Variables: variables})
	}
	return refs, nil
}

// parseSegmentsFile reads a YAML file containing an array of FilterSegmentRef entries.
func parseSegmentsFile(path string) ([]exec.FilterSegmentRef, error) {
	data, err := readFileFlag("segments-file", path)
	if err != nil {
		return nil, fmt.Errorf("failed to read segments file: %w", err)
	}

	var refs []exec.FilterSegmentRef
	if err := yaml.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("failed to parse segments file %q: %w", path, err)
	}

	// Validate entries
	for i, ref := range refs {
		if ref.ID == "" {
			return nil, fmt.Errorf("segment entry %d in %q is missing required 'id' field", i+1, path)
		}
	}

	return refs, nil
}

// parseSegmentVarFlags parses --segment-var flag values into a map of segment ID -> variables.
// Format: "SEGMENT:VARIABLE=VALUE[,VALUE,...]"
//
// Examples:
//
//	"seg-uid:host=HOST-001"           -> seg-uid: [{name: "host", values: ["HOST-001"]}]
//	"seg-uid:host=HOST-001,HOST-002"  -> seg-uid: [{name: "host", values: ["HOST-001", "HOST-002"]}]
//
// Multiple --segment-var flags for the same segment accumulate variables.
func parseSegmentVarFlags(vars []string) (map[string][]exec.FilterSegmentVariable, error) {
	result := make(map[string][]exec.FilterSegmentVariable)

	for _, v := range vars {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, fmt.Errorf("--segment-var value must not be empty")
		}

		// Split on first ":" to get segment ID and variable assignment
		colonIdx := strings.Index(v, ":")
		if colonIdx < 0 {
			return nil, fmt.Errorf("invalid --segment-var %q: expected format SEGMENT:VARIABLE=VALUE[,VALUE,...]", v)
		}

		segmentID := strings.TrimSpace(v[:colonIdx])
		varAssignment := strings.TrimSpace(v[colonIdx+1:])

		if segmentID == "" {
			return nil, fmt.Errorf("invalid --segment-var %q: segment ID must not be empty", v)
		}
		if varAssignment == "" {
			return nil, fmt.Errorf("invalid --segment-var %q: variable assignment must not be empty", v)
		}

		// Split variable assignment on first "=" to get name and values
		eqIdx := strings.Index(varAssignment, "=")
		if eqIdx < 0 {
			return nil, fmt.Errorf("invalid --segment-var %q: expected VARIABLE=VALUE[,VALUE,...] after segment ID", v)
		}

		varName := strings.TrimSpace(varAssignment[:eqIdx])
		valuesStr := strings.TrimSpace(varAssignment[eqIdx+1:])

		if varName == "" {
			return nil, fmt.Errorf("invalid --segment-var %q: variable name must not be empty", v)
		}
		if valuesStr == "" {
			return nil, fmt.Errorf("invalid --segment-var %q: variable value must not be empty", v)
		}

		// Split values on comma
		values := strings.Split(valuesStr, ",")
		for i, val := range values {
			values[i] = strings.TrimSpace(val)
		}

		// Check if we already have a variable with this name for this segment
		// (merge values if so)
		found := false
		for i, existing := range result[segmentID] {
			if existing.Name == varName {
				result[segmentID][i].Values = append(result[segmentID][i].Values, values...)
				found = true
				break
			}
		}
		if !found {
			result[segmentID] = append(result[segmentID], exec.FilterSegmentVariable{
				Name:   varName,
				Values: values,
			})
		}
	}

	return result, nil
}

// applySegmentVars applies parsed --segment-var bindings to a slice of segment refs.
// Variables are matched by the original (pre-resolution) segment identifier, which is
// looked up via the origIDs map (resolved ID -> original flag value). This allows users
// to specify variables using the same name/UID they passed to --segment.
//
// Returns an error if a --segment-var references a segment not present in the refs.
func applySegmentVars(refs []exec.FilterSegmentRef, varMap map[string][]exec.FilterSegmentVariable, origIDs map[string]string) ([]exec.FilterSegmentRef, error) {
	if len(varMap) == 0 {
		return refs, nil
	}

	// Build reverse lookup: original ID -> index in refs (using origIDs map)
	origToIdx := make(map[string]int, len(refs))
	for i, ref := range refs {
		if orig, ok := origIDs[ref.ID]; ok {
			origToIdx[orig] = i
		}
		// Also allow matching by resolved ID directly
		origToIdx[ref.ID] = i
	}

	for segID, variables := range varMap {
		idx, ok := origToIdx[segID]
		if !ok {
			return nil, fmt.Errorf("--segment-var references segment %q which is not specified via --segment or --segments-file", segID)
		}
		// Merge variables: CLI vars take precedence over file vars for the same name
		existing := refs[idx].Variables
		existingMap := make(map[string]int, len(existing))
		for i, v := range existing {
			existingMap[v.Name] = i
		}
		for _, newVar := range variables {
			if i, ok := existingMap[newVar.Name]; ok {
				// Replace existing variable values
				existing[i] = newVar
			} else {
				existing = append(existing, newVar)
			}
		}
		refs[idx].Variables = existing
	}

	return refs, nil
}

// mergeSegmentRefs merges segment refs from --segment flags and --segments-file.
// File entries win on ID conflict (they may carry variables). Duplicates by ID are deduplicated.
func mergeSegmentRefs(flagRefs, fileRefs []exec.FilterSegmentRef) []exec.FilterSegmentRef {
	// Build map keyed by ID; file entries are added first so flag entries
	// only fill in IDs not already present (file wins).
	seen := make(map[string]exec.FilterSegmentRef, len(flagRefs)+len(fileRefs))
	order := make([]string, 0, len(flagRefs)+len(fileRefs))

	// File entries first (higher priority)
	for _, ref := range fileRefs {
		if _, exists := seen[ref.ID]; !exists {
			order = append(order, ref.ID)
		}
		seen[ref.ID] = ref
	}

	// Flag entries only if not already present from file
	for _, ref := range flagRefs {
		if _, exists := seen[ref.ID]; !exists {
			order = append(order, ref.ID)
			seen[ref.ID] = ref
		}
	}

	merged := make([]exec.FilterSegmentRef, 0, len(order))
	for _, id := range order {
		merged = append(merged, seen[id])
	}
	return merged
}

func init() {
	rootCmd.AddCommand(queryCmd)

	// Flags for main query command
	queryCmd.Flags().StringP("file", "f", "", "read query from file")
	rejectEmptyFlag(queryCmd, "file")
	queryCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	queryCmd.Flags().String("dql", "", "DQL text (alias for the positional argument)")

	// Live mode flags
	queryCmd.Flags().Bool("live", false, "enable live mode with periodic updates")
	queryCmd.Flags().Duration("interval", 60*time.Second, "refresh interval for live mode")

	addDQLExecutionFlags(queryCmd, false)

	// --from/--to resolve on the client to the same default timeframe the
	// --default-timeframe-* flags send, but take a duration ago as well.
	queryCmd.Flags().String("from", "", "start of the query window: a duration ago (2h, 30m, 7d) or an RFC3339 timestamp; queries without their own from: use it")
	queryCmd.Flags().String("to", "", "end of the query window: a duration ago or an RFC3339 timestamp (default: now; needs --from)")
	stability.MarkFlag(queryCmd, "from", stability.Experimental, recipesSince)
	stability.MarkFlag(queryCmd, "to", stability.Experimental, recipesSince)
	queryCmd.MarkFlagsMutuallyExclusive("from", "default-timeframe-start")
	queryCmd.MarkFlagsMutuallyExclusive("to", "default-timeframe-end")
}

// resolveMetadataFlag reads --metadata. Agent mode is token-optimal by default:
// without the flag it gets the minimal set (defaulted=true, so the envelope can
// name the -M=all opt-out when that dropped something). An explicit value
// always wins, and outside agent mode an absent flag means no metadata.
func resolveMetadataFlag(cmd *cobra.Command, agentMode bool) (fields []string, defaulted bool, err error) {
	if agentMode && !cmd.Flags().Changed("metadata") {
		return []string{output.MetadataMinimal}, true, nil
	}
	val, _ := cmd.Flags().GetString("metadata")
	fields, err = output.ParseMetadataFields(val)
	return fields, false, err
}

// metadataFieldCompletion provides shell completion for --metadata flag values.
// It supports comma-separated field selection: already-typed fields are excluded
// from suggestions, and completions include the existing prefix so the shell
// appends correctly (e.g., typing "scannedRecords," suggests "scannedRecords,queryId").
func metadataFieldCompletion(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	allFields := output.ValidMetadataFieldNames()

	// If nothing typed yet, offer "all" and "minimal" plus individual field names
	if toComplete == "" {
		suggestions := make([]string, 0, len(allFields)+2)
		suggestions = append(suggestions, "all\tInclude all metadata fields")
		suggestions = append(suggestions, output.MetadataMinimal+"\tInclude only cost and sampling fields worth acting on")
		suggestions = append(suggestions, allFields...)
		return suggestions, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
	}

	// Split on comma to find already-selected fields and the current partial
	parts := strings.Split(toComplete, ",")
	currentPartial := parts[len(parts)-1]
	prefix := ""
	if len(parts) > 1 {
		prefix = strings.Join(parts[:len(parts)-1], ",") + ","
	}

	// Build set of already-selected fields
	selected := make(map[string]bool, len(parts)-1)
	for _, p := range parts[:len(parts)-1] {
		selected[strings.TrimSpace(p)] = true
	}

	// Suggest unselected fields (and the minimal selector) that match the current partial
	var suggestions []string
	for _, f := range append([]string{output.MetadataMinimal}, allFields...) {
		if selected[f] {
			continue
		}
		if strings.HasPrefix(f, currentPartial) {
			suggestions = append(suggestions, prefix+f)
		}
	}

	return suggestions, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(queryCmd)
}
