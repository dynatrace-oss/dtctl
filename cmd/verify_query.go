package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// verifyQueryCmd represents the verify query subcommand
var verifyQueryCmd = &cobra.Command{
	Use:     "query [dql-string]",
	Aliases: []string{"q"},
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
  dtctl verify query "fetch logs | limit 10"

  # Verify query from file
  dtctl verify query -f query.dql

  # Read from stdin (recommended for complex queries)
  dtctl verify query -f - <<'EOF'
  fetch logs | filter status == "ERROR"
  EOF

  # Pipe query from file or command
  cat query.dql | dtctl verify query
  echo 'fetch logs | limit 10' | dtctl verify query

  # PowerShell: Use here-strings for complex queries
  dtctl verify query -f - @'
  fetch logs, bucket:{"custom-logs"} | filter contains(host.name, "api")
  '@

  # Verify with template variables
  dtctl verify query -f query.dql --set host=h-123 --set timerange=1h

  # Get canonical query representation (normalized format)
  dtctl verify query "fetch logs" --canonical

  # Verify with specific timezone and locale
  dtctl verify query "fetch logs" --timezone "Europe/Paris" --locale "fr_FR"

  # Get structured output (JSON or YAML)
  dtctl verify query "fetch logs" -o json
  dtctl verify query "fetch logs" -o yaml

  # CI/CD: Fail on warnings (strict validation)
  dtctl verify query -f query.dql --fail-on-warn
  if [ $? -eq 0 ]; then echo "Query is valid"; fi

  # CI/CD: Validate all queries in a directory
  for file in queries/*.dql; do
    echo "Verifying $file..."
    dtctl verify query -f "$file" --fail-on-warn || exit 1
  done

  # CI/CD: Validate query with canonical output
  dtctl verify query -f query.dql --canonical -o json | jq '.canonicalQuery'

  # Pre-commit hook: Verify staged query files
  git diff --cached --name-only --diff-filter=ACM "*.dql" | \
    xargs -I {} dtctl verify query -f {} --fail-on-warn

  # Check exit codes for different scenarios
  dtctl verify query "invalid query syntax"       # Exit 1: syntax error
  dtctl verify query "fetch logs" --fail-on-warn  # Exit 0 or 1 based on warnings

  # Verify query with all options
  dtctl verify query -f query.dql --canonical --timezone "UTC" --locale "en_US" --fail-on-warn

  # Verify template query before execution
  dtctl verify query -f template.dql --set env=prod --set timerange=1h
  dtctl query -f template.dql --set env=prod --set timerange=1h

  # Script usage: check if query is valid before running
  if dtctl verify query -f query.dql --fail-on-warn 2>/dev/null; then
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
	verifyCmd.AddCommand(verifyQueryCmd)

	// Flags for verify query command
	verifyQueryCmd.Flags().StringP("file", "f", "", "read query from file (use '-' for stdin)")
	verifyQueryCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	verifyQueryCmd.Flags().Bool("canonical", false, "print canonical query representation")
	verifyQueryCmd.Flags().String("timezone", "", "timezone for query verification (IANA, CET, +01:00, etc.)")
	verifyQueryCmd.Flags().String("locale", "", "locale for query verification (en, en_US, de_AT, etc.)")
	verifyQueryCmd.Flags().Bool("fail-on-warn", false, "exit with non-zero status on warnings (useful for CI/CD)")
}
