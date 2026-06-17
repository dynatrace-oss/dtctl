package agent

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	aiconfig "github.com/dynatrace-oss/dtctl/pkg/ai/config"
)

var (
	promptText  string
	serverURL   string
	modelName   string
	provider    string
	interactive bool
)

var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Send a prompt to an AI agent backed by MCP server tools",
	Long:  "Send a prompt to an AI agent that uses MCP server tools to execute the request",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !interactive && strings.TrimSpace(promptText) == "" {
			return fmt.Errorf("prompt is required (use -p or --prompt)")
		}

		agent := NewLLMAgent(serverURL, modelName, provider)
		fmt.Printf("Using model: %s (provider: %s)\n", agent.modelName, agent.provider)
		if interactive {
			return runInteractiveMode(agent, promptText)
		}

		result, err := agent.ProcessPrompt(promptText)
		if err != nil {
			return fmt.Errorf("failed to process prompt: %w", err)
		}

		fmt.Println(result)
		return nil
	},
}

func init() {
	AgentCmd.Flags().StringVarP(&promptText, "prompt", "p", "", "The prompt to send to the LLM")
	AgentCmd.Flags().StringVarP(&serverURL, "server", "s", "http://127.0.0.1:8080/mcp", "The MCP server URL")
	AgentCmd.Flags().StringVarP(&modelName, "model", "m", aiconfig.GetDefaultAiModel(), "The LLM model to use (default: llama for openrouter, gpt-4o-mini for openai, gemini-2.5-flash for google, deepseek-chat for deepseek, claude-haiku-4-5 for anthropic)")
	AgentCmd.Flags().StringVar(&provider, "provider", aiconfig.GetDefaultAiProvider(), "LLM provider: openrouter, openai, google, deepseek or anthropic")
	AgentCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive mode (optional)")
}

func runInteractiveMode(agent *LLMAgent, initialPrompt string) error {
	fmt.Println("Interactive mode enabled. Type your prompt and press Enter.")
	fmt.Println("Type 'exit' or 'quit' to leave.")

	scanner := bufio.NewScanner(os.Stdin)

	processPrompt := func(prompt string) {
		result, err := agent.ProcessPrompt(prompt)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Println(result)
	}

	if strings.TrimSpace(initialPrompt) != "" {
		processPrompt(initialPrompt)
	}

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			fmt.Println()
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if strings.TrimSpace(input) == "" {
			continue
		}

		lower := strings.ToLower(input)
		if lower == "exit" || lower == "quit" {
			return nil
		}

		processPrompt(input)
	}
}
