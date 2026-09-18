//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The inventory surface is classified experimental on its subtree root, which
// makes it the case that exercises *inheritance* — of the level down to the
// subcommand, and of the level onto flags that declare nothing themselves.
// Both were wrong once: a command-level exception admitted `inventory` and then
// refused `inventory --budget-queries`, which made a command-level grant
// unusable for any command that has flags.
//
// The environment is unroutable, so every assertion here resolves before the
// first request.
func TestInventoryIsExperimental(t *testing.T) {
	exe := BuildDtctl(t)

	t.Run("the subtree root's level reaches its subcommand", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)

		for _, args := range [][]string{{"inventory", "--help"}, {"inventory", "arrivals", "--help"}} {
			code, stdout, stderr := RunDtctl(t, exe, cfg, nil, args...)
			require.Zero(t, code, "stderr: %s", stderr)
			require.Contains(t, stdout, "[Experimental]")
			require.Contains(t, stdout, "may change or be removed",
				"the badge states the guarantee, so nobody has to look it up")
		}
	})

	t.Run("a stable floor blocks the whole subtree", func(t *testing.T) {
		cfg := stabilityConfig(t, "stable", nil)

		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"inventory"}, `command "inventory" is experimental`},
			{[]string{"inventory", "arrivals"}, `command "inventory arrivals" is experimental`},
		} {
			code, stdout, stderr := RunDtctl(t, exe, cfg, nil, tc.args...)
			require.NotZero(t, code)
			out := stdout + stderr
			require.Contains(t, out, tc.want)
			// Masked like a profile mask: no arg or flag shape may leak, or the
			// floor would still be describing a surface it refuses to serve.
			require.NotContains(t, out, "arg(s)")
			require.NotContains(t, out, "unknown flag")
		}
	})

	t.Run("a command exception carries the command's own flags", func(t *testing.T) {
		// The flags below declare no level of their own; they are experimental
		// only because `inventory` is. Requiring a separate entry for each
		// would mean this exception had to list all of them — and would break
		// the next time `inventory` gained one.
		cfg := stabilityConfig(t, "stable", []string{"inventory"})

		code, stdout, stderr := RunDtctl(t, exe, cfg, nil,
			"inventory", "--budget-queries", "1", "--scan-limit-gbytes", "1")
		out := stdout + stderr
		require.NotContains(t, out, "stability", "the flags come with the command they belong to")
		// It got as far as the unroutable environment, which is proof it passed
		// every narrowing stage.
		require.NotZero(t, code)
	})

	t.Run("a command exception does not carry its subcommands", func(t *testing.T) {
		// A subcommand is a distinct command with its own behaviour, so an
		// audited grant should name it. This is the line: the flags of what you
		// named are included, other commands under it are not.
		cfg := stabilityConfig(t, "stable", []string{"inventory"})

		code, stdout, stderr := RunDtctl(t, exe, cfg, nil, "inventory", "arrivals")
		require.NotZero(t, code)
		require.Contains(t, stdout+stderr, `command "inventory arrivals" is experimental`)

		// In agent mode the refusal also names the entry that would admit it,
		// with the narrow grant first: an agent that can only read one
		// suggestion should not be steered into lowering the whole floor.
		code, stdout, stderr = RunDtctl(t, exe, cfg, nil, "inventory", "arrivals", "--agent")
		require.NotZero(t, code)
		out := stdout + stderr
		require.Contains(t, out, `"code":"stability_blocked"`)
		require.Contains(t, out, `\"inventory arrivals\" to stability-exceptions`)
	})

	t.Run("naming the subcommand admits it", func(t *testing.T) {
		cfg := stabilityConfig(t, "stable", []string{"inventory arrivals"})

		code, _, stderr := RunDtctl(t, exe, cfg, nil, "inventory", "arrivals", "--budget-queries", "1")
		require.NotContains(t, stderr, "stability")
		require.NotZero(t, code, "the unroutable environment still fails the call")

		// ... and the parent stays blocked, so the grant is exactly what it says.
		code, stdout, stderr := RunDtctl(t, exe, cfg, nil, "inventory")
		require.NotZero(t, code)
		require.Contains(t, stdout+stderr, `command "inventory" is experimental`)
	})

	t.Run("the catalog omits the subtree at a stable floor", func(t *testing.T) {
		cfg := stabilityConfig(t, "stable", nil)

		code, stdout, stderr := RunDtctl(t, exe, cfg, nil, "commands", "-o", "json")
		require.Zero(t, code, "stderr: %s", stderr)
		require.Contains(t, stdout, `"min_stability": "stable"`)
		require.NotContains(t, stdout, "inventory")

		// At the default floor it is listed *with* its tier, so a reader is
		// never left inferring a guarantee from a name.
		cfg = stabilityConfig(t, "", nil)
		code, stdout, stderr = RunDtctl(t, exe, cfg, nil, "commands", "--full", "-o", "json")
		require.Zero(t, code, "stderr: %s", stderr)
		require.Contains(t, stdout, "inventory")
		require.Contains(t, stdout, `"stability": "experimental"`)
	})
}
