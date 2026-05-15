package analyzer

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles Davis analyzer resources
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new analyzer handler
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// AnalyzerCategory represents the category of an analyzer
type AnalyzerCategory struct {
	DisplayName string `json:"displayName"`
}

// Analyzer represents an analyzer definition
type Analyzer struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"displayName"`
	Description  string            `json:"description,omitempty"`
	Category     *AnalyzerCategory `json:"category,omitempty"`
	Type         string            `json:"type,omitempty"`
	BaseAnalyzer string            `json:"baseAnalyzer,omitempty"`
}

// AnalyzerList represents a list of analyzers
type AnalyzerList struct {
	Analyzers   []Analyzer `json:"analyzers"`
	TotalCount  int        `json:"totalCount"`
	NextPageKey string     `json:"nextPageKey,omitempty"`
}

// AnalyzerDefinition represents detailed analyzer definition
type AnalyzerDefinition struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"displayName"`
	Description  string            `json:"description,omitempty"`
	Category     *AnalyzerCategory `json:"category,omitempty"`
	Type         string            `json:"type,omitempty"`
	BaseAnalyzer string            `json:"baseAnalyzer,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Input        json.RawMessage   `json:"input,omitempty"`
	Output       json.RawMessage   `json:"output,omitempty"`
	AnalyzerCall json.RawMessage   `json:"analyzerCall,omitempty"`
}

// ExecuteRequest represents an analyzer execution request
type ExecuteRequest struct {
	Input map[string]interface{} `json:"input"`
}

// ExecuteResult represents an analyzer execution result
type ExecuteResult struct {
	RequestToken string          `json:"requestToken,omitempty"`
	TTLInSeconds int64           `json:"ttlInSeconds,omitempty"`
	Result       *AnalyzerResult `json:"result"`
}

// AnalyzerResult represents the result of an analyzer execution
type AnalyzerResult struct {
	ResultID        string                   `json:"resultId"`
	ResultStatus    string                   `json:"resultStatus"`
	ExecutionStatus string                   `json:"executionStatus"`
	Input           map[string]interface{}   `json:"input,omitempty"`
	Output          []map[string]interface{} `json:"output,omitempty"`
	Data            []map[string]interface{} `json:"data,omitempty"`
	Logs            []ExecutionLog           `json:"logs,omitempty"`
}

// ExecutionLog represents an execution log entry
type ExecutionLog struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// ValidationResult represents the result of input validation
type ValidationResult struct {
	Valid   bool                   `json:"valid"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// List retrieves all available analyzers
func (h *Handler) List(filter string) (*AnalyzerList, error) {
	req := h.client.HTTP().R()

	if filter != "" {
		req.SetQueryParam("filter", filter)
	}
	req.SetQueryParam("add-fields", "category,type")

	resp, err := req.Get("/platform/davis/analyzers/v1/analyzers")
	if err != nil {
		return nil, fmt.Errorf("failed to list analyzers: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to list analyzers: %w", err)
	}

	var result AnalyzerList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list analyzers: parse response: %w", err)
	}

	return &result, nil
}

// Get retrieves a specific analyzer definition
func (h *Handler) Get(name string) (*AnalyzerDefinition, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s", name))
	if err != nil {
		return nil, fmt.Errorf("failed to get analyzer: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		var apiErr *httpclient.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return nil, fmt.Errorf("analyzer %q not found", name)
		}
		return nil, fmt.Errorf("failed to get analyzer: %w", err)
	}

	var result AnalyzerDefinition
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get analyzer: parse response: %w", err)
	}

	return &result, nil
}

// GetDocumentation retrieves the documentation for an analyzer
func (h *Handler) GetDocumentation(name string) (string, error) {
	resp, err := h.client.HTTP().R().
		SetHeader("Accept", "text/markdown").
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s/documentation", name))
	if err != nil {
		return "", fmt.Errorf("failed to get analyzer documentation: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		var apiErr *httpclient.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return "", fmt.Errorf("documentation for analyzer %q not found", name)
		}
		return "", fmt.Errorf("failed to get analyzer documentation: %w", err)
	}
	return resp.String(), nil
}

// GetInputSchema retrieves the JSON schema for analyzer input
func (h *Handler) GetInputSchema(name string) (map[string]interface{}, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s/json-schema/input", name))
	if err != nil {
		return nil, fmt.Errorf("failed to get input schema: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to get input schema: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get input schema: parse response: %w", err)
	}
	return result, nil
}

// GetResultSchema retrieves the JSON schema for analyzer result
func (h *Handler) GetResultSchema(name string) (map[string]interface{}, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s/json-schema/result", name))
	if err != nil {
		return nil, fmt.Errorf("failed to get result schema: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to get result schema: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get result schema: parse response: %w", err)
	}
	return result, nil
}

// Execute runs an analyzer with the given input
func (h *Handler) Execute(name string, input map[string]interface{}, timeoutSeconds int) (*ExecuteResult, error) {
	req := h.client.HTTP().R()

	if timeoutSeconds > 0 {
		req.SetQueryParam("timeout-seconds", fmt.Sprintf("%d", timeoutSeconds))
	}

	var result ExecuteResult
	resp, err := req.
		SetBody(input).
		Post(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:execute", name))
	if err != nil {
		return nil, fmt.Errorf("failed to execute analyzer: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to execute analyzer: %w", err)
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("execute analyzer: parse response: %w", err)
	}

	return &result, nil
}

// Default polling parameters for ExecuteAndWait.
const (
	defaultInitialTimeout = 30 // seconds for the initial execute call
	defaultPollTimeout    = 10 // seconds for each poll call
	defaultPollInterval   = 2 * time.Second
)

// ExecuteAndWait runs an analyzer and waits for completion
func (h *Handler) ExecuteAndWait(name string, input map[string]interface{}, maxWaitSeconds int) (*ExecuteResult, error) {
	// Start execution with initial timeout
	result, err := h.Execute(name, input, defaultInitialTimeout)
	if err != nil {
		return nil, err
	}

	// If already completed, return
	if result.Result != nil && result.Result.ExecutionStatus == "COMPLETED" {
		return result, nil
	}

	// Poll for completion if we have a request token
	if result.RequestToken == "" {
		return result, nil
	}

	startTime := time.Now()
	maxDuration := time.Duration(maxWaitSeconds) * time.Second

	for {
		if time.Since(startTime) > maxDuration {
			return nil, fmt.Errorf("analyzer execution timed out after %d seconds", maxWaitSeconds)
		}

		pollResult, err := h.Poll(name, result.RequestToken, defaultPollTimeout)
		if err != nil {
			return nil, err
		}

		if pollResult.Result != nil && pollResult.Result.ExecutionStatus == "COMPLETED" {
			return pollResult, nil
		}

		if pollResult.Result != nil && pollResult.Result.ExecutionStatus == "ABORTED" {
			return pollResult, fmt.Errorf("analyzer execution was aborted")
		}

		time.Sleep(defaultPollInterval)
	}
}

// Poll polls for the result of a started analyzer execution
func (h *Handler) Poll(name string, requestToken string, timeoutSeconds int) (*ExecuteResult, error) {
	req := h.client.HTTP().R().
		SetQueryParam("request-token", requestToken)

	if timeoutSeconds > 0 {
		req.SetQueryParam("timeout-seconds", fmt.Sprintf("%d", timeoutSeconds))
	}

	var result ExecuteResult
	resp, err := req.
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:poll", name))
	if err != nil {
		return nil, fmt.Errorf("failed to poll analyzer: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		var apiErr *httpclient.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 410 {
			return nil, fmt.Errorf("analyzer result expired or already consumed")
		}
		return nil, fmt.Errorf("failed to poll analyzer: %w", err)
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("poll analyzer: parse response: %w", err)
	}

	return &result, nil
}

// Cancel cancels a running analyzer execution
func (h *Handler) Cancel(name string, requestToken string) (*ExecuteResult, error) {
	req := h.client.HTTP().R().
		SetQueryParam("request-token", requestToken)

	resp, err := req.
		Post(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:cancel", name))
	if err != nil {
		return nil, fmt.Errorf("failed to cancel analyzer: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to cancel analyzer: %w", err)
	}

	var result ExecuteResult
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("cancel analyzer: parse response: %w", err)
	}
	return &result, nil
}

// Validate validates the input for an analyzer execution
func (h *Handler) Validate(name string, input map[string]interface{}) (*ValidationResult, error) {
	resp, err := h.client.HTTP().R().
		SetBody(input).
		Post(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:validate", name))
	if err != nil {
		return nil, fmt.Errorf("failed to validate analyzer input: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("failed to validate analyzer input: %w", err)
	}

	var result ValidationResult
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("validate analyzer input: parse response: %w", err)
	}
	return &result, nil
}
