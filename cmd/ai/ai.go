package ai

import (
	"github.com/dynatrace-oss/dtctl/cmd/ai/agent"
	aiconfig_cmd "github.com/dynatrace-oss/dtctl/cmd/ai/config"
	"github.com/dynatrace-oss/dtctl/cmd/ai/mcp"
	"github.com/dynatrace-oss/dtctl/cmd/ai/web_agent"

	"github.com/spf13/cobra"
)

var AiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI features and tools",
	Long:  `This command lets you call the AI agent, manage prompts, and more.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	AiCmd.DisableFlagsInUseLine = true
	AiCmd.AddCommand(agent.AgentCmd)
	AiCmd.AddCommand(mcp.McpCmd)
	AiCmd.AddCommand(web_agent.WebAgentCmd)
	AiCmd.AddCommand(aiconfig_cmd.ConfigCmd)
}
