package cmd

import (
	"github.com/spf13/cobra"
)

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit a resource",
	Long:  `Edit a resource using the default editor.`,
	RunE:  requireSubcommand,
}

func init() {
	rootCmd.AddCommand(editCmd)

	editCmd.AddCommand(editWorkflowCmd)
	editCmd.AddCommand(editDashboardCmd)
	editCmd.AddCommand(editNotebookCmd)
	editCmd.AddCommand(editSettingCmd)
}
