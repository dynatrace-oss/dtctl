package agentmode

import (
	"testing"
)

// clearAgentEnv empties every variable Detect consults, so the developer's own
// agent session cannot leak into a test. Empty means "not set" to Detect.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, v := range EnvVars() {
		t.Setenv(v, "")
	}
}

func TestDetect_NoAgent(t *testing.T) {
	clearAgentEnv(t)
	if info := Detect(); info.Detected || info.Name != "" {
		t.Errorf("expected no detection, got %+v", info)
	}
}

// TestDetect_Signals pins the env var each agent actually sets on the
// processes it spawns. Each case sets exactly one variable.
func TestDetect_Signals(t *testing.T) {
	cases := []struct {
		envVar, value, want string
	}{
		{"CLAUDECODE", "1", "claude-code"},
		{"CODEX_CI", "1", "codex"},
		{"CODEX_THREAD_ID", "019a0000-0000-7000-8000-000000000000", "codex"},
		{"CODEX_SANDBOX", "seatbelt", "codex"},
		{"CURSOR_AGENT", "1", "cursor"},
		{"COPILOT_CLI", "1", "github-copilot"},
		{"COPILOT_AGENT", "1", "github-copilot"},
		{"GEMINI_CLI", "1", "gemini-cli"},
		{"QWEN_CODE", "1", "qwen-code"},
		{"OPENCODE", "1", "opencode"},
		{"AGENT_CONTEXT_OUT", "/tmp/agent-context-out-1234-tooluse_abc.fifo", "kiro"},
		{"KIRO_SESSION_ID", "2f6fabda-849b-4aeb-8eae-e2b5905106aa", "kiro"},
		{"OPENCLAW_SHELL", "exec", "openclaw"},
		{"TABNINE_CLI", "1", "tabnine"},
		{"AWS_EXECUTION_ENV", "AmazonQ-For-CLI Version/1.19.0", "amazon-q"},
		{"AWS_EXECUTION_ENV", "AWS_ECS_FARGATE AmazonQ-For-CLI Version/1.19.0", "amazon-q"},
		{"AUGMENT_AGENT", "1", "augment"},
		{"ANTIGRAVITY_AGENT", "1", "antigravity"},
		{"CLINE_ACTIVE", "true", "cline"},
		{"AGENT", "amp", "amp"},
		// Manual overrides kept for compatibility.
		{"CODEX", "1", "codex"},
		{"GITHUB_COPILOT", "1", "github-copilot"},
		{"KIRO", "1", "kiro"},
		{"OPENCLAW", "1", "openclaw"},
		{"JUNIE", "1", "junie"},
		{"CODEIUM_AGENT", "1", "codeium"},
		{"TABNINE_AGENT", "1", "tabnine"},
		{"AMAZON_Q", "1", "amazon-q"},
	}
	for _, tc := range cases {
		t.Run(tc.envVar+"="+tc.value, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv(tc.envVar, tc.value)
			info := Detect()
			if !info.Detected || info.Name != tc.want {
				t.Errorf("got %+v, want %q", info, tc.want)
			}
		})
	}
}

// TestDetect_NotAnAgent covers values of shared variables that must not count:
// false-ish booleans, and the values a human-driven shell carries.
func TestDetect_NotAnAgent(t *testing.T) {
	cases := []struct {
		envVar, value string
	}{
		{"CLAUDECODE", "0"},
		{"CLAUDECODE", "false"},
		{"CURSOR_AGENT", "FALSE"},
		{"AI_AGENT", "0"},
		{"OPENCLAW_SHELL", "tui-local"},
		{"AWS_EXECUTION_ENV", "AWS_Lambda_python3.12"},
		{"AGENT", "1"},
		{"AGENT", "jenkins"},
	}
	for _, tc := range cases {
		t.Run(tc.envVar+"="+tc.value, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv(tc.envVar, tc.value)
			if info := Detect(); info.Detected {
				t.Errorf("expected no detection, got %+v", info)
			}
		})
	}
}

// TestDetect_AIAgent covers the AI_AGENT self-identification convention. A
// value naming a known agent maps to it; any other value is generic-ai and
// never echoed, since the name ends up in request headers.
func TestDetect_AIAgent(t *testing.T) {
	cases := []struct {
		value, want string
	}{
		{"claude-code_2-1-282_agent", "claude-code"},
		{"github_copilot_vscode_agent", "github-copilot"},
		{"github-copilot", "github-copilot"},
		{"codex@1", "codex"},
		{"Cursor", "cursor"},
		{"devin@1", "generic-ai"},
		{"claude-codex", "generic-ai"},
		{"evil) injected (", "generic-ai"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("AI_AGENT", tc.value)
			info := Detect()
			if !info.Detected || info.Name != tc.want {
				t.Errorf("got %+v, want %q", info, tc.want)
			}
		})
	}
}

// TestDetect_Precedence pins the order among co-set variables. The detector
// used to range over a map, so the name was random whenever two were set —
// and Claude Code always sets both CLAUDECODE and AI_AGENT.
func TestDetect_Precedence(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "Claude Code sets CLAUDECODE and AI_AGENT",
			env:  map[string]string{"CLAUDECODE": "1", "AI_AGENT": "claude-code_2-1-282_agent"},
			want: "claude-code",
		},
		{
			name: "unknown AI_AGENT defers to a specific signal",
			env:  map[string]string{"CLAUDECODE": "1", "AI_AGENT": "something-new"},
			want: "claude-code",
		},
		{
			name: "known AI_AGENT outranks inherited signals",
			env:  map[string]string{"CLAUDECODE": "1", "AI_AGENT": "github_copilot_vscode_agent"},
			want: "github-copilot",
		},
		{
			name: "Amp also sets CLAUDECODE",
			env:  map[string]string{"CLAUDECODE": "1", "AGENT": "amp"},
			want: "amp",
		},
		{
			name: "real signal outranks a manual override",
			env:  map[string]string{"JUNIE": "1", "CURSOR_AGENT": "1"},
			want: "cursor",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			// Repeat: an order bug shows up as a flaky name, not a wrong one.
			for range 50 {
				if info := Detect(); info.Name != tc.want {
					t.Fatalf("got %+v, want %q", info, tc.want)
				}
			}
		})
	}
}

func TestEnvVars_CoversEverySignal(t *testing.T) {
	vars := map[string]bool{}
	for _, v := range EnvVars() {
		if vars[v] {
			t.Errorf("EnvVars lists %s twice", v)
		}
		vars[v] = true
	}
	if !vars["AI_AGENT"] {
		t.Error("EnvVars is missing AI_AGENT")
	}
	for _, s := range signals {
		if !vars[s.envVar] {
			t.Errorf("EnvVars is missing %s", s.envVar)
		}
	}
}

func TestUserAgentSuffix_NoAgent(t *testing.T) {
	clearAgentEnv(t)
	if s := UserAgentSuffix(); s != "" {
		t.Errorf("expected empty suffix, got %q", s)
	}
}

func TestUserAgentSuffix_WithAgent(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CURSOR_AGENT", "1")
	if s := UserAgentSuffix(); s != " (AI-Agent: cursor)" {
		t.Errorf("got %q", s)
	}
}
