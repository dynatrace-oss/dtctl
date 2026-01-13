package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/spf13/cobra"
)

// historyCmd represents the history command
var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show version history of resources",
	Long:  `Show the version history of resources like workflows, notebooks, and dashboards.`,
	RunE:  requireSubcommand,
}

// historyWorkflowCmd shows version history for a workflow
var historyWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id-or-name>",
	Aliases: []string{"workflows", "wf"},
	Short:   "Show version history of a workflow",
	Long: `Show the version history of a workflow.

Workflow history is automatically tracked when workflows are modified.

Examples:
  # Show version history by ID
  dtctl history workflow a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Show version history by name
  dtctl history workflow "My Workflow"

  # Output as JSON
  dtctl history workflow "My Workflow" -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
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
		printer := NewPrinter()

		history, err := handler.ListHistory(workflowID)
		if err != nil {
			return err
		}

		if len(history.Results) == 0 {
			fmt.Println("No history found for this workflow")
			return nil
		}

		return printer.PrintList(history.Results)
	},
}

// historyDashboardCmd shows version history for a dashboard
var historyDashboardCmd = &cobra.Command{
	Use:     "dashboard <dashboard-id-or-name>",
	Aliases: []string{"dashboards", "dash", "db"},
	Short:   "Show version history of a dashboard",
	Long: `Show the version history (snapshots) of a dashboard.

Snapshots are created when updating a document with the create-snapshot option.
Each snapshot captures the document's content at a specific point in time.

Examples:
  # Show version history by ID
  dtctl history dashboard a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Show version history by name
  dtctl history dashboard "Production Dashboard"

  # Output as JSON
  dtctl history dashboard "Production Dashboard" -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
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
		printer := NewPrinter()

		snapshots, err := handler.ListSnapshots(dashboardID)
		if err != nil {
			return err
		}

		if len(snapshots.Snapshots) == 0 {
			fmt.Println("No snapshots found for this dashboard")
			return nil
		}

		return printer.PrintList(snapshots.Snapshots)
	},
}

// historyNotebookCmd shows version history for a notebook
var historyNotebookCmd = &cobra.Command{
	Use:     "notebook <notebook-id-or-name>",
	Aliases: []string{"notebooks", "nb"},
	Short:   "Show version history of a notebook",
	Long: `Show the version history (snapshots) of a notebook.

Snapshots are created when updating a document with the create-snapshot option.
Each snapshot captures the document's content at a specific point in time.

Examples:
  # Show version history by ID
  dtctl history notebook a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Show version history by name
  dtctl history notebook "Analysis Notebook"

  # Output as JSON
  dtctl history notebook "Analysis Notebook" -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
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
		printer := NewPrinter()

		snapshots, err := handler.ListSnapshots(notebookID)
		if err != nil {
			return err
		}

		if len(snapshots.Snapshots) == 0 {
			fmt.Println("No snapshots found for this notebook")
			return nil
		}

		return printer.PrintList(snapshots.Snapshots)
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)

	historyCmd.AddCommand(historyWorkflowCmd)
	historyCmd.AddCommand(historyDashboardCmd)
	historyCmd.AddCommand(historyNotebookCmd)
}
