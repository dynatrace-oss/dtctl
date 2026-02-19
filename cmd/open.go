package cmd

import (
	"github.com/spf13/cobra"
)

// openCmd represents the open command
var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open resources in browser",
	Long:  `Open Dynatrace resources in a web browser.`,
	RunE:  requireSubcommand,
}

func init() {
	rootCmd.AddCommand(openCmd)
}
