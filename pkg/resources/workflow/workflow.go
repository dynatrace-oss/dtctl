package workflow

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkworkflow "github.com/dynatrace-oss/dtctl/sdk/api/workflow"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types that have no table tags.
type WorkflowFilters = sdkworkflow.WorkflowFilters

// Workflow represents a workflow resource (CLI version with table tags).
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

// HistoryRecord represents a workflow version history record (CLI version with table tags).
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

// fromSDKWorkflow converts an SDK Workflow to a CLI Workflow.
func fromSDKWorkflow(s *sdkworkflow.Workflow) Workflow {
	return Workflow{
		ID:          s.ID,
		Title:       s.Title,
		Owner:       s.Owner,
		OwnerType:   s.OwnerType,
		Description: s.Description,
		Private:     s.Private,
		IsDeployed:  s.IsDeployed,
		Tasks:       s.Tasks,
		Trigger:     s.Trigger,
		Actor:       s.Actor,
	}
}

// fromSDKHistoryRecord converts an SDK HistoryRecord to a CLI HistoryRecord.
func fromSDKHistoryRecord(s *sdkworkflow.HistoryRecord) HistoryRecord {
	return HistoryRecord{
		Version:     s.Version,
		User:        s.User,
		DateCreated: s.DateCreated,
	}
}

// Handler handles workflow resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	sdk *sdkworkflow.Handler
}

// NewHandler creates a new workflow handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkworkflow.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List retrieves workflows with optional filters
func (h *Handler) List(filters WorkflowFilters) (*WorkflowList, error) {
	sdkResult, err := h.sdk.List(filters)
	if err != nil {
		return nil, err
	}
	results := make([]Workflow, len(sdkResult.Results))
	for i := range sdkResult.Results {
		results[i] = fromSDKWorkflow(&sdkResult.Results[i])
	}
	return &WorkflowList{Count: sdkResult.Count, Results: results}, nil
}

// Get retrieves a specific workflow
func (h *Handler) Get(id string) (*Workflow, error) {
	sdkResult, err := h.sdk.Get(id)
	if err != nil {
		return nil, err
	}
	w := fromSDKWorkflow(sdkResult)
	return &w, nil
}

// Delete deletes a workflow
func (h *Handler) Delete(id string) error {
	return h.sdk.Delete(id)
}

// GetRaw retrieves a workflow as raw JSON (for editing)
func (h *Handler) GetRaw(id string) ([]byte, error) {
	return h.sdk.GetRaw(id)
}

// Update updates a workflow
func (h *Handler) Update(id string, data []byte) (*Workflow, error) {
	sdkResult, err := h.sdk.Update(id, data)
	if err != nil {
		return nil, err
	}
	w := fromSDKWorkflow(sdkResult)
	return &w, nil
}

// Create creates a new workflow
func (h *Handler) Create(data []byte) (*Workflow, error) {
	sdkResult, err := h.sdk.Create(data)
	if err != nil {
		return nil, err
	}
	w := fromSDKWorkflow(sdkResult)
	return &w, nil
}

// ListHistory retrieves version history for a workflow
func (h *Handler) ListHistory(workflowID string) (*HistoryList, error) {
	sdkResult, err := h.sdk.ListHistory(workflowID)
	if err != nil {
		return nil, err
	}
	results := make([]HistoryRecord, len(sdkResult.Results))
	for i := range sdkResult.Results {
		results[i] = fromSDKHistoryRecord(&sdkResult.Results[i])
	}
	return &HistoryList{Count: sdkResult.Count, Results: results}, nil
}

// GetHistoryRecord retrieves a specific version of a workflow
func (h *Handler) GetHistoryRecord(workflowID string, version int) (*Workflow, error) {
	sdkResult, err := h.sdk.GetHistoryRecord(workflowID, version)
	if err != nil {
		return nil, err
	}
	w := fromSDKWorkflow(sdkResult)
	return &w, nil
}

// RestoreHistory restores a workflow to a specific version
func (h *Handler) RestoreHistory(workflowID string, version int) (*Workflow, error) {
	sdkResult, err := h.sdk.RestoreHistory(workflowID, version)
	if err != nil {
		return nil, err
	}
	w := fromSDKWorkflow(sdkResult)
	return &w, nil
}
