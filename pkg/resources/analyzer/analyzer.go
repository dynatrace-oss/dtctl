package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles Davis analyzer resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new analyzer handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// AnalyzerCategory represents the category of an analyzer
type AnalyzerCategory struct {
	DisplayName string `json:"displayName"`
}

// Analyzer represents an analyzer definition
type Analyzer struct {
	Name         string            `json:"name" table:"NAME"`
	DisplayName  string            `json:"displayName" table:"DISPLAY NAME"`
	Description  string            `json:"description,omitempty" table:"DESCRIPTION,wide"`
	Category     *AnalyzerCategory `json:"category,omitempty" table:"-"`
	CategoryName string            `json:"-" table:"CATEGORY"`
	Type         string            `json:"type,omitempty" table:"TYPE"`
	BaseAnalyzer string            `json:"baseAnalyzer,omitempty" table:"-"`
}

// AnalyzerList represents a list of analyzers
type AnalyzerList struct {
	Analyzers   []Analyzer `json:"analyzers"`
	TotalCount  int        `json:"totalCount"`
	NextPageKey string     `json:"nextPageKey,omitempty"`
}

// AnalyzerDefinition represents detailed analyzer definition
type AnalyzerDefinition struct {
	Name         string            `json:"name" table:"NAME"`
	DisplayName  string            `json:"displayName" table:"DISPLAY NAME"`
	Description  string            `json:"description,omitempty" table:"DESCRIPTION"`
	Category     *AnalyzerCategory `json:"category,omitempty" table:"-"`
	CategoryName string            `json:"-" table:"CATEGORY"`
	Type         string            `json:"type,omitempty" table:"TYPE"`
	BaseAnalyzer string            `json:"baseAnalyzer,omitempty" table:"BASE ANALYZER"`
	Labels       []string          `json:"labels,omitempty" table:"-"`
	Input        json.RawMessage   `json:"input,omitempty" table:"-"`
	Output       json.RawMessage   `json:"output,omitempty" table:"-"`
	AnalyzerCall json.RawMessage   `json:"analyzerCall,omitempty" table:"-"`
}

// ExecuteRequest represents an analyzer execution request
type ExecuteRequest struct {
	Input map[string]interface{} `json:"input"`
}

// ExecuteResult represents an analyzer execution result
type ExecuteResult struct {
	RequestToken string          `json:"requestToken,omitempty" table:"REQUEST TOKEN,wide"`
	TTLInSeconds int64           `json:"ttlInSeconds,omitempty" table:"-"`
	Result       *AnalyzerResult `json:"result" table:"-"`
	// Flattened fields for table display
	ResultID        string `json:"-" table:"RESULT ID"`
	ResultStatus    string `json:"-" table:"STATUS"`
	ExecutionStatus string `json:"-" table:"EXECUTION"`
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

	var result AnalyzerList
	resp, err := req.
		SetResult(&result).
		Get("/platform/davis/analyzers/v1/analyzers")

	if err != nil {
		return nil, fmt.Errorf("failed to list analyzers: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list analyzers: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Populate CategoryName for table display
	for i := range result.Analyzers {
		if result.Analyzers[i].Category != nil {
			result.Analyzers[i].CategoryName = result.Analyzers[i].Category.DisplayName
		}
	}

	return &result, nil
}

// Get retrieves a specific analyzer definition
func (h *Handler) Get(name string) (*AnalyzerDefinition, error) {
	var result AnalyzerDefinition

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s", name))

	if err != nil {
		return nil, fmt.Errorf("failed to get analyzer: %w", err)
	}

	if resp.IsError() {
		if resp.StatusCode() == 404 {
			return nil, fmt.Errorf("analyzer %q not found", name)
		}
		return nil, fmt.Errorf("failed to get analyzer: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Populate CategoryName for table display
	if result.Category != nil {
		result.CategoryName = result.Category.DisplayName
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

	if resp.IsError() {
		if resp.StatusCode() == 404 {
			return "", fmt.Errorf("documentation for analyzer %q not found", name)
		}
		return "", fmt.Errorf("failed to get analyzer documentation: status %d: %s", resp.StatusCode(), resp.String())
	}

	return resp.String(), nil
}

// GetInputSchema retrieves the JSON schema for analyzer input
func (h *Handler) GetInputSchema(name string) (map[string]interface{}, error) {
	var result map[string]interface{}

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s/json-schema/input", name))

	if err != nil {
		return nil, fmt.Errorf("failed to get input schema: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get input schema: status %d: %s", resp.StatusCode(), resp.String())
	}

	return result, nil
}

// GetResultSchema retrieves the JSON schema for analyzer result
func (h *Handler) GetResultSchema(name string) (map[string]interface{}, error) {
	var result map[string]interface{}

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s/json-schema/result", name))

	if err != nil {
		return nil, fmt.Errorf("failed to get result schema: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get result schema: status %d: %s", resp.StatusCode(), resp.String())
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
		SetResult(&result).
		Post(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:execute", name))

	if err != nil {
		return nil, fmt.Errorf("failed to execute analyzer: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to execute analyzer: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Populate flattened fields for table display
	result.populateTableFields()

	return &result, nil
}

// populateTableFields copies Result fields to top-level for table display
func (r *ExecuteResult) populateTableFields() {
	if r.Result != nil {
		r.ResultID = r.Result.ResultID
		r.ResultStatus = r.Result.ResultStatus
		r.ExecutionStatus = r.Result.ExecutionStatus
	}
}

// ExecuteAndWait runs an analyzer and waits for completion
func (h *Handler) ExecuteAndWait(name string, input map[string]interface{}, maxWaitSeconds int) (*ExecuteResult, error) {
	// Start execution with initial timeout
	result, err := h.Execute(name, input, 30)
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

		pollResult, err := h.Poll(name, result.RequestToken, 10)
		if err != nil {
			return nil, err
		}

		if pollResult.Result != nil && pollResult.Result.ExecutionStatus == "COMPLETED" {
			return pollResult, nil
		}

		if pollResult.Result != nil && pollResult.Result.ExecutionStatus == "ABORTED" {
			return pollResult, fmt.Errorf("analyzer execution was aborted")
		}

		time.Sleep(2 * time.Second)
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
		SetResult(&result).
		Get(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:poll", name))

	if err != nil {
		return nil, fmt.Errorf("failed to poll analyzer: %w", err)
	}

	if resp.IsError() {
		if resp.StatusCode() == 410 {
			return nil, fmt.Errorf("analyzer result expired or already consumed")
		}
		return nil, fmt.Errorf("failed to poll analyzer: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Populate flattened fields for table display
	result.populateTableFields()

	return &result, nil
}

// Cancel cancels a running analyzer execution
func (h *Handler) Cancel(name string, requestToken string) (*ExecuteResult, error) {
	req := h.client.HTTP().R().
		SetQueryParam("request-token", requestToken)

	var result ExecuteResult
	resp, err := req.
		SetResult(&result).
		Post(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:cancel", name))

	if err != nil {
		return nil, fmt.Errorf("failed to cancel analyzer: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to cancel analyzer: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// Validate validates the input for an analyzer execution
func (h *Handler) Validate(name string, input map[string]interface{}) (*ValidationResult, error) {
	var result ValidationResult

	resp, err := h.client.HTTP().R().
		SetBody(input).
		SetResult(&result).
		Post(fmt.Sprintf("/platform/davis/analyzers/v1/analyzers/%s:validate", name))

	if err != nil {
		return nil, fmt.Errorf("failed to validate analyzer input: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to validate analyzer input: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// ParseInputFromFile reads and parses analyzer input from a file
func ParseInputFromFile(filename string) (map[string]interface{}, error) {
	content, err := readFile(filename)
	if err != nil {
		return nil, err
	}

	var input map[string]interface{}
	if err := json.Unmarshal(content, &input); err != nil {
		return nil, fmt.Errorf("failed to parse input file: %w", err)
	}

	return input, nil
}

// readFile reads content from a file
func readFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
