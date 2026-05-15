// Package workflow provides access to the Dynatrace Automation Workflow API.
package workflow

import (
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
	ID          string                 `json:"id" table:"ID"`
	Title       string                 `json:"title" table:"TITLE"`
	Owner       string                 `json:"owner,omitempty" table:"-"`
	OwnerType   string                 `json:"ownerType,omitempty" table:"-"`
	Description string                 `json:"description,omitempty" table:"DESCRIPTION,wide"`
	Private     bool                   `json:"isPrivate" table:"-"`
	IsDeployed  bool                   `json:"isDeployed,omitempty" table:"DEPLOYED"`
	Tasks       map[string]interface{} `json:"tasks,omitempty" table:"-"`
	Trigger     map[string]interface{} `json:"trigger,omitempty" table:"-"`
	Actor       string                 `json:"actor,omitempty" table:"-"`
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
func (h *Handler) List(filters WorkflowFilters) (*WorkflowList, error) {
	var result WorkflowList

	req := h.client.HTTP().R().
		SetResult(&result)

	if filters.Owner != "" {
		req.SetQueryParam("owner", filters.Owner)
	}

	resp, err := req.Get("/platform/automation/v1/workflows")
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// Get retrieves a specific workflow.
func (h *Handler) Get(id string) (*Workflow, error) {
	var result Workflow

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// Delete deletes a workflow.
func (h *Handler) Delete(id string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return err
	}

	return nil
}

// Update updates a workflow.
func (h *Handler) Update(id string, data []byte) (*Workflow, error) {
	var result Workflow

	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Put(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))
	if err != nil {
		return nil, fmt.Errorf("update workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// Create creates a new workflow.
func (h *Handler) Create(data []byte) (*Workflow, error) {
	var result Workflow

	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Post("/platform/automation/v1/workflows")
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// HistoryRecord represents a workflow version history record.
type HistoryRecord struct {
	Version     int    `json:"version" table:"VERSION"`
	User        string `json:"user" table:"USER"`
	DateCreated string `json:"dateCreated" table:"CREATED"`
}

// HistoryList represents a paginated list of history records.
type HistoryList struct {
	Count   int             `json:"count"`
	Results []HistoryRecord `json:"results"`
}

// ListHistory retrieves version history for a workflow.
func (h *Handler) ListHistory(workflowID string) (*HistoryList, error) {
	var result HistoryList

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s/history", workflowID))
	if err != nil {
		return nil, fmt.Errorf("list workflow history: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetHistoryRecord retrieves a specific version of a workflow.
func (h *Handler) GetHistoryRecord(workflowID string, version int) (*Workflow, error) {
	var result Workflow

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s/history/%d", workflowID, version))
	if err != nil {
		return nil, fmt.Errorf("get workflow history record: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}

// RestoreHistory restores a workflow to a specific version.
func (h *Handler) RestoreHistory(workflowID string, version int) (*Workflow, error) {
	var result Workflow

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Post(fmt.Sprintf("/platform/automation/v1/workflows/%s/history/%d/restore", workflowID, version))
	if err != nil {
		return nil, fmt.Errorf("restore workflow: %w", err)
	}

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, err
	}

	return &result, nil
}
