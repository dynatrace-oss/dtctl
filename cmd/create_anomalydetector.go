package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/anomalydetector"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
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
			return dryRunCreateAnomalyDetector(jsonData)
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		handler := anomalydetector.NewHandler(c).WithDefaultActor(currentActor(c))

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

// currentActor resolves the identity used for executionSettings.actor when a
// definition omits it. An unresolvable identity is not fatal: the environment
// may accept the detector without an actor, and the resulting API error names
// the field if it does not.
func currentActor(c *client.Client) string {
	actor, err := c.CurrentUserID()
	if err != nil {
		return ""
	}
	return actor
}

// dryRunCreateAnomalyDetector prints the payload dtctl would send and reports
// the verdict of server-side schema validation. Echoing the input back
// unchecked reported success for definitions the live call rejects (issue #369).
func dryRunCreateAnomalyDetector(jsonData []byte) error {
	_, c, err := SetupClient()
	if err != nil {
		// No usable environment: fall back to local validation only.
		body, prepErr := anomalydetector.NewHandler(nil).PrepareCreateBody(jsonData)
		if prepErr != nil {
			return prepErr
		}
		printDryRunAnomalyDetector(body)
		output.PrintWarning("schema validation skipped: %v", err)
		return nil
	}

	handler := anomalydetector.NewHandler(c).WithDefaultActor(currentActor(c))
	body, err := handler.PrepareCreateBody(jsonData)
	if err != nil {
		return err
	}
	printDryRunAnomalyDetector(body)

	var unavailable *anomalydetector.ValidationUnavailableError
	switch err := handler.ValidateCreate(jsonData); {
	case err == nil:
		output.PrintSuccess("Schema validation passed")
	case errors.As(err, &unavailable):
		output.PrintWarning("schema validation skipped: %v", unavailable.Err)
	default:
		return err
	}
	return nil
}

// printDryRunAnomalyDetector shows the request body, including the schema
// defaults dtctl fills in, so the dry run reflects what would actually be sent.
func printDryRunAnomalyDetector(body map[string]any) {
	output.PrintInfo("Dry run: would create anomaly detector")
	output.PrintInfo("---")
	rendered, err := json.Marshal(body)
	if err != nil {
		output.PrintInfo("%v", body)
	} else {
		output.PrintInfo("%s", rendered)
	}
	output.PrintInfo("---")
}

func init() {
	createAnomalyDetectorCmd.Flags().StringP("file", "f", "", "file containing anomaly detector definition (required)")
	createAnomalyDetectorCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createAnomalyDetectorCmd.MarkFlagRequired("file")
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(createAnomalyDetectorCmd)
}
