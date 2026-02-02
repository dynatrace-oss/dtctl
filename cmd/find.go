package cmd

import (
	"github.com/spf13/cobra"
)

// findCmd represents the find command
var findCmd = &cobra.Command{
	Use:   "find",
	Short: "Find resources based on criteria",
	Long:  `Find Dynatrace resources that match specific criteria.`,
	RunE:  requireSubcommand,
}

func init() {
	rootCmd.AddCommand(findCmd)
	findCmd.AddCommand(findIntentsCmd)
}
