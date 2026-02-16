package aidetect

import (
	"os"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected AgentInfo
	}{
		{
			name:    "no AI agent detected",
			envVars: map[string]string{},
			expected: AgentInfo{
				Detected: false,
				Name:     "",
			},
		},
		{
			name: "Claude Code detected",
			envVars: map[string]string{
				"CLAUDECODE": "1",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "claude-code",
			},
		},
		{
			name: "Cursor detected",
			envVars: map[string]string{
				"CURSOR_AGENT": "true",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "cursor",
			},
		},
		{
			name: "GitHub Copilot detected",
			envVars: map[string]string{
				"GITHUB_COPILOT": "true",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "github-copilot",
			},
		},
		{
			name: "Codeium detected",
			envVars: map[string]string{
				"CODEIUM_AGENT": "1",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "codeium",
			},
		},
		{
			name: "TabNine detected",
			envVars: map[string]string{
				"TABNINE_AGENT": "yes",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "tabnine",
			},
		},
		{
			name: "Amazon Q detected",
			envVars: map[string]string{
				"AMAZON_Q": "enabled",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "amazon-q",
			},
		},
		{
			name: "OpenCode detected",
			envVars: map[string]string{
				"OPENCODE": "1",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "opencode",
			},
		},
		{
			name: "generic AI agent detected",
			envVars: map[string]string{
				"AI_AGENT": "custom",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "generic-ai",
			},
		},
		{
			name: "env var set to 0 not detected",
			envVars: map[string]string{
				"CLAUDECODE": "0",
			},
			expected: AgentInfo{
				Detected: false,
				Name:     "",
			},
		},
		{
			name: "env var set to false not detected",
			envVars: map[string]string{
				"CURSOR_AGENT": "false",
			},
			expected: AgentInfo{
				Detected: false,
				Name:     "",
			},
		},
		{
			name: "env var set to FALSE (uppercase) not detected",
			envVars: map[string]string{
				"CURSOR_AGENT": "FALSE",
			},
			expected: AgentInfo{
				Detected: false,
				Name:     "",
			},
		},
		{
			name: "multiple agents set - first one wins",
			envVars: map[string]string{
				"CLAUDECODE":     "1",
				"CURSOR_AGENT":   "true",
				"GITHUB_COPILOT": "true",
			},
			expected: AgentInfo{
				Detected: true,
				Name:     "", // Name depends on map iteration order, just check Detected
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all known environment variables
			for envVar := range knownAgents {
				os.Unsetenv(envVar)
			}

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Cleanup after test
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			got := Detect()

			if got.Detected != tt.expected.Detected {
				t.Errorf("Detect() Detected = %v, want %v", got.Detected, tt.expected.Detected)
			}

			// Only check name if we expect one and it's not the multi-agent test
			if tt.expected.Name != "" && got.Name != tt.expected.Name {
				t.Errorf("Detect() Name = %v, want %v", got.Name, tt.expected.Name)
			}

			// For multi-agent test, just ensure we got one of the agents
			if tt.name == "multiple agents set - first one wins" && got.Detected {
				validNames := []string{"claude-code", "cursor", "github-copilot"}
				found := false
				for _, name := range validNames {
					if got.Name == name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Detect() Name = %v, want one of %v", got.Name, validNames)
				}
			}
		})
	}
}

func TestUserAgentSuffix(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no AI agent - empty suffix",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "Claude Code - proper suffix",
			envVars: map[string]string{
				"CLAUDECODE": "1",
			},
			expected: " (AI-Agent: claude-code)",
		},
		{
			name: "Cursor - proper suffix",
			envVars: map[string]string{
				"CURSOR_AGENT": "true",
			},
			expected: " (AI-Agent: cursor)",
		},
		{
			name: "OpenCode - proper suffix",
			envVars: map[string]string{
				"OPENCODE": "1",
			},
			expected: " (AI-Agent: opencode)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all known environment variables
			for envVar := range knownAgents {
				os.Unsetenv(envVar)
			}

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Cleanup after test
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			got := UserAgentSuffix()

			if got != tt.expected {
				t.Errorf("UserAgentSuffix() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestDetectRealEnvironment tests detection in the actual environment
func TestDetectRealEnvironment(t *testing.T) {
	// This test just ensures Detect() doesn't panic in real environment
	info := Detect()
	_ = info.Detected // Use the value to avoid "unused" warning

	suffix := UserAgentSuffix()
	_ = suffix

	// Test passes if we get here without panic
	t.Log("AI detection works in real environment")
}
