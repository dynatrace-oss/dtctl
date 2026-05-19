package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
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

func isSupportedQueryOutputFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "table", "wide", "json", "yaml", "yml", "csv", "toon", "chart", "sparkline", "spark", "barchart", "bar", "braille", "br":
		return true
	default:
		return false
	}
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

  # PowerShell: Use here-strings to avoid quote issues
  dtctl query -f - -o json @'
  fetch logs, bucket:{"custom-logs"} | filter contains(host.name, "api")
  '@

  # Pipe query from file
  cat query.dql | dtctl query -o json

  # Execute with template variables
  dtctl query -f query.dql --set host=h-123 --set timerange=1h

  # Output as JSON or CSV
  dtctl query "fetch logs" -o json
  dtctl query "fetch logs" -o csv

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

  # Include query metadata (execution time, scanned records, etc.)
  dtctl query "fetch logs | limit 10" --metadata
  dtctl query "fetch logs | limit 10" -M -o json

  # Include only selected metadata fields
  dtctl query "fetch logs | limit 10" --metadata=executionTimeMilliseconds,scannedRecords,scannedBytes
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

		executor := NewDQLExecutorFromConfig(cfg, c)

		// Set up signal handling so a running Grail query is cancelled on Ctrl+C / SIGTERM.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigCh)
		go func() {
			<-sigCh
			cancel()
		}()

		queryFile, _ := cmd.Flags().GetString("file")
		setFlags, _ := cmd.Flags().GetStringArray("set")

		var query string

		if queryFile != "" {
			// Read query from file (use "-" for stdin)
			if queryFile == "-" {
				content, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read query from stdin: %w", err)
				}
				query = string(content)
			} else {
				content, err := os.ReadFile(queryFile)
				if err != nil {
					return fmt.Errorf("failed to read query file: %w", err)
				}
				query = string(content)
			}
		} else if len(args) > 0 {
			// Use inline query
			query = args[0]
		} else if !isTerminal(os.Stdin) {
			// Read from piped stdin
			content, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read query from stdin: %w", err)
			}
			query = string(content)
		} else {
			return fmt.Errorf("query string or --file is required")
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

		// Get visualization options
		live, _ := cmd.Flags().GetBool("live")
		interval, _ := cmd.Flags().GetDuration("interval")
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")
		fullscreen, _ := cmd.Flags().GetBool("fullscreen")

		// Get query limit options
		maxResultRecords, _ := cmd.Flags().GetInt64("max-result-records")
		maxResultBytes, _ := cmd.Flags().GetInt64("max-result-bytes")
		defaultScanLimitGbytes, _ := cmd.Flags().GetFloat64("default-scan-limit-gbytes")

		// Get query execution options
		defaultSamplingRatio, _ := cmd.Flags().GetFloat64("default-sampling-ratio")
		fetchTimeoutSeconds, _ := cmd.Flags().GetInt32("fetch-timeout-seconds")
		enablePreview, _ := cmd.Flags().GetBool("enable-preview")
		enforceQueryConsumptionLimit, _ := cmd.Flags().GetBool("enforce-query-consumption-limit")
		includeTypes, _ := cmd.Flags().GetBool("include-types")
		includeContributions, _ := cmd.Flags().GetBool("include-contributions")

		// Get timeframe options
		defaultTimeframeStart, _ := cmd.Flags().GetString("default-timeframe-start")
		defaultTimeframeEnd, _ := cmd.Flags().GetString("default-timeframe-end")

		// Get localization options
		locale, _ := cmd.Flags().GetString("locale")
		timezone, _ := cmd.Flags().GetString("timezone")

		// Get metadata option
		metadataVal, _ := cmd.Flags().GetString("metadata")
		// In agent mode, always include metadata unless explicitly disabled
		if agentMode && !cmd.Flags().Changed("metadata") {
			metadataVal = "all"
		}
		var metadataFields []string
		if metadataVal != "" {
			var err error
			metadataFields, err = output.ParseMetadataFields(metadataVal)
			if err != nil {
				return err
			}
		}

		// Get snapshot decode option
		decodeVal, _ := cmd.Flags().GetString("decode-snapshots")
		var decodeMode exec.DecodeMode
		if cmd.Flags().Changed("decode-snapshots") {
			switch decodeVal {
			case "", "simplified":
				decodeMode = exec.DecodeSimplified
			case "full":
				decodeMode = exec.DecodeFull
			default:
				return fmt.Errorf("unsupported --decode-snapshots value %q (use \"simplified\" or \"full\")", decodeVal)
			}
		}

		// Parse filter segments
		segmentFlags, _ := cmd.Flags().GetStringArray("segment")
		segmentsFile, _ := cmd.Flags().GetString("segments-file")
		segmentVarFlags, _ := cmd.Flags().GetStringArray("segment-var")

		var segments []exec.FilterSegmentRef
		if len(segmentFlags) > 0 || segmentsFile != "" {
			var flagRefs, fileRefs []exec.FilterSegmentRef

			// Track original (pre-resolution) IDs so --segment-var can
			// reference segments by the same name/UID the user typed.
			origIDs := make(map[string]string) // resolved UID -> original flag value

			if len(segmentFlags) > 0 {
				flagRefs, err = parseSegmentFlags(segmentFlags)
				if err != nil {
					return err
				}

				// Resolve segment names to UIDs for --segment flag values.
				// IDs from --segments-file are assumed to be UIDs already (the file
				// format mirrors the API and should use UIDs).
				res := resolver.NewResolver(c)
				for i, ref := range flagRefs {
					orig := ref.ID
					resolved, resolveErr := res.ResolveID(resolver.TypeSegment, ref.ID)
					if resolveErr != nil {
						return fmt.Errorf("failed to resolve segment %q: %w", ref.ID, resolveErr)
					}
					flagRefs[i].ID = resolved
					origIDs[resolved] = orig
				}
			}

			if segmentsFile != "" {
				fileRefs, err = parseSegmentsFile(segmentsFile)
				if err != nil {
					return err
				}
			}

			segments = mergeSegmentRefs(flagRefs, fileRefs)

			// Apply --segment-var bindings
			if len(segmentVarFlags) > 0 {
				varMap, varErr := parseSegmentVarFlags(segmentVarFlags)
				if varErr != nil {
					return varErr
				}
				segments, err = applySegmentVars(segments, varMap, origIDs)
				if err != nil {
					return err
				}
			}

			if len(segments) > maxSegmentsPerQuery {
				return fmt.Errorf("too many segments: %d specified, maximum is %d per query", len(segments), maxSegmentsPerQuery)
			}
		} else if len(segmentVarFlags) > 0 {
			return fmt.Errorf("--segment-var requires at least one --segment or --segments-file")
		}

		clientContext, _ := cmd.Flags().GetString("client-context")

		opts := exec.DQLExecuteOptions{
			OutputFormat:                 outputFormat,
			Decode:                       decodeMode,
			Width:                        width,
			Height:                       height,
			Fullscreen:                   fullscreen,
			MaxResultRecords:             maxResultRecords,
			MaxResultBytes:               maxResultBytes,
			DefaultScanLimitGbytes:       defaultScanLimitGbytes,
			DefaultSamplingRatio:         defaultSamplingRatio,
			FetchTimeoutSeconds:          fetchTimeoutSeconds,
			EnablePreview:                enablePreview,
			EnforceQueryConsumptionLimit: enforceQueryConsumptionLimit,
			IncludeTypes:                 includeTypes,
			IncludeContributions:         includeContributions,
			DefaultTimeframeStart:        defaultTimeframeStart,
			DefaultTimeframeEnd:          defaultTimeframeEnd,
			Locale:                       locale,
			Timezone:                     timezone,
			MetadataFields:               metadataFields,
			Segments:                     segments,
			ClientContext:                clientContext,
		}

		// Handle live mode
		if live {
			// Warn about flags that are not meaningfully applicable in live mode
			if len(metadataFields) > 0 {
				output.PrintWarning("--metadata is ignored in live mode (metadata is not displayed during live updates)")
			}
			if agentMode {
				output.PrintWarning("--agent is ignored in live mode (live mode requires an interactive terminal)")
			}
			if includeContributions {
				output.PrintWarning("--include-contributions is ignored in live mode (contribution data is not displayed during live updates)")
			}
			if dryRun {
				output.PrintWarning("--dry-run is ignored in live mode (live mode always executes queries)")
			}

			if interval == 0 {
				interval = output.DefaultLiveInterval
			}

			// Create printer options for live mode (needed for resize support)
			printerOpts := output.PrinterOptions{
				Format:     outputFormat,
				Width:      width,
				Height:     height,
				Fullscreen: fullscreen,
			}

			printer := output.NewPrinterWithOpts(printerOpts)
			livePrinter := output.NewLivePrinterWithOpts(printer, interval, os.Stdout, printerOpts)

			// Create data fetcher that re-executes the query
			fetcher := func(fetchCtx context.Context) (interface{}, error) {
				result, err := executor.ExecuteQueryWithContext(fetchCtx, query, opts)
				if err != nil {
					return nil, err
				}
				if result == nil {
					return nil, nil // context cancelled; message already printed
				}
				// Extract records
				records := result.Records
				if result.Result != nil && len(result.Result.Records) > 0 {
					records = result.Result.Records
				}
				// Apply snapshot decoding if requested
				if decodeMode != exec.DecodeNone && len(records) > 0 {
					simplify := decodeMode == exec.DecodeSimplified
					records = output.DecodeSnapshotRecords(records, simplify)

					// For tabular formats, replace parsed_snapshot with a summary string
					switch outputFormat {
					case "", "table", "wide", "csv":
						records = output.SummarizeSnapshotForTable(records)
					}
				}
				return map[string]interface{}{"records": records}, nil
			}

			return livePrinter.RunLive(ctx, fetcher)
		}

		return executor.ExecuteWithContext(ctx, query, opts)
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
	data, err := os.ReadFile(path)
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
	queryCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")

	// Live mode flags
	queryCmd.Flags().Bool("live", false, "enable live mode with periodic updates")
	queryCmd.Flags().Duration("interval", 60*time.Second, "refresh interval for live mode")

	// Chart sizing flags
	queryCmd.Flags().Int("width", 0, "chart width in characters (0 = default)")
	queryCmd.Flags().Int("height", 0, "chart height in lines (0 = default)")
	queryCmd.Flags().Bool("fullscreen", false, "use terminal dimensions for chart")

	// Query limit flags
	queryCmd.Flags().Int64("max-result-records", 0, "maximum number of result records to return (0 = use default, typically 1000)")
	queryCmd.Flags().Int64("max-result-bytes", 0, "maximum result size in bytes (0 = use default)")
	queryCmd.Flags().Float64("default-scan-limit-gbytes", 0, "scan limit in gigabytes (0 = use default)")

	// Query execution flags
	queryCmd.Flags().Float64("default-sampling-ratio", 0, "default sampling ratio (0 = use default, normalized to power of 10 <= 100000)")
	queryCmd.Flags().Int32("fetch-timeout-seconds", 0, "time limit for fetching data in seconds (0 = use default)")
	queryCmd.Flags().Bool("enable-preview", false, "request preview results if available within timeout")
	queryCmd.Flags().Bool("enforce-query-consumption-limit", false, "enforce query consumption limit")
	queryCmd.Flags().Bool("include-types", false, "include type information in query results")
	queryCmd.Flags().Bool("include-contributions", false, "include bucket contribution information in query results")

	// Timeframe flags
	queryCmd.Flags().String("default-timeframe-start", "", "query timeframe start timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T12:10:04.123Z')")
	queryCmd.Flags().String("default-timeframe-end", "", "query timeframe end timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T13:10:04.123Z')")

	// Localization flags
	queryCmd.Flags().String("locale", "", "query locale (e.g., 'en_US', 'de_DE')")
	queryCmd.Flags().String("timezone", "", "query timezone (e.g., 'UTC', 'Europe/Paris', 'America/New_York')")

	// Metadata flag
	queryCmd.Flags().StringP("metadata", "M", "", `include query metadata in output (use = for field selection)
bare --metadata or -M shows all fields; --metadata=field1,field2 selects specific fields
available: executionTimeMilliseconds,scannedRecords,scannedBytes,scannedDataPoints,
sampled,queryId,dqlVersion,query,canonicalQuery,timezone,locale,
analysisTimeframe,contributions`)
	queryCmd.Flags().Lookup("metadata").NoOptDefVal = "all"

	// Snapshot decode flag
	queryCmd.Flags().String("decode-snapshots", "", `(experimental) decode Live Debugger snapshot payloads in query results
bare --decode-snapshots simplifies variant wrappers to plain values;
--decode-snapshots=full preserves the full decoded tree with type annotations`)
	queryCmd.Flags().Lookup("decode-snapshots").NoOptDefVal = "simplified"

	// Filter segment flags
	queryCmd.Flags().StringArrayP("segment", "S", nil, `filter segment ID or name (repeatable, max 10, AND-combined)
supports inline variables: -S "SEGMENT?var=val&var2=val1,val2"`)
	queryCmd.Flags().String("segments-file", "", "YAML file with filter segment definitions (supports variables)")
	queryCmd.Flags().StringArrayP("segment-var", "V", nil, `override a segment variable (repeatable)
format: SEGMENT:VARIABLE=VALUE[,VALUE,...]
takes precedence over --segments-file variables`)

	// Shell completion for --metadata field names (supports comma-separated values)
	_ = queryCmd.RegisterFlagCompletionFunc("metadata", metadataFieldCompletion)

	// Shell completion for --decode-snapshots values
	_ = queryCmd.RegisterFlagCompletionFunc("decode-snapshots", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"simplified\tFlatten variant wrappers to plain values (default)",
			"full\tPreserve full decoded tree with type annotations",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// Shell completion for --segments-file (YAML files)
	_ = queryCmd.RegisterFlagCompletionFunc("segments-file", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
	})

	// Client context flag
	queryCmd.Flags().String("client-context", "", `optional caller context included in the dt-client-context request header
useful for AI agents or scripts to declare their intent (e.g. "root-cause-analysis", "anomaly-investigation")`)
}

// metadataFieldCompletion provides shell completion for --metadata flag values.
// It supports comma-separated field selection: already-typed fields are excluded
// from suggestions, and completions include the existing prefix so the shell
// appends correctly (e.g., typing "scannedRecords," suggests "scannedRecords,queryId").
func metadataFieldCompletion(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	allFields := output.ValidMetadataFieldNames()

	// If nothing typed yet, offer "all" plus individual field names
	if toComplete == "" {
		suggestions := make([]string, 0, len(allFields)+1)
		suggestions = append(suggestions, "all\tInclude all metadata fields")
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

	// Suggest unselected fields that match the current partial
	var suggestions []string
	for _, f := range allFields {
		if selected[f] {
			continue
		}
		if strings.HasPrefix(f, currentPartial) {
			suggestions = append(suggestions, prefix+f)
		}
	}

	return suggestions, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
}
