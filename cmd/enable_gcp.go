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
	enableGCPMonitoringName             string
	enableGCPMonitoringServiceAccountID string
)

var enableGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Enable GCP resources (Preview)",
	RunE:  requireSubcommand,
}

var enableGCPMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Enable GCP monitoring configuration",
	Long: `Enable a GCP monitoring configuration by optionally updating the linked connection
credentials and then enabling the monitoring config in a single step.

If --serviceAccountId is provided, the linked GCP connection will be updated
with the specified service account before enabling the monitoring config.
If the connection credentials are already set, --serviceAccountId can be omitted.

Examples:
  dtctl enable gcp monitoring --name "my-gcp-monitoring" --serviceAccountId "sa@project.iam.gserviceaccount.com"
  dtctl enable gcp monitoring <id> --serviceAccountId "sa@project.iam.gserviceaccount.com"
  dtctl enable gcp monitoring --name "my-gcp-monitoring"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Early flag validation — before any auth/network calls
		if len(args) == 0 && enableGCPMonitoringName == "" {
			return fmt.Errorf("provide monitoring config ID argument or --name")
		}

		if dryRun {
			name := enableGCPMonitoringName
			if len(args) > 0 {
				name = args[0]
			}
			output.PrintInfo("Dry run: would resolve GCP monitoring config %q", name)
			if enableGCPMonitoringServiceAccountID != "" {
				output.PrintInfo("Dry run: would update linked GCP connection with service account %q", enableGCPMonitoringServiceAccountID)
			}
			output.PrintInfo("Dry run: would enable monitoring config and all credentials")
			return nil
		}

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
					return fmt.Errorf("GCP monitoring config %q not found by name or ID", identifier)
				}
			}
		} else {
			existing, err = monitoringHandler.FindByName(enableGCPMonitoringName)
			if err != nil {
				return err
			}
		}

		configName := existing.Value.Description
		if configName == "" {
			configName = existing.ObjectID
		}

		// Step 1: Update connection credentials if --serviceAccountId provided
		if enableGCPMonitoringServiceAccountID != "" {
			if len(existing.Value.GoogleCloud.Credentials) == 0 {
				return fmt.Errorf("monitoring config %q has no credentials configured", configName)
			}
			if len(existing.Value.GoogleCloud.Credentials) > 1 {
				output.PrintWarning("monitoring config %q has %d credentials — only the first connection will be updated; use 'dtctl update gcp connection' for the others",
					configName, len(existing.Value.GoogleCloud.Credentials))
			}

			connectionID := existing.Value.GoogleCloud.Credentials[0].ConnectionID
			output.PrintInfo("Updating GCP connection %q with service account...", connectionID)

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
			value.ServiceAccountImpersonation.ServiceAccountID = enableGCPMonitoringServiceAccountID

			_, err = connectionHandler.Update(conn.ObjectID, value)
			if err != nil {
				if strings.Contains(err.Error(), "GCP authentication failed") {
					return fmt.Errorf("%w\nIAM Policy update can take a couple of minutes before it becomes active, please retry in a moment", err)
				}
				return fmt.Errorf("failed to update connection credentials: %w", err)
			}
			output.PrintSuccess("GCP connection %q updated", connectionID)
		}

		// Step 2: Enable monitoring config and all credentials
		output.PrintInfo("Enabling GCP monitoring config %q...", configName)
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

		output.PrintSuccess("GCP monitoring config %q enabled (%s)", configName, updated.ObjectID)
		return nil
	},
}

func init() {
	enableGCPProviderCmd.AddCommand(enableGCPMonitoringCmd)

	enableGCPMonitoringCmd.Flags().StringVar(&enableGCPMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	enableGCPMonitoringCmd.Flags().StringVar(&enableGCPMonitoringServiceAccountID, "serviceAccountId", "", "Service account email to set on the linked connection (optional)")
}
