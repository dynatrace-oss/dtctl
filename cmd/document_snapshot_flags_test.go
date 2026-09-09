package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestSnapshotFlagsRegistered ensures every document write path offers the
// opt-in snapshot flags, so 'dtctl history'/'dtctl restore' stay usable no
// matter which command performed the update.
func TestSnapshotFlagsRegistered(t *testing.T) {
	commands := map[string]*cobra.Command{
		"apply":           applyCmd,
		"update document": updateDocumentCmd,
		"edit dashboard":  editDashboardCmd,
		"edit notebook":   editNotebookCmd,
		"edit document":   editDocumentCmd,
	}

	for name, c := range commands {
		f := c.Flags().Lookup("create-snapshot")
		if f == nil {
			t.Errorf("%s is missing the --create-snapshot flag", name)
			continue
		}
		if f.Value.Type() != "bool" {
			t.Errorf("%s: --create-snapshot is %q, want bool", name, f.Value.Type())
		}
		if f.DefValue != "false" {
			t.Errorf("%s: --create-snapshot defaults to %q, want false (snapshots are opt-in)", name, f.DefValue)
		}
		if d := c.Flags().Lookup("snapshot-description"); d == nil {
			t.Errorf("%s is missing the --snapshot-description flag", name)
		}
	}
}

// TestEditUpdateRequest_SnapshotOptIn covers the flag → request mapping used by
// the edit commands, including the default (no snapshot) case.
func TestEditUpdateRequest_SnapshotOptIn(t *testing.T) {
	newCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "edit"}
		c.Flags().Bool("create-snapshot", false, "")
		c.Flags().String("snapshot-description", "", "")
		return c
	}

	c := newCmd()
	req := editUpdateRequest(c, []byte(`{"k":"v"}`))
	if req.CreateSnapshot {
		t.Error("expected no snapshot without --create-snapshot")
	}
	if req.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want application/json", req.ContentType)
	}
	if string(req.Content) != `{"k":"v"}` {
		t.Errorf("Content = %q, want the edited payload", req.Content)
	}

	c = newCmd()
	_ = c.Flags().Set("create-snapshot", "true")
	_ = c.Flags().Set("snapshot-description", "before Q3 rework")
	req = editUpdateRequest(c, []byte(`{}`))
	if !req.CreateSnapshot {
		t.Error("expected CreateSnapshot with --create-snapshot")
	}
	if req.SnapshotDescription != "before Q3 rework" {
		t.Errorf("SnapshotDescription = %q, want 'before Q3 rework'", req.SnapshotDescription)
	}
}
