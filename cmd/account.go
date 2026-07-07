package cmd

import "github.com/spf13/cobra"

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Platform account administration",
	Long:  "Commands for managing Dynatrace platform account resources.",
	RunE:  requireSubcommand,
}

func init() {
	rootCmd.AddCommand(accountCmd)
}
