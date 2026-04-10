package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var (
	activateGCPMonitoringName             string
	activateGCPMonitoringServiceAccountID string
)

var activateGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Activate GCP resources (Preview)",
	RunE:  requireSubcommand,
}

var activateGCPMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Activate GCP monitoring configuration",
	Long: `Activate a GCP monitoring configuration by optionally updating the connection
credentials and enabling the monitoring config in a single step.

If --serviceAccountId is provided, the linked GCP connection will be updated
with the specified service account before enabling the monitoring config.

Examples:
  dtctl activate gcp monitoring --name "my-gcp-monitoring" --serviceAccountId "sa@project.iam.gserviceaccount.com"
  dtctl activate gcp monitoring <id> --serviceAccountId "sa@project.iam.gserviceaccount.com"
  dtctl activate gcp monitoring --name "my-gcp-monitoring"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		monitoringHandler := gcpmonitoringconfig.NewHandler(c)
		connectionHandler := gcpconnection.NewHandler(c)

		// Resolve monitoring config by ID arg or --name flag
		var existing *gcpmonitoringconfig.GCPMonitoringConfig
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
			if activateGCPMonitoringName == "" {
				return fmt.Errorf("provide config ID argument or --name")
			}
			existing, err = monitoringHandler.FindByName(activateGCPMonitoringName)
			if err != nil {
				return err
			}
		}

		// Step 1: Update connection credentials if --serviceAccountId provided
		if activateGCPMonitoringServiceAccountID != "" {
			if len(existing.Value.GoogleCloud.Credentials) == 0 {
				return fmt.Errorf("monitoring config %q has no credentials configured", existing.ObjectID)
			}

			connectionID := existing.Value.GoogleCloud.Credentials[0].ConnectionID
			conn, err := connectionHandler.Get(connectionID)
			if err != nil {
				return fmt.Errorf("failed to get linked connection %q: %w", connectionID, err)
			}

			value := conn.Value
			if value.Type == "" {
				value.Type = "serviceAccountImpersonation"
			}
			if value.ServiceAccountImpersonation == nil {
				value.ServiceAccountImpersonation = &gcpconnection.ServiceAccountImpersonation{
					Consumers: []string{"SVC:com.dynatrace.da"},
				}
			}
			if len(value.ServiceAccountImpersonation.Consumers) == 0 {
				value.ServiceAccountImpersonation.Consumers = []string{"SVC:com.dynatrace.da"}
			}
			value.ServiceAccountImpersonation.ServiceAccountID = activateGCPMonitoringServiceAccountID

			_, err = connectionHandler.Update(conn.ObjectID, value)
			if err != nil {
				if strings.Contains(err.Error(), "GCP authentication failed") {
					return fmt.Errorf("%w\nIAM Policy update can take a couple of minutes before it becomes active, please retry in a moment", err)
				}
				return fmt.Errorf("failed to update connection credentials: %w", err)
			}

			output.PrintSuccess("GCP connection %q updated with service account", connectionID)
		}

		// Step 2: Enable monitoring config and credentials
		value := existing.Value
		value.Enabled = true
		for i := range value.GoogleCloud.Credentials {
			value.GoogleCloud.Credentials[i].Enabled = true
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

		output.PrintSuccess("GCP monitoring config activated: %s", updated.ObjectID)
		return nil
	},
}

func init() {
	activateGCPProviderCmd.AddCommand(activateGCPMonitoringCmd)

	activateGCPMonitoringCmd.Flags().StringVar(&activateGCPMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	activateGCPMonitoringCmd.Flags().StringVar(&activateGCPMonitoringServiceAccountID, "serviceAccountId", "", "Service account email to set on the linked connection (optional)")
	activateGCPMonitoringCmd.Flags().StringVar(&activateGCPMonitoringServiceAccountID, "serviceaccountid", "", "Alias for --serviceAccountId")
}
