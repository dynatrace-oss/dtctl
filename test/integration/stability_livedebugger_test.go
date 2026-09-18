//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Live Debugger surface is dtctl's first experimental-tier classification,
// and therefore the first time the stability floor is exercised against the
// real command tree rather than a synthetic one (cmd/stability_test.go builds
// its own trees; pkg/serve/serve_test.go covers the development tier). These
// tests drive the actual built binary, because most of what the tier does
// happens in the startup pipeline — registration, masking, badge rendering —
// none of which an in-process unit test observes the way a caller does.
//
// No tenant is needed: every assertion here resolves before the first HTTP
// call, which is itself part of the contract. A floor that only rejected after
// authenticating would leak the surface it is meant to withhold.

// liveDebuggerCommands is the complete experimental Live Debugger command set,
// each paired with the arguments needed to reach its body. Kept explicit rather
// than derived from the tree so that promoting one command out of the tier
// fails here loudly instead of silently shrinking the test.
var liveDebuggerCommands = []struct {
	name string
	args []string
}{
	{"get breakpoints", []string{"get", "breakpoints"}},
	{"get snapshots", []string{"get", "snapshots", "Example.java:1"}},
	{"create breakpoint", []string{"create", "breakpoint", "Example.java:1"}},
	{"describe breakpoint", []string{"describe", "breakpoint", "Example.java:1"}},
	{"update breakpoint", []string{"update", "breakpoint", "Example.java:1"}},
	{"delete breakpoint", []string{"delete", "breakpoint", "Example.java:1", "--yes"}},
}

func TestLiveDebuggerIsExperimental(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the dtctl binary; skipped in -short mode")
	}
	exe := BuildDtctl(t)

	t.Run("reachable and badged at the default floor", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)

		// The default floor is experimental, so the whole surface is present.
		// Help is the assertion target rather than execution: reaching the body
		// would need a tenant, and the e2e suite covers that.
		for _, c := range liveDebuggerCommands {
			code, stdout, stderr := RunDtctl(t, exe, cfg, nil, append(c.args, "--help")...)
			require.Zero(t, code, "%s --help must succeed at the default floor: %s", c.name, stderr)
			require.Contains(t, stdout, "[Experimental]",
				"%s must carry the experimental badge in its own help", c.name)
			require.Contains(t, stdout, "may change or be removed in any release",
				"%s must state the guarantee, not just the tier name", c.name)
		}

		// Badged where a caller browsing the parent verb sees it, too.
		_, stdout, _ := RunDtctl(t, exe, cfg, nil, "get", "--help")
		require.Regexp(t, `breakpoints\s+\[Experimental\]`, stdout)
		require.Regexp(t, `snapshots\s+\[Experimental\]`, stdout)
	})

	t.Run("blocked under a stable floor", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)
		env := map[string]string{"DTCTL_MIN_STABILITY": "stable"}

		for _, c := range liveDebuggerCommands {
			code, stdout, stderr := RunDtctl(t, exe, cfg, env, c.args...)
			out := stdout + stderr
			require.NotZero(t, code, "%s must fail under a stable floor", c.name)
			require.Contains(t, out, "is experimental", "%s: %s", c.name, out)
			require.Contains(t, out, "requires stable or stronger", "%s: %s", c.name, out)

			// The block must not leak the command's shape. `describe
			// breakpoint` and friends normally reject a missing positional
			// argument, and that error would tell a caller the command exists
			// and takes one argument.
			require.NotContains(t, out, "arg(s)", "%s leaked its arg shape: %s", c.name, out)
			require.NotContains(t, out, "unknown flag", "%s leaked its flag shape: %s", c.name, out)
		}

		// Gone from help, so a caller browsing the surface is not told about a
		// command they cannot run.
		_, stdout, _ := RunDtctl(t, exe, cfg, env, "get", "--help")
		require.NotContains(t, stdout, "breakpoints")
		require.NotContains(t, stdout, "snapshots")
	})

	t.Run("agent mode reports a typed code", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)
		env := map[string]string{"DTCTL_MIN_STABILITY": "stable"}

		for _, c := range liveDebuggerCommands {
			_, stdout, _ := RunDtctl(t, exe, cfg, env, append(c.args, "--agent")...)
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Code        string   `json:"code"`
					Message     string   `json:"message"`
					Suggestions []string `json:"suggestions"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "agent output must be an envelope: %s", stdout)
			require.False(t, envelope.OK)
			require.Equal(t, "stability_blocked", envelope.Error.Code, "%s", c.name)
			require.Contains(t, envelope.Error.Message, c.name)
			// The narrow opt-in is named first: lowering the floor would admit
			// every experimental command, which is almost never the intent.
			require.NotEmpty(t, envelope.Error.Suggestions)
			require.Contains(t, envelope.Error.Suggestions[0], c.name)
		}
	})

	t.Run("a per-command exception admits exactly one command", func(t *testing.T) {
		cfg := stabilityConfig(t, "stable", []string{"get breakpoints"})

		code, stdout, stderr := RunDtctl(t, exe, cfg, nil, "get", "breakpoints", "--help")
		require.Zero(t, code, "the excepted command must be usable: %s", stderr)
		require.Contains(t, stdout, "[Experimental]",
			"an exception admits the command; it does not promote it")

		// Everything else in the family stays blocked. This is the whole reason
		// exceptions are per-target rather than a floor: wanting one Live
		// Debugger command must not grant the other five.
		for _, c := range liveDebuggerCommands[1:] {
			code, stdout, stderr := RunDtctl(t, exe, cfg, nil, c.args...)
			require.NotZero(t, code, "%s must stay blocked", c.name)
			require.Contains(t, stdout+stderr, "is experimental", c.name)
		}
	})

	t.Run("the catalog states the tier in every detail mode", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)

		// Resource-style commands (`get breakpoints`) are rendered as a bare
		// name list, so resource_stability is the only place their contract can
		// appear. An agent bootstrapping from the compact default must see it.
		for _, mode := range []string{"", "--brief", "--full"} {
			args := []string{"commands", "-o", "json"}
			if mode != "" {
				args = append(args, mode)
			}
			_, stdout, stderr := RunDtctl(t, exe, cfg, nil, args...)
			var listing struct {
				Verbs map[string]struct {
					ResourceStability map[string]string `json:"resource_stability"`
					Flags             map[string]struct {
						Stability string `json:"stability"`
					} `json:"flags"`
				} `json:"verbs"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &listing), "mode %q: %s", mode, stderr)

			for verb, resource := range map[string]string{
				"get": "breakpoints", "create": "breakpoint", "delete": "breakpoint",
				"describe": "breakpoint", "update": "breakpoint",
			} {
				require.Equal(t, "experimental", listing.Verbs[verb].ResourceStability[resource],
					"mode %q: %s %s must report its tier", mode, verb, resource)
			}
			require.Equal(t, "experimental", listing.Verbs["get"].ResourceStability["snapshots"],
				"mode %q: get snapshots must report its tier", mode)

			// The minimal default carries no flags at all, by design.
			if mode == "" {
				continue
			}
			require.Equal(t, "experimental", listing.Verbs["query"].Flags["--decode-snapshots"].Stability,
				"mode %q: an experimental flag on a stable command must say so", mode)
		}
	})

	t.Run("the catalog explains a floor it is filtered by", func(t *testing.T) {
		cfg := stabilityConfig(t, "stable", []string{"get breakpoints"})
		_, stdout, stderr := RunDtctl(t, exe, cfg, nil, "commands", "-o", "json")

		var listing struct {
			MinStability        string   `json:"min_stability"`
			StabilityExceptions []string `json:"stability_exceptions"`
			Verbs               map[string]struct {
				Resources         []string          `json:"resources"`
				ResourceStability map[string]string `json:"resource_stability"`
			} `json:"verbs"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &listing), stderr)

		// Without both fields a reader could not tell why one below-floor
		// command is present and the rest are not.
		require.Equal(t, "stable", listing.MinStability)
		require.Equal(t, []string{"get breakpoints"}, listing.StabilityExceptions)

		require.Contains(t, listing.Verbs["get"].Resources, "breakpoints")
		require.NotContains(t, listing.Verbs["get"].Resources, "snapshots",
			"a masked command must be absent from the catalog, not merely flagged")
		require.Equal(t, "experimental", listing.Verbs["get"].ResourceStability["breakpoints"])
		require.NotContains(t, listing.Verbs["get"].ResourceStability, "snapshots")
	})
}

// TestQueryDecodeSnapshotsIsExperimental covers the other half of the
// classification: a Live Debugger *flag* on a stable command. `query` itself
// must keep working under a stable floor — only the flag is withheld — which
// is the case a per-flag tier exists for.
func TestQueryDecodeSnapshotsIsExperimental(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the dtctl binary; skipped in -short mode")
	}
	exe := BuildDtctl(t)
	dql := "fetch logs | limit 1"

	t.Run("badged at the default floor", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)
		_, stdout, _ := RunDtctl(t, exe, cfg, nil, "query", "--help")
		require.Regexp(t, `--decode-snapshots[^\n]*\n?[^\n]*\[Experimental\]`, stdout)
	})

	t.Run("rejected under a stable floor while the command still works", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)
		env := map[string]string{"DTCTL_MIN_STABILITY": "stable"}

		code, stdout, stderr := RunDtctl(t, exe, cfg, env, "query", dql, "--decode-snapshots")
		out := stdout + stderr
		require.NotZero(t, code)
		require.Contains(t, out, `flag --decode-snapshots of command "query" is experimental`)
		// Rejected on *use*, not removed: an unknown-flag error would be
		// indistinguishable from a typo.
		require.NotContains(t, out, "unknown flag")

		// The command that carries the flag is stable and stays reachable.
		code, _, stderr = RunDtctl(t, exe, cfg, env, "query", "--help")
		require.Zero(t, code, stderr)
	})

	t.Run("hidden from help under a stable floor", func(t *testing.T) {
		cfg := stabilityConfig(t, "", nil)
		env := map[string]string{"DTCTL_MIN_STABILITY": "stable"}
		_, stdout, _ := RunDtctl(t, exe, cfg, env, "query", "--help")
		require.NotContains(t, stdout, "--decode-snapshots")
	})

	t.Run("a per-flag exception admits just the flag", func(t *testing.T) {
		cfg := stabilityConfig(t, "stable", []string{"query --decode-snapshots"})

		_, stdout, _ := RunDtctl(t, exe, cfg, nil, "query", "--help")
		require.Contains(t, stdout, "--decode-snapshots",
			"the excepted flag must be visible again")

		// It now gets past the floor and fails on the tenant instead, which is
		// the point: the contract stopped being the obstacle.
		_, stdout, stderr := RunDtctl(t, exe, cfg, nil, "query", dql, "--decode-snapshots")
		require.NotContains(t, stdout+stderr, "is experimental")

		// The exception is scoped to the flag, so the sibling commands in the
		// same family are untouched.
		code, stdout, stderr := RunDtctl(t, exe, cfg, nil, "get", "breakpoints")
		require.NotZero(t, code)
		require.Contains(t, stdout+stderr, `command "get breakpoints" is experimental`)
	})
}

// stabilityConfig writes a config whose only variable parts are the floor and
// its exceptions. The environment is unroutable on purpose: everything
// asserted in this file resolves before the first request, and a config that
// could reach a tenant would hide a floor that only blocks after connecting.
func stabilityConfig(t *testing.T, minStability string, exceptions []string) string {
	t.Helper()
	return WriteCLIConfig(t, CLIConfig{
		Environment:         UnroutableEnvironment,
		Token:               "dt0s16.NOT-A-REAL-TOKEN",
		MinStability:        minStability,
		StabilityExceptions: exceptions,
	})
}
