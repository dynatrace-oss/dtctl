package slo

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles SLO resources
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new SLO handler
func NewHandler(c *httpclient.Client) *Handler {
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

		params := httpclient.PaginationParams{
			Style:         httpclient.PaginationDefault,
			PageKeyParam:  "page-key",
			PageSizeParam: "page-size",
			NextPageKey:   nextPageKey,
			PageSize:      chunkSize,
			Filters:       map[string]string{"filter": filter},
		}.QueryParams()
		req.SetQueryParamsFromValues(params)

		resp, err := req.Get("/platform/slo/v1/slos")
		if err != nil {
			return nil, fmt.Errorf("failed to list SLOs: %w", err)
		}

		if err := httpclient.CheckResponse(resp); err != nil {
			return nil, fmt.Errorf("failed to list SLOs: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to get SLO: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to create SLO: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("failed to update SLO: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("failed to delete SLO: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to list SLO templates: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to get SLO template: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to evaluate SLO: %w", err)
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

	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to poll SLO evaluation: %w", err)
	}

	return &result, nil
}

