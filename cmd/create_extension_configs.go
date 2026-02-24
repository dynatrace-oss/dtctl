package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/util/format"
	"github.com/dynatrace-oss/dtctl/pkg/util/template"
	"github.com/spf13/cobra"
)

// createExtensionConfigCmd creates a monitoring configuration for an extension
var createExtensionConfigCmd = &cobra.Command{
	Use:     "extension-config <extension-name> -f <file>",
	Aliases: []string{"ext-config"},
	Short:   "Create a monitoring configuration for an extension",
	Long: `Create a new monitoring configuration for an Extensions 2.0 extension from a YAML or JSON file.

Examples:
  # Create a monitoring configuration
  dtctl create extension-config com.dynatrace.extension.host-monitoring -f config.yaml

  # Create with a specific scope
  dtctl create extension-config com.dynatrace.extension.host-monitoring -f config.yaml --scope HOST-1234

  # Create with template variables
  dtctl create extension-config com.dynatrace.extension.host-monitoring -f config.yaml --set env=prod

  # Dry run to preview
  dtctl create extension-config com.dynatrace.extension.host-monitoring -f config.yaml --dry-run
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extensionName := args[0]
		file, _ := cmd.Flags().GetString("file")
		scope, _ := cmd.Flags().GetString("scope")
		setFlags, _ := cmd.Flags().GetStringArray("set")

		if file == "" {
			return fmt.Errorf("--file is required")
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

		// Parse the configuration
		var config extension.MonitoringConfigurationCreate
		if err := json.Unmarshal(jsonData, &config); err != nil {
			return fmt.Errorf("failed to parse configuration: %w", err)
		}

		// Handle dry-run
		if dryRun {
			fmt.Println("Dry run: would create extension monitoring configuration")
			fmt.Printf("Extension: %s\n", extensionName)
			if scope != "" {
				fmt.Printf("Scope:     %s\n", scope)
			}
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

		handler := extension.NewHandler(c)

		result, err := handler.CreateMonitoringConfiguration(extensionName, config)
		if err != nil {
			return fmt.Errorf("failed to create monitoring configuration: %w", err)
		}

		fmt.Printf("Monitoring configuration %q created successfully for extension %q\n", result.ObjectID, extensionName)
		return nil
	},
}

func init() {
	createExtensionConfigCmd.Flags().StringP("file", "f", "", "file containing monitoring configuration value (required)")
	createExtensionConfigCmd.Flags().String("scope", "", "scope for the monitoring configuration (e.g. HOST-1234)")
	createExtensionConfigCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = createExtensionConfigCmd.MarkFlagRequired("file")
}
