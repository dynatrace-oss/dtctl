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

// TestUpdateDocumentHasLabelFlag verifies the repeatable --label flag is
// registered so labels can be set on update (the counterpart to create's --label).
func TestUpdateDocumentHasLabelFlag(t *testing.T) {
	f := updateDocumentCmd.Flags().Lookup("label")
	if f == nil {
		t.Fatal("update document is missing the --label flag")
	}
	if f.Value.Type() != "stringArray" {
		t.Errorf("expected --label to be a stringArray, got %q", f.Value.Type())
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
