package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var createBreakpointCmd = &cobra.Command{
	Use:     "breakpoint <filename:line>",
	Aliases: []string{"breakpoints", "bp"},
	Short:   "Create a Live Debugger breakpoint",
	Long: `Create a Live Debugger breakpoint in the current workspace.

Examples:
  # Create a breakpoint
  dtctl create breakpoint OrderController.java:306

  # Dry run to preview
  dtctl create breakpoint OrderController.java:306 --dry-run
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := strings.TrimSpace(args[0])
		fileName, lineNumber, err := parseBreakpoint(identifier)
		if err != nil {
			return err
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		if dryRun {
			return printBreakpointMessage("create", fmt.Sprintf("Dry run: would create breakpoint at %s:%d", fileName, lineNumber))
		}

		verbose := isDebugVerbose()

		ctx, err := cfg.CurrentContextObj()
		if err != nil {
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

		createResp, err := handler.CreateBreakpoint(workspaceID, fileName, lineNumber)
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("createRuleV2", createResp)
			}
			return err
		}
		if verbose {
			if err := printGraphQLResponse("createRuleV2", createResp); err != nil {
				return err
			}
		}

		return printBreakpointMessage("create", fmt.Sprintf("Created breakpoint at %s:%d", fileName, lineNumber))
	},
}
