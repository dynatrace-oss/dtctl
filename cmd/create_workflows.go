package cmd

import (
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// createWorkflowCmd creates a workflow from a file
var createWorkflowCmd = &cobra.Command{
	Use:   "workflow -f <file>",
	Short: "Create a workflow from a file",
	Long: `Create a new workflow from a YAML or JSON file.

Examples:
  # Create a workflow from YAML
  dtctl create workflow -f workflow.yaml

  # Create with template variables
  dtctl create workflow -f workflow.yaml --set env=prod --set owner=team-a

  # Dry run to preview
  dtctl create workflow -f workflow.yaml --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		setFlags, _ := cmd.Flags().GetStringArray("set")

		// Read the file
		fileData, err := os.ReadFile(file)
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
			fmt.Println("Dry run: would create workflow")
			fmt.Println("---")
			fmt.Println(string(jsonData))
			fmt.Println("---")
			return nil
		}

		// Load configuration
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create workflow: %w", err)
		}

		fmt.Println("Workflow created successfully")
		fmt.Printf("  ID:   %s\n", result.ID)
		fmt.Printf("  Name: %s\n", result.Title)
		fmt.Printf("  URL:  %s/ui/apps/dynatrace.automations/workflows/%s\n", c.BaseURL(), result.ID)
		return nil
	},
}

func init() {
	// Workflow flags
	createWorkflowCmd.Flags().StringP("file", "f", "", "file containing workflow definition (required)")
	createWorkflowCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createWorkflowCmd.MarkFlagRequired("file")
}
