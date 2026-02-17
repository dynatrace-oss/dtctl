package cmd

import (
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/spf13/cobra"
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
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
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

		// Execute the function
		result, err := executor.Execute(opts)
		if err != nil {
			return err
		}

		// Handle different result types and print
		printer := NewPrinter()
		return printer.Print(result)
	},
}

func init() {
	// Function flags
	execFunctionCmd.Flags().String("method", "GET", "HTTP method for app function (GET, POST, PUT, PATCH, DELETE)")
	execFunctionCmd.Flags().String("payload", "", "request payload (JSON string)")
	execFunctionCmd.Flags().String("data", "", "read payload from file (use @filename or - for stdin)")
	execFunctionCmd.Flags().String("code", "", "JavaScript code to execute (for ad-hoc execution)")
	execFunctionCmd.Flags().StringP("file", "f", "", "read JavaScript code from file (for ad-hoc execution)")
	execFunctionCmd.Flags().Bool("defer", false, "defer execution (async, for resumable functions)")
}
