package workflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Execution represents a workflow execution
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

// ExecutionList represents a list of executions
type ExecutionList struct {
	Count   int         `json:"count"`
	Results []Execution `json:"results"`
}

// ExecutionHandler handles execution resources
type ExecutionHandler struct {
	client *client.Client
}

// NewExecutionHandler creates a new execution handler
func NewExecutionHandler(c *client.Client) *ExecutionHandler {
	return &ExecutionHandler{client: c}
}

// List retrieves all executions with optional workflow filter
func (h *ExecutionHandler) List(workflowID string) (*ExecutionList, error) {
	var result ExecutionList

	req := h.client.HTTP().R().SetResult(&result)

	if workflowID != "" {
		req.SetQueryParam("workflow", workflowID)
	}

	resp, err := req.Get("/platform/automation/v1/executions")
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list executions: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// Get retrieves a specific execution
func (h *ExecutionHandler) Get(id string) (*Execution, error) {
	var result Execution

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get execution: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// Cancel cancels an active execution
func (h *ExecutionHandler) Cancel(id string) error {
	resp, err := h.client.HTTP().R().
		Post(fmt.Sprintf("/platform/automation/v1/executions/%s/cancel", id))

	if err != nil {
		return fmt.Errorf("failed to cancel execution: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to cancel execution: status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

// TaskExecution represents a task execution within a workflow execution
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

// TaskExecutionMap is a map of task name to task execution
type TaskExecutionMap map[string]TaskExecution

// ListTasks retrieves all task executions for a workflow execution
func (h *ExecutionHandler) ListTasks(executionID string) ([]TaskExecution, error) {
	var result TaskExecutionMap

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks", executionID))

	if err != nil {
		return nil, fmt.Errorf("failed to list task executions: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list task executions: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Convert map to slice
	tasks := make([]TaskExecution, 0, len(result))
	for _, task := range result {
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetTaskLog retrieves the log output of a specific task execution
func (h *ExecutionHandler) GetTaskLog(executionID, taskName string) (string, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks/%s/log", executionID, taskName))

	if err != nil {
		return "", fmt.Errorf("failed to get task log: %w", err)
	}

	if resp.IsError() {
		return "", fmt.Errorf("failed to get task log: status %d: %s", resp.StatusCode(), resp.String())
	}

	// The API returns a JSON-encoded string, so we need to unquote it
	// Use resp.Body() to avoid potential truncation of large logs
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

// GetExecutionLog retrieves the combined log output of all tasks in an execution
func (h *ExecutionHandler) GetExecutionLog(executionID string) (string, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/log", executionID))

	if err != nil {
		return "", fmt.Errorf("failed to get execution log: %w", err)
	}

	if resp.IsError() {
		return "", fmt.Errorf("failed to get execution log: status %d: %s", resp.StatusCode(), resp.String())
	}

	// The API returns a JSON-encoded string, so we need to unquote it
	// Use resp.Body() to avoid potential truncation of large logs
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

// GetFullExecutionLog retrieves logs for all tasks in an execution, formatted with headers
func (h *ExecutionHandler) GetFullExecutionLog(executionID string) (string, error) {
	// Get all tasks
	tasks, err := h.ListTasks(executionID)
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
		log, err := h.GetTaskLog(executionID, task.Name)
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
