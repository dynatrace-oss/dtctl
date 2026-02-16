package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
	"github.com/spf13/cobra"
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

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := analyzer.NewHandler(c)

		// Build input from flags
		var input map[string]interface{}

		inputFile, _ := cmd.Flags().GetString("file")
		inputJSON, _ := cmd.Flags().GetString("input")
		query, _ := cmd.Flags().GetString("query")

		if inputFile != "" {
			input, err = analyzer.ParseInputFromFile(inputFile)
			if err != nil {
				return err
			}
		} else if inputJSON != "" {
			if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
				return fmt.Errorf("failed to parse input JSON: %w", err)
			}
		} else if query != "" {
			// Shorthand for timeseries query
			input = map[string]interface{}{
				"timeSeriesData": query,
			}
		} else {
			return fmt.Errorf("input is required: use --file, --input, or --query")
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
			result, err = handler.ExecuteAndWait(analyzerName, input, timeout)
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

func init() {
	// Analyzer flags
	execAnalyzerCmd.Flags().StringP("file", "f", "", "read input from JSON file")
	execAnalyzerCmd.Flags().String("input", "", "inline JSON input")
	execAnalyzerCmd.Flags().String("query", "", "DQL query shorthand (for timeseries analyzers)")
	execAnalyzerCmd.Flags().Bool("validate", false, "validate input without executing")
	execAnalyzerCmd.Flags().Bool("wait", true, "wait for analyzer execution to complete")
	execAnalyzerCmd.Flags().Int("timeout", 300, "timeout in seconds when waiting for completion")
}
