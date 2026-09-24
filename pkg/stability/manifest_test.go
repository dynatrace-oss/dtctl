package stability

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// A hidden persistent flag on the root is a parser shim, not a promise that
// every command accepts it: the root keeps --dry-run hidden only so cobra can
// still parse it ahead of the subcommand, and a command without a dry run
// rejects it. Listing it under (global) would advertise a stable contract dtctl
// no longer offers.
func TestManifestOmitsHiddenGlobalFlags(t *testing.T) {
	root := &cobra.Command{Use: "dtctl"}
	root.PersistentFlags().Bool("visible", false, "")
	root.PersistentFlags().Bool("shim", false, "")
	_ = root.PersistentFlags().MarkHidden("shim")
	child := &cobra.Command{Use: "get", Run: func(*cobra.Command, []string) {}}
	Mark(child, Stable, "")
	root.AddCommand(child)

	m := Manifest(root)

	if !strings.Contains(m, "  --visible ") {
		t.Errorf("visible global flag missing from the manifest:\n%s", m)
	}
	if strings.Contains(m, "--shim") {
		t.Errorf("hidden global flag listed in the manifest:\n%s", m)
	}
}
