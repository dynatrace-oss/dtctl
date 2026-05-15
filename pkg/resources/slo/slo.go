package slo

import (
	"encoding/json"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkslo "github.com/dynatrace-oss/dtctl/sdk/api/slo"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	SLO                = sdkslo.SLO
	Criteria           = sdkslo.Criteria
	SLOList            = sdkslo.SLOList
	Template           = sdkslo.Template
	TemplateVariable   = sdkslo.TemplateVariable
	TemplateList       = sdkslo.TemplateList
	EvaluationResult   = sdkslo.EvaluationResult
	EvaluationResponse = sdkslo.EvaluationResponse
)

// Handler handles SLO resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	sdk *sdkslo.Handler
}

// NewHandler creates a new SLO handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkslo.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List lists all SLOs with automatic pagination
func (h *Handler) List(filter string, chunkSize int64) (*SLOList, error) {
	return h.sdk.List(filter, chunkSize)
}

// Get gets a specific SLO by ID
func (h *Handler) Get(id string) (*SLO, error) {
	return h.sdk.Get(id)
}

// Create creates a new SLO
func (h *Handler) Create(data []byte) (*SLO, error) {
	return h.sdk.Create(data)
}

// Update updates an existing SLO
func (h *Handler) Update(id string, version string, data []byte) error {
	return h.sdk.Update(id, version, data)
}

// Delete deletes an SLO
func (h *Handler) Delete(id string, version string) error {
	return h.sdk.Delete(id, version)
}

// ListTemplates lists all SLO templates
func (h *Handler) ListTemplates(filter string) (*TemplateList, error) {
	return h.sdk.ListTemplates(filter)
}

// GetTemplate gets a specific SLO template by ID
func (h *Handler) GetTemplate(id string) (*Template, error) {
	return h.sdk.GetTemplate(id)
}

// Evaluate starts an SLO evaluation
func (h *Handler) Evaluate(id string) (*EvaluationResponse, error) {
	return h.sdk.Evaluate(id)
}

// PollEvaluation polls for SLO evaluation results
func (h *Handler) PollEvaluation(token string, timeoutMs int) (*EvaluationResponse, error) {
	return h.sdk.PollEvaluation(token, timeoutMs)
}

// GetRaw gets an SLO as raw JSON bytes (for editing)
func (h *Handler) GetRaw(id string) ([]byte, error) {
	sloObj, err := h.sdk.Get(id)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(sloObj, "", "  ")
}
