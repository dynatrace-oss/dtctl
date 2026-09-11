package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// createSLOCmd creates an SLO from a file
var createSLOCmd = &cobra.Command{
	Use:   "slo -f <file>",
	Short: "Create a service-level objective from a file",
	Long: `Create a new SLO from a YAML or JSON file.

Examples:
  # Create an SLO from YAML
  dtctl create slo -f slo.yaml

  # Create with template variables
  dtctl create slo -f slo.yaml --set target=99.9

  # Dry run to preview
  dtctl create slo -f slo.yaml --dry-run
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
			fmt.Printf("Dry run: would create SLO\n")
			fmt.Println("---")
			fmt.Println(string(jsonData))
			fmt.Println("---")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		handler := slo.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create SLO: %w", err)
		}

		output.PrintSuccess("SLO %q created", result.Name)
		output.PrintInfo("  ID:   %s", result.ID)
		output.PrintInfo("  Name: %s", result.Name)
		output.PrintInfo("  URL:  %s/ui/apps/dynatrace.site.reliability/slos/%s", c.BaseURL(), result.ID)
		return nil
	},
}

func init() {
	// SLO flags
	createSLOCmd.Flags().StringP("file", "f", "", "file containing SLO definition (required)")
	createSLOCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createSLOCmd.MarkFlagRequired("file")
}
