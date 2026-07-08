package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
)

type analyzerPreset struct {
	commandName  string
	analyzerName string
	description  string
}

var analyzerPresets = []analyzerPreset{
	{commandName: "forecast", analyzerName: "dt.statistics.GenericForecastAnalyzer", description: "Run the generic forecast analyzer"},
	{commandName: "anomaly", analyzerName: "dt.statistics.GenericAnomalyDetectionAnalyzer", description: "Run the generic anomaly-detection analyzer"},
	{commandName: "change-point", analyzerName: "dt.statistics.GenericChangePointAnalyzer", description: "Run the generic change-point analyzer"},
	{commandName: "correlation", analyzerName: "dt.statistics.GenericCorrelationAnalyzer", description: "Run the generic correlation analyzer"},
}

// analyzeCmd groups concrete Davis analyzer execution helpers.
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run Davis AI analyzers with concrete input",
	Long: `Run Davis AI analyzers directly from the CLI.

This command mirrors dtctl exec analyzer, but keeps the Davis AI workflow
under a dedicated verb for users who prefer an analysis-oriented command tree.`,
	RunE: requireSubcommand,
}

// analyzeAnalyzerCmd executes a Davis analyzer with explicit input.
var analyzeAnalyzerCmd = &cobra.Command{
	Use:     "analyzer <analyzer-name>",
	Aliases: []string{"az"},
	Short:   "Execute a Davis AI analyzer",
	Long: `Execute a Davis AI analyzer with the given input.

Examples:
  # Execute analyzer with input from file
  dtctl analyze analyzer dt.statistics.GenericForecastAnalyzer -f input.json

  # Execute with inline JSON input
  dtctl analyze analyzer dt.statistics.GenericForecastAnalyzer --input '{"query":"timeseries avg(dt.host.cpu.usage)"}'

  # Execute with DQL query shorthand (for forecast/timeseries analyzers)
  dtctl analyze analyzer dt.statistics.GenericForecastAnalyzer --query "timeseries avg(dt.host.cpu.usage)"

  # Validate input without executing
  dtctl analyze analyzer dt.statistics.GenericForecastAnalyzer -f input.json --validate

  # Execute and wait for completion (default)
  dtctl analyze analyzer dt.statistics.GenericForecastAnalyzer -f input.json --wait

  # Output as JSON
  dtctl analyze analyzer dt.statistics.GenericForecastAnalyzer -f input.json -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: runAnalyzeAnalyzer,
}

func newAnalyzerPresetCmd(preset analyzerPreset) *cobra.Command {
	return &cobra.Command{
		Use:   preset.commandName,
		Short: preset.description,
		Long: fmt.Sprintf(`Execute the %s.

Examples:
  # Execute with a DQL query
  dtctl analyze %s --query "timeseries avg(dt.host.cpu.usage)"

  # Execute from a file
  dtctl analyze %s --file input.json

  # Validate input without executing
  dtctl analyze %s --query "timeseries avg(dt.host.cpu.usage)" --validate
`, preset.analyzerName, preset.commandName, preset.commandName, preset.commandName),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnalyzeAnalyzerPreset(cmd, preset.analyzerName)
		},
	}
}

func runAnalyzeAnalyzer(cmd *cobra.Command, args []string) error {
	return runAnalyzeAnalyzerPresetWithArgs(cmd, args[0])
}

func runAnalyzeAnalyzerPreset(cmd *cobra.Command, analyzerName string) error {
	return runAnalyzeAnalyzerPresetWithArgs(cmd, analyzerName)
}

func runAnalyzeAnalyzerPresetWithArgs(cmd *cobra.Command, analyzerName string) error {

	_, c, _, err := Setup()
	if err != nil {
		return err
	}

	handler := analyzer.NewHandler(c)

	input, err := buildAnalyzerInput(cmd)
	if err != nil {
		return err
	}

	validateOnly, _ := cmd.Flags().GetBool("validate")
	if validateOnly {
		result, err := handler.Validate(analyzerName, input)
		if err != nil {
			return err
		}
		printer := NewPrinter()
		return printer.Print(result)
	}

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

	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "" || outputFormat == "table" {
		outputFormat = "json"
	}

	printer := output.NewPrinter(outputFormat)
	return printer.Print(result)
}

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
		return map[string]interface{}{"timeSeriesData": query}, nil
	default:
		return nil, fmt.Errorf("input is required: use --file, --input, or --query")
	}
}

func init() {
	analyzeCmd.AddCommand(analyzeAnalyzerCmd)
	for _, preset := range analyzerPresets {
		analyzeCmd.AddCommand(newAnalyzerPresetCmd(preset))
	}
	rootCmd.AddCommand(analyzeCmd)

	analyzeAnalyzerCmd.Flags().StringP("file", "f", "", "read input from JSON file")
	analyzeAnalyzerCmd.Flags().String("input", "", "inline JSON input")
	analyzeAnalyzerCmd.Flags().String("query", "", "DQL query shorthand (for timeseries analyzers)")
	analyzeAnalyzerCmd.Flags().Bool("validate", false, "validate input without executing")
	analyzeAnalyzerCmd.Flags().Bool("wait", true, "wait for analyzer execution to complete")
	analyzeAnalyzerCmd.Flags().Int("timeout", 300, "timeout in seconds when waiting for completion")

	for _, cmd := range analyzeCmd.Commands() {
		if cmd == analyzeAnalyzerCmd {
			continue
		}
		addAnalyzerExecutionFlags(cmd)
	}
}

func addAnalyzerExecutionFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("file", "f", "", "read input from JSON file")
	cmd.Flags().String("input", "", "inline JSON input")
	cmd.Flags().String("query", "", "DQL query shorthand (for timeseries analyzers)")
	cmd.Flags().Bool("validate", false, "validate input without executing")
	cmd.Flags().Bool("wait", true, "wait for analyzer execution to complete")
	cmd.Flags().Int("timeout", 300, "timeout in seconds when waiting for completion")
}
