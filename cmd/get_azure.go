package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/spf13/cobra"
)

var getAzureProviderCmd = &cobra.Command{
	Use:   "azure",
	Short: "Get Azure resources",
	RunE:  requireSubcommand,
}

var getAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Get AWS resources",
	RunE:  requireSubcommand,
}

var getGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Get GCP resources",
	RunE:  requireSubcommand,
}

// getAzureConnectionCmd retrieves Azure connections (formerly HAS credentials)
var getAzureConnectionCmd = &cobra.Command{
	Use:     "connections [id]",
	Aliases: []string{"connection"},
	Short:   "Get Azure connections",
	Long:    `Get one or more Azure connections (authentication credentials).`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config", "monitoring-configs"},
	Short:   "Get Azure monitoring configurations",
	Long:    `Get one or more Azure monitoring configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	Use:     "monitoring-locations",
	Aliases: []string{"monitoring-location"},
	Short:   "Get available Azure monitoring config locations",
	Long:    `Get available Azure regions for Azure monitoring configuration based on the latest extension schema.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	Use:     "monitoring-feature-sets",
	Aliases: []string{"monitoring-feature-set"},
	Short:   "Get available Azure monitoring config feature sets",
	Long:    `Get available FeatureSetsType values for Azure monitoring configuration based on the latest extension schema.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	getCmd.AddCommand(getAzureProviderCmd)
	getCmd.AddCommand(getAWSProviderCmd)
	getCmd.AddCommand(getGCPProviderCmd)

	getAzureProviderCmd.AddCommand(getAzureConnectionCmd)
	getAzureProviderCmd.AddCommand(getAzureMonitoringConfigCmd)
	getAzureProviderCmd.AddCommand(getAzureMonitoringConfigLocationsCmd)
	getAzureProviderCmd.AddCommand(getAzureMonitoringConfigFeatureSetsCmd)

	getAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "connections"))
	getAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "monitoring"))
	getGCPProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("gcp", "connections"))
	getGCPProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("gcp", "monitoring"))
}
