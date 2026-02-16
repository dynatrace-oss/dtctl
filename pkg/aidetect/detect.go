package aidetect

import (
	"os"
	"strings"
)

// AgentInfo represents detected AI agent information
type AgentInfo struct {
	Detected bool
	Name     string
}

// knownAgents maps environment variables to AI agent names
var knownAgents = map[string]string{
	"CLAUDECODE":     "claude-code",
	"CURSOR_AGENT":   "cursor",
	"GITHUB_COPILOT": "github-copilot",
	"CODEIUM_AGENT":  "codeium",
	"TABNINE_AGENT":  "tabnine",
	"AMAZON_Q":       "amazon-q",
	"OPENCODE":       "opencode",
	"AI_AGENT":       "generic-ai",
}

// Detect checks environment variables to identify if running under an AI agent
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

// UserAgentSuffix returns a suffix to append to the User-Agent header
// Returns empty string if no AI agent detected
func UserAgentSuffix() string {
	info := Detect()
	if !info.Detected {
		return ""
	}
	return " (AI-Agent: " + info.Name + ")"
}
