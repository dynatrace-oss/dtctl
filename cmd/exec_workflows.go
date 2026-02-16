package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/spf13/cobra"
)

// execWorkflowCmd executes a workflow
var execWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id>",
	Aliases: []string{"wf"},
	Short:   "Execute a workflow",
	Long: `Execute an automation workflow.

Examples:
  # Execute workflow
  dtctl exec workflow my-workflow-id

  # Execute with parameters
  dtctl exec workflow my-workflow-id --params severity=high --params env=prod

  # Execute and wait for completion
  dtctl exec workflow my-workflow-id --wait

  # Execute with custom timeout
  dtctl exec workflow my-workflow-id --wait --timeout 10m
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowID := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		executor := exec.NewWorkflowExecutor(c)

		paramStrings, _ := cmd.Flags().GetStringSlice("params")
		params, err := exec.ParseParams(paramStrings)
		if err != nil {
			return err
		}

		result, err := executor.Execute(workflowID, params)
		if err != nil {
			return err
		}

		fmt.Printf("Workflow execution started\n")
		fmt.Printf("Execution ID: %s\n", result.ID)
		fmt.Printf("State: %s\n", result.State)

		// Handle --wait flag
		wait, _ := cmd.Flags().GetBool("wait")
		if wait {
			timeout, _ := cmd.Flags().GetDuration("timeout")
			if timeout == 0 {
				timeout = 30 * time.Minute
			}

			fmt.Printf("\nWaiting for execution to complete...\n")

			opts := exec.WaitOptions{
				PollInterval: 2 * time.Second,
				Timeout:      timeout,
			}

			status, err := executor.WaitForCompletion(context.Background(), result.ID, opts)
			if err != nil {
				return err
			}

			fmt.Printf("\nExecution completed\n")
			fmt.Printf("Final State: %s\n", status.State)
			if status.StateInfo != nil && *status.StateInfo != "" {
				fmt.Printf("State Info: %s\n", *status.StateInfo)
			}
			fmt.Printf("Duration: %s\n", formatExecutionDuration(status.Runtime))

			// Return error if execution failed
			if status.State == "ERROR" {
				return fmt.Errorf("workflow execution failed")
			}
		}

		return nil
	},
}

// formatExecutionDuration formats seconds into a human-readable duration
func formatExecutionDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		m := seconds / 60
		s := seconds % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

func init() {
	// Workflow flags
	execWorkflowCmd.Flags().StringSlice("params", []string{}, "workflow parameters (key=value)")
	execWorkflowCmd.Flags().Bool("wait", false, "wait for workflow execution to complete")
	execWorkflowCmd.Flags().Duration("timeout", 30*time.Minute, "timeout when waiting for completion")
}
