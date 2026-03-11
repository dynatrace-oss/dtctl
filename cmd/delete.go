package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources",
	Long: `Delete a resource from the Dynatrace platform by ID or name.

Accepts either a resource ID or name. When a name is provided, dtctl resolves
it to an ID automatically. Blocked by safety level if the current context is
set to 'readonly'.

Some resources (dashboards, notebooks) may be recoverable from trash after
deletion — use 'dtctl get trash' to check.

Supported resources:
  workflows (wf)          dashboards (dash, db)     notebooks (nb)
  slos                    settings                  buckets (bkt)
  apps                    edgeconnect (ec)           notifications
  lookup-tables (lu)      trash
  azure connection        azure monitoring`,
	Example: `  # Delete a workflow by ID
  dtctl delete workflow abc-123

  # Delete a dashboard by name
  dtctl delete dashboard "My Dashboard"

  # Delete with dry-run to preview what would be deleted
  dtctl delete workflow abc-123 --dry-run

  # Permanently remove a trashed document
  dtctl delete trash <document-id>`,
	RunE: requireSubcommand,
}

var deleteAzureProviderCmd = &cobra.Command{
	Use:   "azure",
	Short: "Delete Azure resources",
	RunE:  requireSubcommand,
}

var deleteAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Delete AWS resources",
	RunE:  requireSubcommand,
}

var deleteGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Delete GCP resources (Preview)",
	RunE:  requireSubcommand,
}

var deleteAzureConnectionCmd = &cobra.Command{
	Use:     "connection [ID|NAME]",
	Short:   "Delete an Azure connection",
	Aliases: []string{"connections"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		client, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := azureconnection.NewHandler(client)

		objectID := identifier

		// Try to find by name first to resolve ID if it's a name
		item, err := handler.FindByName(identifier)
		if err == nil {
			// Found by name
			objectID = item.ObjectID
			fmt.Printf("Resolved name %q to ID %s\n", identifier, objectID)
		}
		// If not found by name, assume identifier is an ID

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete Azure connection %q: %w", objectID, err)
		}

		fmt.Printf("Deleted Azure connection %s\n", objectID)
		return nil
	},
}

var deleteAzureMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [ID|NAME]",
	Short:   "Delete an Azure monitoring config",
	Aliases: []string{"monitoring-config", "monitoring-configs"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown); err != nil {
			return err
		}

		client, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := azuremonitoringconfig.NewHandler(client)

		objectID := identifier

		// Try to find by description first to resolve ID if it's a name
		item, err := handler.FindByName(identifier)
		if err == nil {
			// Found by name
			objectID = item.ObjectID
			fmt.Printf("Resolved name %q to ID %s\n", identifier, objectID)
		}
		// If not found by name, assume identifier is an ID

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete Azure monitoring config %q: %w", objectID, err)
		}

		fmt.Printf("Deleted Azure monitoring config %s\n", objectID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.AddCommand(deleteAzureProviderCmd)
	deleteCmd.AddCommand(deleteAWSProviderCmd)
	deleteCmd.AddCommand(deleteGCPProviderCmd)
	attachPreviewNotice(deleteGCPProviderCmd, "GCP")

	deleteAzureProviderCmd.AddCommand(deleteAzureConnectionCmd)
	deleteAzureProviderCmd.AddCommand(deleteAzureMonitoringConfigCmd)
	deleteAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "connection"))
	deleteAWSProviderCmd.AddCommand(newNotImplementedProviderResourceCommand("aws", "monitoring"))
}
