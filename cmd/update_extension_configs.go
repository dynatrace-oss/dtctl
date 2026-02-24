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

// updateExtensionConfigCmd updates a monitoring configuration for an extension
var updateExtensionConfigCmd = &cobra.Command{
	Use:     "extension-config <extension-name> -f <file> [--config-id <config-id>]",
	Aliases: []string{"ext-config"},
	Short:   "Update a monitoring configuration for an extension",
	Long: `Update an existing monitoring configuration for an Extensions 2.0 extension from a YAML or JSON file.

The config ID can be provided via --config-id flag or read from the .objectId field in the file.

Examples:
  # Update using objectId from the file
  dtctl update extension-config com.dynatrace.extension.host-monitoring -f config.yaml

  # Update with explicit config ID
  dtctl update extension-config com.dynatrace.extension.host-monitoring --config-id abc123 -f config.yaml

  # Update with template variables
  dtctl update extension-config com.dynatrace.extension.host-monitoring -f config.yaml --set env=prod

  # Dry run to preview
  dtctl update extension-config com.dynatrace.extension.host-monitoring -f config.yaml --dry-run
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extensionName := args[0]
		configID, _ := cmd.Flags().GetString("config-id")
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

		// Extract objectId from file if --config-id not provided
		if configID == "" {
			var raw map[string]any
			if err := json.Unmarshal(jsonData, &raw); err == nil {
				if id, ok := raw["objectId"].(string); ok && id != "" {
					configID = id
				}
			}
		}
		if configID == "" {
			return fmt.Errorf("--config-id is required when the file does not contain an objectId field")
		}

		// Handle dry-run
		if dryRun {
			fmt.Println("Dry run: would update extension monitoring configuration")
			fmt.Printf("Extension: %s\n", extensionName)
			fmt.Printf("Config ID: %s\n", configID)
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
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := extension.NewHandler(c)

		result, err := handler.UpdateMonitoringConfiguration(extensionName, configID, config)
		if err != nil {
			return fmt.Errorf("failed to update monitoring configuration: %w", err)
		}

		fmt.Printf("Monitoring configuration %q updated successfully for extension %q\n", result.ObjectID, extensionName)
		return nil
	},
}

func init() {
	updateExtensionConfigCmd.Flags().String("config-id", "", "monitoring configuration ID to update (optional if objectId is in the file)")
	updateExtensionConfigCmd.Flags().StringP("file", "f", "", "file containing monitoring configuration value (required)")
	updateExtensionConfigCmd.Flags().String("scope", "", "scope for the monitoring configuration (e.g. HOST-1234)")
	updateExtensionConfigCmd.Flags().StringArray("set", []string{}, "set template variable (key=value)")
	_ = updateExtensionConfigCmd.MarkFlagRequired("file")
}
