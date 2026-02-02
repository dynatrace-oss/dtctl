package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// isTerminal checks if the given file is a terminal
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
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
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		executor := exec.NewDQLExecutor(c)

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

		// Get timeframe options
		defaultTimeframeStart, _ := cmd.Flags().GetString("default-timeframe-start")
		defaultTimeframeEnd, _ := cmd.Flags().GetString("default-timeframe-end")

		// Get localization options
		locale, _ := cmd.Flags().GetString("locale")
		timezone, _ := cmd.Flags().GetString("timezone")

		opts := exec.DQLExecuteOptions{
			OutputFormat:                 outputFormat,
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
			DefaultTimeframeStart:        defaultTimeframeStart,
			DefaultTimeframeEnd:          defaultTimeframeEnd,
			Locale:                       locale,
			Timezone:                     timezone,
		}

		// Handle live mode
		if live {
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
			fetcher := func(ctx context.Context) (interface{}, error) {
				result, err := executor.ExecuteQueryWithOptions(query, opts)
				if err != nil {
					return nil, err
				}
				// Extract records
				records := result.Records
				if result.Result != nil && len(result.Result.Records) > 0 {
					records = result.Result.Records
				}
				return map[string]interface{}{"records": records}, nil
			}

			return livePrinter.RunLive(context.Background(), fetcher)
		}

		return executor.ExecuteWithOptions(query, opts)
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)

	// Flags
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

	// Timeframe flags
	queryCmd.Flags().String("default-timeframe-start", "", "query timeframe start timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T12:10:04.123Z')")
	queryCmd.Flags().String("default-timeframe-end", "", "query timeframe end timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T13:10:04.123Z')")

	// Localization flags
	queryCmd.Flags().String("locale", "", "query locale (e.g., 'en_US', 'de_DE')")
	queryCmd.Flags().String("timezone", "", "query timezone (e.g., 'UTC', 'Europe/Paris', 'America/New_York')")
}
