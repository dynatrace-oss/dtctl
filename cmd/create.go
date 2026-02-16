package cmd

import (
	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create resources from files",
	Long:  `Create resources from YAML or JSON files.`,
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.AddCommand(createWorkflowCmd)
	createCmd.AddCommand(createNotebookCmd)
	createCmd.AddCommand(createDashboardCmd)
	createCmd.AddCommand(createSettingsCmd)
	createCmd.AddCommand(createSLOCmd)
	createCmd.AddCommand(createBucketCmd)
	createCmd.AddCommand(createLookupCmd)
	createCmd.AddCommand(createEdgeConnectCmd)
}
