package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/dynatrace-oss/dtctl/pkg/stability"
)

var taskName string
var followLogs bool
var allTaskLogs bool
var tasksOnlyLogs bool

// followPollInterval is how often --follow polls for new log output.
var followPollInterval = 2 * time.Second

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print logs for resources",
	Long:  `Print logs for various resources.`,
}

// logsWorkflowExecutionCmd prints logs for a workflow execution
var logsWorkflowExecutionCmd = &cobra.Command{
	Use:     "workflow-execution <execution-id>",
	Aliases: []string{"wfe"},
	Short:   "Print logs for a workflow execution",
	Long: `Print logs for a workflow execution or a specific task within it.

Examples:
  # Get execution log only (workflow-level log)
  dtctl logs workflow-execution <execution-id>
  dtctl logs wfe <execution-id>

  # Get all logs (workflow execution log + all task logs)
  dtctl logs wfe <execution-id> --all
  dtctl logs wfe <execution-id> -a

  # Get task logs only (all tasks with headers)
  dtctl logs wfe <execution-id> --tasks

  # Get logs for a specific task
  dtctl logs wfe <execution-id> --task <task-name>
  dtctl logs wfe <execution-id> -t <task-name>

  # Follow logs in real-time (stream until execution completes)
  dtctl logs wfe <execution-id> --follow
  dtctl logs wfe <execution-id> -f
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		executionID := args[0]

		// Validate flag combinations
		if allTaskLogs && tasksOnlyLogs {
			return fmt.Errorf("cannot use both --all and --tasks flags together")
		}
		if taskName != "" && (allTaskLogs || tasksOnlyLogs) {
			return fmt.Errorf("cannot use --task with --all or --tasks flags")
		}

		_, c, err := SetupClient()
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)

		if followLogs {
			if !caps.LongRunningStreams {
				return &CapabilityError{Feature: "log following"}
			}
			return followExecutionLogs(cmd.Context(), handler, executionID, taskName, allTaskLogs, tasksOnlyLogs)
		}

		var logs string
		// taskErr is a partial failure: some task logs could not be fetched.
		// The logs that were fetched are printed before it is returned.
		var taskErr *workflow.TaskLogError

		if taskName != "" {
			// Get logs for specific task
			logs, err = handler.GetTaskLog(executionID, taskName)
			if err != nil {
				return err
			}
		} else if allTaskLogs {
			// Get workflow execution log + all task logs
			logs, err = handler.GetCompleteExecutionLog(executionID)
			if err != nil && !errors.As(err, &taskErr) {
				return err
			}
		} else if tasksOnlyLogs {
			// Get task logs only (all tasks with headers)
			logs, err = handler.GetFullExecutionLog(executionID)
			if err != nil && !errors.As(err, &taskErr) {
				return err
			}
		} else {
			// Get execution log only (workflow-level log)
			logs, err = handler.GetExecutionLog(executionID)
			if err != nil {
				return err
			}
		}

		if logs == "" {
			fmt.Println("No logs available.")
		} else {
			fmt.Print(logs)
		}
		if taskErr != nil {
			return taskErr
		}
		return nil
	},
}

// followExecutionLogs streams logs in real-time until the execution completes
func followExecutionLogs(parentCtx context.Context, handler *workflow.ExecutionHandler, executionID, task string, allLogs, tasksOnly bool) error {
	// Compose with SIGINT/SIGTERM for graceful shutdown.
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var lastLogLen int
	// A task log that cannot be fetched mid-stream is warned about, once per
	// distinct failure, and the stream goes on: the fetch is retried on the
	// next poll. Only the final fetch decides the exit code.
	var lastWarning string
	warnTaskErr := func(err error) error {
		var taskErr *workflow.TaskLogError
		if !errors.As(err, &taskErr) {
			return err
		}
		if msg := taskErr.Error(); msg != lastWarning {
			output.PrintWarning("%s", msg)
			lastWarning = msg
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nLog streaming interrupted.")
			return nil
		default:
		}

		// Get current logs
		var logs string
		var err error

		switch {
		case task != "":
			logs, err = handler.GetTaskLog(executionID, task)
		case allLogs:
			logs, err = handler.GetCompleteExecutionLog(executionID)
		case tasksOnly:
			logs, err = handler.GetFullExecutionLog(executionID)
		default:
			logs, err = handler.GetExecutionLog(executionID)
		}

		if err := warnTaskErr(err); err != nil {
			return err
		}

		// Print only new content
		if len(logs) > lastLogLen {
			fmt.Print(logs[lastLogLen:])
			lastLogLen = len(logs)
		}

		// Check execution status
		exec, err := handler.Get(executionID)
		if err != nil {
			return err
		}

		// Check if execution is complete
		if isTerminalState(exec.State) {
			// Final log fetch to ensure we have everything. A plain fetch
			// error here is ignored, as the logs polled so far stand; a task
			// log still missing now is reported, because the stream is over.
			var finalErr error
			switch {
			case task != "":
				logs, _ = handler.GetTaskLog(executionID, task)
			case allLogs:
				logs, finalErr = handler.GetCompleteExecutionLog(executionID)
			case tasksOnly:
				logs, finalErr = handler.GetFullExecutionLog(executionID)
			default:
				logs, _ = handler.GetExecutionLog(executionID)
			}
			if len(logs) > lastLogLen {
				fmt.Print(logs[lastLogLen:])
			}

			fmt.Printf("\n--- Execution %s (state: %s) ---\n", exec.State, exec.State)

			var taskErr *workflow.TaskLogError
			if errors.As(finalErr, &taskErr) {
				return taskErr
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(followPollInterval):
		}
	}
}

// isTerminalState checks if the execution state is terminal
func isTerminalState(state string) bool {
	switch state {
	case "SUCCESS", "ERROR", "CANCELED", "CANCELLED":
		return true
	default:
		return false
	}
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsWorkflowExecutionCmd)
	logsWorkflowExecutionCmd.Flags().StringVarP(&taskName, "task", "t", "", "Get logs for a specific task")
	rejectEmptyFlag(logsWorkflowExecutionCmd, "task")
	logsWorkflowExecutionCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow logs in real-time until execution completes")
	// -f is reserved for --file in 1.0 (contrib breaking-changes/short-flag-f.md).
	stability.MarkFlag(logsWorkflowExecutionCmd, "follow", stability.Experimental, pre10Since)
	logsWorkflowExecutionCmd.Flags().BoolVarP(&allTaskLogs, "all", "a", false, "Get all logs (workflow execution log + all task logs)")
	logsWorkflowExecutionCmd.Flags().BoolVar(&tasksOnlyLogs, "tasks", false, "Get task logs only (all tasks with headers)")

	// In 1.0 this command's output is reshaped, not extended (contrib
	// breaking-changes/agent-output-envelope.md): agent mode wraps the log in
	// the standard envelope with --tasks/--all under result.tasks, --follow -A
	// becomes a usage error, and the plain-mode "No logs available." notice
	// moves to stderr. Nothing a caller parses today survives, and there is no
	// flag to hang that on — the contract is the command's.
	stability.Mark(logsWorkflowExecutionCmd, stability.Experimental, pre10Since)
}

// Declared stable: the invocation and output contract of this command is
// additive-only. Stable is never implied -- see AGENTS.md "Stability Tiers".
func init() {
	stability.MarkStable(logsCmd)
}
