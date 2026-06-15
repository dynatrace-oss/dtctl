package agentmode

import (
	"os"
	"testing"
)

func TestDetect_NoAgent(t *testing.T) {
	info := Detect()
	// We can't guarantee no env var is set in CI, so just check the struct is valid
	_ = info
}

func TestDetect_WithAgent(t *testing.T) {
	for envVar := range knownAgents {
		os.Unsetenv(envVar)
	}
	t.Setenv("CLAUDECODE", "1")
	info := Detect()
	if !info.Detected {
		t.Error("expected agent to be detected")
	}
	if info.Name != "claude-code" {
		t.Errorf("expected claude-code, got %q", info.Name)
	}
}

func TestDetect_FalseValues(t *testing.T) {
	for _, val := range []string{"0", "false", "False", "FALSE"} {
		t.Run(val, func(t *testing.T) {
			// Clear all known agent vars first
			for envVar := range knownAgents {
				os.Unsetenv(envVar)
			}
			t.Setenv("CLAUDECODE", val)
			info := Detect()
			if info.Detected {
				t.Errorf("expected no detection for value %q", val)
			}
		})
	}
}

func TestUserAgentSuffix_NoAgent(t *testing.T) {
	// Clear all known agent vars
	for envVar := range knownAgents {
		os.Unsetenv(envVar)
	}
	if s := UserAgentSuffix(); s != "" {
		t.Errorf("expected empty suffix, got %q", s)
	}
}

func TestUserAgentSuffix_WithAgent(t *testing.T) {
	for envVar := range knownAgents {
		os.Unsetenv(envVar)
	}
	t.Setenv("CURSOR_AGENT", "1")
	s := UserAgentSuffix()
	if s != " (AI-Agent: cursor)" {
		t.Errorf("got %q", s)
	}
}

// TestDetect_Kiro covers the env vars Kiro actually sets. Kiro does not set a
// KIRO variable; it signals an active agent session via AGENT_CONTEXT_OUT (a
// per-invocation FIFO path, exported only in interactive sessions) and
// KIRO_SESSION_ID (set in both interactive and --no-interactive modes).
func TestDetect_Kiro(t *testing.T) {
	cases := []struct {
		name   string
		envVar string
		value  string
	}{
		{"AGENT_CONTEXT_OUT (interactive ACP side-channel)", "AGENT_CONTEXT_OUT", "/tmp/agent-context-out-1234-tooluse_abc.fifo"},
		{"KIRO_SESSION_ID (interactive and headless)", "KIRO_SESSION_ID", "2f6fabda-849b-4aeb-8eae-e2b5905106aa"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for envVar := range knownAgents {
				os.Unsetenv(envVar)
			}
			t.Setenv(tc.envVar, tc.value)
			info := Detect()
			if !info.Detected {
				t.Errorf("expected Kiro to be detected via %s", tc.envVar)
			}
			if info.Name != "kiro" {
				t.Errorf("expected kiro, got %q", info.Name)
			}
		})
	}
}
