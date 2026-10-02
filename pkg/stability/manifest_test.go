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

// The manifest has to tell a chosen stable flag from one that merely inherits,
// or scripts/stability/check_compat.py cannot refuse a new flag that is stable
// by omission (#541). The tag goes on every flag carrying its own declaration,
// global ones included, and on nothing else.
func TestManifestTagsDeclaredFlags(t *testing.T) {
	root := &cobra.Command{Use: "dtctl"}
	root.PersistentFlags().Bool("global-inherited", false, "")
	root.PersistentFlags().Bool("global-declared", false, "")
	MarkFlagStable(root, "global-declared")

	get := &cobra.Command{Use: "get", Run: func(*cobra.Command, []string) {}}
	MarkStable(get)
	get.Flags().Bool("inherited", false, "")
	get.Flags().Bool("stable-declared", false, "")
	get.Flags().Bool("experimental-declared", false, "")
	MarkFlagStable(get, "stable-declared")
	MarkFlag(get, "experimental-declared", Experimental, "0.38.0")
	root.AddCommand(get)

	lines := map[string]string{}
	for _, l := range strings.Split(Manifest(root), "\n") {
		if f := strings.Fields(l); len(f) > 0 && strings.HasPrefix(f[0], "--") {
			lines[f[0]] = l
		}
	}
	for flag, want := range map[string]bool{
		"--global-inherited":      false,
		"--global-declared":       true,
		"--inherited":             false,
		"--stable-declared":       true,
		"--experimental-declared": true,
	} {
		line, ok := lines[flag]
		if !ok {
			t.Errorf("%s missing from the manifest", flag)
			continue
		}
		if got := strings.HasSuffix(line, "  "+declaredMarker); got != want {
			t.Errorf("%s tagged %s = %v, want %v: %q", flag, declaredMarker, got, want, line)
		}
	}
	if line := lines["--stable-declared"]; !strings.Contains(line, " stable  ") {
		t.Errorf("a declared stable flag must still read stable: %q", line)
	}
}
