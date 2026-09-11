package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/anomalydetector"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// createAnomalyDetectorCmd creates an anomaly detector from a file
var createAnomalyDetectorCmd = &cobra.Command{
	Use:     "anomaly-detector -f <file>",
	Aliases: []string{"ad"},
	Short:   "Create a custom anomaly detector from a file",
	Long: `Create a new custom anomaly detector from a YAML or JSON file.

Accepts both flattened YAML format (recommended) and raw Settings API format.
When the source field is omitted in flattened format, it defaults to "dtctl".

Examples:
  # Create from flattened YAML (recommended)
  dtctl create anomaly-detector -f detector.yaml

  # Create from raw Settings API format
  dtctl create anomaly-detector -f detector-raw.yaml

  # Create with template variables
  dtctl create anomaly-detector -f detector.yaml --set threshold=95

  # Dry run to preview
  dtctl create anomaly-detector -f detector.yaml --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		setFlags, _ := cmd.Flags().GetStringArray("set")

		// Read the file
		fileData, err := vfs.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Convert to JSON if needed
		jsonData, err := format.ValidateAndConvert(fileData)
		if err != nil {
			return fmt.Errorf("invalid file format: %w", err)
		}

		// Apply template rendering if variables provided
		if len(setFlags) > 0 {
			templateVars, err := template.ParseSetFlags(setFlags)
			if err != nil {
				return fmt.Errorf("invalid --set flag: %w", err)
			}
			rendered, err := template.RenderTemplate(string(jsonData), templateVars)
			if err != nil {
				return fmt.Errorf("template rendering failed: %w", err)
			}
			jsonData = []byte(rendered)
		}

		// Handle dry-run
		if dryRun {
			output.PrintInfo("Dry run: would create anomaly detector")
			output.PrintInfo("---")
			output.PrintInfo("%s", string(jsonData))
			output.PrintInfo("---")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		handler := anomalydetector.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create anomaly detector: %w", err)
		}

		output.PrintSuccess("Anomaly detector %q created", result.Title)
		output.PrintInfo("  Object ID: %s", result.ObjectID)
		output.PrintInfo("  Title:     %s", result.Title)
		output.PrintInfo("  Analyzer:  %s", result.AnalyzerShort)
		output.PrintInfo("  Enabled:   %v", result.Enabled)
		output.PrintInfo("")
		output.PrintInfo("Run 'dtctl describe anomaly-detector %s' to view details", result.ObjectID)
		return nil
	},
}

func init() {
	createAnomalyDetectorCmd.Flags().StringP("file", "f", "", "file containing anomaly detector definition (required)")
	createAnomalyDetectorCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createAnomalyDetectorCmd.MarkFlagRequired("file")
}
