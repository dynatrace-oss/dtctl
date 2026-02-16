package cmd

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

// workflowFilter holds the workflow ID filter for executions
var workflowFilter string

// getWorkflowsCmd retrieves workflows
var getWorkflowsCmd = &cobra.Command{
	Use:     "workflows [id]",
	Aliases: []string{"workflow", "wf"},
	Short:   "Get workflows",
	Long: `Get one or more workflows.

Examples:
  # List all workflows
  dtctl get workflows

  # Get a specific workflow
  dtctl get workflow <workflow-id>

  # Output as JSON
  dtctl get workflows -o json

  # List only my workflows
  dtctl get workflows --mine
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)
		printer := NewPrinter()

		// Get specific workflow if ID provided
		if len(args) > 0 {
			wf, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(wf)
		}

		// List workflows with filters
		mineOnly, _ := cmd.Flags().GetBool("mine")

		filters := workflow.WorkflowFilters{}

		// If --mine flag is set, get current user ID and filter by owner
		if mineOnly {
			userID, err := c.CurrentUserID()
			if err != nil {
				return fmt.Errorf("failed to get current user ID for --mine filter: %w", err)
			}
			filters.Owner = userID
		}

		// Check if watch mode is enabled
		watchMode, _ := cmd.Flags().GetBool("watch")
		if watchMode {
			fetcher := func() (interface{}, error) {
				list, err := handler.List(filters)
				if err != nil {
					return nil, err
				}
				return list.Results, nil
			}
			return executeWithWatch(cmd, fetcher, printer)
		}

		list, err := handler.List(filters)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

// getWorkflowExecutionsCmd retrieves workflow executions
var getWorkflowExecutionsCmd = &cobra.Command{
	Use:     "workflow-executions [id]",
	Aliases: []string{"workflow-execution", "wfe"},
	Short:   "Get workflow executions",
	Long: `Get one or more workflow executions.

Examples:
  # List all workflow executions
  dtctl get workflow-executions
  dtctl get wfe

  # List executions for a specific workflow
  dtctl get wfe --workflow <workflow-id>
  dtctl get wfe -w <workflow-id>

  # Get a specific execution
  dtctl get wfe <execution-id>

  # Output as JSON
  dtctl get wfe -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)
		printer := NewPrinter()

		// Get specific execution if ID provided
		if len(args) > 0 {
			exec, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			return printer.Print(exec)
		}

		// List executions (optionally filtered by workflow)
		list, err := handler.List(workflowFilter)
		if err != nil {
			return err
		}

		return printer.PrintList(list.Results)
	},
}

// deleteWorkflowCmd deletes a workflow
var deleteWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id-or-name>",
	Aliases: []string{"workflows", "wf"},
	Short:   "Delete a workflow",
	Long: `Delete a workflow by ID or name.

Examples:
  # Delete by ID
  dtctl delete workflow a1b2c3d4-e5f6-7890-abcd-ef1234567890

  # Delete by name (interactive disambiguation if multiple matches)
  dtctl delete workflow "My Workflow"

  # Delete without confirmation
  dtctl delete workflow "My Workflow" -y
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

		// Get workflow details for confirmation and ownership check
		wf, err := handler.Get(workflowID)
		if err != nil {
			return err
		}

		// Safety check with actual ownership
		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		currentUserID, _ := c.CurrentUserID()
		ownership := safety.DetermineOwnership(wf.Owner, currentUserID)
		if err := checker.CheckError(safety.OperationDelete, ownership); err != nil {
			return err
		}

		// Confirm deletion unless --force or --plain
		if !forceDelete && !plainMode {
			if !prompt.ConfirmDeletion("workflow", wf.Title, workflowID) {
				fmt.Println("Deletion cancelled")
				return nil
			}
		}

		if err := handler.Delete(workflowID); err != nil {
			return err
		}

		fmt.Printf("Workflow %q deleted\n", wf.Title)
		return nil
	},
}

func init() {
	addWatchFlags(getWorkflowsCmd)
	addWatchFlags(getWorkflowExecutionsCmd)

	getWorkflowExecutionsCmd.Flags().StringVarP(&workflowFilter, "workflow", "w", "", "Filter executions by workflow ID")
	getWorkflowsCmd.Flags().Bool("mine", false, "Show only workflows owned by current user")

	deleteWorkflowCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}
