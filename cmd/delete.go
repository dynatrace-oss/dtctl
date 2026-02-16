package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources",
	Long:  `Delete resources by ID or name.`,
	RunE:  requireSubcommand,
}

var deleteAzureConnectionCmd = &cobra.Command{
	Use:     "azure_connection [ID|NAME]",
	Short:   "Delete an Azure connection",
	Aliases: []string{"azure_connections"},
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
	Use:     "azure_monitoring_config [ID|NAME]",
	Short:   "Delete an Azure monitoring config",
	Aliases: []string{"azure_monitoring_configs"},
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
	deleteCmd.AddCommand(deleteAzureConnectionCmd)
	deleteCmd.AddCommand(deleteAzureMonitoringConfigCmd)
}
