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
