package cmd

import "github.com/spf13/cobra"

// updateCmd represents the update command.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update resources",
	Long:  `Update resources from files or flags.`,
	RunE:  requireSubcommand,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
