package commands

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func run(*cobra.Command, []string) {}

// newDeepTestRoot has the shapes #500 found missing or wrong in the catalog:
// a help topic, a leaf with its own flags, and a mutating verb whose leaves
// are three levels deep.
func newDeepTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "dtctl"}
	root.AddCommand(&cobra.Command{Use: "token-scopes", Short: "Token scopes", Long: "A help topic."})

	get := &cobra.Command{Use: "get", Short: "List resources", Run: run}
	dashboards := &cobra.Command{Use: "dashboards", Short: "List dashboards", Run: run}
	dashboards.Flags().Bool("mine", false, "only mine")
	dashboards.Flags().StringP("name", "n", "", "filter by name")
	get.AddCommand(dashboards)
	gcp := &cobra.Command{Use: "gcp", Short: "GCP", Run: run}
	connections := &cobra.Command{Use: "connections", Short: "GCP connections", Run: run}
	connections.AddCommand(&cobra.Command{Use: "principal", Short: "Show the principal", Run: run})
	gcp.AddCommand(connections)
	get.AddCommand(gcp)
	root.AddCommand(get)

	enable := &cobra.Command{Use: "enable", Short: "Enable", Run: run}
	aws := &cobra.Command{Use: "aws", Short: "AWS", Run: run}
	monitoring := &cobra.Command{Use: "monitoring", Short: "Enable monitoring", Run: run}
	monitoring.Flags().String("name", "", "config name")
	aws.AddCommand(monitoring)
	enable.AddCommand(aws)
	root.AddCommand(enable)
	return root
}

func TestBuild_ExcludesHelpTopics(t *testing.T) {
	listing := Build(newDeepTestRoot())

	require.NotContains(t, listing.Verbs, "token-scopes")
}

func TestBuild_NestedLeavesAreNotTruncated(t *testing.T) {
	listing := Build(newDeepTestRoot())

	connections := listing.Verbs["get"].Subcommands["gcp"].Subcommands["connections"]
	require.NotNil(t, connections)
	require.Contains(t, connections.Subcommands, "principal")

	minimal := NewMinimal(listing)
	require.Contains(t, minimal.Verbs["get"].Subcommands["gcp"].Subcommands["connections"].Subcommands, "principal")

	brief := NewBrief(listing)
	require.Contains(t, brief.Verbs["get"].Subcommands["gcp"].Subcommands["connections"].Subcommands, "principal")
}

func TestBuild_NestedLeavesInheritMutating(t *testing.T) {
	listing := Build(newDeepTestRoot())

	monitoring := listing.Verbs["enable"].Subcommands["aws"].Subcommands["monitoring"]
	require.True(t, monitoring.Mutating)
	require.Equal(t, "OperationUpdate", monitoring.SafetyOp)
	require.Equal(t, []string{"--name"}, listing.Verbs["enable"].Subcommands["aws"].ResourceFlags["monitoring"])

	brief := NewBrief(listing)
	require.True(t, brief.Verbs["enable"].Subcommands["aws"].Subcommands["monitoring"].Mutating)
}

func TestBuild_ResourceFlagNames(t *testing.T) {
	listing := Build(newDeepTestRoot())

	require.Equal(t, []string{"--mine", "-n/--name"}, listing.Verbs["get"].ResourceFlags["dashboards"])
	require.Nil(t, NewBrief(listing).Verbs["get"].ResourceFlags, "--brief omits resource_flags")
}

// TestBuild_KeepsFlagsShadowingAGlobal covers the flags a command redefines
// under a name the root already uses persistently. cobra binds the command's
// own flag, so the catalog must describe that one — `dtctl diff --context` is a
// line count, not the global context name.
func TestBuild_KeepsFlagsShadowingAGlobal(t *testing.T) {
	root := &cobra.Command{Use: "dtctl"}
	root.PersistentFlags().String("context", "", "use a specific context")
	root.PersistentFlags().Bool("dry-run", false, "print what would be done")

	diff := &cobra.Command{Use: "diff", Short: "Diff", Run: run}
	diff.Flags().Int("context", 3, "Number of context lines")
	root.AddCommand(diff)

	apply := &cobra.Command{Use: "apply", Short: "Apply", Run: run}
	apply.Flags().Bool("dry-run", false, "preview changes without applying")
	root.AddCommand(apply)

	listing := Build(root)

	require.Contains(t, listing.Verbs["diff"].Flags, "--context")
	require.Equal(t, "integer", listing.Verbs["diff"].Flags["--context"].Type)
	require.Contains(t, listing.Verbs["apply"].Flags, "--dry-run")

	require.NotContains(t, listing.Verbs["diff"].Flags, "--dry-run",
		"a global the command does not redefine stays in global_flags only")
	require.Contains(t, listing.GlobalFlags, "--context")
}
