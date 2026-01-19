package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "0.7.3"
	commit  = "unknown"
	date    = "unknown"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit, and build date of dtctl.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dtctl version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built: %s\n", date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
