package aidetect

import "github.com/dynatrace-oss/dtctl/sdk/agentmode"

// AgentInfo represents detected AI agent information.
// Alias for agentmode.AgentInfo from the SDK.
type AgentInfo = agentmode.AgentInfo

// Detect checks environment variables to identify if running under an AI agent.
// Delegates to agentmode.Detect from the SDK.
func Detect() AgentInfo {
	return agentmode.Detect()
}

// knownAgents is kept for test compatibility — mirrors the SDK's internal list.
var knownAgents = map[string]string{
	"CLAUDECODE":     "claude-code",
	"CURSOR_AGENT":   "cursor",
	"GITHUB_COPILOT": "github-copilot",
	"CODEIUM_AGENT":  "codeium",
	"TABNINE_AGENT":  "tabnine",
	"AMAZON_Q":       "amazon-q",
	"JUNIE":          "junie",
	"KIRO":           "kiro",
	"OPENCODE":       "opencode",
	"OPENCLAW":       "openclaw",
	"AI_AGENT":       "generic-ai",
}

// UserAgentSuffix returns a suffix to append to the User-Agent header.
// Delegates to agentmode.UserAgentSuffix from the SDK.
func UserAgentSuffix() string {
	return agentmode.UserAgentSuffix()
}
