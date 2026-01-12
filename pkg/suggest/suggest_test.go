package suggest

import (
	"testing"
)

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"owner", "ownr", 1},
		{"owner", "owenr", 2},
		{"workflow", "workflo", 1},
		{"workflow", "wrkflow", 1},
		{"dashboard", "dashbord", 1},
		{"notebook", "notbook", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := LevenshteinDistance(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestFindClosest(t *testing.T) {
	flags := []string{"owner", "output", "verbose", "config", "context", "workflow"}

	tests := []struct {
		input    string
		expected string
	}{
		{"ownr", "owner"},
		{"owenr", "owner"},
		{"outpt", "output"},
		{"verbos", "verbose"},
		{"confg", "config"},
		{"contxt", "context"},
		{"workflo", "workflow"},
		{"xyz123", ""}, // no match within distance
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			suggestion := FindClosest(tt.input, flags)
			if tt.expected == "" {
				if suggestion != nil {
					t.Errorf("FindClosest(%q) = %v, want nil", tt.input, suggestion)
				}
			} else {
				if suggestion == nil {
					t.Errorf("FindClosest(%q) = nil, want %q", tt.input, tt.expected)
				} else if suggestion.Value != tt.expected {
					t.Errorf("FindClosest(%q) = %q, want %q", tt.input, suggestion.Value, tt.expected)
				}
			}
		})
	}
}

func TestFindClosestN(t *testing.T) {
	commands := []string{"get", "set", "delete", "describe", "edit", "apply"}

	suggestions := FindClosestN("gt", commands, 3)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion for 'gt'")
	}
	if suggestions[0].Value != "get" {
		t.Errorf("expected first suggestion to be 'get', got %q", suggestions[0].Value)
	}
}

func TestFormatSuggestion(t *testing.T) {
	tests := []struct {
		itemType   string
		unknown    string
		suggestion *Suggestion
		expected   string
	}{
		{"flag", "ownr", &Suggestion{Value: "owner", Distance: 1}, `unknown flag "ownr", did you mean "owner"?`},
		{"flag", "xyz", nil, `unknown flag "xyz"`},
		{"command", "gt", &Suggestion{Value: "get", Distance: 1}, `unknown command "gt", did you mean "get"?`},
	}

	for _, tt := range tests {
		t.Run(tt.unknown, func(t *testing.T) {
			got := FormatSuggestion(tt.itemType, tt.unknown, tt.suggestion)
			if got != tt.expected {
				t.Errorf("FormatSuggestion() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParseFlagError(t *testing.T) {
	flags := []string{"owner", "output", "verbose", "config"}

	tests := []struct {
		errMsg   string
		contains string
	}{
		{"unknown flag: --ownr", "did you mean --owner?"},
		{"unknown flag: --outpt", "did you mean --output?"},
		{"unknown shorthand flag: 'x'", "unknown flag --x"},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			err := ParseFlagError(tt.errMsg, flags)
			if err == nil {
				t.Fatal("expected error")
			}
			errStr := err.Error()
			if !contains(errStr, tt.contains) {
				t.Errorf("error %q should contain %q", errStr, tt.contains)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
