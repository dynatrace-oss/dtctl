package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/schedulingrule"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// createSchedulingRuleCmd creates a scheduling rule from a file
var createSchedulingRuleCmd = &cobra.Command{
	Use:     "scheduling-rule -f <file>",
	Aliases: []string{"scheduling-rules", "sr"},
	Short:   "Create a scheduling rule from a file",
	Long: `Create a new scheduling rule from a YAML or JSON file.

Examples:
  # Create a scheduling rule from YAML
  dtctl create scheduling-rule -f rule.yaml

  # Create with template variables
  dtctl create scheduling-rule -f rule.yaml --set freq=WEEKLY

  # Dry run to preview
  dtctl create scheduling-rule -f rule.yaml --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("--file is required")
		}

		setFlags, _ := cmd.Flags().GetStringArray("set")

		fileData, err := vfs.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		jsonData, err := format.ValidateAndConvert(fileData)
		if err != nil {
			return fmt.Errorf("invalid file format: %w", err)
		}

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

		if dryRun {
			fmt.Printf("Dry run: would create scheduling rule\n")
			fmt.Println("---")
			fmt.Println(string(jsonData))
			fmt.Println("---")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		handler := schedulingrule.NewHandler(c)

		result, err := handler.Create(jsonData)
		if err != nil {
			return fmt.Errorf("failed to create scheduling rule: %w", err)
		}

		output.PrintSuccess("Scheduling rule %q created", result.Title)
		output.PrintInfo("  ID:    %s", result.ID)
		output.PrintInfo("  Title: %s", result.Title)
		return nil
	},
}

func init() {
	createSchedulingRuleCmd.Flags().StringP("file", "f", "", "file containing scheduling rule definition (required)")
	createSchedulingRuleCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createSchedulingRuleCmd.MarkFlagRequired("file")
}
