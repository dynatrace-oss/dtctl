package workflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Execution represents a workflow execution.
type Execution struct {
	ID          string     `json:"id" table:"ID"`
	Workflow    string     `json:"workflow" table:"WORKFLOW"`
	Title       string     `json:"title" table:"TITLE"`
	State       string     `json:"state" table:"STATE"`
	StateInfo   *string    `json:"stateInfo,omitempty" table:"-"`
	StartedAt   time.Time  `json:"startedAt" table:"STARTED"`
	EndedAt     *time.Time `json:"endedAt,omitempty" table:"-"`
	Runtime     int        `json:"runtime,omitempty" table:"RUNTIME"`
	Trigger     *string    `json:"trigger,omitempty" table:"-"`
	TriggerType string     `json:"triggerType,omitempty" table:"TRIGGER"`
	User        *string    `json:"user,omitempty" table:"-"`
	Actor       string     `json:"actor,omitempty" table:"-"`
	Input       any        `json:"input,omitempty" table:"-"`
	Params      any        `json:"params,omitempty" table:"-"`
	Result      any        `json:"result,omitempty" table:"-"`
}

// ExecutionList represents a list of executions.
type ExecutionList struct {
	Count   int         `json:"count"`
	Results []Execution `json:"results"`
}

// ExecutionHandler handles execution resources.
type ExecutionHandler struct {
	client *httpclient.Client
}

// NewExecutionHandler creates a new execution handler.
func NewExecutionHandler(c *httpclient.Client) *ExecutionHandler {
	return &ExecutionHandler{client: c}
}

// List retrieves all executions with optional workflow filter.
func (h *ExecutionHandler) List(workflowID string) (*ExecutionList, error) {
	var result ExecutionList

	req := h.client.HTTP().R().SetResult(&result)

	if workflowID != "" {
		req.SetQueryParam("workflow", workflowID)
	}

	resp, err := req.Get("/platform/automation/v1/executions")
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// Get retrieves a specific execution.
func (h *ExecutionHandler) Get(id string) (*Execution, error) {
	var result Execution

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// Cancel cancels an active execution.
func (h *ExecutionHandler) Cancel(id string) error {
	resp, err := h.client.HTTP().R().
		Post(fmt.Sprintf("/platform/automation/v1/executions/%s/cancel", id))
	if err != nil {
		return fmt.Errorf("cancel execution: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return err
	}

	return nil
}

// TaskExecution represents a task execution within a workflow execution.
type TaskExecution struct {
	ID        string     `json:"id" table:"ID"`
	Name      string     `json:"name" table:"NAME"`
	State     string     `json:"state" table:"STATE"`
	StartedAt *time.Time `json:"startedAt,omitempty" table:"STARTED"`
	EndedAt   *time.Time `json:"endedAt,omitempty" table:"-"`
	Runtime   int        `json:"runtime,omitempty" table:"RUNTIME"`
	StateInfo *string    `json:"stateInfo,omitempty" table:"-"`
	Input     any        `json:"input,omitempty" table:"-"`
	Result    any        `json:"result,omitempty" table:"-"`
}

// TaskExecutionMap is a map of task name to task execution.
type TaskExecutionMap map[string]TaskExecution

// ListTasks retrieves all task executions for a workflow execution.
func (h *ExecutionHandler) ListTasks(executionID string) ([]TaskExecution, error) {
	var result TaskExecutionMap

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks", executionID))
	if err != nil {
		return nil, fmt.Errorf("list task executions: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	// Convert map to slice
	tasks := make([]TaskExecution, 0, len(result))
	for _, task := range result {
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetTaskLog retrieves the log output of a specific task execution.
func (h *ExecutionHandler) GetTaskLog(executionID, taskName string) (string, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks/%s/log", executionID, taskName))
	if err != nil {
		return "", fmt.Errorf("get task log: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return "", err
	}

	// The API returns a JSON-encoded string, so we need to unquote it.
	// Use resp.Body() to avoid potential truncation of large logs.
	body := string(resp.Body())
	if len(body) >= 2 && body[0] == '"' && body[len(body)-1] == '"' {
		// Remove surrounding quotes and unescape
		unquoted := body[1 : len(body)-1]
		// Handle common escape sequences
		unquoted = strings.ReplaceAll(unquoted, "\\n", "\n")
		unquoted = strings.ReplaceAll(unquoted, "\\t", "\t")
		unquoted = strings.ReplaceAll(unquoted, "\\\"", "\"")
		unquoted = strings.ReplaceAll(unquoted, "\\\\", "\\")
		return unquoted, nil
	}

	return body, nil
}

// GetTaskResult retrieves the structured return value of a specific task execution.
func (h *ExecutionHandler) GetTaskResult(executionID, taskName string) (any, error) {
	var result any

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks/%s/result", executionID, taskName))
	if err != nil {
		return nil, fmt.Errorf("get task result: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return result, nil
}

// GetExecutionLog retrieves the combined log output of all tasks in an execution.
func (h *ExecutionHandler) GetExecutionLog(executionID string) (string, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/log", executionID))
	if err != nil {
		return "", fmt.Errorf("get execution log: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return "", err
	}

	// The API returns a JSON-encoded string, so we need to unquote it.
	// Use resp.Body() to avoid potential truncation of large logs.
	body := string(resp.Body())
	if len(body) >= 2 && body[0] == '"' && body[len(body)-1] == '"' {
		// Remove surrounding quotes and unescape
		unquoted := body[1 : len(body)-1]
		// Handle common escape sequences
		unquoted = strings.ReplaceAll(unquoted, "\\n", "\n")
		unquoted = strings.ReplaceAll(unquoted, "\\t", "\t")
		unquoted = strings.ReplaceAll(unquoted, "\\\"", "\"")
		unquoted = strings.ReplaceAll(unquoted, "\\\\", "\\")
		return unquoted, nil
	}

	return body, nil
}

// GetFullExecutionLog retrieves logs for all tasks in an execution, formatted with headers.
func (h *ExecutionHandler) GetFullExecutionLog(executionID string) (string, error) {
	tasks, err := h.ListTasks(executionID)
	if err != nil {
		return "", err
	}

	if len(tasks) == 0 {
		return "", nil
	}

	sortTasksByStartTime(tasks)

	var builder strings.Builder

	for i, task := range tasks {
		if i > 0 {
			builder.WriteString("\n")
		}

		builder.WriteString(fmt.Sprintf("=== Task: %s [%s] ===\n", task.Name, task.State))

		log, err := h.GetTaskLog(executionID, task.Name)
		if err != nil {
			builder.WriteString(fmt.Sprintf("(failed to fetch log: %v)\n", err))
			continue
		}

		if log == "" {
			builder.WriteString("(no log output)\n")
		} else {
			builder.WriteString(log)
			if !strings.HasSuffix(log, "\n") {
				builder.WriteString("\n")
			}
		}
	}

	return builder.String(), nil
}

// GetCompleteExecutionLog retrieves both the workflow execution log and all task logs.
func (h *ExecutionHandler) GetCompleteExecutionLog(executionID string) (string, error) {
	var builder strings.Builder

	execLog, err := h.GetExecutionLog(executionID)
	if err != nil {
		return "", err
	}

	if execLog != "" {
		builder.WriteString("=== Workflow Execution Log ===\n")
		builder.WriteString(execLog)
		if !strings.HasSuffix(execLog, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	taskLogs, err := h.GetFullExecutionLog(executionID)
	if err != nil {
		return "", err
	}

	if taskLogs != "" {
		builder.WriteString(taskLogs)
	}

	return builder.String(), nil
}

// sortTasksByStartTime sorts tasks by their start time (nil times go last).
func sortTasksByStartTime(tasks []TaskExecution) {
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[i].StartedAt == nil && tasks[j].StartedAt == nil {
				continue
			}
			if tasks[i].StartedAt == nil {
				tasks[i], tasks[j] = tasks[j], tasks[i]
				continue
			}
			if tasks[j].StartedAt == nil {
				continue
			}
			if tasks[i].StartedAt.After(*tasks[j].StartedAt) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}
