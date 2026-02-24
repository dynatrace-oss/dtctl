package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/spf13/cobra"
)

// describeExtensionCmd shows detailed info about an extension
var describeExtensionCmd = &cobra.Command{
	Use:     "extension <extension-name>",
	Aliases: []string{"ext"},
	Short:   "Show details of an Extensions 2.0 extension",
	Long: `Show detailed information about an Extensions 2.0 extension including versions,
data sources, feature sets, and environment configuration.

Examples:
  # Describe an extension (shows active version details)
  dtctl describe extension com.dynatrace.extension.host-monitoring

  # Describe a specific version
  dtctl describe extension com.dynatrace.extension.host-monitoring --version 1.2.3
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extensionName := args[0]
		versionFlag, _ := cmd.Flags().GetString("version")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := extension.NewHandler(c)

		// Determine which version to describe
		targetVersion := versionFlag
		if targetVersion == "" {
			// Try to get active version from environment configuration
			envConfig, err := handler.GetEnvironmentConfig(extensionName)
			if err == nil && envConfig.Version != "" {
				targetVersion = envConfig.Version
			}
		}

		// List all available versions
		versions, err := handler.Get(extensionName)
		if err != nil {
			return err
		}

		// If no target version, use the latest version from the list
		if targetVersion == "" && len(versions.Items) > 0 {
			targetVersion = versions.Items[0].Version
		}

		if targetVersion == "" {
			return fmt.Errorf("no versions found for extension %q", extensionName)
		}

		// Get detailed information for the target version
		details, err := handler.GetVersion(extensionName, targetVersion)
		if err != nil {
			return err
		}

		// Print extension details
		fmt.Printf("Name:           %s\n", details.ExtensionName)
		fmt.Printf("Version:        %s\n", details.Version)

		if details.Author.Name != "" {
			fmt.Printf("Author:         %s\n", details.Author.Name)
		}
		if details.MinDynatraceVersion != "" {
			fmt.Printf("Min Dynatrace:  %s\n", details.MinDynatraceVersion)
		}
		if details.MinEECVersion != "" {
			fmt.Printf("Min EEC:        %s\n", details.MinEECVersion)
		}
		if details.FileHash != "" {
			fmt.Printf("File Hash:      %s\n", details.FileHash)
		}
		if len(details.DataSources) > 0 {
			fmt.Println()
			fmt.Printf("Data Sources:   %s\n", strings.Join(details.DataSources, ", "))
		}
		// Print feature sets
		if len(details.FeatureSets) > 0 {
			fmt.Println()
			fmt.Println("Feature Sets:")
			for _, fs := range details.FeatureSets {
				fmt.Printf("  - %s\n", fs)
				if detail, ok := details.FeatureSetDetails[fs]; ok && len(detail.Metrics) > 0 {
					for _, m := range detail.Metrics {
						fmt.Printf("      %s\n", m.Key)
					}
				}
			}
		}
		// Print variables
		if len(details.Variables) > 0 {
			fmt.Println()
			fmt.Println("Variables:")
			for _, v := range details.Variables {
				displayName := v.Name
				if v.DisplayName != "" {
					displayName = v.DisplayName
				}
				fmt.Printf("  - %s (%s)\n", displayName, v.Type)
			}
		}

		// Print environment config (active version)
		envConfig, envErr := handler.GetEnvironmentConfig(extensionName)
		if envErr == nil && envConfig.Version != "" {
			fmt.Printf("Active Version: %s\n", envConfig.Version)
		}

		// Print available versions
		if len(versions.Items) > 0 {
			fmt.Println()
			fmt.Println("Available Versions:")
			for _, v := range versions.Items {
				marker := "  "
				if envErr == nil && envConfig.Version == v.Version {
					marker = "* "
				}
				fmt.Printf("  %s%s\n", marker, v.Version)
			}
		}

		// Print monitoring configurations summary
		configs, configErr := handler.ListMonitoringConfigurations(extensionName, "", 0)
		if configErr == nil && len(configs.Items) > 0 {
			fmt.Println()
			fmt.Printf("Monitoring Configurations: %d\n", configs.TotalCount)
			for _, cfg := range configs.Items {
				scope := cfg.Scope
				if scope == "" {
					scope = "(environment)"
				}
				fmt.Printf("  - %s  scope=%s\n", cfg.ObjectID, scope)

				// Show a summary of the config value
				if cfg.Value != nil {
					var val map[string]interface{}
					if err := json.Unmarshal(cfg.Value, &val); err == nil {
						if enabled, ok := val["enabled"]; ok {
							fmt.Printf("    enabled: %v\n", enabled)
						}
						if desc, ok := val["description"]; ok && desc != "" {
							fmt.Printf("    description: %v\n", desc)
						}
					}
				}
			}
		}

		return nil
	},
}

func init() {
	describeExtensionCmd.Flags().String("version", "", "Show details for a specific extension version")
}
