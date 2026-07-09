package cmd

import "github.com/spf13/cobra"

// activateCmd is the parent command for activation operations.
var activateCmd = &cobra.Command{
	Use:   "activate",
	Short: "Activate a resource version",
	Long: `Activate a specific version of a Dynatrace resource.

Available resources:
  extension (ext)     Activate an Extensions 2.0 extension version`,
	Example: `  # Activate extension version 1.0.2
  dtctl activate extension custom:my-extension --version 1.0.2`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(activateCmd)
	activateCmd.AddCommand(activateExtensionCmd)
}
