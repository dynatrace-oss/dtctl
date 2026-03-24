package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
)

// describeExtensionConfigCmd shows detailed info about an extension monitoring configuration
var describeExtensionConfigCmd = &cobra.Command{
	Use:     "extension-config <extension-name> --config-id <config-id>",
	Aliases: []string{"ext-config"},
	Short:   "Show details of an extension monitoring configuration",
	Long: `Show detailed information about an Extensions 2.0 monitoring configuration
including scope, enabled status, version, feature sets, and full value.

Examples:
  # Describe a specific monitoring configuration
  dtctl describe extension-config com.dynatrace.extension.host-monitoring --config-id <config-id>
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extensionName := args[0]
		configID, _ := cmd.Flags().GetString("config-id")

		if configID == "" {
			return fmt.Errorf("--config-id is required")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := extension.NewHandler(c)

		config, err := handler.GetMonitoringConfiguration(extensionName, configID)
		if err != nil {
			return err
		}

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 13
			output.DescribeKV("Extension:", w, "%s", extensionName)
			output.DescribeKV("Config ID:", w, "%s", config.ObjectID)
			if config.Scope != "" {
				output.DescribeKV("Scope:", w, "%s", config.Scope)
			}

			if len(config.Value) > 0 {
				var val map[string]interface{}
				if err := json.Unmarshal(config.Value, &val); err == nil {
					if enabled, ok := val["enabled"]; ok {
						output.DescribeKV("Enabled:", w, "%v", enabled)
					}
					if desc, ok := val["description"]; ok && desc != "" {
						output.DescribeKV("Description:", w, "%s", desc)
					}
					if version, ok := val["version"]; ok && version != "" {
						output.DescribeKV("Version:", w, "%s", version)
					}
					if fs, ok := val["featureSets"].([]interface{}); ok && len(fs) > 0 {
						fmt.Println()
						output.DescribeSection("Feature Sets:")
						for _, f := range fs {
							fmt.Printf("  - %v\n", f)
						}
					}
				}

				fmt.Println()
				output.DescribeSection("Value:")
				valueJSON, err := json.MarshalIndent(json.RawMessage(config.Value), "  ", "  ")
				if err == nil {
					fmt.Printf("  %s\n", string(valueJSON))
				}
			}
			return nil
		}

		// For other formats (JSON, YAML, etc.), use the printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "extension-config")
		return printer.Print(config)
	},
}

func init() {
	describeExtensionConfigCmd.Flags().String("config-id", "", "Monitoring configuration ID (required)")
	_ = describeExtensionConfigCmd.MarkFlagRequired("config-id")
}
