package cmd

import (
	"encoding/json"
	"fmt"

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
	Long: `Enable a GCP monitoring configuration and optionally populate the service account
on the linked connection's monitoring credential.

If --serviceAccountId is provided, dtctl will:
  1. Fetch the linked GCP connection and verify the service account matches
  2. Populate the (possibly empty) serviceAccount field in the monitoring credential
  3. Enable the monitoring config and all credentials

If the serviceAccount on the connection differs from --serviceAccountId, the command
will error and suggest updating the connection first with 'dtctl update gcp connection'.

If --serviceAccountId is omitted, only the enabled state is toggled (no credential validation).

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
				output.PrintInfo("Dry run: would validate service account %q against linked GCP connection and populate empty credential field", enableGCPMonitoringServiceAccountID)
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

		// Step 1: Populate serviceAccount in monitoring credentials if --serviceAccountId provided
		if enableGCPMonitoringServiceAccountID != "" {
			if len(existing.Value.GoogleCloud.Credentials) == 0 {
				return fmt.Errorf("monitoring config %q has no credentials configured", configName)
			}
			if len(existing.Value.GoogleCloud.Credentials) > 1 {
				output.PrintWarning("monitoring config %q has %d credentials — only the first connection will be validated; use 'dtctl update gcp connection' for others",
					configName, len(existing.Value.GoogleCloud.Credentials))
			}

			connectionID := existing.Value.GoogleCloud.Credentials[0].ConnectionID
			output.PrintInfo("Validating service account against GCP connection %q...", connectionID)

			conn, err := connectionHandler.Get(connectionID)
			if err != nil {
				return fmt.Errorf("failed to get linked connection %q: %w", connectionID, err)
			}

			connSA := ""
			if conn.Value.ServiceAccountImpersonation != nil {
				connSA = conn.Value.ServiceAccountImpersonation.ServiceAccountID
			}

			if connSA != "" && enableGCPMonitoringServiceAccountID != connSA {
				return fmt.Errorf(
					"serviceAccountId %q does not match the service account on linked connection %q (%q)\n"+
						"Update the connection first with: dtctl update gcp connection --name %q --serviceAccountId %q",
					enableGCPMonitoringServiceAccountID, connectionID, connSA,
					conn.Value.Name, enableGCPMonitoringServiceAccountID,
				)
			}
		}

		// Step 2: Enable monitoring config and all credentials
		output.PrintInfo("Enabling GCP monitoring config %q...", configName)
		value := existing.Value
		value.Enabled = true
		for i := range value.GoogleCloud.Credentials {
			value.GoogleCloud.Credentials[i].Enabled = true
		}
		// Populate serviceAccount only on the first credential — the only one validated above
		if enableGCPMonitoringServiceAccountID != "" && len(value.GoogleCloud.Credentials) > 0 && value.GoogleCloud.Credentials[0].ServiceAccount == "" {
			value.GoogleCloud.Credentials[0].ServiceAccount = enableGCPMonitoringServiceAccountID
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
