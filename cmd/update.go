package cmd

import "github.com/spf13/cobra"

// updateCmd represents the update command.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update resources",
	Long: `Update existing resources on the Dynatrace platform using flags.

Unlike 'apply' which works from YAML/JSON files, 'update' modifies individual
fields of a resource using command-line flags. This is useful for quick,
targeted changes without exporting and re-importing a full resource definition.

Available resources:
  breakpoint              Update breakpoint condition/enabled state or workspace filters
  azure connection        Update Azure connection credentials
  azure monitoring        Update Azure monitoring configuration
  gcp connection          Update GCP connection credentials (Preview)
  gcp monitoring          Update GCP monitoring configuration (Preview)`,
	Example: `  # Update an Azure connection
  dtctl update azure connection <id> --name "New Name"

  # Update Live Debugger workspace filters
  dtctl update breakpoint --filters k8s.namespace.name:prod

  # Update an Azure monitoring config
  dtctl update azure monitoring <id> --enabled=false

  # Update a GCP connection
  dtctl update gcp connection <id> --project-id my-project`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
