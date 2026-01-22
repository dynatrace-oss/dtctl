package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/version"
	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit, and build date of dtctl.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dtctl version %s\n", version.Version)
		fmt.Printf("commit: %s\n", version.Commit)
		fmt.Printf("built: %s\n", version.Date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
