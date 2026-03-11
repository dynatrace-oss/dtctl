package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/prompt"
	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var deleteBreakpointCmd = &cobra.Command{
	Use:     "breakpoint <id|filename:line>",
	Aliases: []string{"breakpoints", "bp", "brakpoints"},
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
			return runDeleteAllBreakpoints(handler, workspaceID, rows, yes, verbose)
		}

		identifier := args[0]
		if fileName, lineNumber, err := parseBreakpoint(identifier); err == nil {
			targets := findBreakpointRowsByLocation(rows, fileName, lineNumber)
			if len(targets) == 0 {
				return fmt.Errorf("no breakpoints found at %s:%d", fileName, lineNumber)
			}
			return runDeleteBreakpointRows(handler, workspaceID, targets, yes, verbose)
		}

		if row, ok := findBreakpointRowByID(rows, identifier); ok {
			return runDeleteBreakpointRows(handler, workspaceID, []breakpointRow{row}, yes, verbose)
		}

		return runDeleteBreakpointRows(handler, workspaceID, []breakpointRow{{ID: identifier}}, yes, verbose)
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
	if len(rows) == 0 {
		printOutln("No breakpoints found in the current workspace")
		return nil
	}

	if !yes && !plainMode {
		confirmMsg := fmt.Sprintf("Delete ALL %d breakpoint(s) in the current workspace?", len(rows))
		if !prompt.Confirm(confirmMsg) {
			printOutln("Deletion cancelled")
			return nil
		}
	}

	if dryRun {
		printOutf("Dry run: would delete %d breakpoint(s) from the current workspace\n", len(rows))
		return nil
	}

	deleteResp, err := handler.DeleteAllBreakpoints(workspaceID)
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
		printOutln("Deleted 0 breakpoints")
		return nil
	}

	printOutf("Deleted %d breakpoint(s)\n", len(deletedIDs))
	return nil
}

func runDeleteBreakpointRows(handler *livedebugger.Handler, workspaceID string, rows []breakpointRow, yes bool, verbose bool) error {
	if len(rows) == 0 {
		return nil
	}

	if !yes && !plainMode {
		if len(rows) == 1 {
			row := rows[0]
			if !prompt.ConfirmDeletion("breakpoint", formatBreakpointLocation(row), row.ID) {
				printOutln("Deletion cancelled")
				return nil
			}
		} else {
			confirmMsg := fmt.Sprintf("Delete %d breakpoint(s) at %s?", len(rows), formatBreakpointLocation(rows[0]))
			if !prompt.Confirm(confirmMsg) {
				printOutln("Deletion cancelled")
				return nil
			}
		}
	}

	if dryRun {
		for _, row := range rows {
			printOutf("Dry run: would delete breakpoint %s (%s)\n", row.ID, formatBreakpointLocation(row))
		}
		return nil
	}

	deletedRows := make([]breakpointRow, 0, len(rows))
	failures := make([]string, 0)
	for _, row := range rows {
		deleteResp, err := handler.DeleteBreakpoint(workspaceID, row.ID)
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
		printOutf("Deleted breakpoint %s (%s)\n", deletedRows[0].ID, formatBreakpointLocation(deletedRows[0]))
	} else if len(deletedRows) > 1 {
		printOutf("Deleted %d breakpoint(s) at %s\n", len(deletedRows), formatBreakpointLocation(deletedRows[0]))
	}

	if len(failures) > 0 {
		if len(deletedRows) > 0 {
			printOutf("Failed to delete %d breakpoint(s) after deleting %d successfully\n", len(failures), len(deletedRows))
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
	dataObj, ok := deleteResp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing data object")
	}

	orgObj, ok := dataObj["org"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing org object")
	}

	workspaceObj, ok := orgObj["workspace"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing workspace object")
	}

	deletedIfc, ok := workspaceObj["deleteAllRulesFromWorkspaceV2"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing deleted ids list")
	}

	deletedIDs := make([]string, 0, len(deletedIfc))
	for _, idIfc := range deletedIfc {
		id, ok := idIfc.(string)
		if !ok || id == "" {
			continue
		}
		deletedIDs = append(deletedIDs, id)
	}

	return deletedIDs, nil
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
