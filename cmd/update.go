package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

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

// updateSettingsHintCmd redirects users to 'apply' for file-based settings updates.
var updateSettingsHintCmd = &cobra.Command{
	Use:     "settings",
	Aliases: []string{"setting"},
	Short:   "Update a settings object (use 'apply' instead)",
	Hidden:  true,
	// Accept any args/flags so the command doesn't fail before RunE.
	Args:               cobra.ArbitraryArgs,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("to update settings objects from a file, use 'dtctl apply -f <file>' instead\n\n" +
			"The file should include objectId, schemaId, scope, and value fields.\n" +
			"If the objectId exists it will be updated; otherwise a new object is created.\n\n" +
			"Example:\n" +
			"  dtctl apply -f settings.yaml\n" +
			"  dtctl apply -f settings.yaml --dry-run")
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.AddCommand(updateSettingsHintCmd)
}
