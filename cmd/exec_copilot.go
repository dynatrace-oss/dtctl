package cmd

import (
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/copilot"
	"github.com/spf13/cobra"
)

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
		cfg, err := LoadConfig()
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
			_, err = handler.ChatWithOptions(message, opts, func(chunk copilot.StreamChunk) error {
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
		cfg, err := LoadConfig()
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
		cfg, err := LoadConfig()
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
		cfg, err := LoadConfig()
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
	// CoPilot subcommands
	execCopilotCmd.AddCommand(execCopilotNl2DqlCmd)
	execCopilotCmd.AddCommand(execCopilotDql2NlCmd)
	execCopilotCmd.AddCommand(execCopilotDocSearchCmd)

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
