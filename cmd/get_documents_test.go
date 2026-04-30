package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func newDocFlagsCmd(includeType bool) *cobra.Command {
	cmd := &cobra.Command{Use: "get"}
	addDocumentListFlags(cmd, includeType)
	return cmd
}

func TestAddDocumentListFlags_RegistersAll(t *testing.T) {
	cmd := newDocFlagsCmd(true)
	for _, name := range []string{"type", "name", "mine", "filter", "sort", "add-fields", "admin-access"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag %q registered with includeType=true", name)
		}
	}
}

func TestAddDocumentListFlags_SkipsTypeForImplicitTypeCommands(t *testing.T) {
	cmd := newDocFlagsCmd(false)
	if cmd.Flags().Lookup("type") != nil {
		t.Error("expected --type flag NOT registered when includeType=false")
	}
	for _, name := range []string{"name", "mine", "filter", "sort", "add-fields", "admin-access"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag %q registered", name)
		}
	}
}

func TestBuildDocumentFilters_DefaultsImplicitType(t *testing.T) {
	cmd := newDocFlagsCmd(false)
	filters, err := buildDocumentFilters(cmd, nil, "dashboard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filters.Type != "dashboard" {
		t.Errorf("expected Type=dashboard, got %q", filters.Type)
	}
	if filters.Filter != "" {
		t.Errorf("expected empty Filter, got %q", filters.Filter)
	}
	if filters.Owner != "" {
		t.Errorf("expected empty Owner, got %q", filters.Owner)
	}
}

func TestBuildDocumentFilters_RawFilterOverrides(t *testing.T) {
	cmd := newDocFlagsCmd(true)
	_ = cmd.Flags().Set("filter", "originAppId exists")
	_ = cmd.Flags().Set("name", "ignored")

	filters, err := buildDocumentFilters(cmd, nil, "dashboard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filters.Filter != "originAppId exists" {
		t.Errorf("expected raw Filter passed through, got %q", filters.Filter)
	}
	if filters.Type != "" {
		t.Errorf("expected Type cleared when raw Filter set, got %q", filters.Type)
	}
	if filters.Name != "" {
		t.Errorf("expected Name cleared when raw Filter set, got %q", filters.Name)
	}
}

func TestBuildDocumentFilters_WiresSortAddFieldsAdminAccess(t *testing.T) {
	cmd := newDocFlagsCmd(true)
	_ = cmd.Flags().Set("sort", "name,-modificationInfo.lastModifiedTime")
	_ = cmd.Flags().Set("add-fields", "originExtensionId,labels")
	_ = cmd.Flags().Set("admin-access", "true")

	filters, err := buildDocumentFilters(cmd, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filters.Sort != "name,-modificationInfo.lastModifiedTime" {
		t.Errorf("expected Sort wired, got %q", filters.Sort)
	}
	if len(filters.AddFields) != 2 || filters.AddFields[0] != "originExtensionId" || filters.AddFields[1] != "labels" {
		t.Errorf("expected AddFields [originExtensionId labels], got %v", filters.AddFields)
	}
	if !filters.AdminAccess {
		t.Errorf("expected AdminAccess=true, got false")
	}
}
