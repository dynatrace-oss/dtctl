package slo

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles SLO resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new SLO handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// SLO represents a service-level objective
type SLO struct {
	ID          string                 `json:"id" table:"ID"`
	Name        string                 `json:"name" table:"NAME"`
	Description string                 `json:"description,omitempty" table:"DESCRIPTION,wide"`
	Version     string                 `json:"version,omitempty" table:"-"`
	Criteria    []Criteria             `json:"criteria,omitempty" table:"-"`
	Tags        []string               `json:"tags,omitempty" table:"-"`
	CustomSli   map[string]interface{} `json:"customSli,omitempty" table:"-"`
	ExternalID  string                 `json:"externalId,omitempty" table:"-"`
}

// Criteria represents SLO criteria
type Criteria struct {
	TimeframeFrom string   `json:"timeframeFrom"`
	TimeframeTo   string   `json:"timeframeTo,omitempty"`
	Target        float64  `json:"target"`
	Warning       *float64 `json:"warning,omitempty"`
}

// SLOList represents a list of SLOs
type SLOList struct {
	SLOs        []SLO  `json:"slos"`
	TotalCount  int    `json:"totalCount"`
	NextPageKey string `json:"nextPageKey,omitempty"`
}

// Template represents an SLO objective template
type Template struct {
	ID              string             `json:"id" table:"ID"`
	Name            string             `json:"name" table:"NAME"`
	Description     string             `json:"description,omitempty" table:"DESCRIPTION,wide"`
	BuiltIn         bool               `json:"builtIn" table:"BUILTIN"`
	ApplicableScope string             `json:"applicableScope,omitempty" table:"SCOPE,wide"`
	Indicator       string             `json:"indicator,omitempty" table:"-"`
	Variables       []TemplateVariable `json:"variables,omitempty" table:"-"`
	Version         string             `json:"version,omitempty" table:"-"`
}

// TemplateVariable represents a variable in an SLO template
type TemplateVariable struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

// TemplateList represents a list of templates
type TemplateList struct {
	Items       []Template `json:"items"`
	TotalCount  int        `json:"totalCount"`
	NextPageKey string     `json:"nextPageKey,omitempty"`
}

// EvaluationResult represents an SLO evaluation result
type EvaluationResult struct {
	Criteria    string   `json:"criteria" table:"CRITERIA"`
	Status      string   `json:"status" table:"STATUS"`
	Value       *float64 `json:"value,omitempty" table:"VALUE"`
	ErrorBudget *float64 `json:"errorBudget,omitempty" table:"ERROR_BUDGET"`
	Message     string   `json:"message,omitempty" table:"MESSAGE,wide"`
}

// EvaluationResponse represents the response from SLO evaluation
type EvaluationResponse struct {
	Definition        *SLO               `json:"definition,omitempty"`
	EvaluationResults []EvaluationResult `json:"evaluationResults,omitempty"`
	EvaluationToken   string             `json:"evaluationToken,omitempty"`
	TTLSeconds        int64              `json:"ttlSeconds,omitempty"`
}

// List lists all SLOs with automatic pagination
func (h *Handler) List(filter string, chunkSize int64) (*SLOList, error) {
	var allSLOs []SLO
	var totalCount int
	nextPageKey := ""

	for {
		var result SLOList
		req := h.client.HTTP().R().SetResult(&result)

		if filter != "" {
			req.SetQueryParam("filter", filter)
		}

		// Set page size if chunking is enabled (chunkSize > 0)
		if chunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		resp, err := req.Get("/platform/slo/v1/slos")
		if err != nil {
			return nil, fmt.Errorf("failed to list SLOs: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list SLOs: status %d: %s", resp.StatusCode(), resp.String())
		}

		allSLOs = append(allSLOs, result.SLOs...)
		totalCount = result.TotalCount

		// If chunking is disabled (chunkSize == 0), return first page only
		if chunkSize == 0 {
			return &result, nil
		}

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &SLOList{
		SLOs:       allSLOs,
		TotalCount: totalCount,
	}, nil
}

// Get gets a specific SLO by ID
func (h *Handler) Get(id string) (*SLO, error) {
	var result SLO

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/slo/v1/slos/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get SLO: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("SLO %q not found", id)
		case 403:
			return nil, fmt.Errorf("access denied to SLO %q", id)
		default:
			return nil, fmt.Errorf("failed to get SLO: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// Create creates a new SLO
func (h *Handler) Create(data []byte) (*SLO, error) {
	var result SLO

	resp, err := h.client.HTTP().R().
		SetBody(data).
		SetResult(&result).
		SetHeader("Content-Type", "application/json").
		Post("/platform/slo/v1/slos")

	if err != nil {
		return nil, fmt.Errorf("failed to create SLO: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid SLO configuration: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create SLO")
		default:
			return nil, fmt.Errorf("failed to create SLO: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// Update updates an existing SLO
func (h *Handler) Update(id string, version string, data []byte) error {
	resp, err := h.client.HTTP().R().
		SetBody(data).
		SetHeader("Content-Type", "application/json").
		SetQueryParam("optimistic-locking-version", version).
		Put(fmt.Sprintf("/platform/slo/v1/slos/%s", id))

	if err != nil {
		return fmt.Errorf("failed to update SLO: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("invalid SLO configuration: %s", resp.String())
		case 403:
			return fmt.Errorf("access denied to update SLO %q", id)
		case 404:
			return fmt.Errorf("SLO %q not found", id)
		case 409:
			return fmt.Errorf("SLO version conflict (object was modified)")
		default:
			return fmt.Errorf("failed to update SLO: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Delete deletes an SLO
func (h *Handler) Delete(id string, version string) error {
	resp, err := h.client.HTTP().R().
		SetQueryParam("optimistic-locking-version", version).
		Delete(fmt.Sprintf("/platform/slo/v1/slos/%s", id))

	if err != nil {
		return fmt.Errorf("failed to delete SLO: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 403:
			return fmt.Errorf("access denied to delete SLO %q", id)
		case 404:
			return fmt.Errorf("SLO %q not found", id)
		case 409:
			return fmt.Errorf("SLO version conflict (object was modified)")
		default:
			return fmt.Errorf("failed to delete SLO: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// ListTemplates lists all SLO templates
func (h *Handler) ListTemplates(filter string) (*TemplateList, error) {
	var result TemplateList

	req := h.client.HTTP().R().SetResult(&result)

	if filter != "" {
		req.SetQueryParam("filter", filter)
	}

	resp, err := req.Get("/platform/slo/v1/objective-templates")

	if err != nil {
		return nil, fmt.Errorf("failed to list SLO templates: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list SLO templates: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// GetTemplate gets a specific SLO template by ID
func (h *Handler) GetTemplate(id string) (*Template, error) {
	var result Template

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/slo/v1/objective-templates/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get SLO template: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("SLO template %q not found", id)
		default:
			return nil, fmt.Errorf("failed to get SLO template: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// Evaluate starts an SLO evaluation
func (h *Handler) Evaluate(id string) (*EvaluationResponse, error) {
	body := map[string]interface{}{
		"id": id,
	}

	var result EvaluationResponse

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetResult(&result).
		SetHeader("Content-Type", "application/json").
		Post("/platform/slo/v1/slos/evaluation:start")

	if err != nil {
		return nil, fmt.Errorf("failed to evaluate SLO: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("SLO %q not found", id)
		default:
			return nil, fmt.Errorf("failed to evaluate SLO: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// PollEvaluation polls for SLO evaluation results
func (h *Handler) PollEvaluation(token string, timeoutMs int) (*EvaluationResponse, error) {
	var result EvaluationResponse

	req := h.client.HTTP().R().
		SetResult(&result).
		SetQueryParam("evaluation-token", token)

	if timeoutMs > 0 {
		req.SetQueryParam("request-timeout-milliseconds", fmt.Sprintf("%d", timeoutMs))
	}

	resp, err := req.Get("/platform/slo/v1/slos/evaluation:poll")

	if err != nil {
		return nil, fmt.Errorf("failed to poll SLO evaluation: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 410:
			return nil, fmt.Errorf("evaluation token expired or invalid")
		default:
			return nil, fmt.Errorf("failed to poll SLO evaluation: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// GetRaw gets an SLO as raw JSON bytes (for editing)
func (h *Handler) GetRaw(id string) ([]byte, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/slo/v1/slos/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get SLO: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("SLO %q not found", id)
		default:
			return nil, fmt.Errorf("failed to get SLO: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	// Pretty print the JSON
	var data interface{}
	if err := json.Unmarshal(resp.Body(), &data); err != nil {
		return resp.Body(), nil
	}

	return json.MarshalIndent(data, "", "  ")
}
