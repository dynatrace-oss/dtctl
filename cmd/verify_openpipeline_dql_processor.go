package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/dqlprocessorverify"
)

// verifyOpenPipelineDQLProcessorCmd validates a DQL processor script against
// the OpenPipeline DQL processor verify endpoint.
var verifyOpenPipelineDQLProcessorCmd = &cobra.Command{
	Use:   "openpipeline-dql-processor [dql-script]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Verify an OpenPipeline DQL processor script",
	Long: `Verify an OpenPipeline DQL processor script without applying it.

The command sends the script to the OpenPipeline DQL processor verify endpoint
and reports whether it is valid. Diagnostics (errors, warnings) are printed
with their source positions when available.

The script can be provided as a positional argument or read from a file (use
"-" for stdin). Exactly one of these must be supplied.

Exit codes:
  0 - Script is valid
  1 - Script is invalid, or the command failed (auth, network, or server error)

Examples:
  # Verify inline DQL processor script
  dtctl verify openpipeline-dql-processor 'fieldsAdd severity = "INFO"'

  # Read script from a file
  dtctl verify openpipeline-dql-processor -f processor.dql

  # Read script from stdin
  cat processor.dql | dtctl verify openpipeline-dql-processor -f -

  # Verify scoped to a specific configuration
  dtctl verify openpipeline-dql-processor 'fieldsAdd x = 1' --config-id logs

  # Get structured output
  dtctl verify openpipeline-dql-processor 'fieldsAdd x = 1' -o json
  dtctl verify openpipeline-dql-processor 'fieldsAdd x = 1' -o yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fileFlag, _ := cmd.Flags().GetString("file")
		configIDFlag, _ := cmd.Flags().GetString("config-id")

		// Exactly one of: positional arg or --file
		hasArg := len(args) > 0
		hasFile := fileFlag != ""
		if hasArg && hasFile {
			return fmt.Errorf("provide either a positional DQL script or --file, not both")
		}
		if !hasArg && !hasFile {
			return fmt.Errorf("a DQL script or --file is required")
		}

		var script string
		if hasFile {
			content, err := readVerifyExpressionFromFile(fileFlag)
			if err != nil {
				return err
			}
			script = content
		} else {
			script = args[0]
		}

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := dqlprocessorverify.NewHandler(c)
		result, err := handler.Verify(dqlprocessorverify.VerifyOptions{
			Script:          script,
			ConfigurationID: configIDFlag,
		})
		if err != nil {
			return err
		}

		ap := enrichAgent(printer, "verify", "openpipeline-dql-processor")

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

func init() {
	verifyOpenPipelineDQLProcessorCmd.Flags().StringP("file", "f", "", `read the DQL script from a file ("-" for stdin)`)
	verifyOpenPipelineDQLProcessorCmd.Flags().String("config-id", "", `configuration scope, e.g. "logs"`)
}
