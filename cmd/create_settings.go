package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// createSettingsCmd creates a settings object from a file
var createSettingsCmd = &cobra.Command{
	Use:   "settings -f <file> --schema <schema-id> --scope <scope>",
	Short: "Create a settings object from a file",
	Long: `Create a new settings object from a YAML or JSON file.

Examples:
  # Create a settings object
  dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment

  # Create with template variables
  dtctl create settings -f settings.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --set name=prod

  # Dry run to preview
  dtctl create settings -f settings.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --dry-run
`,
	Aliases: []string{"setting"},
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		schemaID, _ := cmd.Flags().GetString("schema")
		scope, _ := cmd.Flags().GetString("scope")
		setFlags, _ := cmd.Flags().GetStringArray("set")

		if file == "" {
			return fmt.Errorf("--file is required")
		}
		if schemaID == "" {
			return fmt.Errorf("--schema is required")
		}
		if scope == "" {
			return fmt.Errorf("--scope is required")
		}

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

		// Parse the value
		var value map[string]any
		if err := json.Unmarshal(jsonData, &value); err != nil {
			return fmt.Errorf("failed to parse settings value: %w", err)
		}

		// Handle dry-run
		if dryRun {
			fmt.Printf("Dry run: would create settings object\n")
			fmt.Printf("Schema: %s\n", schemaID)
			fmt.Printf("Scope: %s\n", scope)
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

		handler := settings.NewHandler(c)

		result, err := handler.Create(settings.SettingsObjectCreate{
			SchemaID: schemaID,
			Scope:    scope,
			Value:    value,
		})
		if err != nil {
			return fmt.Errorf("failed to create settings object: %w", err)
		}

		fmt.Printf("Settings object %q created successfully\n", result.ObjectID)
		return nil
	},
}

func init() {
	// Settings flags
	createSettingsCmd.Flags().StringP("file", "f", "", "file containing settings value (required)")
	createSettingsCmd.Flags().String("schema", "", "schema ID (required)")
	createSettingsCmd.Flags().String("scope", "", "scope for the settings object (required)")
	createSettingsCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createSettingsCmd.MarkFlagRequired("file")
	_ = createSettingsCmd.MarkFlagRequired("schema")
	_ = createSettingsCmd.MarkFlagRequired("scope")
}
