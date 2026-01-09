package exec

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
)

// FunctionExecutor handles function execution
type FunctionExecutor struct {
	handler *appengine.FunctionHandler
}

// NewFunctionExecutor creates a new function executor
func NewFunctionExecutor(c *client.Client) *FunctionExecutor {
	return &FunctionExecutor{
		handler: appengine.NewFunctionHandler(c),
	}
}

// FunctionExecuteOptions contains options for function execution
type FunctionExecuteOptions struct {
	// For app functions
	AppID        string
	FunctionName string
	Method       string
	Payload      string
	PayloadFile  string

	// For ad-hoc execution
	SourceCode     string
	SourceCodeFile string

	// For deferred execution
	Defer bool
}

// Execute executes a function based on the provided options
func (e *FunctionExecutor) Execute(opts FunctionExecuteOptions) (interface{}, error) {
	// Determine execution mode
	if opts.SourceCode != "" || opts.SourceCodeFile != "" {
		// Ad-hoc code execution
		return e.executeCode(opts)
	}

	// App function execution
	return e.executeAppFunction(opts)
}

// executeCode executes ad-hoc JavaScript code
func (e *FunctionExecutor) executeCode(opts FunctionExecuteOptions) (*appengine.FunctionExecutorResponse, error) {
	sourceCode := opts.SourceCode

	// Read from file if specified
	if opts.SourceCodeFile != "" {
		content, err := appengine.ReadFileOrStdin(opts.SourceCodeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read source code: %w", err)
		}
		sourceCode = content
	}

	if sourceCode == "" {
		return nil, fmt.Errorf("source code is required (use --code or -f)")
	}

	// Read payload if specified
	payload := opts.Payload
	if opts.PayloadFile != "" {
		content, err := appengine.ReadFileOrStdin(opts.PayloadFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read payload: %w", err)
		}
		payload = content
	}

	return e.handler.ExecuteCode(sourceCode, payload)
}

// executeAppFunction executes a function from an installed app
func (e *FunctionExecutor) executeAppFunction(opts FunctionExecuteOptions) (interface{}, error) {
	// Parse function reference (supports "appId/functionName" or separate args)
	appID := opts.AppID
	functionName := opts.FunctionName

	// If function name contains '/', split it
	if functionName != "" && strings.Contains(functionName, "/") {
		parts := strings.SplitN(functionName, "/", 2)
		appID = parts[0]
		functionName = parts[1]
	}

	if appID == "" || functionName == "" {
		return nil, fmt.Errorf("app ID and function name are required (use 'app-id/function-name' or separate arguments)")
	}

	// Read payload if specified
	payload := opts.Payload
	if opts.PayloadFile != "" {
		content, err := appengine.ReadFileOrStdin(opts.PayloadFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read payload: %w", err)
		}
		payload = content
	}

	// Check if this is a deferred execution
	if opts.Defer {
		return e.handler.DeferExecution(&appengine.DeferredExecutionRequest{
			AppID:        appID,
			FunctionName: functionName,
			Body:         payload,
		})
	}

	// Execute synchronously
	method := opts.Method
	if method == "" {
		method = "GET"
	}

	req := &appengine.FunctionInvokeRequest{
		Method:       method,
		AppID:        appID,
		FunctionName: functionName,
		Payload:      payload,
		Headers:      make(map[string]string),
	}

	return e.handler.InvokeFunction(req)
}

// GetSDKVersions returns available SDK versions for the function executor
func (e *FunctionExecutor) GetSDKVersions() (*appengine.SDKVersionsResponse, error) {
	return e.handler.GetSDKVersions()
}

// ReadFileOrStdin reads content from a file or stdin
func ReadFileOrStdin(filename string) (string, error) {
	if filename == "-" {
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return string(content), nil
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	return string(content), nil
}
