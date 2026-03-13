package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

type breakpointDeleteOps struct {
	deleteAll func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error)
	deleteOne func(handler *livedebugger.Handler, workspaceID, breakpointID string) (map[string]interface{}, error)
}

func defaultBreakpointDeleteOps() breakpointDeleteOps {
	return breakpointDeleteOps{
		deleteAll: func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
			return handler.DeleteAllBreakpoints(workspaceID)
		},
		deleteOne: func(handler *livedebugger.Handler, workspaceID, breakpointID string) (map[string]interface{}, error) {
			return handler.DeleteBreakpoint(workspaceID, breakpointID)
		},
	}
}

var deleteBreakpointCmd = &cobra.Command{
	Use:     "breakpoint <id|filename:line>",
	Aliases: []string{"breakpoints", "bp"},
	Short:   "Delete Live Debugger breakpoint(s)",
	Long: `Delete Live Debugger breakpoints by mutable rule ID or by source location.

Examples:
  # Delete a single breakpoint by ID
  dtctl delete breakpoint 1232343453242

  # Delete all breakpoints found at a file and line
  dtctl delete breakpoint MyFile.java:1234

  # Delete all breakpoints in the current workspace
  dtctl delete breakpoint --all

  # Preview deletion without making changes
  dtctl delete breakpoint MyFile.java:1234 --dry-run
`,
	Args: validateDeleteBreakpointArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ops := defaultBreakpointDeleteOps()
		deleteAll, _ := cmd.Flags().GetBool("all")
		yes, _ := cmd.Flags().GetBool("yes")
		verbose := isDebugVerbose()

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		ctx, err := cfg.CurrentContextObj()
		if err != nil {
			return err
		}

		if err := checkDeleteBreakpointSafety(cfg); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler, err := livedebugger.NewHandler(c, ctx.Environment)
		if err != nil {
			return err
		}

		workspaceResp, workspaceID, err := handler.GetOrCreateWorkspace(currentProjectPath())
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("getOrCreateWorkspaceV2", workspaceResp)
			}
			return err
		}
		if verbose {
			if err := printGraphQLResponse("getOrCreateWorkspaceV2", workspaceResp); err != nil {
				return err
			}
		}

		workspaceRulesResp, err := handler.GetWorkspaceRules(workspaceID)
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("getWorkspaceRules", workspaceRulesResp)
			}
			return err
		}
		if verbose {
			if err := printGraphQLResponse("getWorkspaceRules", workspaceRulesResp); err != nil {
				return err
			}
		}

		rows, err := extractBreakpointRows(workspaceRulesResp)
		if err != nil {
			return err
		}

		if deleteAll {
			return runDeleteAllBreakpointsWithOps(handler, workspaceID, rows, yes, verbose, ops)
		}

		identifier := args[0]
		if fileName, lineNumber, err := parseBreakpoint(identifier); err == nil {
			targets := findBreakpointRowsByLocation(rows, fileName, lineNumber)
			if len(targets) == 0 {
				return fmt.Errorf("no breakpoints found at %s:%d", fileName, lineNumber)
			}
			return runDeleteBreakpointRowsWithOps(handler, workspaceID, targets, yes, verbose, ops)
		}

		if row, ok := findBreakpointRowByID(rows, identifier); ok {
			return runDeleteBreakpointRowsWithOps(handler, workspaceID, []breakpointRow{row}, yes, verbose, ops)
		}

		return runDeleteBreakpointRowsWithOps(handler, workspaceID, []breakpointRow{{ID: identifier}}, yes, verbose, ops)
	},
}

func checkDeleteBreakpointSafety(cfg *config.Config) error {
	checker, err := NewSafetyChecker(cfg)
	if err != nil {
		return err
	}
	return checker.CheckError(safety.OperationDelete, safety.OwnershipUnknown)
}

func validateDeleteBreakpointArgs(cmd *cobra.Command, args []string) error {
	deleteAll, _ := cmd.Flags().GetBool("all")
	if deleteAll {
		if len(args) != 0 {
			return fmt.Errorf("--all does not accept an identifier")
		}
		return nil
	}

	if len(args) != 1 {
		return cobra.ExactArgs(1)(cmd, args)
	}

	return nil
}

func runDeleteAllBreakpoints(handler *livedebugger.Handler, workspaceID string, rows []breakpointRow, yes bool, verbose bool) error {
	return runDeleteAllBreakpointsWithOps(handler, workspaceID, rows, yes, verbose, defaultBreakpointDeleteOps())
}

func runDeleteAllBreakpointsWithOps(handler *livedebugger.Handler, workspaceID string, rows []breakpointRow, yes bool, verbose bool, ops breakpointDeleteOps) error {
	if len(rows) == 0 {
		return printBreakpointMessage("delete", "No breakpoints found in the current workspace")
	}

	if !yes && !plainMode {
		confirmMsg := fmt.Sprintf("Delete ALL %d breakpoint(s) in the current workspace?", len(rows))
		if !prompt.Confirm(confirmMsg) {
			return printBreakpointMessage("delete", "Deletion cancelled")
		}
	}

	if dryRun {
		return printBreakpointMessage("delete", fmt.Sprintf("Dry run: would delete %d breakpoint(s) from the current workspace", len(rows)))
	}

	deleteResp, err := ops.deleteAll(handler, workspaceID)
	if err != nil {
		if verbose {
			_ = printGraphQLResponse("deleteAllRulesFromWorkspaceV2", deleteResp)
		}
		return err
	}
	if verbose {
		if err := printGraphQLResponse("deleteAllRulesFromWorkspaceV2", deleteResp); err != nil {
			return err
		}
	}

	deletedIDs, err := extractDeletedBreakpointIDs(deleteResp)
	if err != nil {
		return err
	}
	if len(deletedIDs) == 0 {
		return printBreakpointMessage("delete", "Deleted 0 breakpoints")
	}

	return printBreakpointMessage("delete", fmt.Sprintf("Deleted %d breakpoint(s)", len(deletedIDs)))
}

func runDeleteBreakpointRows(handler *livedebugger.Handler, workspaceID string, rows []breakpointRow, yes bool, verbose bool) error {
	return runDeleteBreakpointRowsWithOps(handler, workspaceID, rows, yes, verbose, defaultBreakpointDeleteOps())
}

func runDeleteBreakpointRowsWithOps(handler *livedebugger.Handler, workspaceID string, rows []breakpointRow, yes bool, verbose bool, ops breakpointDeleteOps) error {
	if len(rows) == 0 {
		return nil
	}

	if !yes && !plainMode {
		if len(rows) == 1 {
			row := rows[0]
			if !prompt.ConfirmDeletion("breakpoint", formatBreakpointLocation(row), row.ID) {
				return printBreakpointMessage("delete", "Deletion cancelled")
			}
		} else {
			confirmMsg := fmt.Sprintf("Delete %d breakpoint(s) at %s?", len(rows), formatBreakpointLocation(rows[0]))
			if !prompt.Confirm(confirmMsg) {
				return printBreakpointMessage("delete", "Deletion cancelled")
			}
		}
	}

	if dryRun {
		for _, row := range rows {
			if err := printBreakpointMessage("delete", fmt.Sprintf("Dry run: would delete breakpoint %s (%s)", row.ID, formatBreakpointLocation(row))); err != nil {
				return err
			}
		}
		return nil
	}

	deletedRows := make([]breakpointRow, 0, len(rows))
	failures := make([]string, 0)
	for _, row := range rows {
		deleteResp, err := ops.deleteOne(handler, workspaceID, row.ID)
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("deleteRuleV2", deleteResp)
			}
			failures = append(failures, fmt.Sprintf("%s (%s): %v", row.ID, formatBreakpointLocation(row), err))
			continue
		}
		if verbose {
			if err := printGraphQLResponse("deleteRuleV2", deleteResp); err != nil {
				return err
			}
		}
		deletedRows = append(deletedRows, row)
	}

	if len(deletedRows) == 1 {
		if err := printBreakpointMessage("delete", fmt.Sprintf("Deleted breakpoint %s (%s)", deletedRows[0].ID, formatBreakpointLocation(deletedRows[0]))); err != nil {
			return err
		}
	} else if len(deletedRows) > 1 {
		if err := printBreakpointMessage("delete", fmt.Sprintf("Deleted %d breakpoint(s) at %s", len(deletedRows), formatBreakpointLocation(deletedRows[0]))); err != nil {
			return err
		}
	}

	if len(failures) > 0 {
		if len(deletedRows) > 0 {
			if err := printBreakpointMessage("delete", fmt.Sprintf("Failed to delete %d breakpoint(s) after deleting %d successfully", len(failures), len(deletedRows))); err != nil {
				return err
			}
		}
		return fmt.Errorf("failed to delete breakpoint(s): %s", strings.Join(failures, "; "))
	}

	return nil
}

func findBreakpointRowsByLocation(rows []breakpointRow, fileName string, lineNumber int) []breakpointRow {
	matches := make([]breakpointRow, 0)
	for _, row := range rows {
		if row.Filename == fileName && row.Line == lineNumber {
			matches = append(matches, row)
		}
	}
	return matches
}

func findBreakpointRowByID(rows []breakpointRow, id string) (breakpointRow, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return breakpointRow{}, false
}

func extractDeletedBreakpointIDs(deleteResp map[string]interface{}) ([]string, error) {
	return livedebugger.ExtractDeletedRuleIDs(deleteResp)
}

func formatBreakpointLocation(row breakpointRow) string {
	if row.Filename == "" || row.Line <= 0 {
		return "unknown location"
	}
	return fmt.Sprintf("%s:%d", row.Filename, row.Line)
}

func init() {
	deleteCmd.AddCommand(deleteBreakpointCmd)
	deleteBreakpointCmd.Flags().Bool("all", false, "Delete all breakpoints in the current workspace")
	deleteBreakpointCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}
