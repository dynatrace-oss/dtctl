// Package workflow provides access to the Dynatrace Automation Workflow API.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles workflow resources.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new workflow handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// Workflow represents a workflow resource.
type Workflow struct {
	ID                   string                 `json:"id"`
	Title                string                 `json:"title"`
	IsDeployed           bool                   `json:"isDeployed"`
	Description          string                 `json:"description,omitempty"`
	Actor                string                 `json:"actor,omitempty"`
	Owner                string                 `json:"owner,omitempty"`
	OwnerType            string                 `json:"ownerType,omitempty"`
	Private              bool                   `json:"isPrivate"`
	SchemaVersion        int                    `json:"schemaVersion,omitempty"`
	Trigger              map[string]interface{} `json:"trigger,omitempty"`
	Result               *string                `json:"result,omitempty"`
	Type                 string                 `json:"type,omitempty"`
	Input                map[string]interface{} `json:"input,omitempty"`
	HourlyExecutionLimit *int                   `json:"hourlyExecutionLimit,omitempty"`
	Guide                *string                `json:"guide,omitempty"`
	Tasks                map[string]interface{} `json:"tasks"`
}

// WorkflowList represents a list of workflows.
type WorkflowList struct {
	Count   int        `json:"count"`
	Results []Workflow `json:"results"`
}

// WorkflowFilters contains filter options for listing workflows.
type WorkflowFilters struct {
	Owner string // Filter by owner ID (user ID)
}

// List retrieves workflows with optional filters.
func (h *Handler) List(ctx context.Context, filters WorkflowFilters) (*WorkflowList, error) {
	req := h.client.HTTP().R().SetContext(ctx)

	if filters.Owner != "" {
		req.SetQueryParam("owner", filters.Owner)
	}

	resp, err := req.Get("/platform/automation/v1/workflows")
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	var result WorkflowList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list workflows: parse response: %w", err)
	}

	return &result, nil
}

// Get retrieves a specific workflow.
func (h *Handler) Get(ctx context.Context, id string) (*Workflow, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	var result Workflow
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get workflow: parse response: %w", err)
	}

	return &result, nil
}

// GetRaw retrieves a workflow as raw JSON bytes.
func (h *Handler) GetRaw(ctx context.Context, id string) ([]byte, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	return resp.Body(), nil
}

// Delete deletes a workflow.
func (h *Handler) Delete(ctx context.Context, id string) error {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Delete(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}

	return nil
}

// Update updates a workflow.
func (h *Handler) Update(ctx context.Context, id string, data []byte) (*Workflow, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		Put(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return nil, fmt.Errorf("update workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("update workflow: %w", err)
	}

	var result Workflow
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("update workflow: parse response: %w", err)
	}

	return &result, nil
}

// Create creates a new workflow.
func (h *Handler) Create(ctx context.Context, data []byte) (*Workflow, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		Post("/platform/automation/v1/workflows")
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	var result Workflow
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("create workflow: parse response: %w", err)
	}

	return &result, nil
}

// HistoryRecord represents a workflow version history record.
type HistoryRecord struct {
	Version     int    `json:"version"`
	User        string `json:"user"`
	DateCreated string `json:"dateCreated"`
}

// HistoryList represents a paginated list of history records.
type HistoryList struct {
	Count   int             `json:"count"`
	Results []HistoryRecord `json:"results"`
}

// ListHistory retrieves version history for a workflow.
func (h *Handler) ListHistory(ctx context.Context, workflowID string) (*HistoryList, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s/history", workflowID))
	if err != nil {
		return nil, fmt.Errorf("list workflow history: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list workflow history: %w", err)
	}

	var result HistoryList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list workflow history: parse response: %w", err)
	}

	return &result, nil
}

// GetHistoryRecord retrieves a specific version of a workflow.
func (h *Handler) GetHistoryRecord(ctx context.Context, workflowID string, version int) (*Workflow, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s/history/%d", workflowID, version))
	if err != nil {
		return nil, fmt.Errorf("get workflow history record: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get workflow history record: %w", err)
	}

	var result Workflow
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get workflow history record: parse response: %w", err)
	}

	return &result, nil
}

// RestoreHistory restores a workflow to a specific version.
func (h *Handler) RestoreHistory(ctx context.Context, workflowID string, version int) (*Workflow, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Post(fmt.Sprintf("/platform/automation/v1/workflows/%s/history/%d/restore", workflowID, version))
	if err != nil {
		return nil, fmt.Errorf("restore workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("restore workflow: %w", err)
	}

	var result Workflow
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("restore workflow: parse response: %w", err)
	}

	return &result, nil
}
