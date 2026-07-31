package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/matcherverify"
)

// verifyOpenPipelineMatcherCmd validates a DQL matcher expression against the
// OpenPipeline matcher verify endpoint.
var verifyOpenPipelineMatcherCmd = &cobra.Command{
	Use:   "openpipeline-matcher [matcher-expression]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Verify an OpenPipeline DQL matcher expression",
	Long: `Verify an OpenPipeline DQL matcher expression without applying it.

The command sends the matcher expression to the OpenPipeline matcher verify
endpoint and reports whether it is valid. Diagnostics (errors, warnings) are
printed with their source positions when available.

The matcher can be provided as a positional argument or read from a file (use
"-" for stdin). Exactly one of these must be supplied.

Exit codes:
  0 - Matcher is valid
  1 - Matcher is invalid, or the command failed (auth, network, or server error)

Examples:
  # Verify inline matcher expression
  dtctl verify openpipeline-matcher 'matchesValue(content, "error")'

  # Read matcher from a file
  dtctl verify openpipeline-matcher -f matcher.dql

  # Read matcher from stdin
  echo 'matchesValue(content, "error")' | dtctl verify openpipeline-matcher -f -

  # Verify with a stage context
  dtctl verify openpipeline-matcher 'matchesValue(content, "error")' --context ROUTING_RULE

  # Verify scoped to a specific configuration
  dtctl verify openpipeline-matcher 'matchesValue(content, "error")' --config-id logs

  # Get structured output
  dtctl verify openpipeline-matcher 'matchesValue(content, "error")' -o json
  dtctl verify openpipeline-matcher 'matchesValue(content, "error")' -o yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isSupportedVerifyOutputFormat(outputFormat) {
			return fmt.Errorf("unsupported output format %q for verify openpipeline-matcher (supported: json, yaml, toon)", outputFormat)
		}

		fileFlag, _ := cmd.Flags().GetString("file")
		contextFlag, _ := cmd.Flags().GetString("context")
		configIDFlag, _ := cmd.Flags().GetString("config-id")

		// Exactly one of: positional arg or --file
		hasArg := len(args) > 0
		hasFile := fileFlag != ""
		if hasArg && hasFile {
			return fmt.Errorf("provide either a positional matcher expression or --file, not both")
		}
		if !hasArg && !hasFile {
			return fmt.Errorf("a matcher expression or --file is required")
		}

		var query string
		if hasFile {
			content, err := readVerifyExpressionFromFile(fileFlag)
			if err != nil {
				return err
			}
			query = content
		} else {
			query = args[0]
		}

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := matcherverify.NewHandler(c)
		result, err := handler.Verify(matcherverify.VerifyOptions{
			Query:           query,
			ConfigurationID: configIDFlag,
			Context:         contextFlag,
		})
		if err != nil {
			return err
		}

		ap := enrichAgent(printer, "verify", "openpipeline-matcher")

		// Agent mode, an explicit -o format, or a --jq filter: delegate to the
		// printer (agent envelope carrying valid/notifications, or
		// json/yaml/table/wide/csv/toon with jq applied). Otherwise emit the
		// default human-readable summary.
		if ap != nil || cmd.Root().PersistentFlags().Lookup("output").Changed || jqFilter != "" {
			if err := printer.Print(result); err != nil {
				return err
			}
		} else {
			if result.Valid {
				fmt.Println("✓ Valid")
			} else {
				fmt.Println("✗ Invalid")
			}
			if result.Summary != "" {
				for _, line := range strings.Split(result.Summary, "\n") {
					fmt.Printf("  %s\n", line)
				}
			}
		}

		// A false verdict is a successful API call that must still exit non-zero
		// in every mode. silentExitError is intercepted in root.go before the
		// agent error-envelope path, so the ok:true envelope printed above stands
		// as the sole output and the process still exits with ExitError.
		if !result.Valid {
			return &silentExitError{code: client.ExitError}
		}
		return nil
	},
}

// readVerifyExpressionFromFile reads a text expression from a file or stdin.
func readVerifyExpressionFromFile(path string) (string, error) {
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("open %q: %w", path, err)
		}
		defer func() { _ = f.Close() }()
		reader = f
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read expression: %w", err)
	}
	return string(content), nil
}

func init() {
	verifyOpenPipelineMatcherCmd.Flags().StringP("file", "f", "", `read the matcher from a file ("-" for stdin)`)
	verifyOpenPipelineMatcherCmd.Flags().String("context", "", `stage context, e.g. "processing" or "ROUTING_RULE"`)
	verifyOpenPipelineMatcherCmd.Flags().String("config-id", "", `configuration scope, e.g. "logs"`)
}
