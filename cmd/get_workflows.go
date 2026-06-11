package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/resolver"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

// workflowFilter holds the workflow ID filter for executions
var workflowFilter string

// minWorkflowChunkSize is the smallest allowed --chunk-size for workflow listing.
// Smaller pages multiply the request count for no benefit and risk hammering the API.
const minWorkflowChunkSize = 20

// validateWorkflowChunkSize rejects tiny page sizes that fan a full listing out
// into excessive API requests. 0 (single page) and >= minWorkflowChunkSize are allowed.
func validateWorkflowChunkSize(chunk int64) error {
	if chunk > 0 && chunk < minWorkflowChunkSize {
		return fmt.Errorf("--chunk-size must be 0 or at least %d (got %d)", minWorkflowChunkSize, chunk)
	}
	return nil
}

// triggerTypeCaser normalizes trigger-type filter values to the API's title-case form
// (e.g. "schedule" -> "Schedule").
var triggerTypeCaser = cases.Title(language.Und)

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
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := workflow.NewHandler(c)
		ap := enrichAgent(printer, "get", "workflow")

		// Get specific workflow if ID provided
		if len(args) > 0 {
			wf, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			if ap != nil {
				ap.SetSuggestions([]string{
					fmt.Sprintf("Run 'dtctl exec workflow %s' to trigger this workflow", args[0]),
					fmt.Sprintf("Run 'dtctl get workflow-executions --workflow %s' to see past executions", args[0]),
				})
			}
			return printer.Print(wf)
		}

		// List workflows with filters
		mineOnly, _ := cmd.Flags().GetBool("mine")
		filterStr, _ := cmd.Flags().GetString("filter")
		typeStr, _ := cmd.Flags().GetString("type")
		triggerStr, _ := cmd.Flags().GetString("trigger")
		limit, _ := cmd.Flags().GetInt64("limit")

		chunk := GetChunkSize()
		if err := validateWorkflowChunkSize(chunk); err != nil {
			return err
		}

		filters := workflow.WorkflowFilters{
			Search:      filterStr,
			TriggerType: triggerTypeCaser.String(strings.ToLower(triggerStr)),
		}

		if typeStr != "" {
			filters.Type = strings.ToUpper(typeStr)
		}

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
				list, err := handler.List(filters, chunk, limit)
				if err != nil {
					return nil, err
				}
				return list.Results, nil
			}
			return executeWithWatch(cmd, fetcher, printer)
		}

		list, err := handler.List(filters, chunk, limit)
		if err != nil {
			return err
		}

		if ap != nil {
			ap.SetTotal(len(list.Results))
			suggestions := []string{
				"Run 'dtctl describe workflow <id>' for details",
				"Run 'dtctl exec workflow <id>' to trigger a workflow",
			}
			// If count from API exceeds returned results, more data exists. The
			// remedy depends on what capped the result: an explicit --limit, or
			// single-page mode (--chunk-size 0).
			if list.Count > len(list.Results) {
				ap.SetHasMore(true)
				if limit > 0 {
					suggestions = append(suggestions, fmt.Sprintf("Showing %d of %d. Raise --limit (currently %d) or set it to 0 for unlimited.", len(list.Results), list.Count, limit))
				} else {
					suggestions = append(suggestions, fmt.Sprintf("Showing %d of %d. Increase --chunk-size to page through all results.", len(list.Results), list.Count))
				}
			}
			ap.SetSuggestions(suggestions)
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
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)
		ap := enrichAgent(printer, "get", "workflow-execution")

		// Get specific execution if ID provided
		if len(args) > 0 {
			exec, err := handler.Get(args[0])
			if err != nil {
				return err
			}
			if ap != nil {
				ap.SetSuggestions([]string{
					fmt.Sprintf("Run 'dtctl logs workflow-execution %s' to view execution logs", args[0]),
				})
			}
			return printer.Print(exec)
		}

		// List executions (optionally filtered)
		limit, _ := cmd.Flags().GetInt64("limit")
		stateStr, _ := cmd.Flags().GetString("state")
		triggerStr, _ := cmd.Flags().GetString("trigger")
		sinceStr, _ := cmd.Flags().GetString("started-since")
		untilStr, _ := cmd.Flags().GetString("started-until")

		since, err := parseExecTime(sinceStr, false)
		if err != nil {
			return fmt.Errorf("invalid --started-since: %w", err)
		}
		until, err := parseExecTime(untilStr, true)
		if err != nil {
			return fmt.Errorf("invalid --started-until: %w", err)
		}

		list, err := handler.List(workflow.ExecutionFilters{
			WorkflowID:   workflowFilter,
			State:        strings.ToUpper(stateStr),
			TriggerType:  triggerTypeCaser.String(strings.ToLower(triggerStr)),
			StartedSince: since,
			StartedUntil: until,
		}, limit)
		if err != nil {
			return err
		}

		if ap != nil {
			ap.SetTotal(len(list.Results))
			suggestions := []string{
				"Run 'dtctl get workflow-executions <id>' for execution details",
				"Run 'dtctl logs workflow-execution <id>' to view execution logs",
			}
			// Executions are limit-windowed (not fully paginated); flag when the
			// server total exceeds what was returned so agents don't assume completeness.
			if list.Count > len(list.Results) {
				ap.SetHasMore(true)
				suggestions = append(suggestions, fmt.Sprintf("Showing %d of %d. Raise --limit (currently %d) or narrow the window with --started-since/--state.", len(list.Results), list.Count, limit))
			}
			ap.SetSuggestions(suggestions)
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

		// In agent mode, output structured response
		if agentMode {
			printer := NewPrinter()
			ap := enrichAgent(printer, "delete", "workflow")
			if ap != nil {
				ap.SetSuggestions([]string{
					"Deleted. Verify with 'dtctl get workflows'",
				})
			}
			return printer.Print(map[string]string{
				"id":     workflowID,
				"title":  wf.Title,
				"status": "deleted",
			})
		}

		output.PrintSuccess("Workflow %q deleted", wf.Title)
		return nil
	},
}

func init() {
	addWatchFlags(getWorkflowsCmd)

	getWorkflowExecutionsCmd.Flags().StringVarP(&workflowFilter, "workflow", "w", "", "Filter executions by workflow ID")
	getWorkflowExecutionsCmd.Flags().Int64("limit", 100, "Maximum number of executions to return (max 1000)")
	getWorkflowExecutionsCmd.Flags().String("state", "", "Filter by state: RUNNING, SUCCESS, ERROR, CANCELLED, UNKNOWN")
	getWorkflowExecutionsCmd.Flags().String("trigger", "", "Filter by trigger type: Manual, Schedule, Event, Workflow")
	getWorkflowExecutionsCmd.Flags().String("started-since", "", "Show executions started at or after this time (YYYY-MM-DD or ISO 8601)")
	getWorkflowExecutionsCmd.Flags().String("started-until", "", "Show executions started at or before this time (YYYY-MM-DD = end of day 23:59:59, or ISO 8601)")
	getWorkflowsCmd.Flags().Bool("mine", false, "Show only workflows owned by current user")
	getWorkflowsCmd.Flags().String("filter", "", "Search workflows by title")
	getWorkflowsCmd.Flags().String("type", "", "Filter by workflow type: standard or simple")
	getWorkflowsCmd.Flags().String("trigger", "", "Filter by trigger type: Manual, Schedule, Event")
	getWorkflowsCmd.Flags().Int64("limit", 0, "Maximum number of workflows to return (0 = unlimited)")

	deleteWorkflowCmd.Flags().BoolVarP(&forceDelete, "yes", "y", false, "Skip confirmation prompt")
}

// parseExecTime parses a date string as YYYY-MM-DD or ISO 8601 and returns RFC3339.
// When endOfDay is true and input is date-only, the time is set to 23:59:59.
func parseExecTime(s string, endOfDay bool) (string, error) {
	if s == "" {
		return "", nil
	}
	// Common ISO 8601 date-time forms (with/without seconds, with/without zone).
	for _, layout := range []string{
		time.RFC3339,             // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04Z07:00", // seconds omitted, with zone
		"2006-01-02T15:04:05",    // no zone (treated as UTC)
		"2006-01-02T15:04",       // no seconds, no zone
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}
	// Fall back to date-only
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return "", fmt.Errorf("use YYYY-MM-DD or ISO 8601 (e.g. 2006-01-02T15:04:05Z)")
	}
	if endOfDay {
		t = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}
	return t.UTC().Format(time.RFC3339), nil
}
