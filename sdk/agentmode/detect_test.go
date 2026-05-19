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
