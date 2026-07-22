package cmd

import (
	"strings"
	"testing"
)

func TestCreateBreakpointCommandRegistration(t *testing.T) {
	createCmd, _, err := rootCmd.Find([]string{"create"})
	if err != nil {
		t.Fatalf("expected create command to exist, got error: %v", err)
	}
	if createCmd == nil || createCmd.Name() != "create" {
		t.Fatalf("expected create command to exist")
	}

	breakpointCmd, _, err := rootCmd.Find([]string{"create", "breakpoint"})
	if err != nil {
		t.Fatalf("expected create breakpoint command to exist, got error: %v", err)
	}
	if breakpointCmd == nil || breakpointCmd.Name() != "breakpoint" {
		t.Fatalf("expected create breakpoint command to exist")
	}
}

func TestCreateBreakpointFiltersFlagRegistered(t *testing.T) {
	if createBreakpointCmd.Flags().Lookup("filters") == nil {
		t.Fatalf("expected --filters flag to be registered on create breakpoint")
	}
}

func TestCreateBreakpointYesFlagRegistered(t *testing.T) {
	flag := createBreakpointCmd.Flags().Lookup("yes")
	if flag == nil {
		t.Fatalf("expected --yes flag to be registered on create breakpoint")
	}
	if flag.Shorthand != "y" {
		t.Fatalf("expected --yes shorthand 'y', got %q", flag.Shorthand)
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected --yes default 'false', got %q", flag.DefValue)
	}
}

func TestExtractCreateBreakpointImmutableID(t *testing.T) {
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"org": map[string]interface{}{
				"workspace": map[string]interface{}{
					"createRuleV2": map[string]interface{}{
						"immutableId": "96294",
					},
				},
			},
		},
	}
	if id := extractCreateBreakpointImmutableID(resp); id != "00000000000000000000000000096294" {
		t.Fatalf("expected padded immutable ID, got %q", id)
	}
	if id := extractCreateBreakpointImmutableID(map[string]interface{}{}); id != "" {
		t.Fatalf("expected empty string for missing immutable ID, got %q", id)
	}
}

func TestPadBreakpointID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"96294", "00000000000000000000000000096294"},
		{"1", "00000000000000000000000000000001"},
		{"", ""},
		{"00000000000000000000000000096294", "00000000000000000000000000096294"},
		{"999999999999999999999999999999999", "999999999999999999999999999999999"}, // longer than 32 — pass through
	}
	for _, tt := range tests {
		if got := padBreakpointID(tt.input); got != tt.want {
			t.Fatalf("padBreakpointID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestCreateBreakpointFiltersValidation exercises the early --filters validation
// that runs before any config or network call, so no client/config setup is
// required. The flag state is saved and restored to avoid leaking into other tests.
func TestCreateBreakpointFiltersValidation(t *testing.T) {
	cmd := createBreakpointCmd
	flag := cmd.Flags().Lookup("filters")
	if flag == nil {
		t.Fatalf("expected --filters flag to be registered on create breakpoint")
	}

	originalValue := flag.Value.String()
	originalChanged := flag.Changed
	defer func() {
		_ = flag.Value.Set(originalValue)
		flag.Changed = originalChanged
	}()

	t.Run("empty value", func(t *testing.T) {
		if err := flag.Value.Set(""); err != nil {
			t.Fatalf("failed to set flag: %v", err)
		}
		flag.Changed = true

		err := cmd.RunE(cmd, []string{"OrderController.java:306"})
		if err == nil || !strings.Contains(err.Error(), "provided without a value") {
			t.Fatalf("expected 'provided without a value' error, got %v", err)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		if err := flag.Value.Set("no-separator"); err != nil {
			t.Fatalf("failed to set flag: %v", err)
		}
		flag.Changed = true

		err := cmd.RunE(cmd, []string{"OrderController.java:306"})
		if err == nil || !strings.Contains(err.Error(), "invalid filter") {
			t.Fatalf("expected invalid filter error, got %v", err)
		}
	})
}
