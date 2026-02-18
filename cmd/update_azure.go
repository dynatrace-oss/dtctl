package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var (
	updateAzureConnectionName          string
	updateAzureConnectionDirectoryID   string
	updateAzureConnectionApplicationID string
	updateCloudConnectionProvider      string

	updateAzureMonitoringConfigName              string
	updateAzureMonitoringConfigLocationFiltering string
	updateAzureMonitoringConfigFeatureSets       string
	updateCloudMonitoringConfigProvider          string
)

var updateAzureConnectionCmd = &cobra.Command{
	Use:   "cloud_connection [id]",
	Aliases: []string{"azure_connection"},
	Short: "Update Azure connection from flags",
	Long: `Update Azure connection by ID argument or by --name.

Examples:
  dtctl update cloud_connection --provider azure --name "siwek" --directoryId "XYZ" --applicationId "ZUZ"
  dtctl update cloud_connection --provider azure <id> --directoryId "XYZ" --applicationId "ZUZ"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(updateCloudConnectionProvider); err != nil {
			return err
		}

		if updateAzureConnectionDirectoryID == "" && updateAzureConnectionApplicationID == "" {
			return fmt.Errorf("at least one of --directoryId or --applicationId is required")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

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

		handler := azureconnection.NewHandler(c)

		var existing *azureconnection.AzureConnection
		if len(args) > 0 {
			existing, err = handler.Get(args[0])
			if err != nil {
				return err
			}
		} else {
			if updateAzureConnectionName == "" {
				return fmt.Errorf("provide connection ID argument or --name")
			}
			existing, err = handler.FindByName(updateAzureConnectionName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		switch value.Type {
		case "federatedIdentityCredential":
			if value.FederatedIdentityCredential == nil {
				value.FederatedIdentityCredential = &azureconnection.FederatedIdentityCredential{}
			}
			if updateAzureConnectionDirectoryID != "" {
				value.FederatedIdentityCredential.DirectoryID = updateAzureConnectionDirectoryID
			}
			if updateAzureConnectionApplicationID != "" {
				value.FederatedIdentityCredential.ApplicationID = updateAzureConnectionApplicationID
			}
		case "clientSecret":
			if value.ClientSecret == nil {
				value.ClientSecret = &azureconnection.ClientSecretCredential{}
			}
			if updateAzureConnectionDirectoryID != "" {
				value.ClientSecret.DirectoryID = updateAzureConnectionDirectoryID
			}
			if updateAzureConnectionApplicationID != "" {
				value.ClientSecret.ApplicationID = updateAzureConnectionApplicationID
			}
		default:
			return fmt.Errorf("unsupported azure connection type %q", value.Type)
		}

		updated, err := handler.Update(existing.ObjectID, value)
		if err != nil {
			return err
		}

		fmt.Printf("Azure connection updated: %s\n", updated.ObjectID)
		return nil
	},
}

var updateAzureMonitoringConfigCmd = &cobra.Command{
	Use:   "cloud_monitoring_config [id]",
	Aliases: []string{"azure_monitoring_config"},
	Short: "Update Azure monitoring config from flags",
	Long: `Update Azure monitoring configuration by ID argument or by --name.

Examples:
  dtctl update cloud_monitoring_config --provider azure --name "siwek" --locationFiltering "eastus,westeurope"
  dtctl update cloud_monitoring_config --provider azure --name "siwek" --featureSets "microsoft_compute.virtualmachines_essential,microsoft_web.sites_functionapp_essential"
  dtctl update cloud_monitoring_config --provider azure <id> --locationFiltering "eastus,westeurope"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(updateCloudMonitoringConfigProvider); err != nil {
			return err
		}

		if strings.TrimSpace(updateAzureMonitoringConfigLocationFiltering) == "" &&
			strings.TrimSpace(updateAzureMonitoringConfigFeatureSets) == "" {
			return fmt.Errorf("at least one of --locationFiltering or --featureSets is required")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

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

		handler := azuremonitoringconfig.NewHandler(c)

		var existing *azuremonitoringconfig.AzureMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]
			existing, err = handler.FindByName(identifier)
			if err != nil {
				existing, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
			}
		} else {
			if updateAzureMonitoringConfigName == "" {
				return fmt.Errorf("provide config ID argument or --name")
			}
			existing, err = handler.FindByName(updateAzureMonitoringConfigName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		if strings.TrimSpace(updateAzureMonitoringConfigLocationFiltering) != "" {
			locations := azuremonitoringconfig.SplitCSV(updateAzureMonitoringConfigLocationFiltering)
			if len(locations) == 0 {
				return fmt.Errorf("--locationFiltering must contain at least one location")
			}
			value.Azure.LocationFiltering = locations
		}
		if strings.TrimSpace(updateAzureMonitoringConfigFeatureSets) != "" {
			featureSets := azuremonitoringconfig.SplitCSV(updateAzureMonitoringConfigFeatureSets)
			if len(featureSets) == 0 {
				return fmt.Errorf("--featureSets must contain at least one feature set")
			}
			value.FeatureSets = featureSets
		}

		payload := azuremonitoringconfig.AzureMonitoringConfig{Scope: existing.Scope, Value: value}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := handler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		fmt.Printf("Azure monitoring config updated: %s\n", updated.ObjectID)
		return nil
	},
}

func init() {
	updateCmd.AddCommand(updateAzureConnectionCmd)
	updateCmd.AddCommand(updateAzureMonitoringConfigCmd)

	addRequiredProviderFlagVar(updateAzureConnectionCmd, &updateCloudConnectionProvider)
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionName, "name", "", "Azure connection name (used when ID argument is not provided)")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionDirectoryID, "directoryId", "", "Directory ID to set")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionDirectoryID, "directoryID", "", "Alias for --directoryId")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionApplicationID, "applicationId", "", "Application ID to set")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionApplicationID, "applicationID", "", "Alias for --applicationId")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionApplicationID, "aplicationID", "", "Compatibility alias for typo --aplicationID")

	addRequiredProviderFlagVar(updateAzureMonitoringConfigCmd, &updateCloudMonitoringConfigProvider)
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigLocationFiltering, "locationFiltering", "", "Comma-separated locations")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
}
