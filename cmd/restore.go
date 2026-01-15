package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// restoreCmd represents the restore command
var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore resources to a previous version",
	Long:  `Restore resources like workflows, notebooks, and dashboards to a previous version.`,
	RunE:  requireSubcommand,
}

// restoreWorkflowCmd restores a workflow to a specific version
var restoreWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id-or-name> <version>",
	Aliases: []string{"workflows", "wf"},
	Short:   "Restore a workflow to a previous version",
	Long: `Restore a workflow to a previous version from its history.

This operation restores the workflow to the specified version and deploys it.

Examples:
  # Restore by ID to version 5
  dtctl restore workflow a1b2c3d4-e5f6-7890-abcd-ef1234567890 5

  # Restore by name
  dtctl restore workflow "My Workflow" 3

  # Restore without confirmation
  dtctl restore workflow "My Workflow" 3 --force
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]
		version, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version number: %s", args[1])
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - restore modifies the workflow
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}
		if checker.IsOverridden() {
			fmt.Fprintln(os.Stderr, "⚠️ ", checker.OverrideWarning(safety.OperationUpdate))
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		workflowID, err := res.ResolveID(resolver.TypeWorkflow, identifier)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)

		// Get workflow for confirmation
		wf, err := handler.Get(workflowID)
		if err != nil {
			return err
		}

		// Confirm restore unless --force or --plain
		if !forceDelete && !plainMode {
			confirmMsg := fmt.Sprintf("Restore workflow %q to version %d?", wf.Title, version)
			if !prompt.Confirm(confirmMsg) {
				fmt.Println("Restore cancelled")
				return nil
			}
		}

		result, err := handler.RestoreHistory(workflowID, version)
		if err != nil {
			return err
		}

		fmt.Printf("Workflow %q restored to version %d\n", result.Title, version)
		return nil
	},
}

// restoreDashboardCmd restores a dashboard to a specific version
var restoreDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id-or-name> <version>",
	Aliases: []string{"dashboards", "dash", "db"},
	Short:   "Restore a dashboard to a previous version",
	Long: `Restore a dashboard to a previous snapshot version.

This operation resets the document's content to the state it had when the snapshot
was created. A new snapshot of the current state is automatically created before
restoring (if one doesn't exist).

Note: Only the document owner can restore snapshots.

Examples:
  # Restore by ID to version 5
  dtctl restore dashboard a1b2c3d4-e5f6-7890-abcd-ef1234567890 5

  # Restore by name
  dtctl restore dashboard "Production Dashboard" 3

  # Restore without confirmation
  dtctl restore dashboard "Production Dashboard" 3 --force
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]
		version, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version number: %s", args[1])
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - restore modifies the dashboard
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}
		if checker.IsOverridden() {
			fmt.Fprintln(os.Stderr, "⚠️ ", checker.OverrideWarning(safety.OperationUpdate))
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		dashboardID, err := res.ResolveID(resolver.TypeDashboard, identifier)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get dashboard metadata for confirmation
		metadata, err := handler.GetMetadata(dashboardID)
		if err != nil {
			return err
		}

		// Confirm restore unless --force or --plain
		if !forceDelete && !plainMode {
			confirmMsg := fmt.Sprintf("Restore dashboard %q from snapshot %d?", metadata.Name, version)
			if !prompt.Confirm(confirmMsg) {
				fmt.Println("Restore cancelled")
				return nil
			}
		}

		result, err := handler.RestoreSnapshot(dashboardID, version)
		if err != nil {
			return err
		}

		fmt.Printf("Dashboard %q restored from snapshot %d (new document version: %d)\n", metadata.Name, version, result.Version)
		return nil
	},
}

// restoreNotebookCmd restores a notebook to a specific version
var restoreNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id-or-name> <version>",
	Aliases: []string{"notebooks", "nb"},
	Short:   "Restore a notebook to a previous version",
	Long: `Restore a notebook to a previous snapshot version.

This operation resets the document's content to the state it had when the snapshot
was created. A new snapshot of the current state is automatically created before
restoring (if one doesn't exist).

Note: Only the document owner can restore snapshots.

Examples:
  # Restore by ID to version 5
  dtctl restore notebook a1b2c3d4-e5f6-7890-abcd-ef1234567890 5

  # Restore by name
  dtctl restore notebook "Analysis Notebook" 3

  # Restore without confirmation
  dtctl restore notebook "Analysis Notebook" 3 --force
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]
		version, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version number: %s", args[1])
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Safety check - restore modifies the notebook
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}
		if checker.IsOverridden() {
			fmt.Fprintln(os.Stderr, "⚠️ ", checker.OverrideWarning(safety.OperationUpdate))
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Resolve name to ID
		res := resolver.NewResolver(c)
		notebookID, err := res.ResolveID(resolver.TypeNotebook, identifier)
		if err != nil {
			return err
		}

		handler := document.NewHandler(c)

		// Get notebook metadata for confirmation
		metadata, err := handler.GetMetadata(notebookID)
		if err != nil {
			return err
		}

		// Confirm restore unless --force or --plain
		if !forceDelete && !plainMode {
			confirmMsg := fmt.Sprintf("Restore notebook %q from snapshot %d?", metadata.Name, version)
			if !prompt.Confirm(confirmMsg) {
				fmt.Println("Restore cancelled")
				return nil
			}
		}

		result, err := handler.RestoreSnapshot(notebookID, version)
		if err != nil {
			return err
		}

		fmt.Printf("Notebook %q restored from snapshot %d (new document version: %d)\n", metadata.Name, version, result.Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)

	restoreCmd.AddCommand(restoreWorkflowCmd)
	restoreCmd.AddCommand(restoreDashboardCmd)
	restoreCmd.AddCommand(restoreNotebookCmd)

	// Add --force flag to restore commands
	restoreWorkflowCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Skip confirmation prompt")
	restoreDashboardCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Skip confirmation prompt")
	restoreNotebookCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Skip confirmation prompt")
}
