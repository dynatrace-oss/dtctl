package cmd

import (
	"strings"
	"testing"
)

// TestUpdateDocumentRegistered verifies the 'document' subcommand (and its
// alias) is wired under 'update'.
func TestUpdateDocumentRegistered(t *testing.T) {
	c, _, err := updateCmd.Find([]string{"document"})
	if err != nil {
		t.Fatalf("update document not found: %v", err)
	}
	if c.Name() != "document" {
		t.Errorf("expected command 'document', got %q", c.Name())
	}

	ca, _, err := updateCmd.Find([]string{"doc"})
	if err != nil || ca.Name() != "document" {
		t.Errorf("expected 'doc' alias to resolve to 'document', got %q (err=%v)", ca.Name(), err)
	}
}

// TestUpdateDocumentRequiresFile ensures the command fails fast when --file is
// omitted, before any network/config access.
func TestUpdateDocumentRequiresFile(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	rootCmd.SetArgs([]string{"update", "document"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention the required 'file' flag, got: %v", err)
	}
}
