package appengine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// FunctionHandler handles App Engine function operations
type FunctionHandler struct {
	client *client.Client
}

// NewFunctionHandler creates a new function handler
func NewFunctionHandler(c *client.Client) *FunctionHandler {
	return &FunctionHandler{client: c}
}

// FunctionInvokeRequest represents a function invocation request
type FunctionInvokeRequest struct {
	Method       string            // HTTP method (GET, POST, PUT, PATCH, DELETE)
	AppID        string            // App ID
	FunctionName string            // Function name
	Payload      string            // Request body/payload
	Headers      map[string]string // Additional headers
}

// FunctionInvokeResponse represents a function invocation response
type FunctionInvokeResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body"`
	RawBody    interface{}       `json:"-"` // For direct JSON output
}

// DeferredExecutionRequest represents a deferred execution request
type DeferredExecutionRequest struct {
	AppID        string `json:"appId"`
	FunctionName string `json:"functionName"`
	Body         string `json:"body,omitempty"`
}

// DeferredExecutionResponse represents a deferred execution response
type DeferredExecutionResponse struct {
	ID string `json:"id" table:"ID"`
}

// FunctionExecutorRequest represents an ad-hoc function execution request
type FunctionExecutorRequest struct {
	SourceCode string `json:"sourceCode"`
	Payload    string `json:"payload,omitempty"`
}

// FunctionExecutorResponse represents an ad-hoc function execution response
type FunctionExecutorResponse struct {
	Result interface{} `json:"result"`
	Logs   string      `json:"logs"`
}

// SDKVersion represents an SDK version
type SDKVersion struct {
	Version string `json:"version" table:"VERSION"`
	Default bool   `json:"default" table:"DEFAULT"`
}

// SDKVersionsResponse represents SDK versions response
type SDKVersionsResponse struct {
	Versions []SDKVersion `json:"versions"`
}

// InvokeFunction invokes an app function
func (h *FunctionHandler) InvokeFunction(req *FunctionInvokeRequest) (*FunctionInvokeResponse, error) {
	// Build the URL
	url := fmt.Sprintf("/platform/app-engine/app-functions/v1/apps/%s/api/%s",
		req.AppID, req.FunctionName)

	// Build the request
	httpReq := h.client.HTTP().R()

	// Add custom headers
	for key, value := range req.Headers {
		httpReq.SetHeader(key, value)
	}

	// Set body if provided
	if req.Payload != "" {
		httpReq.SetBody(req.Payload)
	}

	// Execute the request based on method
	var resp interface {
		IsError() bool
		StatusCode() int
		String() string
		Body() []byte
	}
	var err error

	switch req.Method {
	case "GET":
		resp, err = httpReq.Get(url)
	case "POST":
		resp, err = httpReq.Post(url)
	case "PUT":
		resp, err = httpReq.Put(url)
	case "PATCH":
		resp, err = httpReq.Patch(url)
	case "DELETE":
		resp, err = httpReq.Delete(url)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", req.Method)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to invoke function: %w", err)
	}

	// Handle error responses
	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("app %q or function %q not found", req.AppID, req.FunctionName)
		case 409:
			return nil, fmt.Errorf("app backend is being deployed and not ready yet")
		case 540:
			return nil, fmt.Errorf("JavaScript error occurred: %s", resp.String())
		case 541:
			return nil, fmt.Errorf("runtime error occurred: %s", resp.String())
		default:
			return nil, fmt.Errorf("function invocation failed: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	// Try to parse as JSON first
	var jsonBody interface{}
	// Use resp.Body() to avoid potential truncation of large function responses
	bodyBytes := resp.Body()
	body := string(bodyBytes)
	if err := json.Unmarshal(bodyBytes, &jsonBody); err == nil {
		// Valid JSON response
		return &FunctionInvokeResponse{
			StatusCode: resp.StatusCode(),
			Headers:    make(map[string]string),
			Body:       body,
			RawBody:    jsonBody,
		}, nil
	}

	// Return as plain text
	return &FunctionInvokeResponse{
		StatusCode: resp.StatusCode(),
		Headers:    make(map[string]string),
		Body:       body,
	}, nil
}

// DeferExecution defers execution of a resumable function
func (h *FunctionHandler) DeferExecution(req *DeferredExecutionRequest) (*DeferredExecutionResponse, error) {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/app-engine/app-functions/v1/deferred-execution")

	if err != nil {
		return nil, fmt.Errorf("failed to defer execution: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to defer execution: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result DeferredExecutionResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse deferred execution response: %w", err)
	}

	return &result, nil
}

// ExecuteCode executes ad-hoc JavaScript code using the function executor
func (h *FunctionHandler) ExecuteCode(sourceCode, payload string) (*FunctionExecutorResponse, error) {
	req := FunctionExecutorRequest{
		SourceCode: sourceCode,
		Payload:    payload,
	}

	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/app-engine/function-executor/v1/executions")

	if err != nil {
		return nil, fmt.Errorf("failed to execute code: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 540:
			return nil, fmt.Errorf("JavaScript error occurred: %s", resp.String())
		case 541:
			return nil, fmt.Errorf("runtime error occurred: %s", resp.String())
		default:
			return nil, fmt.Errorf("code execution failed: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result FunctionExecutorResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse execution response: %w", err)
	}

	return &result, nil
}

// GetSDKVersions lists available SDK versions
func (h *FunctionHandler) GetSDKVersions() (*SDKVersionsResponse, error) {
	resp, err := h.client.HTTP().R().
		Get("/platform/app-engine/function-executor/v1/sdk-versions")

	if err != nil {
		return nil, fmt.Errorf("failed to get SDK versions: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get SDK versions: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result SDKVersionsResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse SDK versions response: %w", err)
	}

	return &result, nil
}

// ReadFileOrStdin reads content from a file or stdin
func ReadFileOrStdin(filename string) (string, error) {
	var reader io.Reader
	if filename == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("failed to open file: %w", err)
		}
		defer func() {
			_ = f.Close()
		}()
		reader = f
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	return string(content), nil
}
