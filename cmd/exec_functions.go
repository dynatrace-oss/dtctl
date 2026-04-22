package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
)

// execFunctionCmd executes an app function or ad-hoc code
var execFunctionCmd = &cobra.Command{
	Use:     "function [app-id/function-name]",
	Aliases: []string{"fn", "func"},
	Short:   "Execute an app function or ad-hoc JavaScript code",
	Long: `Execute a function from an installed app or run ad-hoc JavaScript code.

App Function Execution:
  Execute a function from an installed app by providing the app ID and function name.

Ad-hoc Code Execution:
  Execute JavaScript code directly without deploying an app.

Examples:
  # Execute an app function (GET)
  dtctl exec function myapp/myfunction

  # Execute with POST and payload
  dtctl exec function myapp/myfunction --method POST --payload '{"key":"value"}'

  # Execute with payload from file
  dtctl exec function myapp/myfunction --method POST --data @payload.json

  # Defer execution (async, for resumable functions)
  dtctl exec function myapp/myfunction --defer

  # Execute ad-hoc JavaScript code
  dtctl exec function --code 'export default async function() { return "hello" }'

  # Execute JavaScript from file
  dtctl exec function -f script.js

  # Execute with payload
  dtctl exec function -f script.js --payload '{"input":"data"}'

  # Print only the response body; status code goes to stderr
  dtctl exec function myapp/myfunction --body-only

  # Clean body-only stream — suppress status/logs on stderr
  dtctl exec function myapp/myfunction --body-only 2>/dev/null

  # Combine with output format flags
  dtctl exec function myapp/myfunction --body-only -o yaml
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, err := SetupClient()
		if err != nil {
			return err
		}

		executor := exec.NewFunctionExecutor(c)

		// Get flags
		method, _ := cmd.Flags().GetString("method")
		payload, _ := cmd.Flags().GetString("payload")
		payloadFile, _ := cmd.Flags().GetString("data")
		sourceCode, _ := cmd.Flags().GetString("code")
		sourceCodeFile, _ := cmd.Flags().GetString("file")
		defer_, _ := cmd.Flags().GetBool("defer")

		opts := exec.FunctionExecuteOptions{
			Method:         method,
			Payload:        payload,
			PayloadFile:    payloadFile,
			SourceCode:     sourceCode,
			SourceCodeFile: sourceCodeFile,
			Defer:          defer_,
		}

		// Parse function reference from args if provided
		if len(args) > 0 {
			opts.FunctionName = args[0]
		}

		// Read before Execute so all flag reads are co-located
		bodyOnly, _ := cmd.Flags().GetBool("body-only")

		// Execute the function
		result, err := executor.Execute(opts)
		if err != nil {
			return err
		}
		if bodyOnly {
			return printBodyOnly(result)
		}

		// Handle different result types and print
		printer := NewPrinter()
		return printer.Print(result)
	},
}

// extractBodyValue extracts the printable body from a function execution result.
// Returns the Go value to print, whether it is a plain string (format flag does
// not apply), and any metadata line to emit on stderr before the body.
func extractBodyValue(result interface{}) (value interface{}, isPlainString bool, metadata string) {
	switch r := result.(type) {
	case *appengine.FunctionInvokeResponse:
		if envelope, ok := r.RawBody.(map[string]interface{}); ok {
			// Prefer the envelope's statusCode; functions often respond with HTTP
			// 200 but set their own status code inside the envelope.
			statusCode := r.StatusCode
			if sc, ok := envelope["statusCode"].(float64); ok {
				statusCode = int(sc)
			}
			if statusCode != 0 {
				metadata = fmt.Sprintf("Status: %d", statusCode)
			}
			if bodyVal, hasBody := envelope["body"]; hasBody {
				if bodyStr, ok := bodyVal.(string); ok {
					var parsed interface{}
					if err := json.Unmarshal([]byte(bodyStr), &parsed); err == nil {
						return parsed, false, metadata
					}
					return bodyStr, true, metadata
				}
				return bodyVal, false, metadata
			}
			return envelope, false, metadata
		}
		if r.StatusCode != 0 {
			metadata = fmt.Sprintf("Status: %d", r.StatusCode)
		}
		if r.RawBody != nil {
			// Non-nil but non-object RawBody (e.g. JSON array or scalar) — return
			// it so -o yaml and other format flags apply.
			return r.RawBody, false, metadata
		}
		return r.Body, true, metadata

	case *appengine.FunctionExecutorResponse:
		logs := strings.TrimRight(r.Logs, "\n")
		if r.Result == nil {
			return "null", true, logs
		}
		return r.Result, false, logs

	default:
		return result, false, ""
	}
}

// printBodyOnly writes just the function response body to stdout. Metadata
// (status code, execution logs) goes to stderr so that 2>/dev/null gives a
// clean body-only stream. The body is formatted according to the global -o
// flag; plain-text bodies are always written verbatim.
func printBodyOnly(result interface{}) error {
	value, isPlainString, metadata := extractBodyValue(result)

	if metadata != "" {
		fmt.Fprintln(os.Stderr, metadata)
	}

	if isPlainString {
		_, err := fmt.Fprintln(os.Stdout, value)
		return err
	}

	printer := NewPrinter()
	return printer.Print(value)
}

func init() {
	// Function flags
	execFunctionCmd.Flags().String("method", "GET", "HTTP method for app function (GET, POST, PUT, PATCH, DELETE)")
	execFunctionCmd.Flags().String("payload", "", "request payload (JSON string)")
	execFunctionCmd.Flags().String("data", "", "read payload from file (use @filename or - for stdin)")
	execFunctionCmd.Flags().String("code", "", "JavaScript code to execute (for ad-hoc execution)")
	execFunctionCmd.Flags().StringP("file", "f", "", "read JavaScript code from file (for ad-hoc execution)")
	execFunctionCmd.Flags().Bool("defer", false, "defer execution (async, for resumable functions)")
	execFunctionCmd.Flags().Bool("body-only", false, "strip the HTTP envelope and print only the inner response body; metadata (status code, logs) goes to stderr")
}
