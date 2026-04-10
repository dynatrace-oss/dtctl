package cmd

import "github.com/spf13/cobra"

var activateCmd = &cobra.Command{
	Use:   "activate",
	Short: "Activate cloud monitoring configurations",
	Long: `Activate a cloud monitoring configuration by updating the connection credentials
and enabling the monitoring config in a single step.

This is a convenience command that combines two operations:
  1. Updates the cloud connection with authentication details (service account, directory/app ID)
  2. Enables the monitoring configuration and its credentials

Available resources:
  gcp monitoring          Activate GCP monitoring configuration (Preview)
  azure monitoring        Activate Azure monitoring configuration`,
	Example: `  # Activate GCP monitoring with service account
  dtctl activate gcp monitoring --name "my-gcp-monitoring" --serviceAccountId "sa@project.iam.gserviceaccount.com"

  # Activate Azure monitoring with federated identity
  dtctl activate azure monitoring --name "my-azure-monitoring" --directoryId "$TENANT_ID" --applicationId "$CLIENT_ID"

  # Activate monitoring without updating connection credentials
  dtctl activate gcp monitoring --name "my-gcp-monitoring"`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(activateCmd)
}
