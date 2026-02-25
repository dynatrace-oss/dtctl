package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var deleteAWSConnectionCmd = &cobra.Command{
	Use:     "connection [ID|NAME]",
	Short:   "Delete an AWS connection",
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

		handler := awsconnection.NewHandler(client)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			fmt.Printf("Resolved name %q to ID %s\n", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete AWS connection %q: %w", objectID, err)
		}

		fmt.Printf("Deleted AWS connection %s\n", objectID)
		return nil
	},
}

var deleteAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [ID|NAME]",
	Short:   "Delete an AWS monitoring config",
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

		handler := awsmonitoringconfig.NewHandler(client)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			fmt.Printf("Resolved name %q to ID %s\n", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete AWS monitoring config %q: %w", objectID, err)
		}

		fmt.Printf("Deleted AWS monitoring config %s\n", objectID)
		return nil
	},
}

func init() {
	deleteAWSProviderCmd.AddCommand(deleteAWSConnectionCmd)
	deleteAWSProviderCmd.AddCommand(deleteAWSMonitoringConfigCmd)
}
