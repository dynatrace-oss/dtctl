package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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

  # Write raw result to file (console shows metadata only)
  dtctl exec function myapp/myfunction --method POST --payload '{"key":"value"}' --outfile result.json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, err := SetupClient()
		if err != nil {
			return err
		}

		executor := exec.NewFunctionExecutor(c)

		method, _ := cmd.Flags().GetString("method")
		payload, _ := cmd.Flags().GetString("payload")
		payloadFile, _ := cmd.Flags().GetString("data")
		sourceCode, _ := cmd.Flags().GetString("code")
		sourceCodeFile, _ := cmd.Flags().GetString("file")
		defer_, _ := cmd.Flags().GetBool("defer")
		outfile, _ := cmd.Flags().GetString("outfile")

		opts := exec.FunctionExecuteOptions{
			Method:         method,
			Payload:        payload,
			PayloadFile:    payloadFile,
			SourceCode:     sourceCode,
			SourceCodeFile: sourceCodeFile,
			Defer:          defer_,
		}

		if len(args) > 0 {
			opts.FunctionName = args[0]
		}

		result, err := executor.Execute(opts)
		if err != nil {
			return err
		}

		if outfile != "" {
			if err := writeFunctionResultToFile(result, outfile); err != nil {
				return err
			}
			printer := NewPrinter()
			return printer.Print(outfileSummary(result, outfile))
		}

		printer := NewPrinter()
		return printer.Print(result)
	},
}

// writeFunctionResultToFile writes the raw function result to outfile.
// 0600 so the output isn't world-readable on shared systems (CI runners, multi-user hosts).
func writeFunctionResultToFile(result interface{}, outfile string) error {
	f, err := os.OpenFile(outfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create output file %q: %w", outfile, err)
	}
	defer func() { _ = f.Close() }()

	switch r := result.(type) {
	case *appengine.FunctionInvokeResponse:
		// Write verbatim to preserve the original representation (JSON formatting, plain text, etc.)
		if _, err := fmt.Fprint(f, r.Body); err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
	case *appengine.FunctionExecutorResponse:
		// Only the Result field goes to the file; logs stay on the console via outfileSummary.
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r.Result); err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
	default:
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
	}
	return nil
}

type invokeOutfileSummary struct {
	StatusCode int    `json:"statusCode" yaml:"statusCode" table:"STATUS CODE"`
	Result     string `json:"result"     yaml:"result"     table:"RESULT"`
}

type executorOutfileSummary struct {
	Logs   string `json:"logs"   yaml:"logs"   table:"LOGS"`
	Result string `json:"result" yaml:"result" table:"RESULT"`
}

type genericOutfileSummary struct {
	Result string `json:"result" yaml:"result" table:"RESULT"`
}

// outfileSummary returns a printer-friendly summary with metadata and a "result" pointer to the
// outfile, so the caller can pass it through the regular printer (respecting -o json/yaml/etc.).
func outfileSummary(result interface{}, outfile string) interface{} {
	ref := fmt.Sprintf("written to %s", outfile)
	switch r := result.(type) {
	case *appengine.FunctionInvokeResponse:
		return invokeOutfileSummary{StatusCode: r.StatusCode, Result: ref}
	case *appengine.FunctionExecutorResponse:
		if r.Logs != "" {
			return executorOutfileSummary{Logs: r.Logs, Result: ref}
		}
		return genericOutfileSummary{Result: ref}
	default:
		return genericOutfileSummary{Result: ref}
	}
}

func init() {
	execFunctionCmd.Flags().String("method", "GET", "HTTP method for app function (GET, POST, PUT, PATCH, DELETE)")
	execFunctionCmd.Flags().String("payload", "", "request payload (JSON string)")
	execFunctionCmd.Flags().String("data", "", "read payload from file (use @filename or - for stdin)")
	execFunctionCmd.Flags().String("code", "", "JavaScript code to execute (for ad-hoc execution)")
	execFunctionCmd.Flags().StringP("file", "f", "", "read JavaScript code from file (for ad-hoc execution)")
	execFunctionCmd.Flags().Bool("defer", false, "defer execution (async, for resumable functions)")
	execFunctionCmd.Flags().String("outfile", "", "write raw function result to file (console shows metadata only)")
}
