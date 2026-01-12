package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
	"github.com/spf13/cobra"
)

var taskName string
var followLogs bool
var allTaskLogs bool

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
  # Get execution log (default API response)
  dtctl logs workflow-execution <execution-id>
  dtctl logs wfe <execution-id>

  # Get full logs for all tasks with headers
  dtctl logs wfe <execution-id> --all
  dtctl logs wfe <execution-id> -a

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

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)

		if followLogs {
			return followExecutionLogs(handler, executionID, taskName, allTaskLogs)
		}

		var logs string

		if taskName != "" {
			// Get logs for specific task
			logs, err = handler.GetTaskLog(executionID, taskName)
			if err != nil {
				return err
			}
		} else if allTaskLogs {
			// Get full logs for all tasks with headers
			logs, err = handler.GetFullExecutionLog(executionID)
			if err != nil {
				return err
			}
		} else {
			// Get execution log (default API response)
			logs, err = handler.GetExecutionLog(executionID)
			if err != nil {
				return err
			}
		}

		if logs == "" {
			fmt.Println("No logs available.")
			return nil
		}

		fmt.Print(logs)
		return nil
	},
}

// followExecutionLogs streams logs in real-time until the execution completes
func followExecutionLogs(handler *workflow.ExecutionHandler, executionID, task string, allTasks bool) error {
	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var lastLogLen int
	pollInterval := 2 * time.Second

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

		if task != "" {
			logs, err = handler.GetTaskLog(executionID, task)
		} else if allTasks {
			logs, err = handler.GetFullExecutionLog(executionID)
		} else {
			logs, err = handler.GetExecutionLog(executionID)
		}

		if err != nil {
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
			// Final log fetch to ensure we have everything
			if task != "" {
				logs, _ = handler.GetTaskLog(executionID, task)
			} else if allTasks {
				logs, _ = handler.GetFullExecutionLog(executionID)
			} else {
				logs, _ = handler.GetExecutionLog(executionID)
			}
			if len(logs) > lastLogLen {
				fmt.Print(logs[lastLogLen:])
			}

			fmt.Printf("\n--- Execution %s (state: %s) ---\n", exec.State, exec.State)
			return nil
		}

		time.Sleep(pollInterval)
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
	logsWorkflowExecutionCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow logs in real-time until execution completes")
	logsWorkflowExecutionCmd.Flags().BoolVarP(&allTaskLogs, "all", "a", false, "Get full logs for all tasks with headers")
}
