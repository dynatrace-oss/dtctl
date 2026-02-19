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

func TestFormatSuggestions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		itemType    string
		unknown     string
		suggestions []Suggestion
		expected    string
	}{
		{
			name:        "no suggestions",
			itemType:    "command",
			unknown:     "xyz",
			suggestions: []Suggestion{},
			expected:    `unknown command "xyz"`,
		},
		{
			name:     "single suggestion",
			itemType: "resource",
			unknown:  "workflo",
			suggestions: []Suggestion{
				{Value: "workflow", Distance: 1},
			},
			expected: `unknown resource "workflo", did you mean "workflow"?`,
		},
		{
			name:     "multiple suggestions",
			itemType: "command",
			unknown:  "gt",
			suggestions: []Suggestion{
				{Value: "get", Distance: 1},
				{Value: "set", Distance: 2},
			},
			expected: `unknown command "gt", did you mean one of: "get", "set"?`,
		},
		{
			name:     "three suggestions",
			itemType: "flag",
			unknown:  "vrbose",
			suggestions: []Suggestion{
				{Value: "verbose", Distance: 1},
				{Value: "version", Distance: 3},
				{Value: "verify", Distance: 3},
			},
			expected: `unknown flag "vrbose", did you mean one of: "verbose", "version", "verify"?`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatSuggestions(tt.itemType, tt.unknown, tt.suggestions)
			if got != tt.expected {
				t.Errorf("FormatSuggestions() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCommandError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      *CommandError
		contains []string
	}{
		{
			name: "with single suggestion",
			err: &CommandError{
				Command: "gt",
				Message: "unknown command \"gt\"",
				Suggestion: &Suggestion{
					Value:    "get",
					Distance: 1,
				},
			},
			contains: []string{"unknown command", "gt", "did you mean \"get\"?"},
		},
		{
			name: "with multiple suggestions",
			err: &CommandError{
				Command: "dlt",
				Message: "unknown command \"dlt\"",
				Suggestions: []Suggestion{
					{Value: "delete", Distance: 2},
					{Value: "edit", Distance: 3},
				},
			},
			contains: []string{"unknown command", "dlt", "did you mean one of:", "\"delete\"", "\"edit\""},
		},
		{
			name: "with usage hint",
			err: &CommandError{
				Command:   "xyz",
				Message:   "unknown command \"xyz\"",
				UsageHint: "Run 'dtctl --help' for usage.",
			},
			contains: []string{"unknown command", "xyz", "Run 'dtctl --help' for usage."},
		},
		{
			name: "with suggestion and usage hint",
			err: &CommandError{
				Command: "gt",
				Message: "unknown command \"gt\"",
				Suggestion: &Suggestion{
					Value:    "get",
					Distance: 1,
				},
				UsageHint: "See available commands with --help",
			},
			contains: []string{"unknown command", "gt", "did you mean \"get\"?", "See available commands with --help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			errMsg := tt.err.Error()
			for _, expected := range tt.contains {
				if !contains(errMsg, expected) {
					t.Errorf("Error() = %q, should contain %q", errMsg, expected)
				}
			}
		})
	}
}

func TestParseCommandError(t *testing.T) {
	t.Parallel()

	commands := []string{"get", "set", "delete", "describe", "edit", "apply"}

	tests := []struct {
		name     string
		errMsg   string
		contains string
		wantCmd  string
	}{
		{
			name:     "unknown command gt",
			errMsg:   `unknown command "gt" for "dtctl"`,
			contains: "did you mean \"get\"?",
			wantCmd:  "gt",
		},
		{
			name:     "unknown command dlt",
			errMsg:   `unknown command "dlt" for "dtctl"`,
			contains: "did you mean",
			wantCmd:  "dlt",
		},
		{
			name:     "unknown command xyz - no match",
			errMsg:   `unknown command "xyz" for "dtctl"`,
			contains: "unknown command \"xyz\"",
			wantCmd:  "xyz",
		},
		{
			name:     "non-matching error",
			errMsg:   "some other error message",
			contains: "some other error message",
			wantCmd:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ParseCommandError(tt.errMsg, commands)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			errMsg := err.Error()
			if !contains(errMsg, tt.contains) {
				t.Errorf("error %q should contain %q", errMsg, tt.contains)
			}
			if cmdErr, ok := err.(*CommandError); ok && tt.wantCmd != "" {
				if cmdErr.Command != tt.wantCmd {
					t.Errorf("CommandError.Command = %q, want %q", cmdErr.Command, tt.wantCmd)
				}
			}
		})
	}
}

func TestParseCommandErrorWithHint(t *testing.T) {
	t.Parallel()

	commands := []string{"get", "set", "delete"}
	usageHint := "Run 'dtctl --help' for available commands"

	err := ParseCommandErrorWithHint(`unknown command "gt" for "dtctl"`, commands, usageHint)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if !contains(errMsg, "did you mean \"get\"?") {
		t.Errorf("error should contain suggestion: %q", errMsg)
	}
	if !contains(errMsg, usageHint) {
		t.Errorf("error should contain usage hint: %q", errMsg)
	}

	cmdErr, ok := err.(*CommandError)
	if !ok {
		t.Fatal("expected *CommandError")
	}
	if cmdErr.UsageHint != usageHint {
		t.Errorf("UsageHint = %q, want %q", cmdErr.UsageHint, usageHint)
	}
}

func TestFindClosestN_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		candidates []string
		n          int
		wantLen    int
		wantFirst  string
	}{
		{
			name:       "empty candidates",
			input:      "test",
			candidates: []string{},
			n:          3,
			wantLen:    0,
		},
		{
			name:       "n is zero",
			input:      "test",
			candidates: []string{"test", "rest", "best"},
			n:          0,
			wantLen:    0,
		},
		{
			name:       "n is negative",
			input:      "test",
			candidates: []string{"test", "rest", "best"},
			n:          -1,
			wantLen:    0,
		},
		{
			name:       "n greater than matches",
			input:      "gt",
			candidates: []string{"get", "set"},
			n:          5,
			wantLen:    2,
			wantFirst:  "get",
		},
		{
			name:       "case insensitive matching",
			input:      "GET",
			candidates: []string{"get", "set", "delete"},
			n:          1,
			wantLen:    1,
			wantFirst:  "get",
		},
		{
			name:       "alphabetical sorting for same distance",
			input:      "gt",
			candidates: []string{"set", "get", "bet"},
			n:          3,
			wantLen:    3,
			wantFirst:  "get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			suggestions := FindClosestN(tt.input, tt.candidates, tt.n)
			if len(suggestions) != tt.wantLen {
				t.Errorf("FindClosestN() returned %d suggestions, want %d", len(suggestions), tt.wantLen)
			}
			if tt.wantLen > 0 && tt.wantFirst != "" {
				if suggestions[0].Value != tt.wantFirst {
					t.Errorf("First suggestion = %q, want %q", suggestions[0].Value, tt.wantFirst)
				}
			}
		})
	}
}

func TestParseFlagError_EdgeCases(t *testing.T) {
	t.Parallel()

	flags := []string{"owner", "output", "verbose"}

	tests := []struct {
		name     string
		errMsg   string
		wantFlag string
		contains string
	}{
		{
			name:     "flag with double dash",
			errMsg:   "unknown flag: --ownr",
			wantFlag: "ownr",
			contains: "did you mean --owner?",
		},
		{
			name:     "shorthand flag with quotes",
			errMsg:   "unknown shorthand flag: 'x'",
			wantFlag: "x",
			contains: "unknown flag --x",
		},
		{
			name:     "non-matching error message",
			errMsg:   "some other error",
			contains: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ParseFlagError(tt.errMsg, flags)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			errMsg := err.Error()
			if !contains(errMsg, tt.contains) {
				t.Errorf("error %q should contain %q", errMsg, tt.contains)
			}
			if flagErr, ok := err.(*FlagError); ok && tt.wantFlag != "" {
				if flagErr.Flag != tt.wantFlag {
					t.Errorf("FlagError.Flag = %q, want %q", flagErr.Flag, tt.wantFlag)
				}
			}
		})
	}
}

func TestFlagError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      *FlagError
		contains string
	}{
		{
			name: "with suggestion",
			err: &FlagError{
				Flag:    "ownr",
				Message: "unknown flag --ownr",
				Suggestion: &Suggestion{
					Value:    "owner",
					Distance: 1,
				},
			},
			contains: "did you mean --owner?",
		},
		{
			name: "without suggestion",
			err: &FlagError{
				Flag:       "xyz",
				Message:    "unknown flag --xyz",
				Suggestion: nil,
			},
			contains: "unknown flag --xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			errMsg := tt.err.Error()
			if !contains(errMsg, tt.contains) {
				t.Errorf("Error() = %q, should contain %q", errMsg, tt.contains)
			}
		})
	}
}
