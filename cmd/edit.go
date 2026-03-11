package cmd

import (
	"github.com/spf13/cobra"
)

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit a resource",
	Long: `Edit a resource interactively using your default editor ($EDITOR).

Fetches the current resource definition, opens it in your editor as YAML, and
applies the changes when the file is saved and closed. If the editor exits
without changes or with a non-zero exit code, the update is cancelled.

The editor is determined by the EDITOR environment variable (defaults to vi).
Blocked by safety level if the current context is set to 'readonly'.

Supported resources:
  workflows (wf)          dashboards (dash, db)     notebooks (nb)
  settings`,
	Example: `  # Edit a workflow in your default editor
  dtctl edit workflow my-workflow

  # Edit a dashboard by name
  dtctl edit dashboard "My Dashboard"

  # Edit a settings object by ID
  dtctl edit setting <object-id>`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(editCmd)

	editCmd.AddCommand(editWorkflowCmd)
	editCmd.AddCommand(editDashboardCmd)
	editCmd.AddCommand(editNotebookCmd)
	editCmd.AddCommand(editDocumentCmd)
	editCmd.AddCommand(editSettingCmd)
}
