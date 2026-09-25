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

// genericAgent is the name reported when AI_AGENT is set to a value that
// names no agent dtctl knows. The raw value is never echoed: Name ends up in
// the User-Agent and dt-client-context headers.
const genericAgent = "generic-ai"

// signal is one environment variable that identifies an agent. match decides
// whether the variable's value counts; nil means any truthy value.
type signal struct {
	envVar string
	name   string
	match  func(val string) bool
}

// signals is checked in order and the first match wins. The order matters
// because agents leak into each other: Amp also sets CLAUDECODE=1, and Claude
// Code also sets AI_AGENT. A map here once made the reported name random
// whenever two of these were set — which under Claude Code is every run.
//
// Detection flips output to the JSON envelope, so a false positive costs a
// human their table output. Only variables an agent sets on the processes *it*
// spawns belong here — never one an IDE sets in every integrated terminal
// (CURSOR_TRACE_ID, TERM_PROGRAM=kiro, Q_TERM) or a platform sets everywhere
// (REPL_ID).
//
// AI_AGENT (Vercel's detect-agent convention) is consulted before all of these
// when its value names a known agent; see Detect.
var signals = []signal{
	// Amp sets CLAUDECODE=1 as well, so it must be checked before Claude Code.
	{envVar: "AGENT", name: "amp", match: equals("amp")},
	// Documented: set for Bash/PowerShell tools, hooks and MCP servers.
	{envVar: "CLAUDECODE", name: "claude-code"},
	// Codex sets CODEX_CI=1 and CODEX_THREAD_ID on every command it runs,
	// sandboxed or not (codex-rs core/src/unified_exec/process_manager.rs,
	// core/src/exec_env.rs). CODEX_SANDBOX is set only under macOS seatbelt.
	{envVar: "CODEX_CI", name: "codex"},
	{envVar: "CODEX_THREAD_ID", name: "codex"},
	{envVar: "CODEX_SANDBOX", name: "codex"},
	{envVar: "CURSOR_AGENT", name: "cursor"},
	// The copilot CLI (and VS Code's bundled Copilot, which shares the
	// @github/copilot SDK) sets COPILOT_CLI=1 — verified on copilot v1.0.63.
	// VS Code's own agent terminals set COPILOT_AGENT=1.
	{envVar: "COPILOT_CLI", name: "github-copilot"},
	{envVar: "COPILOT_AGENT", name: "github-copilot"},
	{envVar: "GEMINI_CLI", name: "gemini-cli"},
	{envVar: "QWEN_CODE", name: "qwen-code"},
	// OpenCode sets OPENCODE=1 in its own process env, which the shell tool
	// inherits (packages/opencode/src/index.ts).
	{envVar: "OPENCODE", name: "opencode"},
	// Kiro signals an active agent session via AGENT_CONTEXT_OUT, a
	// per-invocation FIFO path (Kiro's documented ACP side-channel, exported
	// only in interactive sessions), and via KIRO_SESSION_ID, which is set in
	// both interactive and --no-interactive modes (verified on kiro-cli
	// 2.6.1). Detecting on AGENT_CONTEXT_OUT alone would miss headless Kiro.
	// https://kiro.dev/docs/cli/reference/built-in-tools/#side-channels-for-wrapper-scripts
	{envVar: "AGENT_CONTEXT_OUT", name: "kiro"},
	{envVar: "KIRO_SESSION_ID", name: "kiro"},
	// OpenClaw's exec tool sets OPENCLAW_SHELL=exec; other values mark shells
	// a human drives (e.g. tui-local).
	{envVar: "OPENCLAW_SHELL", name: "openclaw", match: equals("exec")},
	{envVar: "TABNINE_CLI", name: "tabnine"},
	// Amazon Q CLI appends "AmazonQ-For-CLI Version/<v>" to AWS_EXECUTION_ENV
	// for the commands it runs (chat-cli/src/cli/chat/tools/mod.rs).
	{envVar: "AWS_EXECUTION_ENV", name: "amazon-q", match: contains("AmazonQ-For-CLI")},
	{envVar: "AUGMENT_AGENT", name: "augment"},
	{envVar: "ANTIGRAVITY_AGENT", name: "antigravity"},
	// Cline's VS Code extension marks the terminals it drives.
	{envVar: "CLINE_ACTIVE", name: "cline"},

	// Manual overrides. No agent is known to set these; they predate the
	// signals above and stay so that anyone exporting one by hand keeps
	// working. Last, so they never outrank a real signal.
	{envVar: "CODEX", name: "codex"},
	{envVar: "GITHUB_COPILOT", name: "github-copilot"},
	{envVar: "KIRO", name: "kiro"},
	{envVar: "OPENCLAW", name: "openclaw"},
	{envVar: "JUNIE", name: "junie"},
	{envVar: "CODEIUM_AGENT", name: "codeium"},
	{envVar: "TABNINE_AGENT", name: "tabnine"},
	{envVar: "AMAZON_Q", name: "amazon-q"},
}

// aiAgentEnvVar is the self-identification convention from Vercel's
// detect-agent. Values are free-form in practice: Claude Code sends
// "claude-code_2-1-282_agent", VS Code Copilot "github_copilot_vscode_agent".
const aiAgentEnvVar = "AI_AGENT"

// Detect checks environment variables to identify if running under an AI agent.
//
// An AI_AGENT value that names a known agent wins, because it is the agent
// identifying itself. Otherwise the agent-specific signals decide, in order.
// An AI_AGENT value naming no known agent still counts as detection, reported
// as "generic-ai", but only once every specific signal has had its turn.
func Detect() AgentInfo {
	aiAgent := os.Getenv(aiAgentEnvVar)
	if name := knownAgentName(aiAgent); name != "" {
		return AgentInfo{Detected: true, Name: name}
	}
	for _, s := range signals {
		val := os.Getenv(s.envVar)
		if s.match == nil && truthy(val) || s.match != nil && s.match(val) {
			return AgentInfo{Detected: true, Name: s.name}
		}
	}
	if truthy(aiAgent) {
		return AgentInfo{Detected: true, Name: genericAgent}
	}
	return AgentInfo{Detected: false}
}

// EnvVars returns every environment variable Detect consults. Tests clear
// these to keep a developer's own agent session from flipping output to the
// agent envelope.
func EnvVars() []string {
	vars := []string{aiAgentEnvVar}
	seen := map[string]bool{aiAgentEnvVar: true}
	for _, s := range signals {
		if !seen[s.envVar] {
			seen[s.envVar] = true
			vars = append(vars, s.envVar)
		}
	}
	return vars
}

// knownAgentName maps an AI_AGENT value to a canonical agent name, or "" when
// it names none. Underscores count as hyphens and anything after the name
// (a version, a surface) is ignored: "claude-code_2-1-282_agent" is
// claude-code, "github_copilot_vscode_agent" is github-copilot, "codex@1" is
// codex.
func knownAgentName(val string) string {
	v := strings.ReplaceAll(strings.ToLower(val), "_", "-")
	best := ""
	for _, s := range signals {
		n := s.name
		if len(n) > len(best) && (v == n || strings.HasPrefix(v, n+"-") || strings.HasPrefix(v, n+"@")) {
			best = n
		}
	}
	return best
}

func truthy(val string) bool {
	return val != "" && val != "0" && !strings.EqualFold(val, "false")
}

func equals(want string) func(string) bool {
	return func(val string) bool { return strings.EqualFold(val, want) }
}

func contains(sub string) func(string) bool {
	return func(val string) bool { return strings.Contains(val, sub) }
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
