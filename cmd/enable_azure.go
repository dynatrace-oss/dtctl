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
	enableAzureMonitoringName          string
	enableAzureMonitoringDirectoryID   string
	enableAzureMonitoringApplicationID string
)

var enableAzureProviderCmd = &cobra.Command{
	Use:   "azure",
	Short: "Enable Azure resources",
	RunE:  requireSubcommand,
}

var enableAzureMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Enable Azure monitoring configuration",
	Long: `Enable an Azure monitoring configuration by optionally updating the linked connection
credentials and then enabling the monitoring config in a single step.

If --directoryId and/or --applicationId are provided, the linked Azure connection
will be updated with the specified credentials before enabling the monitoring config.
If the connection credentials are already set, these flags can be omitted.

Examples:
  dtctl enable azure monitoring --name "my-azure-monitoring" --directoryId "$TENANT_ID" --applicationId "$CLIENT_ID"
  dtctl enable azure monitoring <id> --directoryId "$TENANT_ID" --applicationId "$CLIENT_ID"
  dtctl enable azure monitoring --name "my-azure-monitoring"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Early flag validation — before any auth/network calls
		if len(args) == 0 && enableAzureMonitoringName == "" {
			return fmt.Errorf("provide monitoring config ID argument or --name")
		}

		if dryRun {
			name := enableAzureMonitoringName
			if len(args) > 0 {
				name = args[0]
			}
			output.PrintInfo("Dry run: would resolve Azure monitoring config %q", name)
			if enableAzureMonitoringDirectoryID != "" || enableAzureMonitoringApplicationID != "" {
				msg := "Dry run: would update linked Azure connection"
				if enableAzureMonitoringDirectoryID != "" {
					msg += fmt.Sprintf(" directoryId=%q", enableAzureMonitoringDirectoryID)
				}
				if enableAzureMonitoringApplicationID != "" {
					msg += fmt.Sprintf(" applicationId=%q", enableAzureMonitoringApplicationID)
				}
				output.PrintInfo(msg)
			}
			output.PrintInfo("Dry run: would enable monitoring config and all credentials")
			return nil
		}

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
					return fmt.Errorf("azure monitoring config %q not found by name or ID", identifier)
				}
			}
		} else {
			existing, err = monitoringHandler.FindByName(enableAzureMonitoringName)
			if err != nil {
				return err
			}
		}

		configName := existing.Value.Description
		if configName == "" {
			configName = existing.ObjectID
		}

		// Step 1: Update connection credentials if directoryId or applicationId provided
		if enableAzureMonitoringDirectoryID != "" || enableAzureMonitoringApplicationID != "" {
			if len(existing.Value.Azure.Credentials) == 0 {
				return fmt.Errorf("monitoring config %q has no credentials configured", configName)
			}
			if len(existing.Value.Azure.Credentials) > 1 {
				output.PrintWarning("monitoring config %q has %d credentials — only the first connection will be updated; use 'dtctl update azure connection' for the others",
					configName, len(existing.Value.Azure.Credentials))
			}

			connectionID := existing.Value.Azure.Credentials[0].ConnectionId
			output.PrintInfo("Updating Azure connection %q with credentials...", connectionID)

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
				if enableAzureMonitoringDirectoryID != "" {
					value.FederatedIdentityCredential.DirectoryID = enableAzureMonitoringDirectoryID
				}
				if enableAzureMonitoringApplicationID != "" {
					value.FederatedIdentityCredential.ApplicationID = enableAzureMonitoringApplicationID
				}
			case "clientSecret":
				if value.ClientSecret == nil {
					value.ClientSecret = &azureconnection.ClientSecretCredential{}
				}
				if enableAzureMonitoringDirectoryID != "" {
					value.ClientSecret.DirectoryID = enableAzureMonitoringDirectoryID
				}
				if enableAzureMonitoringApplicationID != "" {
					value.ClientSecret.ApplicationID = enableAzureMonitoringApplicationID
				}
			default:
				return fmt.Errorf("unsupported azure connection type %q", value.Type)
			}

			_, err = connectionHandler.Update(conn.ObjectID, value)
			if err != nil {
				return fmt.Errorf("failed to update connection credentials: %w", err)
			}
			output.PrintSuccess("Azure connection %q updated", connectionID)
		}

		// Step 2: Enable monitoring config and all credentials
		output.PrintInfo("Enabling Azure monitoring config %q...", configName)
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

		output.PrintSuccess("Azure monitoring config %q enabled (%s)", configName, updated.ObjectID)
		return nil
	},
}

func init() {
	enableCmd.AddCommand(enableAzureProviderCmd)

	enableAzureProviderCmd.AddCommand(enableAzureMonitoringCmd)

	enableAzureMonitoringCmd.Flags().StringVar(&enableAzureMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	enableAzureMonitoringCmd.Flags().StringVar(&enableAzureMonitoringDirectoryID, "directoryId", "", "Directory (tenant) ID to set on the linked connection (optional)")
	enableAzureMonitoringCmd.Flags().StringVar(&enableAzureMonitoringApplicationID, "applicationId", "", "Application (client) ID to set on the linked connection (optional)")
}
