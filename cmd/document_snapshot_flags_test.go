package cmd

import (
	"strings"
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

// TestValidateSnapshotFlags pins the dependency between the two flags: a
// description on its own would otherwise be dropped silently, leaving the user
// with neither a snapshot nor a warning.
func TestValidateSnapshotFlags(t *testing.T) {
	tests := []struct {
		name           string
		createSnapshot bool
		description    string
		wantErr        bool
	}{
		{name: "neither flag", wantErr: false},
		{name: "flag alone", createSnapshot: true, wantErr: false},
		{name: "flag with description", createSnapshot: true, description: "before Q3 rework", wantErr: false},
		{name: "orphaned description", description: "before Q3 rework", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cobra.Command{Use: "x"}
			c.Flags().Bool("create-snapshot", false, "")
			c.Flags().String("snapshot-description", "", "")
			if tt.createSnapshot {
				_ = c.Flags().Set("create-snapshot", "true")
			}
			if tt.description != "" {
				_ = c.Flags().Set("snapshot-description", tt.description)
			}

			err := validateSnapshotFlags(c)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error for a description without --create-snapshot")
				}
				if !strings.Contains(err.Error(), "--snapshot-description requires --create-snapshot") {
					t.Errorf("error = %v, want it to name both flags", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestEditCommandsValidateSnapshotFlags ensures the edit commands reject an
// orphaned description before the editor opens, not after the user has already
// written their changes.
func TestEditCommandsValidateSnapshotFlags(t *testing.T) {
	for name, c := range map[string]*cobra.Command{
		"edit dashboard": editDashboardCmd,
		"edit notebook":  editNotebookCmd,
		"edit document":  editDocumentCmd,
	} {
		if c.PreRunE == nil {
			t.Errorf("%s has no PreRunE to validate the snapshot flags", name)
			continue
		}

		t.Cleanup(func() { _ = c.Flags().Set("snapshot-description", "") })
		if err := c.Flags().Set("snapshot-description", "orphaned"); err != nil {
			t.Fatalf("%s: setting the flag: %v", name, err)
		}
		if err := c.PreRunE(c, []string{"doc-1"}); err == nil {
			t.Errorf("%s accepted --snapshot-description without --create-snapshot", name)
		}
		_ = c.Flags().Set("snapshot-description", "")
	}
}

// TestSnapshotFlagsValidatedBeforeIO checks the file-driven commands reject the
// orphaned description up front — before reading the file or loading config.
func TestSnapshotFlagsValidatedBeforeIO(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{
			name: "update document",
			cmd:  updateDocumentCmd,
			args: []string{"update", "document", "-f", "does-not-exist.yaml", "--snapshot-description", "orphaned"},
		},
		{
			name: "apply",
			cmd:  applyCmd,
			args: []string{"apply", "-f", "does-not-exist.yaml", "--snapshot-description", "orphaned"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				rootCmd.SetArgs(nil)
				_ = tt.cmd.Flags().Set("snapshot-description", "")
				_ = tt.cmd.Flags().Set("file", "")
			})

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected an error for a description without --create-snapshot")
			}
			if !strings.Contains(err.Error(), "--snapshot-description requires --create-snapshot") {
				t.Errorf("error = %v, want the flag-dependency error (not a file/config error)", err)
			}
		})
	}
}
