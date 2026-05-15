package workflow

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkworkflow "github.com/dynatrace-oss/dtctl/sdk/api/workflow"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	Workflow        = sdkworkflow.Workflow
	WorkflowList    = sdkworkflow.WorkflowList
	WorkflowFilters = sdkworkflow.WorkflowFilters
	HistoryRecord   = sdkworkflow.HistoryRecord
	HistoryList     = sdkworkflow.HistoryList
)

// Handler handles workflow resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	sdk    *sdkworkflow.Handler
	client *client.Client
}

// NewHandler creates a new workflow handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk:    sdkworkflow.NewHandler(httpclient.Wrap(c.HTTP())),
		client: c,
	}
}

// List retrieves workflows with optional filters
func (h *Handler) List(filters WorkflowFilters) (*WorkflowList, error) {
	return h.sdk.List(filters)
}

// Get retrieves a specific workflow
func (h *Handler) Get(id string) (*Workflow, error) {
	return h.sdk.Get(id)
}

// Delete deletes a workflow
func (h *Handler) Delete(id string) error {
	return h.sdk.Delete(id)
}

// GetRaw retrieves a workflow as raw JSON (for editing)
func (h *Handler) GetRaw(id string) ([]byte, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/automation/v1/workflows/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get workflow: status %d: %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// Update updates a workflow
func (h *Handler) Update(id string, data []byte) (*Workflow, error) {
	return h.sdk.Update(id, data)
}

// Create creates a new workflow
func (h *Handler) Create(data []byte) (*Workflow, error) {
	return h.sdk.Create(data)
}

// ListHistory retrieves version history for a workflow
func (h *Handler) ListHistory(workflowID string) (*HistoryList, error) {
	return h.sdk.ListHistory(workflowID)
}

// GetHistoryRecord retrieves a specific version of a workflow
func (h *Handler) GetHistoryRecord(workflowID string, version int) (*Workflow, error) {
	return h.sdk.GetHistoryRecord(workflowID, version)
}

// RestoreHistory restores a workflow to a specific version
func (h *Handler) RestoreHistory(workflowID string, version int) (*Workflow, error) {
	return h.sdk.RestoreHistory(workflowID, version)
}
