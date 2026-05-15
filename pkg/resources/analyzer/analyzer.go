package analyzer

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkana "github.com/dynatrace-oss/dtctl/sdk/api/analyzer"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	AnalyzerCategory   = sdkana.AnalyzerCategory
	Analyzer           = sdkana.Analyzer
	AnalyzerList       = sdkana.AnalyzerList
	AnalyzerDefinition = sdkana.AnalyzerDefinition
	ExecuteRequest     = sdkana.ExecuteRequest
	ExecuteResult      = sdkana.ExecuteResult
	AnalyzerResult     = sdkana.AnalyzerResult
	ExecutionLog       = sdkana.ExecutionLog
	ValidationResult   = sdkana.ValidationResult
)

// ParseInputFromFile reads and parses analyzer input from a file
var ParseInputFromFile = sdkana.ParseInputFromFile

// Handler handles Davis analyzer resources.
type Handler struct {
	sdk *sdkana.Handler
}

// NewHandler creates a new analyzer handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkana.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List retrieves all available analyzers
func (h *Handler) List(filter string) (*AnalyzerList, error) {
	return h.sdk.List(filter)
}

// Get retrieves a specific analyzer definition
func (h *Handler) Get(name string) (*AnalyzerDefinition, error) {
	return h.sdk.Get(name)
}

// GetDocumentation retrieves the documentation for an analyzer
func (h *Handler) GetDocumentation(name string) (string, error) {
	return h.sdk.GetDocumentation(name)
}

// GetInputSchema retrieves the JSON schema for analyzer input
func (h *Handler) GetInputSchema(name string) (map[string]interface{}, error) {
	return h.sdk.GetInputSchema(name)
}

// GetResultSchema retrieves the JSON schema for analyzer result
func (h *Handler) GetResultSchema(name string) (map[string]interface{}, error) {
	return h.sdk.GetResultSchema(name)
}

// Execute runs an analyzer with the given input
func (h *Handler) Execute(name string, input map[string]interface{}, timeoutSeconds int) (*ExecuteResult, error) {
	return h.sdk.Execute(name, input, timeoutSeconds)
}

// ExecuteAndWait runs an analyzer and waits for completion
func (h *Handler) ExecuteAndWait(name string, input map[string]interface{}, maxWaitSeconds int) (*ExecuteResult, error) {
	return h.sdk.ExecuteAndWait(name, input, maxWaitSeconds)
}

// Poll polls for the result of a started analyzer execution
func (h *Handler) Poll(name string, requestToken string, timeoutSeconds int) (*ExecuteResult, error) {
	return h.sdk.Poll(name, requestToken, timeoutSeconds)
}

// Cancel cancels a running analyzer execution
func (h *Handler) Cancel(name string, requestToken string) (*ExecuteResult, error) {
	return h.sdk.Cancel(name, requestToken)
}

// Validate validates the input for an analyzer execution
func (h *Handler) Validate(name string, input map[string]interface{}) (*ValidationResult, error) {
	return h.sdk.Validate(name, input)
}
