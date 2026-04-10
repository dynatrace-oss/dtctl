package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var (
	activateAzureMonitoringName          string
	activateAzureMonitoringDirectoryID   string
	activateAzureMonitoringApplicationID string
)

var activateAzureProviderCmd = &cobra.Command{
	Use:   "azure",
	Short: "Activate Azure resources",
	RunE:  requireSubcommand,
}

var activateAzureMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Activate Azure monitoring configuration",
	Long: `Activate an Azure monitoring configuration by optionally updating the connection
credentials and enabling the monitoring config in a single step.

If --directoryId and/or --applicationId are provided, the linked Azure connection
will be updated with the specified credentials before enabling the monitoring config.

Examples:
  dtctl activate azure monitoring --name "my-azure-monitoring" --directoryId "$TENANT_ID" --applicationId "$CLIENT_ID"
  dtctl activate azure monitoring <id> --directoryId "$TENANT_ID" --applicationId "$CLIENT_ID"
  dtctl activate azure monitoring --name "my-azure-monitoring"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		monitoringHandler := azuremonitoringconfig.NewHandler(c)
		connectionHandler := azureconnection.NewHandler(c)

		// Resolve monitoring config by ID arg or --name flag
		var existing *azuremonitoringconfig.AzureMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]
			existing, err = monitoringHandler.FindByName(identifier)
			if err != nil {
				existing, err = monitoringHandler.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
			}
		} else {
			if activateAzureMonitoringName == "" {
				return fmt.Errorf("provide config ID argument or --name")
			}
			existing, err = monitoringHandler.FindByName(activateAzureMonitoringName)
			if err != nil {
				return err
			}
		}

		// Step 1: Update connection credentials if directoryId or applicationId provided
		if activateAzureMonitoringDirectoryID != "" || activateAzureMonitoringApplicationID != "" {
			if len(existing.Value.Azure.Credentials) == 0 {
				return fmt.Errorf("monitoring config %q has no credentials configured", existing.ObjectID)
			}

			connectionID := existing.Value.Azure.Credentials[0].ConnectionId
			conn, err := connectionHandler.Get(connectionID)
			if err != nil {
				return fmt.Errorf("failed to get linked connection %q: %w", connectionID, err)
			}

			value := conn.Value
			switch value.Type {
			case "federatedIdentityCredential":
				if value.FederatedIdentityCredential == nil {
					value.FederatedIdentityCredential = &azureconnection.FederatedIdentityCredential{}
				}
				if activateAzureMonitoringDirectoryID != "" {
					value.FederatedIdentityCredential.DirectoryID = activateAzureMonitoringDirectoryID
				}
				if activateAzureMonitoringApplicationID != "" {
					value.FederatedIdentityCredential.ApplicationID = activateAzureMonitoringApplicationID
				}
			case "clientSecret":
				if value.ClientSecret == nil {
					value.ClientSecret = &azureconnection.ClientSecretCredential{}
				}
				if activateAzureMonitoringDirectoryID != "" {
					value.ClientSecret.DirectoryID = activateAzureMonitoringDirectoryID
				}
				if activateAzureMonitoringApplicationID != "" {
					value.ClientSecret.ApplicationID = activateAzureMonitoringApplicationID
				}
			default:
				return fmt.Errorf("unsupported azure connection type %q", value.Type)
			}

			_, err = connectionHandler.Update(conn.ObjectID, value)
			if err != nil {
				return fmt.Errorf("failed to update connection credentials: %w", err)
			}

			output.PrintSuccess("Azure connection %q updated with credentials", connectionID)
		}

		// Step 2: Enable monitoring config and credentials
		value := existing.Value
		value.Enabled = true
		for i := range value.Azure.Credentials {
			value.Azure.Credentials[i].Enabled = true
		}

		payload := azuremonitoringconfig.AzureMonitoringConfig{Scope: existing.Scope, Value: value}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := monitoringHandler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		output.PrintSuccess("Azure monitoring config activated: %s", updated.ObjectID)
		return nil
	},
}

func init() {
	activateCmd.AddCommand(activateAzureProviderCmd)

	activateAzureProviderCmd.AddCommand(activateAzureMonitoringCmd)

	activateAzureMonitoringCmd.Flags().StringVar(&activateAzureMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	activateAzureMonitoringCmd.Flags().StringVar(&activateAzureMonitoringDirectoryID, "directoryId", "", "Directory (tenant) ID to set on the linked connection (optional)")
	activateAzureMonitoringCmd.Flags().StringVar(&activateAzureMonitoringDirectoryID, "directoryID", "", "Alias for --directoryId")
	activateAzureMonitoringCmd.Flags().StringVar(&activateAzureMonitoringApplicationID, "applicationId", "", "Application (client) ID to set on the linked connection (optional)")
	activateAzureMonitoringCmd.Flags().StringVar(&activateAzureMonitoringApplicationID, "applicationID", "", "Alias for --applicationId")
}
