package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
)

// execAnalyzerCmd executes a Davis analyzer
var execAnalyzerCmd = &cobra.Command{
	Use:     "analyzer <analyzer-name>",
	Aliases: []string{"az"},
	Short:   "Execute a Davis AI analyzer",
	Long: `Execute a Davis AI analyzer with the given input.

Examples:
  # Execute analyzer with input from file
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json

  # Execute with inline JSON input
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --input '{"query":"timeseries avg(dt.host.cpu.usage)"}'

  # Execute with DQL query shorthand (for forecast/timeseries analyzers)
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --query "timeseries avg(dt.host.cpu.usage)"

  # Validate input without executing
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json --validate

  # Execute and wait for completion (default)
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json --wait

  # Output as JSON
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		analyzerName := args[0]

		_, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := analyzer.NewHandler(c)

		// Build input from flags (shared with "verify analyzer")
		input, err := buildAnalyzerInput(cmd)
		if err != nil {
			return err
		}

		// Handle validate-only mode
		validateOnly, _ := cmd.Flags().GetBool("validate")
		if validateOnly {
			result, err := handler.Validate(analyzerName, input)
			if err != nil {
				return err
			}
			printer := NewPrinter()
			return printer.Print(result)
		}

		// Execute analyzer
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetInt("timeout")

		var result *analyzer.ExecuteResult
		if wait {
			result, err = handler.ExecuteAndWait(cmd.Context(), analyzerName, input, timeout)
		} else {
			result, err = handler.Execute(analyzerName, input, 30)
		}

		if err != nil {
			return err
		}

		// Default to JSON output for analyzer results since table doesn't show the actual data
		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "" || outputFormat == "table" {
			outputFormat = "json"
		}
		printer := output.NewPrinter(outputFormat)
		return printer.Print(result)
	},
}

// addAnalyzerInputFlags registers the input-source flags shared by
// "exec analyzer" and "verify analyzer".
func addAnalyzerInputFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("file", "f", "", "read input from JSON file")
	cmd.Flags().String("input", "", "inline JSON input")
	cmd.Flags().String("query", "", "DQL query shorthand (for timeseries analyzers)")
}

// buildAnalyzerInput assembles an analyzer input map from the --file, --input,
// or --query flags. It is shared by "exec analyzer" and "verify analyzer" so the
// two commands accept identical input. Exactly one source must be provided.
func buildAnalyzerInput(cmd *cobra.Command) (map[string]interface{}, error) {
	inputFile, _ := cmd.Flags().GetString("file")
	inputJSON, _ := cmd.Flags().GetString("input")
	query, _ := cmd.Flags().GetString("query")

	switch {
	case inputFile != "":
		return analyzer.ParseInputFromFile(inputFile)
	case inputJSON != "":
		var input map[string]interface{}
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			return nil, fmt.Errorf("failed to parse input JSON: %w", err)
		}
		return input, nil
	case query != "":
		// Shorthand for timeseries analyzers.
		return map[string]interface{}{"timeSeriesData": query}, nil
	default:
		return nil, fmt.Errorf("input is required: use --file, --input, or --query")
	}
}

func init() {
	// Analyzer flags
	addAnalyzerInputFlags(execAnalyzerCmd)
	execAnalyzerCmd.Flags().Bool("validate", false, "validate input without executing")
	execAnalyzerCmd.Flags().Bool("wait", true, "wait for analyzer execution to complete")
	execAnalyzerCmd.Flags().Int("timeout", 300, "timeout in seconds when waiting for completion")
}
