package agentmode

import (
	"os"
	"strings"
)

// AgentInfo represents detected AI agent information.
type AgentInfo struct {
	// Detected is true if an AI agent environment was identified.
	Detected bool
	// Name is the canonical name of the detected agent (e.g. "claude-code").
	Name string
}

// knownAgents maps environment variables to AI agent names.
var knownAgents = map[string]string{
	"CLAUDECODE":     "claude-code",
	"CURSOR_AGENT":   "cursor",
	"GITHUB_COPILOT": "github-copilot",
	"CODEIUM_AGENT":  "codeium",
	"TABNINE_AGENT":  "tabnine",
	"AMAZON_Q":       "amazon-q",
	"JUNIE":          "junie",
	"KIRO":           "kiro",
	// Kiro does not set a KIRO env var; the entry above is kept only as a
	// manual override. Kiro signals an active agent session via
	// AGENT_CONTEXT_OUT, a per-invocation FIFO path (Kiro's documented ACP
	// side-channel, exported only in interactive sessions), and via
	// KIRO_SESSION_ID, which is set in both interactive and --no-interactive
	// modes (verified on kiro-cli 2.6.1). Detecting on AGENT_CONTEXT_OUT alone
	// would miss headless/scripted Kiro, so key on both.
	// https://kiro.dev/docs/cli/reference/built-in-tools/#side-channels-for-wrapper-scripts
	"AGENT_CONTEXT_OUT": "kiro",
	"KIRO_SESSION_ID":   "kiro",
	"OPENCODE":          "opencode",
	"OPENCLAW":          "openclaw",
	"AI_AGENT":          "generic-ai",
}

// Detect checks environment variables to identify if running under an AI agent.
func Detect() AgentInfo {
	for envVar, agentName := range knownAgents {
		if val := os.Getenv(envVar); val != "" && val != "0" && strings.ToLower(val) != "false" {
			return AgentInfo{
				Detected: true,
				Name:     agentName,
			}
		}
	}
	return AgentInfo{Detected: false}
}

// UserAgentSuffix returns a suffix to append to the User-Agent header.
// Returns empty string if no AI agent is detected.
func UserAgentSuffix() string {
	info := Detect()
	if !info.Detected {
		return ""
	}
	return " (AI-Agent: " + info.Name + ")"
}
