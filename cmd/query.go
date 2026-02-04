package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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

// formatVerifyResultHuman prints verification results in human-readable format
func formatVerifyResultHuman(result *exec.DQLVerifyResponse, query string, showCanonical bool) error {
	useColor := isStderrTerminal()

	// Print validation status
	if result.Valid {
		if useColor {
			fmt.Fprintf(os.Stderr, "%s✔%s Query is valid\n", colorGreen, colorReset)
		} else {
			fmt.Fprintf(os.Stderr, "✔ Query is valid\n")
		}
	} else {
		if useColor {
			fmt.Fprintf(os.Stderr, "%s✖%s Query is invalid\n", colorRed, colorReset)
		} else {
			fmt.Fprintf(os.Stderr, "✖ Query is invalid\n")
		}
	}

	// Print notifications grouped by severity
	for _, notification := range result.Notifications {
		severity := notification.Severity
		if severity == "" {
			severity = "INFO"
		}

		// Determine color based on severity
		var color string
		switch severity {
		case "ERROR":
			color = colorRed
		case "WARN", "WARNING":
			color = colorYellow
		case "INFO":
			color = colorCyan
		default:
			color = colorCyan
		}

		// Format notification message
		var prefix string
		if useColor {
			prefix = fmt.Sprintf("%s%s:%s", color, severity, colorReset)
		} else {
			prefix = fmt.Sprintf("%s:", severity)
		}

		// Print notification type and message
		if notification.NotificationType != "" {
			fmt.Fprintf(os.Stderr, "%s %s: %s", prefix, notification.NotificationType, notification.Message)
		} else {
			fmt.Fprintf(os.Stderr, "%s %s", prefix, notification.Message)
		}

		// Add line/column info if available
		if notification.SyntaxPosition != nil && notification.SyntaxPosition.Start != nil {
			fmt.Fprintf(os.Stderr, " (line %d, col %d)", notification.SyntaxPosition.Start.Line, notification.SyntaxPosition.Start.Column)
		}
		fmt.Fprintf(os.Stderr, "\n")

		// Print caret indicator for syntax errors with position
		if severity == "ERROR" && notification.SyntaxPosition != nil && notification.SyntaxPosition.Start != nil {
			if err := printSyntaxError(query, notification.SyntaxPosition, useColor); err != nil {
				// If we can't print the caret, just continue
				continue
			}
		}
	}

	// Print canonical query if requested
	if showCanonical && result.CanonicalQuery != "" {
		fmt.Fprintf(os.Stderr, "\nCanonical Query:\n%s\n", result.CanonicalQuery)
	}

	return nil
}

// printSyntaxError prints the query line with a caret indicator pointing to the error position
func printSyntaxError(query string, pos *exec.SyntaxPosition, useColor bool) error {
	if pos == nil || pos.Start == nil {
		return fmt.Errorf("no position information")
	}

	// Split query into lines
	lines := strings.Split(query, "\n")

	// Line numbers are 1-based
	lineNum := pos.Start.Line
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line number out of range")
	}

	// Get the relevant line (0-indexed)
	line := lines[lineNum-1]

	// Column is 1-based
	col := pos.Start.Column
	if col < 1 {
		col = 1
	}

	// Print the line
	fmt.Fprintf(os.Stderr, "  %s\n", line)

	// Print caret indicator
	// Account for the "  " indent
	spaces := strings.Repeat(" ", col+1) // +2 for indent, -1 for 1-based column

	// Determine caret length (if End position is available)
	caretLen := 1
	if pos.End != nil && pos.End.Line == lineNum && pos.End.Column > col {
		caretLen = pos.End.Column - col + 1
	}
	carets := strings.Repeat("^", caretLen)

	if useColor {
		fmt.Fprintf(os.Stderr, "%s%s%s%s\n", spaces, colorRed, carets, colorReset)
	} else {
		fmt.Fprintf(os.Stderr, "%s%s\n", spaces, carets)
	}

	return nil
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

// queryVerifyCmd represents the query verify subcommand
var queryVerifyCmd = &cobra.Command{
	Use:     "verify [dql-string]",
	Aliases: []string{"v"},
	Short:   "Verify a DQL query without executing it",
	Long: `Verify a DQL query without executing it against Grail storage.

This command validates query syntax, checks for errors and warnings, and optionally 
returns the canonical representation of the query. This is useful for testing queries 
in CI/CD pipelines or checking query correctness before execution.

The verify command returns different exit codes based on the result:
  0 - Query is valid
  1 - Query is invalid or has errors (or warnings with --fail-on-warn)
  2 - Authentication/permission error
  3 - Network/server error

DQL (Dynatrace Query Language) queries can be verified inline or from a file.
Template variables can be used with the --set flag for reusable queries.

Template Syntax:
  Use {{.variable}} to reference variables.
  Use {{.variable | default "value"}} for default values.

Examples:
  # Verify inline query
  dtctl query verify "fetch logs | limit 10"

  # Verify query from file
  dtctl query verify -f query.dql

  # Read from stdin (recommended for complex queries)
  dtctl query verify -f - <<'EOF'
  fetch logs | filter status == "ERROR"
  EOF

  # Pipe query from file or command
  cat query.dql | dtctl query verify
  echo 'fetch logs | limit 10' | dtctl query verify

  # PowerShell: Use here-strings for complex queries
  dtctl query verify -f - @'
  fetch logs, bucket:{"custom-logs"} | filter contains(host.name, "api")
  '@

  # Verify with template variables
  dtctl query verify -f query.dql --set host=h-123 --set timerange=1h

  # Get canonical query representation (normalized format)
  dtctl query verify "fetch logs" --canonical

  # Verify with specific timezone and locale
  dtctl query verify "fetch logs" --timezone "Europe/Paris" --locale "fr_FR"

  # Get structured output (JSON or YAML)
  dtctl query verify "fetch logs" -o json
  dtctl query verify "fetch logs" -o yaml

  # CI/CD: Fail on warnings (strict validation)
  dtctl query verify -f query.dql --fail-on-warn
  if [ $? -eq 0 ]; then echo "Query is valid"; fi

  # CI/CD: Validate all queries in a directory
  for file in queries/*.dql; do
    echo "Verifying $file..."
    dtctl query verify -f "$file" --fail-on-warn || exit 1
  done

  # CI/CD: Validate query with canonical output
  dtctl query verify -f query.dql --canonical -o json | jq '.canonicalQuery'

  # Pre-commit hook: Verify staged query files
  git diff --cached --name-only --diff-filter=ACM "*.dql" | \
    xargs -I {} dtctl query verify -f {} --fail-on-warn

  # Check exit codes for different scenarios
  dtctl query verify "invalid query syntax"       # Exit 1: syntax error
  dtctl query verify "fetch logs" --fail-on-warn  # Exit 0 or 1 based on warnings

  # Verify query with all options
  dtctl query verify -f query.dql --canonical --timezone "UTC" --locale "en_US" --fail-on-warn

  # Verify template query before execution
  dtctl query verify -f template.dql --set env=prod --set timerange=1h
  dtctl query -f template.dql --set env=prod --set timerange=1h

  # Script usage: check if query is valid before running
  if dtctl query verify -f query.dql --fail-on-warn 2>/dev/null; then
    dtctl query -f query.dql -o csv > results.csv
  else
    echo "Query validation failed"
    exit 1
  fi
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

		// Get verify options
		canonical, _ := cmd.Flags().GetBool("canonical")
		timezone, _ := cmd.Flags().GetString("timezone")
		locale, _ := cmd.Flags().GetString("locale")
		failOnWarn, _ := cmd.Flags().GetBool("fail-on-warn")

		opts := exec.DQLVerifyOptions{
			GenerateCanonicalQuery: canonical,
			Timezone:               timezone,
			Locale:                 locale,
		}

		// Call VerifyQuery and handle response
		result, err := executor.VerifyQuery(query, opts)

		// Get exit code first (needed for all output formats)
		exitCode := getVerifyExitCode(result, err, failOnWarn)

		// Handle errors (network, auth, API)
		if err != nil {
			// Exit with appropriate code
			if exitCode != 0 {
				os.Exit(exitCode)
			}
			return err
		}

		// Format output based on --output flag
		outputFmt, _ := cmd.Flags().GetString("output")

		switch outputFmt {
		case "json":
			// Print full DQLVerifyResponse as JSON
			printer := output.NewPrinter("json")
			if err := printer.Print(result); err != nil {
				return fmt.Errorf("failed to print JSON output: %w", err)
			}
		case "yaml", "yml":
			// Print full DQLVerifyResponse as YAML
			printer := output.NewPrinter("yaml")
			if err := printer.Print(result); err != nil {
				return fmt.Errorf("failed to print YAML output: %w", err)
			}
		default:
			// Default: human-readable format
			if err := formatVerifyResultHuman(result, query, canonical); err != nil {
				return fmt.Errorf("failed to format output: %w", err)
			}
		}

		// Exit with appropriate code if non-zero
		if exitCode != 0 {
			os.Exit(exitCode)
		}

		return nil
	},
}

// getVerifyExitCode determines the exit code based on verification results and errors
func getVerifyExitCode(result *exec.DQLVerifyResponse, err error, failOnWarn bool) int {
	// Handle errors first
	if err != nil {
		errMsg := err.Error()

		// Check for auth/permission errors (401, 403)
		if strings.Contains(errMsg, "status 401") || strings.Contains(errMsg, "status 403") {
			return 2
		}

		// Check for network/server errors (timeout, 5xx)
		if strings.Contains(errMsg, "status 5") ||
			strings.Contains(errMsg, "timeout") ||
			strings.Contains(errMsg, "connection") {
			return 3
		}

		// Other errors (likely client-side issues)
		return 1
	}

	// No error from API call, check verification result
	if result == nil {
		return 1
	}

	// Check if query is invalid
	if !result.Valid {
		return 1
	}

	// Check for ERROR notifications
	for _, notification := range result.Notifications {
		if notification.Severity == "ERROR" {
			return 1
		}
	}

	// Check for WARN notifications if --fail-on-warn is set
	if failOnWarn {
		for _, notification := range result.Notifications {
			if notification.Severity == "WARN" || notification.Severity == "WARNING" {
				return 1
			}
		}
	}

	// Valid query with no errors (and no warnings, or warnings without --fail-on-warn)
	return 0
}

func init() {
	rootCmd.AddCommand(queryCmd)

	// Register subcommands
	queryCmd.AddCommand(queryVerifyCmd)

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

	// Timeframe flags
	queryCmd.Flags().String("default-timeframe-start", "", "query timeframe start timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T12:10:04.123Z')")
	queryCmd.Flags().String("default-timeframe-end", "", "query timeframe end timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T13:10:04.123Z')")

	// Localization flags
	queryCmd.Flags().String("locale", "", "query locale (e.g., 'en_US', 'de_DE')")
	queryCmd.Flags().String("timezone", "", "query timezone (e.g., 'UTC', 'Europe/Paris', 'America/New_York')")

	// Flags for query verify subcommand
	queryVerifyCmd.Flags().StringP("file", "f", "", "read query from file (use '-' for stdin)")
	queryVerifyCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	queryVerifyCmd.Flags().Bool("canonical", false, "print canonical query representation")
	queryVerifyCmd.Flags().String("timezone", "", "timezone for query verification (IANA, CET, +01:00, etc.)")
	queryVerifyCmd.Flags().String("locale", "", "locale for query verification (en, en_US, de_AT, etc.)")
	queryVerifyCmd.Flags().Bool("fail-on-warn", false, "exit with non-zero status on warnings (useful for CI/CD)")
}
