package workflow

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkworkflow "github.com/dynatrace-oss/dtctl/sdk/api/workflow"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	Execution        = sdkworkflow.Execution
	ExecutionList    = sdkworkflow.ExecutionList
	TaskExecution    = sdkworkflow.TaskExecution
	TaskExecutionMap = sdkworkflow.TaskExecutionMap
)

// ExecutionHandler handles execution resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type ExecutionHandler struct {
	sdk *sdkworkflow.ExecutionHandler
}

// NewExecutionHandler creates a new execution handler
func NewExecutionHandler(c *client.Client) *ExecutionHandler {
	return &ExecutionHandler{
		sdk: sdkworkflow.NewExecutionHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List retrieves all executions with optional workflow filter
func (h *ExecutionHandler) List(workflowID string) (*ExecutionList, error) {
	return h.sdk.List(workflowID)
}

// Get retrieves a specific execution
func (h *ExecutionHandler) Get(id string) (*Execution, error) {
	return h.sdk.Get(id)
}

// Cancel cancels an active execution
func (h *ExecutionHandler) Cancel(id string) error {
	return h.sdk.Cancel(id)
}

// ListTasks retrieves all task executions for a workflow execution
func (h *ExecutionHandler) ListTasks(executionID string) ([]TaskExecution, error) {
	return h.sdk.ListTasks(executionID)
}

// GetTaskLog retrieves the log output of a specific task execution
func (h *ExecutionHandler) GetTaskLog(executionID, taskName string) (string, error) {
	return h.sdk.GetTaskLog(executionID, taskName)
}

// GetTaskResult retrieves the structured return value of a specific task execution
func (h *ExecutionHandler) GetTaskResult(executionID, taskName string) (any, error) {
	return h.sdk.GetTaskResult(executionID, taskName)
}

// GetExecutionLog retrieves the combined log output of all tasks in an execution
func (h *ExecutionHandler) GetExecutionLog(executionID string) (string, error) {
	return h.sdk.GetExecutionLog(executionID)
}

// GetFullExecutionLog retrieves logs for all tasks in an execution, formatted with headers
func (h *ExecutionHandler) GetFullExecutionLog(executionID string) (string, error) {
	// Get all tasks
	tasks, err := h.sdk.ListTasks(executionID)
	if err != nil {
		return "", err
	}

	if len(tasks) == 0 {
		return "", nil
	}

	// Sort tasks by start time
	sortTasksByStartTime(tasks)

	var builder strings.Builder

	for i, task := range tasks {
		// Add separator between tasks
		if i > 0 {
			builder.WriteString("\n")
		}

		// Task header
		builder.WriteString(fmt.Sprintf("=== Task: %s [%s] ===\n", task.Name, task.State))

		// Get task log
		log, err := h.sdk.GetTaskLog(executionID, task.Name)
		if err != nil {
			builder.WriteString(fmt.Sprintf("(failed to fetch log: %v)\n", err))
			continue
		}

		if log == "" {
			builder.WriteString("(no log output)\n")
		} else {
			builder.WriteString(log)
			// Ensure log ends with newline
			if !strings.HasSuffix(log, "\n") {
				builder.WriteString("\n")
			}
		}
	}

	return builder.String(), nil
}

// GetCompleteExecutionLog retrieves both the workflow execution log and all task logs
func (h *ExecutionHandler) GetCompleteExecutionLog(executionID string) (string, error) {
	var builder strings.Builder

	// Get workflow execution log first
	execLog, err := h.sdk.GetExecutionLog(executionID)
	if err != nil {
		return "", err
	}

	if execLog != "" {
		builder.WriteString("=== Workflow Execution Log ===\n")
		builder.WriteString(execLog)
		// Ensure log ends with newline
		if !strings.HasSuffix(execLog, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	// Get all task logs
	taskLogs, err := h.GetFullExecutionLog(executionID)
	if err != nil {
		return "", err
	}

	if taskLogs != "" {
		builder.WriteString(taskLogs)
	}

	return builder.String(), nil
}

// sortTasksByStartTime sorts tasks by their start time (nil times go last)
func sortTasksByStartTime(tasks []TaskExecution) {
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			// Both nil - keep order
			if tasks[i].StartedAt == nil && tasks[j].StartedAt == nil {
				continue
			}
			// i is nil, j is not - swap (nil goes last)
			if tasks[i].StartedAt == nil {
				tasks[i], tasks[j] = tasks[j], tasks[i]
				continue
			}
			// j is nil - keep order (nil goes last)
			if tasks[j].StartedAt == nil {
				continue
			}
			// Both have times - sort ascending
			if tasks[i].StartedAt.After(*tasks[j].StartedAt) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}
