package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

var deleteGCPConnectionCmd = &cobra.Command{
	Use:     "connection [ID|NAME]",
	Short:   "Delete a GCP connection",
	Aliases: []string{"connections"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		_, client, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := gcpconnection.NewHandler(client)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			output.PrintInfo("Resolved name %q to ID %s", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete GCP connection %q: %w", objectID, err)
		}

		output.PrintSuccess("GCP connection %s deleted", objectID)
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

		_, client, err := SetupWithSafety(safety.OperationDelete)
		if err != nil {
			return err
		}

		handler := gcpmonitoringconfig.NewHandler(client)

		objectID := identifier
		item, err := handler.FindByName(identifier)
		if err == nil {
			objectID = item.ObjectID
			output.PrintInfo("Resolved name %q to ID %s", identifier, objectID)
		}

		if err := handler.Delete(objectID); err != nil {
			return fmt.Errorf("failed to delete GCP monitoring config %q: %w", objectID, err)
		}

		output.PrintSuccess("GCP monitoring config %s deleted", objectID)
		return nil
	},
}

func init() {
	deleteGCPProviderCmd.AddCommand(deleteGCPConnectionCmd)
	deleteGCPProviderCmd.AddCommand(deleteGCPMonitoringConfigCmd)
}

// Declared stable: the invocation and output contract of these commands is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(deleteGCPConnectionCmd)
	stability.MarkStable(deleteGCPMonitoringConfigCmd)
}
