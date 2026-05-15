package workflow

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Execution represents a workflow execution.
type Execution struct {
	ID          string     `json:"id"`
	Workflow    string     `json:"workflow"`
	Title       string     `json:"title"`
	State       string     `json:"state"`
	StateInfo   *string    `json:"stateInfo,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	Runtime     int        `json:"runtime,omitempty"`
	Trigger     *string    `json:"trigger,omitempty"`
	TriggerType string     `json:"triggerType,omitempty"`
	User        *string    `json:"user,omitempty"`
	Actor       string     `json:"actor,omitempty"`
	Input       any        `json:"input,omitempty"`
	Params      any        `json:"params,omitempty"`
	Result      any        `json:"result,omitempty"`
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
	req := h.client.HTTP().R()

	if workflowID != "" {
		req.SetQueryParam("workflow", workflowID)
	}

	resp, err := req.Get("/platform/automation/v1/executions")
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}

	var result ExecutionList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list executions: parse response: %w", err)
	}

	return &result, nil
}

// Get retrieves a specific execution.
func (h *ExecutionHandler) Get(id string) (*Execution, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}

	var result Execution
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get execution: parse response: %w", err)
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
		return fmt.Errorf("cancel execution: %w", err)
	}

	return nil
}

// TaskExecution represents a task execution within a workflow execution.
type TaskExecution struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	Runtime   int        `json:"runtime,omitempty"`
	StateInfo *string    `json:"stateInfo,omitempty"`
	Input     any        `json:"input,omitempty"`
	Result    any        `json:"result,omitempty"`
}

// TaskExecutionMap is a map of task name to task execution.
type TaskExecutionMap map[string]TaskExecution

// ListTasks retrieves all task executions for a workflow execution.
func (h *ExecutionHandler) ListTasks(executionID string) ([]TaskExecution, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks", executionID))
	if err != nil {
		return nil, fmt.Errorf("list task executions: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list task executions: %w", err)
	}

	var result TaskExecutionMap
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list task executions: parse response: %w", err)
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
		return "", fmt.Errorf("get task log: %w", err)
	}

	// The API returns a JSON-encoded string, so we need to unquote it.
	return unquoteJSONString(resp.Body())
}

// GetTaskResult retrieves the structured return value of a specific task execution.
func (h *ExecutionHandler) GetTaskResult(executionID, taskName string) (any, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/executions/%s/tasks/%s/result", executionID, taskName))
	if err != nil {
		return nil, fmt.Errorf("get task result: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get task result: %w", err)
	}

	var result any
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get task result: parse response: %w", err)
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
		return "", fmt.Errorf("get execution log: %w", err)
	}

	// The API returns a JSON-encoded string, so we need to unquote it.
	return unquoteJSONString(resp.Body())
}

// unquoteJSONString attempts to JSON-unmarshal a byte slice as a quoted string.
// If the input is a valid JSON string, the unquoted value is returned.
// Otherwise the raw bytes are returned as-is.
func unquoteJSONString(data []byte) (string, error) {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s, nil
	}
	return string(data), nil
}
