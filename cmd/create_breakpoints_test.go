package cmd

import "testing"

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
