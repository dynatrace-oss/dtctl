package cmd

import (
	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create resources from files",
	Long: `Create a new resource on the Dynatrace platform from a YAML or JSON file.

Reads a resource definition from a file and creates it. If the resource already
exists, the command fails — use 'dtctl apply' for create-or-update semantics.

For most workflows, 'dtctl apply -f <file>' is preferred over 'create' because
apply is idempotent (creates if new, updates if existing).

Supported resources:
  workflows (wf)          dashboards (dash, db)     notebooks (nb)
  slos                    settings                  buckets (bkt)
  edgeconnect (ec)        lookup-tables (lu)`,
	Example: `  # Create a workflow from a YAML file
  dtctl create workflow -f workflow.yaml

  # Create a dashboard from JSON
  dtctl create dashboard -f dashboard.json

  # Create a settings object
  dtctl create settings -f settings.yaml

  # Preview what would be created
  dtctl create workflow -f workflow.yaml --dry-run`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.AddCommand(createWorkflowCmd)
	createCmd.AddCommand(createNotebookCmd)
	createCmd.AddCommand(createDashboardCmd)
	createCmd.AddCommand(createDocumentCmd)
	createCmd.AddCommand(createSettingsCmd)
	createCmd.AddCommand(createSLOCmd)
	createCmd.AddCommand(createBucketCmd)
	createCmd.AddCommand(createLookupCmd)
	createCmd.AddCommand(createEdgeConnectCmd)
	createCmd.AddCommand(createBreakpointCmd)
}
