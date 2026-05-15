package appengine

import (
	"fmt"
	"io"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkae "github.com/dynatrace-oss/dtctl/sdk/api/appengine"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	FunctionInvokeRequest    = sdkae.FunctionInvokeRequest
	FunctionInvokeResponse   = sdkae.FunctionInvokeResponse
	DeferredExecutionRequest  = sdkae.DeferredExecutionRequest
	DeferredExecutionResponse = sdkae.DeferredExecutionResponse
	FunctionExecutorRequest  = sdkae.FunctionExecutorRequest
	FunctionExecutorResponse = sdkae.FunctionExecutorResponse
	SDKVersion               = sdkae.SDKVersion
	SDKVersionsResponse      = sdkae.SDKVersionsResponse
)

// ReadFileOrStdin reads content from a file or stdin.
// This is a CLI-layer helper and intentionally not part of the SDK.
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

// FunctionHandler handles App Engine function operations.
type FunctionHandler struct {
	sdk *sdkae.FunctionHandler
}

// NewFunctionHandler creates a new function handler
func NewFunctionHandler(c *client.Client) *FunctionHandler {
	return &FunctionHandler{
		sdk: sdkae.NewFunctionHandler(httpclient.Wrap(c.HTTP())),
	}
}

// InvokeFunction invokes an app function
func (h *FunctionHandler) InvokeFunction(req *FunctionInvokeRequest) (*FunctionInvokeResponse, error) {
	return h.sdk.InvokeFunction(req)
}

// DeferExecution defers execution of a resumable function
func (h *FunctionHandler) DeferExecution(req *DeferredExecutionRequest) (*DeferredExecutionResponse, error) {
	return h.sdk.DeferExecution(req)
}

// ExecuteCode executes ad-hoc JavaScript code using the function executor
func (h *FunctionHandler) ExecuteCode(sourceCode, payload string) (*FunctionExecutorResponse, error) {
	return h.sdk.ExecuteCode(sourceCode, payload)
}

// GetSDKVersions lists available SDK versions
func (h *FunctionHandler) GetSDKVersions() (*SDKVersionsResponse, error) {
	return h.sdk.GetSDKVersions()
}
