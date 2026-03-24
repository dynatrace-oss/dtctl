package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
)

// extensionDescription is a rich struct for JSON/YAML output of describe extension
type extensionDescription struct {
	Name                string                        `json:"name" yaml:"name"`
	Version             string                        `json:"version" yaml:"version"`
	Author              string                        `json:"author,omitempty" yaml:"author,omitempty"`
	MinDynatraceVersion string                        `json:"minDynatraceVersion,omitempty" yaml:"minDynatraceVersion,omitempty"`
	MinEECVersion       string                        `json:"minEECVersion,omitempty" yaml:"minEECVersion,omitempty"`
	FileHash            string                        `json:"fileHash,omitempty" yaml:"fileHash,omitempty"`
	DataSources         []string                      `json:"dataSources,omitempty" yaml:"dataSources,omitempty"`
	FeatureSets         map[string][]string           `json:"featureSets,omitempty" yaml:"featureSets,omitempty"`
	Variables           []extension.ExtensionVariable `json:"variables,omitempty" yaml:"variables,omitempty"`
	ActiveVersion       string                        `json:"activeVersion,omitempty" yaml:"activeVersion,omitempty"`
	AvailableVersions   []string                      `json:"availableVersions,omitempty" yaml:"availableVersions,omitempty"`
	MonitoringConfigs   []monitoringConfigSummary     `json:"monitoringConfigurations,omitempty" yaml:"monitoringConfigurations,omitempty"`
}

type monitoringConfigSummary struct {
	ObjectID    string `json:"objectId" yaml:"objectId"`
	Scope       string `json:"scope,omitempty" yaml:"scope,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

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

		// Get environment config (active version)
		var activeVersion string
		envConfig, envErr := handler.GetEnvironmentConfig(extensionName)
		if envErr == nil && envConfig.Version != "" {
			activeVersion = envConfig.Version
		}

		// Get monitoring configurations summary
		var configSummaries []monitoringConfigSummary
		configs, configErr := handler.ListMonitoringConfigurations(extensionName, "", 0)
		if configErr == nil {
			for _, cfg := range configs.Items {
				summary := monitoringConfigSummary{
					ObjectID: cfg.ObjectID,
					Scope:    cfg.Scope,
				}
				if cfg.Value != nil {
					var val map[string]interface{}
					if err := json.Unmarshal(cfg.Value, &val); err == nil {
						if enabled, ok := val["enabled"].(bool); ok {
							summary.Enabled = &enabled
						}
						if desc, ok := val["description"].(string); ok && desc != "" {
							summary.Description = desc
						}
					}
				}
				configSummaries = append(configSummaries, summary)
			}
		}

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 16
			output.DescribeKV("Name:", w, "%s", details.ExtensionName)
			output.DescribeKV("Version:", w, "%s", details.Version)

			if details.Author.Name != "" {
				output.DescribeKV("Author:", w, "%s", details.Author.Name)
			}
			if details.MinDynatraceVersion != "" {
				output.DescribeKV("Min Dynatrace:", w, "%s", details.MinDynatraceVersion)
			}
			if details.MinEECVersion != "" {
				output.DescribeKV("Min EEC:", w, "%s", details.MinEECVersion)
			}
			if details.FileHash != "" {
				output.DescribeKV("File Hash:", w, "%s", details.FileHash)
			}
			if len(details.DataSources) > 0 {
				fmt.Println()
				output.DescribeKV("Data Sources:", w, "%s", strings.Join(details.DataSources, ", "))
			}
			if len(details.FeatureSets) > 0 {
				fmt.Println()
				output.DescribeSection("Feature Sets:")
				for _, fs := range details.FeatureSets {
					fmt.Printf("  - %s\n", fs)
					if detail, ok := details.FeatureSetDetails[fs]; ok && len(detail.Metrics) > 0 {
						for _, m := range detail.Metrics {
							fmt.Printf("      %s\n", m.Key)
						}
					}
				}
			}
			if len(details.Variables) > 0 {
				fmt.Println()
				output.DescribeSection("Variables:")
				for _, v := range details.Variables {
					displayName := v.Name
					if v.DisplayName != "" {
						displayName = v.DisplayName
					}
					fmt.Printf("  - %s (%s)\n", displayName, v.Type)
				}
			}
			if activeVersion != "" {
				output.DescribeKV("Active Version:", w, "%s", activeVersion)
			}
			if len(versions.Items) > 0 {
				fmt.Println()
				output.DescribeSection("Available Versions:")
				for _, v := range versions.Items {
					marker := "  "
					if activeVersion == v.Version {
						marker = "* "
					}
					fmt.Printf("  %s%s\n", marker, v.Version)
				}
			}
			if len(configSummaries) > 0 {
				fmt.Println()
				output.DescribeSection(fmt.Sprintf("Monitoring Configurations: %d", len(configSummaries)))
				for _, cfg := range configSummaries {
					scope := cfg.Scope
					if scope == "" {
						scope = "(environment)"
					}
					fmt.Printf("  - %s  scope=%s\n", cfg.ObjectID, scope)
					if cfg.Enabled != nil {
						fmt.Printf("    enabled: %v\n", *cfg.Enabled)
					}
					if cfg.Description != "" {
						fmt.Printf("    description: %v\n", cfg.Description)
					}
				}
			}
			return nil
		}

		// For other formats (JSON, YAML, etc.), use the printer
		featureSets := make(map[string][]string)
		for _, fs := range details.FeatureSets {
			var metrics []string
			if detail, ok := details.FeatureSetDetails[fs]; ok {
				for _, m := range detail.Metrics {
					metrics = append(metrics, m.Key)
				}
			}
			featureSets[fs] = metrics
		}

		var availableVersions []string
		for _, v := range versions.Items {
			availableVersions = append(availableVersions, v.Version)
		}

		desc := &extensionDescription{
			Name:                details.ExtensionName,
			Version:             details.Version,
			Author:              details.Author.Name,
			MinDynatraceVersion: details.MinDynatraceVersion,
			MinEECVersion:       details.MinEECVersion,
			FileHash:            details.FileHash,
			DataSources:         details.DataSources,
			FeatureSets:         featureSets,
			Variables:           details.Variables,
			ActiveVersion:       activeVersion,
			AvailableVersions:   availableVersions,
			MonitoringConfigs:   configSummaries,
		}

		printer := NewPrinter()
		enrichAgent(printer, "describe", "extension")
		return printer.Print(desc)
	},
}

func init() {
	describeExtensionCmd.Flags().String("version", "", "Show details for a specific extension version")
}
