package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/spf13/cobra"
)

var (
	getCloudConnectionProvider         string
	getCloudMonitoringConfigProvider   string
	getCloudMonitoringSchemaProvider   string
)

// getAzureConnectionCmd retrieves Azure connections (formerly HAS credentials)
var getAzureConnectionCmd = &cobra.Command{
	Use:     "cloud_connection [id]",
	Aliases: []string{"cloud_connections", "azure_connection", "azure_connections"},
	Short:   "Get Azure connections",
	Long:    `Get one or more Azure connections (authentication credentials).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(getCloudConnectionProvider); err != nil {
			return err
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := azureconnection.NewHandler(c)
		printer := NewPrinter()

		if len(args) > 0 {
			identifier := args[0]

			item, err := handler.FindByName(identifier)
			if err == nil {
				return printer.Print(item)
			}

			if strings.Contains(err.Error(), "not found") {
				item, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("connection with name or ID %q not found", identifier)
				}
				return printer.Print(item)
			}
			return err
		}

		items, err := handler.List()
		if err != nil {
			return err
		}
		return printer.PrintList(items)
	},
}

// getAzureMonitoringConfigCmd retrieves Azure monitoring configurations
var getAzureMonitoringConfigCmd = &cobra.Command{
	Use:     "cloud_monitoring_config [id]",
	Aliases: []string{"cloud_monitoring_configs", "azure_monitoring_config", "azure_monitoring_configs"},
	Short:   "Get Azure monitoring configurations",
	Long:    `Get one or more Azure monitoring configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(getCloudMonitoringConfigProvider); err != nil {
			return err
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := azuremonitoringconfig.NewHandler(c)
		printer := NewPrinter()

		if len(args) > 0 {
			identifier := args[0]

			item, err := handler.FindByName(identifier)
			if err == nil {
				return printer.Print(item)
			}

			if strings.Contains(err.Error(), "not found") {
				item, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
				return printer.Print(item)
			}
			return err
		}

		items, err := handler.List()
		if err != nil {
			return err
		}
		return printer.PrintList(items)
	},
}

// getAzureMonitoringConfigLocationsCmd retrieves available Azure monitoring config locations from extension schema
var getAzureMonitoringConfigLocationsCmd = &cobra.Command{
	Use:     "cloud_monitoring_config_locations",
	Aliases: []string{"azure_monitoring_config_locations", "azure_monitoring_config_location", "azure_monitoring_locations"},
	Short:   "Get available Azure monitoring config locations",
	Long:    `Get available Azure regions for Azure monitoring configuration based on the latest extension schema.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(getCloudMonitoringSchemaProvider); err != nil {
			return err
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := azuremonitoringconfig.NewHandler(c)
		printer := NewPrinter()

		locations, err := handler.ListAvailableLocations()
		if err != nil {
			return err
		}

		return printer.PrintList(locations)
	},
}

// getAzureMonitoringConfigFeatureSetsCmd retrieves available Azure monitoring config feature sets from extension schema
var getAzureMonitoringConfigFeatureSetsCmd = &cobra.Command{
	Use:     "cloud_monitoring_config_feature_sets",
	Aliases: []string{"azure_monitoring_config_feature_sets", "azure_monitoring_config_feature_set", "azure_monitoring_feature_sets"},
	Short:   "Get available Azure monitoring config feature sets",
	Long:    `Get available FeatureSetsType values for Azure monitoring configuration based on the latest extension schema.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(getCloudMonitoringSchemaProvider); err != nil {
			return err
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := azuremonitoringconfig.NewHandler(c)
		printer := NewPrinter()

		featureSets, err := handler.ListAvailableFeatureSets()
		if err != nil {
			return err
		}

		return printer.PrintList(featureSets)
	},
}

func init() {
	addRequiredProviderFlagVar(getAzureConnectionCmd, &getCloudConnectionProvider)
	addRequiredProviderFlagVar(getAzureMonitoringConfigCmd, &getCloudMonitoringConfigProvider)
	addRequiredProviderFlagVar(getAzureMonitoringConfigLocationsCmd, &getCloudMonitoringSchemaProvider)
	addRequiredProviderFlagVar(getAzureMonitoringConfigFeatureSetsCmd, &getCloudMonitoringSchemaProvider)
}
