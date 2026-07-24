package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/resources/matcherlqltodql"
)

var translateLqlToDqlCmd = &cobra.Command{
	Use:   "lql-to-dql [lql-expression]",
	Short: "Translate an LQL matcher expression into a DQL matcher expression",
	Long: `Translate a single LQL matcher expression into its semantically equivalent
DQL matcher expression using the OpenPipeline translation endpoint.

The translated DQL expression is returned verbatim. By default it is printed as
plain text, ready to copy-paste or pipe into another command. Use -o json or
-o yaml for structured output; -o table for a single-row table.

The LQL expression can be supplied as a positional argument or read from a file
(or stdin with "-"). Exactly one of the two forms must be given.

Required OAuth scope: openpipeline:configurations:read

Examples:
  # Translate an inline LQL expression
  dtctl translate lql-to-dql 'log.source="snmptraps" AND snmp.trap_oid="F5-BIGIP-COMMON-MIB"'

  # Read the LQL expression from a file
  dtctl translate lql-to-dql -f matcher.lql

  # Read from stdin
  echo 'log.source="snmptraps"' | dtctl translate lql-to-dql -f -

  # Output as JSON
  dtctl translate lql-to-dql 'log.source="snmptraps"' -o json

  # Use in agent mode
  dtctl translate lql-to-dql 'log.source="snmptraps"' -A`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")

		hasArg := len(args) == 1
		hasFile := filePath != ""

		if hasArg && hasFile {
			return fmt.Errorf("provide either a positional LQL expression or --file, not both")
		}
		if !hasArg && !hasFile {
			return fmt.Errorf("provide an LQL expression as an argument or via --file (use \"-\" for stdin)")
		}

		var lql string
		if hasFile {
			content, err := appengine.ReadFileOrStdin(filePath)
			if err != nil {
				return fmt.Errorf("read LQL expression: %w", err)
			}
			lql = strings.TrimSpace(content)
		} else {
			lql = args[0]
		}

		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := matcherlqltodql.NewHandler(c)
		result, err := handler.Translate(lql)
		if err != nil {
			return err
		}

		ap := enrichAgent(printer, "translate", "lql-to-dql")
		if ap != nil {
			ap.SetSuggestions([]string{
				"Exchange your LQL matcher with the translated DQL matcher",
			})
			return printer.Print(result)
		}

		// Default (no explicit -o): print the raw DQL string, ready to pipe.
		// An explicit -o flag routes to the structured printer.
		outputFlag := cmd.Root().PersistentFlags().Lookup("output")
		if outputFlag != nil && outputFlag.Changed {
			return printer.Print(result)
		}
		_, err = fmt.Println(result.Query)
		return err
	},
}

func init() {
	translateLqlToDqlCmd.Flags().StringP("file", "f", "", `Read the LQL expression from a file ("-" for stdin)`)
}
