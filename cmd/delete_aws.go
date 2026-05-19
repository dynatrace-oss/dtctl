package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var deleteAWSConnectionCmd = &cobra.Command{
	Use:     "connection [ID|NAME]",
	Short:   "Delete an AWS connection",
	Aliases: []string{"connections"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		_, c, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := awsconnection.NewHandler(c)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			output.PrintInfo("Resolved name %q to ID %s", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete AWS connection %q: %w", objectID, err)
		}

		output.PrintSuccess("AWS connection %s deleted", objectID)
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

		_, c, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := awsmonitoringconfig.NewHandler(c)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			output.PrintInfo("Resolved name %q to ID %s", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete AWS monitoring config %q: %w", objectID, err)
		}

		output.PrintSuccess("AWS monitoring config %s deleted", objectID)
		return nil
	},
}

func init() {
	deleteAWSProviderCmd.AddCommand(deleteAWSConnectionCmd)
	deleteAWSProviderCmd.AddCommand(deleteAWSMonitoringConfigCmd)
}
