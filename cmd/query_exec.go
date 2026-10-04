package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/recipes"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

// recipesSince is the release that introduced recipes (`dtctl run`, `get
// recipes`, `describe recipe`, `verify recipe`) and `query --from/--to`.
const recipesSince = "0.42.0"

// addDQLExecutionFlags registers the flags that shape a query's execution and
// output. `dtctl query` and every recipe under `dtctl run` carry them, so a
// recipe takes -o, the limits, spill and the segment flags exactly as query
// does. forRecipe leaves out what only makes sense for free-form DQL: live
// mode, the raw --default-timeframe-* flags (recipes take --from/--to) and
// snapshot decoding.
func addDQLExecutionFlags(c *cobra.Command, forRecipe bool) {
	// Chart sizing flags
	c.Flags().Int("width", 0, "chart width in characters (0 = default)")
	c.Flags().Int("height", 0, "chart height in lines (0 = default)")
	c.Flags().Bool("fullscreen", false, "use terminal dimensions for chart")

	// Query limit flags
	c.Flags().Int64("max-result-records", 0, "maximum number of result records to return (0 = use default, typically 1000)")
	c.Flags().Int64("max-result-bytes", 0, "maximum result size in bytes (0 = use default)")
	c.Flags().Float64("default-scan-limit-gbytes", 0, "scan limit in gigabytes (0 = use default)")
	addQueryLimitFlags(c)

	// Query execution flags
	c.Flags().Float64("default-sampling-ratio", 0, "default sampling ratio (0 = use default, normalized to power of 10 <= 100000)")
	c.Flags().Int32("fetch-timeout-seconds", 0, "time limit for fetching data in seconds (0 = use default)")
	c.Flags().Bool("enable-preview", false, "request preview results if available within timeout")
	c.Flags().Bool("no-progress", false, "disable the live progress bar shown on stderr for long queries")
	c.Flags().Bool("enforce-query-consumption-limit", false, "enforce query consumption limit")
	c.Flags().Bool("include-types", false, "surface DQL per-column type info as a top-level \"types\" key (json/yaml output)")
	c.Flags().Bool("include-contributions", false, "include bucket contribution information in query results")
	c.Flags().Bool("typed", false, "cast scalar columns (long, double, duration, boolean) to native JSON/YAML types instead of the API's string encoding; opt-in, implies --include-types")
	c.Flags().Bool("compact", false, `omit null values and print columns that hold one value in every row once, under "constant"
(json/yaml/toon and the agent envelope; also collapses them in a spill summary). Default: on in agent mode`)
	stability.MarkFlag(c, "compact", stability.Experimental, compactSince)

	if !forRecipe {
		// Timeframe flags
		c.Flags().String("default-timeframe-start", "", "query timeframe start timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T12:10:04.123Z')")
		c.Flags().String("default-timeframe-end", "", "query timeframe end timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T13:10:04.123Z')")
	}

	// Localization flags
	c.Flags().String("locale", "", "query locale (e.g., 'en_US', 'de_DE')")
	c.Flags().String("timezone", "", "query timezone (e.g., 'UTC', 'Europe/Paris', 'America/New_York')")

	// Metadata flag
	c.Flags().StringP("metadata", "M", "", `include query metadata in output (use = for field selection)
bare --metadata or -M shows all fields; --metadata=field1,field2 selects specific fields
--metadata=minimal keeps only execution time, scanned bytes/data points, sampled (when true),
approximations (when present), and analysisTimeframe (when the query named no window);
it combines with field names. In agent mode the default is minimal; -M=all restores the full block
available: executionTimeMilliseconds,scannedRecords,scannedBytes,scannedDataPoints,
sampled,approximations,notifications,queryId,dqlVersion,query,canonicalQuery,timezone,locale,
analysisTimeframe,contributions,metrics`)
	c.Flags().Lookup("metadata").NoOptDefVal = "all"

	if !forRecipe {
		// Snapshot decode flag
		c.Flags().String("decode-snapshots", "", `decode Live Debugger snapshot payloads in query results
	bare --decode-snapshots simplifies variant wrappers to plain values;
	--decode-snapshots=full preserves the full decoded tree with type annotations`)
		c.Flags().Lookup("decode-snapshots").NoOptDefVal = "simplified"
		// The flag is weaker than the command that carries it: `query` is stable,
		// but this decodes Live Debugger snapshot payloads (see
		// markLiveDebuggerExperimental). Marking the flag rather than the command
		// is exactly the case a per-flag tier exists for.
		stability.MarkFlag(c, "decode-snapshots", stability.Experimental, liveDebuggerSince)
	}

	// Filter segment flags
	c.Flags().StringArrayP("segment", "S", nil, `filter segment ID or name (repeatable, max 10, AND-combined)
supports inline variables: -S "SEGMENT?var=val&var2=val1,val2"`)
	c.Flags().String("segments-file", "", "YAML file with filter segment definitions (supports variables)")
	c.Flags().StringArrayP("segment-var", "V", nil, `override a segment variable (repeatable)
format: SEGMENT:VARIABLE=VALUE[,VALUE,...]
takes precedence over --segments-file variables`)

	// Shell completion for --metadata field names (supports comma-separated values)
	_ = c.RegisterFlagCompletionFunc("metadata", metadataFieldCompletion)

	if !forRecipe {
		// Shell completion for --decode-snapshots values
		_ = c.RegisterFlagCompletionFunc("decode-snapshots", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"simplified\tFlatten variant wrappers to plain values (default)",
				"full\tPreserve full decoded tree with type annotations",
			}, cobra.ShellCompDirectiveNoFileComp
		})
	}

	// Shell completion for --segments-file (YAML files)
	_ = c.RegisterFlagCompletionFunc("segments-file", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
	})

	// Client context flag
	c.Flags().String("client-context", "", `optional caller context included in the dt-client-context request header
useful for AI agents or scripts to declare their intent (e.g. "root-cause-analysis", "anomaly-investigation")`)

	// Result spill flags. A large result is written to a local file and a compact
	// summary (column stats + sample rows + a file handle) is returned in its
	// place, so the rows never flood an agent's context.
	c.Flags().String("spill", "", `spill a large result to a local file and return a summary instead of the rows
bare --spill = always; --spill=auto spills above --spill-threshold; --spill=never forces rows
default: never for a bare command, auto in agent mode`)
	c.Flags().Lookup("spill").NoOptDefVal = "always"
	c.Flags().String("spill-to", "", "explicit spill destination file (implies --spill=always; format inferred from extension)")
	c.Flags().String("spill-format", "", "spill file format when spilling to the default dir: jsonl|json|csv|parquet (default jsonl)")
	c.Flags().String("spill-threshold", "", "serialised output size above which a result spills, e.g. 50KB (default 50KB)")

	// In-response output bounds for agent mode (#583): a per-value cap and a
	// budget on the encoded envelope, for the middle zone below the spill
	// threshold that a row limit alone does not bound.
	addOutputBoundFlags(c)
	stability.MarkFlag(c, "max-field-chars", stability.Experimental, outputBoundsSince)
	stability.MarkFlag(c, "max-output-bytes", stability.Experimental, outputBoundsSince)
	stability.MarkFlag(c, "max-output-tokens", stability.Experimental, outputBoundsSince)

	_ = c.RegisterFlagCompletionFunc("spill", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"auto\tspill above the threshold, inline below",
			"always\talways spill",
			"never\tnever spill (rows inline)",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = c.RegisterFlagCompletionFunc("spill-format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"jsonl", "json", "csv", "parquet"}, cobra.ShellCompDirectiveNoFileComp
	})
}

// resolveQueryFromTo reads `query --from/--to` into an absolute window, or nil
// when neither is set.
func resolveQueryFromTo(cmd *cobra.Command) (*recipes.Window, error) {
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	return recipes.ResolveQueryWindow(from, to, time.Now())
}

// dqlRun is one query execution: the DQL plus what the caller resolved
// beyond the flags (`--from/--to`, a recipe's window and envelope additions).
type dqlRun struct {
	Query string
	// TimeframeStart/End override --default-timeframe-start/-end (RFC3339).
	TimeframeStart, TimeframeEnd string
	Decorate                     func(*output.ResponseContext, *exec.DQLQueryResponse, []map[string]interface{})
	EmptyHint                    string
	IsEmpty                      func([]map[string]interface{}) bool
	SkipEmptyDiagnosis           bool
}

// runDQL executes a query with the execution flags of cmd (see
// addDQLExecutionFlags). It is `dtctl query` after the query text is known,
// shared with `dtctl run` so a recipe is a pre-filled query, not a second
// query engine: limits, spill, output bounds, empty-result diagnosis and every
// printer apply unchanged.
func runDQL(cmd *cobra.Command, cfg *config.Config, c *client.Client, run dqlRun) error {
	query := run.Query
	executor := NewDQLExecutorFromConfig(cfg, c)

	// Set up signal handling so a running Grail query is cancelled on Ctrl+C / SIGTERM.
	// NotifyContext leaves no goroutine behind when the command ends or the
	// caller's context is cancelled first.
	ctx, stop := signal.NotifyContext(cmdContext(cmd), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Fail fast, before any request, when the query provably needs a
	// storage scope the token lacks — instead of one NOT_AUTHORIZED_FOR_TABLE
	// per query an agent fans out.
	if err := dqlScopePrecheck(query); err != nil {
		return err
	}

	// Get visualization options
	live, _ := cmd.Flags().GetBool("live")
	interval, _ := cmd.Flags().GetDuration("interval")
	width, _ := cmd.Flags().GetInt("width")
	height, _ := cmd.Flags().GetInt("height")
	fullscreen, _ := cmd.Flags().GetBool("fullscreen")

	// Query limits resolve flag -> context config -> global config -> server
	// default, so a context-level ceiling also covers invocations that never
	// passed the flags. See resolveQueryLimits.
	queryLimits, err := resolveQueryLimits(cmd, cfg)
	if err != nil {
		return err
	}

	// Get query execution options
	fetchTimeoutSeconds, _ := cmd.Flags().GetInt32("fetch-timeout-seconds")
	enablePreview, _ := cmd.Flags().GetBool("enable-preview")
	noProgress, _ := cmd.Flags().GetBool("no-progress")
	enforceQueryConsumptionLimit, _ := cmd.Flags().GetBool("enforce-query-consumption-limit")
	includeTypes, _ := cmd.Flags().GetBool("include-types")
	typed, _ := cmd.Flags().GetBool("typed")
	// Only an explicit --include-types surfaces the DQL type block as a
	// top-level "types" key in structured output. Parquet/--typed force
	// includeTypes on below to *consume* the metadata internally, but that
	// must not start emitting the block, so capture intent before forcing.
	emitTypes := cmd.Flags().Changed("include-types")
	// Only an explicit, true --include-types asked for the block, so only
	// that warns when a format or live mode cannot show it;
	// --include-types=false declines it and must stay quiet.
	typesRequested := emitTypes && includeTypes
	// Parquet derives its column schema from DQL types, so request them even
	// if the user did not pass --include-types. The type metadata is consumed
	// to build the schema and is not added to the output rows.
	if formatRequiresIncludeTypes(outputFormat) {
		includeTypes = true
	}
	// --typed casts scalar columns using the DQL type metadata, so it likewise
	// needs the types requested even without an explicit --include-types.
	if typed {
		includeTypes = true
	}
	includeContributions, _ := cmd.Flags().GetBool("include-contributions")

	// Get timeframe options
	defaultTimeframeStart, _ := cmd.Flags().GetString("default-timeframe-start")
	defaultTimeframeEnd, _ := cmd.Flags().GetString("default-timeframe-end")
	if run.TimeframeStart != "" {
		defaultTimeframeStart, defaultTimeframeEnd = run.TimeframeStart, run.TimeframeEnd
	}

	// Get localization options
	locale, _ := cmd.Flags().GetString("locale")
	timezone, _ := cmd.Flags().GetString("timezone")

	metadataFields, metadataDefaulted, err := resolveMetadataFlag(cmd, agentMode)
	if err != nil {
		return err
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

	// Resolved against the effective format, so the agent-mode -o auto
	// default composes with the agent-mode --compact default.
	queryFormat, autoByDefault := agentResultFormat()
	compact, compactWarning := resolveQueryCompact(cmd, agentMode, queryFormat)
	if compactWarning != "" {
		output.PrintWarning("%s", compactWarning)
	}

	seriesVal, _ := cmd.Flags().GetString("series")
	precision, _ := cmd.Flags().GetInt("precision")
	seriesOpts, err := querySeriesOptions(seriesVal, cmd.Flags().Changed("series"),
		precision, cmd.Flags().Changed("precision"))
	if err != nil {
		return err
	}

	spillOpts, err := resolveSpillOptions(cmd, cfg)
	if err != nil {
		return err
	}
	bounds, err := resolveOutputBounds(cmd)
	if err != nil {
		return err
	}
	for _, w := range bounds.Warnings {
		output.PrintWarning("%s", w)
	}
	spillTenantID, spillContextName := spillProvenance(cfg)
	// A Parquet spill derives its columnar schema from DQL types, so request
	// them even when the displayed output format does not — same reasoning as
	// the -o parquet path above, applied to the spill destination.
	if spillOpts.Enabled() && spillWritesParquet(spillOpts) {
		includeTypes = true
	}

	opts := exec.DQLExecuteOptions{
		OutputFormat:                 queryFormat,
		AutoFormatByDefault:          autoByDefault,
		JQFilter:                     jqFilter,
		AgentMode:                    agentMode,
		Decode:                       decodeMode,
		Width:                        width,
		Height:                       height,
		Fullscreen:                   fullscreen,
		MaxResultRecords:             queryLimits.MaxResultRecords,
		MaxResultBytes:               queryLimits.MaxResultBytes,
		DefaultScanLimitGbytes:       queryLimits.ScanLimitGbytes,
		DefaultSamplingRatio:         queryLimits.SamplingRatio,
		FetchTimeoutSeconds:          fetchTimeoutSeconds,
		EnablePreview:                enablePreview,
		EnforceQueryConsumptionLimit: enforceQueryConsumptionLimit,
		IncludeTypes:                 includeTypes,
		EmitTypes:                    emitTypes,
		TypesRequested:               typesRequested,
		IncludeContributions:         includeContributions,
		Typed:                        typed,
		Compact:                      compact,
		Series:                       seriesOpts.Mode,
		Precision:                    seriesOpts.Precision,
		SeriesDefaulted:              seriesOpts.SeriesDefaulted,
		PrecisionDefaulted:           seriesOpts.PrecisionDefaulted,
		DefaultTimeframeStart:        defaultTimeframeStart,
		DefaultTimeframeEnd:          defaultTimeframeEnd,
		Locale:                       locale,
		Timezone:                     timezone,
		MetadataFields:               metadataFields,
		MetadataDefaulted:            metadataDefaulted,
		Verbose:                      verbosity > 0,
		Segments:                     segments,
		ClientContext:                clientContext,
		Spill:                        spillOpts,
		MaxFieldChars:                bounds.MaxFieldChars,
		MaxOutputBytes:               bounds.MaxOutputBytes,
		TenantID:                     spillTenantID,
		ContextName:                  spillContextName,
		// The progress bar is a user-facing affordance of the `query`
		// command only; opt in here (subject to --no-progress) so internal
		// query callers stay silent by default.
		ShowProgress: !noProgress,
		Decorate:     run.Decorate,
		EmptyHint:    run.EmptyHint,
		IsEmpty:      run.IsEmpty,

		SkipEmptyDiagnosis: run.SkipEmptyDiagnosis,
	}

	// Handle live mode
	if live {
		if !caps.LongRunningStreams {
			return &CapabilityError{Feature: "live mode"}
		}
		// Warn about flags that are not meaningfully applicable in live mode
		if len(metadataFields) > 0 {
			output.PrintWarning("--metadata is ignored in live mode (metadata is not displayed during live updates)")
		}
		if agentMode {
			output.PrintWarning("--agent is ignored in live mode (live mode requires an interactive terminal)")
		}
		if spillOpts.Enabled() {
			output.PrintWarning("--spill is ignored in live mode (live mode streams rows to the terminal)")
		}
		if includeContributions {
			output.PrintWarning("--include-contributions is ignored in live mode (contribution data is not displayed during live updates)")
		}
		if typed {
			output.PrintWarning("--typed is ignored in live mode (live mode renders a table, where the API's string encoding is not surfaced)")
		}
		if typesRequested {
			output.PrintWarning("--include-types is ignored in live mode (live updates render the records only, without the types block)")
		}
		if cmd.Flags().Changed("series") || cmd.Flags().Changed("precision") {
			output.PrintWarning("--series and --precision are ignored in live mode (live mode renders the full series)")
		}

		if interval == 0 {
			interval = output.DefaultLiveInterval
		}

		// Live mode owns the terminal via its own printer; a per-fetch
		// progress bar would fight it, so keep it off.
		opts.ShowProgress = false

		// Create printer options for live mode (needed for resize support)
		printerOpts := output.PrinterOptions{
			Format:     outputFormat,
			JQFilter:   jqFilter,
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
}
