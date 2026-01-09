package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/analyzer"
	"github.com/dynatrace-oss/dtctl/pkg/resources/copilot"
	"github.com/spf13/cobra"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute queries, workflows, or functions",
	Long:  `Execute DQL queries, workflows, SLO evaluations, or functions.`,
}

// execDQLCmd executes a DQL query (DEPRECATED)
var execDQLCmd = &cobra.Command{
	Use:    "dql [query]",
	Short:  "Execute a DQL query (DEPRECATED: use 'dtctl query')",
	Hidden: true, // Hide from help output
	Long: `Execute a DQL query against Grail storage.

DEPRECATED: This command is deprecated. Use 'dtctl query' instead.
The 'dtctl query' command provides the same functionality with additional
features like template variables.

Examples:
  # Execute inline query (use 'dtctl query' instead)
  dtctl query "fetch logs | limit 10"

  # Execute from file (use 'dtctl query -f' instead)
  dtctl query -f query.dql

  # Output as JSON (use 'dtctl query -o json' instead)
  dtctl query "fetch logs" -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show deprecation warning
		fmt.Fprintln(os.Stderr, "Warning: 'dtctl exec dql' is deprecated. Use 'dtctl query' instead.")
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		executor := exec.NewDQLExecutor(c)

		queryFile, _ := cmd.Flags().GetString("file")

		if queryFile != "" {
			return executor.ExecuteFromFile(queryFile, outputFormat)
		}

		if len(args) == 0 {
			return fmt.Errorf("query string or --file is required")
		}

		query := args[0]
		return executor.Execute(query, outputFormat)
	},
}

// execWorkflowCmd executes a workflow
var execWorkflowCmd = &cobra.Command{
	Use:     "workflow <workflow-id>",
	Aliases: []string{"wf"},
	Short:   "Execute a workflow",
	Long: `Execute an automation workflow.

Examples:
  # Execute workflow
  dtctl exec workflow my-workflow-id

  # Execute with parameters
  dtctl exec workflow my-workflow-id --params severity=high --params env=prod

  # Execute and wait for completion
  dtctl exec workflow my-workflow-id --wait

  # Execute with custom timeout
  dtctl exec workflow my-workflow-id --wait --timeout 10m
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowID := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		executor := exec.NewWorkflowExecutor(c)

		paramStrings, _ := cmd.Flags().GetStringSlice("params")
		params, err := exec.ParseParams(paramStrings)
		if err != nil {
			return err
		}

		result, err := executor.Execute(workflowID, params)
		if err != nil {
			return err
		}

		fmt.Printf("Workflow execution started\n")
		fmt.Printf("Execution ID: %s\n", result.ID)
		fmt.Printf("State: %s\n", result.State)

		// Handle --wait flag
		wait, _ := cmd.Flags().GetBool("wait")
		if wait {
			timeout, _ := cmd.Flags().GetDuration("timeout")
			if timeout == 0 {
				timeout = 30 * time.Minute
			}

			fmt.Printf("\nWaiting for execution to complete...\n")

			opts := exec.WaitOptions{
				PollInterval: 2 * time.Second,
				Timeout:      timeout,
			}

			status, err := executor.WaitForCompletion(context.Background(), result.ID, opts)
			if err != nil {
				return err
			}

			fmt.Printf("\nExecution completed\n")
			fmt.Printf("Final State: %s\n", status.State)
			if status.StateInfo != nil && *status.StateInfo != "" {
				fmt.Printf("State Info: %s\n", *status.StateInfo)
			}
			fmt.Printf("Duration: %s\n", formatExecutionDuration(status.Runtime))

			// Return error if execution failed
			if status.State == "ERROR" {
				return fmt.Errorf("workflow execution failed")
			}
		}

		return nil
	},
}

// formatExecutionDuration formats seconds into a human-readable duration
func formatExecutionDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		m := seconds / 60
		s := seconds % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

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
		cfg, err := config.Load()
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

// execAnalyzerCmd executes a Davis analyzer
var execAnalyzerCmd = &cobra.Command{
	Use:     "analyzer <analyzer-name>",
	Aliases: []string{"az"},
	Short:   "Execute a Davis AI analyzer",
	Long: `Execute a Davis AI analyzer with the given input.

Examples:
  # Execute analyzer with input from file
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json

  # Execute with inline JSON input
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --input '{"query":"timeseries avg(dt.host.cpu.usage)"}'

  # Execute with DQL query shorthand (for forecast/timeseries analyzers)
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --query "timeseries avg(dt.host.cpu.usage)"

  # Validate input without executing
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json --validate

  # Execute and wait for completion (default)
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json --wait

  # Output as JSON
  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json -o json
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		analyzerName := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := analyzer.NewHandler(c)

		// Build input from flags
		var input map[string]interface{}

		inputFile, _ := cmd.Flags().GetString("file")
		inputJSON, _ := cmd.Flags().GetString("input")
		query, _ := cmd.Flags().GetString("query")

		if inputFile != "" {
			input, err = analyzer.ParseInputFromFile(inputFile)
			if err != nil {
				return err
			}
		} else if inputJSON != "" {
			if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
				return fmt.Errorf("failed to parse input JSON: %w", err)
			}
		} else if query != "" {
			// Shorthand for timeseries query
			input = map[string]interface{}{
				"timeSeriesData": query,
			}
		} else {
			return fmt.Errorf("input is required: use --file, --input, or --query")
		}

		// Handle validate-only mode
		validateOnly, _ := cmd.Flags().GetBool("validate")
		if validateOnly {
			result, err := handler.Validate(analyzerName, input)
			if err != nil {
				return err
			}
			printer := NewPrinter()
			return printer.Print(result)
		}

		// Execute analyzer
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetInt("timeout")

		var result *analyzer.ExecuteResult
		if wait {
			result, err = handler.ExecuteAndWait(analyzerName, input, timeout)
		} else {
			result, err = handler.Execute(analyzerName, input, 30)
		}

		if err != nil {
			return err
		}

		// Default to JSON output for analyzer results since table doesn't show the actual data
		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "" || outputFormat == "table" {
			outputFormat = "json"
		}
		printer := output.NewPrinter(outputFormat)
		return printer.Print(result)
	},
}

// execCopilotCmd executes a Davis CoPilot query
var execCopilotCmd = &cobra.Command{
	Use:     "copilot [message]",
	Aliases: []string{"cp", "chat"},
	Short:   "Chat with Davis CoPilot",
	Long: `Send a message to Davis CoPilot and get a response.

Examples:
  # Ask a question
  dtctl exec copilot "What caused the CPU spike on host-123?"

  # Read question from file
  dtctl exec copilot -f question.txt

  # Stream response in real-time
  dtctl exec copilot "Explain the recent errors" --stream

  # Provide additional context
  dtctl exec copilot "Analyze this" --context "Error logs from production"

  # Disable document retrieval (Dynatrace docs)
  dtctl exec copilot "What is DQL?" --no-docs

  # Add formatting instructions
  dtctl exec copilot "List top errors" --instruction "Answer in bullet points"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := copilot.NewHandler(c)

		// Get message from args or file
		var message string
		inputFile, _ := cmd.Flags().GetString("file")

		if inputFile != "" {
			content, err := os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			message = string(content)
		} else if len(args) > 0 {
			message = args[0]
		} else {
			return fmt.Errorf("message is required: provide as argument or use --file")
		}

		// Build options
		stream, _ := cmd.Flags().GetBool("stream")
		contextStr, _ := cmd.Flags().GetString("context")
		instruction, _ := cmd.Flags().GetString("instruction")
		noDocs, _ := cmd.Flags().GetBool("no-docs")

		opts := copilot.ChatOptions{
			Stream:        stream,
			Supplementary: contextStr,
			Instruction:   instruction,
		}

		if noDocs {
			opts.DocumentRetrieval = "disabled"
		}

		// Execute chat
		var result *copilot.ConversationResponse

		if stream {
			result, err = handler.ChatWithOptions(message, opts, func(chunk copilot.StreamChunk) error {
				if chunk.Data != nil && len(chunk.Data.Tokens) > 0 {
					for _, token := range chunk.Data.Tokens {
						fmt.Print(token)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Println() // Final newline after streaming
		} else {
			result, err = handler.ChatWithOptions(message, opts, nil)
			if err != nil {
				return err
			}
			fmt.Println(result.Text)
		}

		return nil
	},
}

// execCopilotNl2DqlCmd converts natural language to DQL
var execCopilotNl2DqlCmd = &cobra.Command{
	Use:   "nl2dql [text]",
	Short: "Convert natural language to a DQL query",
	Long: `Generate a DQL query from a natural language description.

Examples:
  # Generate DQL from natural language
  dtctl exec copilot nl2dql "show me error logs from the last hour"

  # Read prompt from file
  dtctl exec copilot nl2dql -f prompt.txt

  # Output as JSON (includes messageToken for feedback)
  dtctl exec copilot nl2dql "find hosts with high CPU" -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := copilot.NewHandler(c)

		// Get text from args or file
		var text string
		inputFile, _ := cmd.Flags().GetString("file")

		if inputFile != "" {
			content, err := os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			text = string(content)
		} else if len(args) > 0 {
			text = args[0]
		} else {
			return fmt.Errorf("text is required: provide as argument or use --file")
		}

		result, err := handler.Nl2Dql(text)
		if err != nil {
			return err
		}

		// Check output format
		outputFmt, _ := cmd.Flags().GetString("output")
		if outputFmt == "" || outputFmt == "table" {
			// Default: just print the DQL
			fmt.Println(result.DQL)
			return nil
		}

		printer := output.NewPrinter(outputFmt)
		return printer.Print(result)
	},
}

// execCopilotDql2NlCmd explains a DQL query in natural language
var execCopilotDql2NlCmd = &cobra.Command{
	Use:   "dql2nl [query]",
	Short: "Explain a DQL query in natural language",
	Long: `Get a natural language explanation of a DQL query.

Examples:
  # Explain a DQL query
  dtctl exec copilot dql2nl "fetch logs | filter status='ERROR' | limit 10"

  # Read query from file
  dtctl exec copilot dql2nl -f query.dql

  # Output as JSON
  dtctl exec copilot dql2nl "fetch logs | limit 10" -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := copilot.NewHandler(c)

		// Get query from args or file
		var query string
		inputFile, _ := cmd.Flags().GetString("file")

		if inputFile != "" {
			content, err := os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			query = string(content)
		} else if len(args) > 0 {
			query = args[0]
		} else {
			return fmt.Errorf("query is required: provide as argument or use --file")
		}

		result, err := handler.Dql2Nl(query)
		if err != nil {
			return err
		}

		// Check output format
		outputFmt, _ := cmd.Flags().GetString("output")
		if outputFmt == "" || outputFmt == "table" {
			// Default: print summary and explanation
			fmt.Printf("Summary: %s\n\n%s\n", result.Summary, result.Explanation)
			return nil
		}

		printer := output.NewPrinter(outputFmt)
		return printer.Print(result)
	},
}

// execCopilotDocSearchCmd searches for relevant documents
var execCopilotDocSearchCmd = &cobra.Command{
	Use:     "document-search [query]",
	Aliases: []string{"doc-search", "ds"},
	Short:   "Search for relevant notebooks and dashboards",
	Long: `Search for notebooks and dashboards relevant to your query.

Examples:
  # Search for documents about CPU analysis
  dtctl exec copilot document-search "CPU performance" --collections notebooks

  # Search across multiple collections
  dtctl exec copilot document-search "error monitoring" --collections dashboards,notebooks

  # Exclude specific documents
  dtctl exec copilot document-search "performance" --exclude doc-123,doc-456

  # Output as JSON
  dtctl exec copilot document-search "kubernetes" --collections notebooks -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := copilot.NewHandler(c)

		// Get query from args
		if len(args) == 0 {
			return fmt.Errorf("search query is required")
		}
		query := args[0]

		// Get collections (optional - valid values are undocumented)
		collections, _ := cmd.Flags().GetStringSlice("collections")

		// Get exclude list (optional)
		exclude, _ := cmd.Flags().GetStringSlice("exclude")

		result, err := handler.DocumentSearch([]string{query}, collections, exclude)
		if err != nil {
			return err
		}

		// Check output format
		outputFmt, _ := cmd.Flags().GetString("output")
		if outputFmt == "" || outputFmt == "table" {
			printer := output.NewPrinter("table")
			return printer.Print(result.Documents)
		}

		printer := output.NewPrinter(outputFmt)
		return printer.Print(result)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.AddCommand(execDQLCmd)
	execCmd.AddCommand(execWorkflowCmd)
	execCmd.AddCommand(execFunctionCmd)
	execCmd.AddCommand(execAnalyzerCmd)
	execCmd.AddCommand(execCopilotCmd)

	// CoPilot subcommands
	execCopilotCmd.AddCommand(execCopilotNl2DqlCmd)
	execCopilotCmd.AddCommand(execCopilotDql2NlCmd)
	execCopilotCmd.AddCommand(execCopilotDocSearchCmd)

	// DQL flags
	execDQLCmd.Flags().StringP("file", "f", "", "read query from file")

	// Workflow flags
	execWorkflowCmd.Flags().StringSlice("params", []string{}, "workflow parameters (key=value)")
	execWorkflowCmd.Flags().Bool("wait", false, "wait for workflow execution to complete")
	execWorkflowCmd.Flags().Duration("timeout", 30*time.Minute, "timeout when waiting for completion")

	// Function flags
	execFunctionCmd.Flags().String("method", "GET", "HTTP method for app function (GET, POST, PUT, PATCH, DELETE)")
	execFunctionCmd.Flags().String("payload", "", "request payload (JSON string)")
	execFunctionCmd.Flags().String("data", "", "read payload from file (use @filename or - for stdin)")
	execFunctionCmd.Flags().String("code", "", "JavaScript code to execute (for ad-hoc execution)")
	execFunctionCmd.Flags().StringP("file", "f", "", "read JavaScript code from file (for ad-hoc execution)")
	execFunctionCmd.Flags().Bool("defer", false, "defer execution (async, for resumable functions)")

	// Analyzer flags
	execAnalyzerCmd.Flags().StringP("file", "f", "", "read input from JSON file")
	execAnalyzerCmd.Flags().String("input", "", "inline JSON input")
	execAnalyzerCmd.Flags().String("query", "", "DQL query shorthand (for timeseries analyzers)")
	execAnalyzerCmd.Flags().Bool("validate", false, "validate input without executing")
	execAnalyzerCmd.Flags().Bool("wait", true, "wait for analyzer execution to complete")
	execAnalyzerCmd.Flags().Int("timeout", 300, "timeout in seconds when waiting for completion")

	// CoPilot flags
	execCopilotCmd.Flags().StringP("file", "f", "", "read message from file")
	execCopilotCmd.Flags().Bool("stream", false, "stream response in real-time")
	execCopilotCmd.Flags().String("context", "", "additional context for the conversation")
	execCopilotCmd.Flags().String("instruction", "", "formatting instructions (e.g., 'Answer in bullet points')")
	execCopilotCmd.Flags().Bool("no-docs", false, "disable Dynatrace documentation retrieval")

	// CoPilot nl2dql flags
	execCopilotNl2DqlCmd.Flags().StringP("file", "f", "", "read prompt from file")

	// CoPilot dql2nl flags
	execCopilotDql2NlCmd.Flags().StringP("file", "f", "", "read DQL query from file")

	// CoPilot document-search flags
	execCopilotDocSearchCmd.Flags().StringSlice("collections", []string{}, "document collections to search (e.g., notebooks,dashboards)")
	execCopilotDocSearchCmd.Flags().StringSlice("exclude", []string{}, "document IDs to exclude from results")
}
