package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var disableGCPMonitoringName string

var disableGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Disable GCP resources (Preview)",
	RunE:  requireSubcommand,
}

var disableGCPMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Disable GCP monitoring configuration",
	Long: `Disable a GCP monitoring configuration by setting it and all its credentials
to disabled in a single step.

The monitoring configuration and linked connection are preserved — only the
enabled flag is toggled off.

Examples:
  dtctl disable gcp monitoring --name "my-gcp-monitoring"
  dtctl disable gcp monitoring <id>`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && disableGCPMonitoringName == "" {
			return fmt.Errorf("provide monitoring config ID argument or --name")
		}

		if dryRun {
			name := disableGCPMonitoringName
			if len(args) > 0 {
				name = args[0]
			}
			output.PrintInfo("Dry run: would resolve GCP monitoring config %q", name)
			output.PrintInfo("Dry run: would disable monitoring config and all credentials")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		monitoringHandler := gcpmonitoringconfig.NewHandler(c)

		var existing *gcpmonitoringconfig.GCPMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]
			existing, err = monitoringHandler.FindByName(identifier)
			if err != nil {
				existing, err = monitoringHandler.Get(identifier)
				if err != nil {
					return fmt.Errorf("GCP monitoring config %q not found by name or ID", identifier)
				}
			}
		} else {
			existing, err = monitoringHandler.FindByName(disableGCPMonitoringName)
			if err != nil {
				return err
			}
		}

		configName := existing.Value.Description
		if configName == "" {
			configName = existing.ObjectID
		}

		output.PrintInfo("Disabling GCP monitoring config %q...", configName)
		value := existing.Value
		value.Enabled = false
		for i := range value.GoogleCloud.Credentials {
			value.GoogleCloud.Credentials[i].Enabled = false
		}

		payload := gcpmonitoringconfig.GCPMonitoringConfig{Scope: existing.Scope, Value: value}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := monitoringHandler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		output.PrintSuccess("GCP monitoring config %q disabled (%s)", configName, updated.ObjectID)
		return nil
	},
}

func init() {
	disableGCPProviderCmd.AddCommand(disableGCPMonitoringCmd)

	disableGCPMonitoringCmd.Flags().StringVar(&disableGCPMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
}
