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
