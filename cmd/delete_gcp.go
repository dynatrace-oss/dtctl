package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var deleteGCPConnectionCmd = &cobra.Command{
	Use:     "connection [ID|NAME]",
	Short:   "Delete a GCP connection",
	Aliases: []string{"connections"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

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

		handler := gcpconnection.NewHandler(client)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			fmt.Printf("Resolved name %q to ID %s\n", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete GCP connection %q: %w", objectID, err)
		}

		fmt.Printf("Deleted GCP connection %s\n", objectID)
		return nil
	},
}

var deleteGCPMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [ID|NAME]",
	Short:   "Delete a GCP monitoring config",
	Aliases: []string{"monitoring-config", "monitoring-configs"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

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

		handler := gcpmonitoringconfig.NewHandler(client)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			fmt.Printf("Resolved name %q to ID %s\n", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete GCP monitoring config %q: %w", objectID, err)
		}

		fmt.Printf("Deleted GCP monitoring config %s\n", objectID)
		return nil
	},
}

func init() {
	deleteGCPProviderCmd.AddCommand(deleteGCPConnectionCmd)
	deleteGCPProviderCmd.AddCommand(deleteGCPMonitoringConfigCmd)
}
