package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// RunOptions configures a single embedded invocation. The zero value is the
// CLI default: every capability granted, config and credentials resolved from
// the host (config file, env, keyring), host environment untouched.
type RunOptions struct {
	// Capabilities restricts process-level abilities (subprocess spawns) for
	// this invocation. nil grants everything (the CLI default); embedded
	// callers typically pass &Capabilities{} to grant nothing.
	Capabilities *Capabilities

	// Session, when non-nil, pins the invocation to one environment + token
	// and detaches it from the host's config file, contexts, keyring, and
	// credential env vars. See Session.
	Session *Session

	// Env sets environment variables for the duration of the invocation
	// (restored afterwards), e.g. DTCTL_PROFILE to select a command profile
	// per request. Applied after session scrubbing, so an explicit entry wins.
	// The mutation is process-wide while the invocation runs — see
	// applyRunEnvironment.
	Env map[string]string

	// FS resolves user-supplied file paths (-f and friends) for this
	// invocation, including writebacks like `apply --write-id`. nil reads and
	// writes the host filesystem (the CLI default); embedded callers pass a
	// vfs.MapFS built from the request's virtual files. See pkg/vfs.
	FS vfs.FS

	// Stdout and Stderr receive the invocation's output; Stdin feeds commands
	// that read it ("-" file arguments, piped input). nil means the process
	// streams (the CLI default). The redirection captures every output path —
	// fmt.Print*, the output package, cobra help, error envelopes — as the
	// exact byte stream the CLI would print. See redirectStdio.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// BlockedCommands removes top-level commands (with their whole subtree)
	// from this invocation's surface: hidden from help, the `dtctl commands`
	// catalog, and completion, and guarded so dispatch returns an
	// UnsupportedCommandError (agent-mode code "unsupported_in_service").
	// Keyed by top-level command name; the value is the human-readable reason.
	// nil is the full surface (the CLI default). See applyBlockedCommands.
	BlockedCommands map[string]string
}

// runMu serializes invocations. The command tree is package state (277
// command values wired by init), so two interleaved executions would share
// flag values and tree mutations. In-process callers therefore queue;
// parallelism comes from running more instances — the model a WASI spike
// measured at 20-47ms per-instance overhead — or more processes.
// See docs/dev/SERVICE_ENGINE_DESIGN.md ("Serialization").
var runMu sync.Mutex

// runActive is true while an invocation executes (between runMu acquisition
// and release).
var runActive atomic.Bool

// RunActive reports whether a Run invocation is currently executing. Long-
// running commands that themselves embed Run — `dtctl serve` accepting
// requests — use it to refuse execution from inside another invocation:
// blocking in a RunE would hold the invocation lock for the server's whole
// lifetime and deadlock every request (main dispatches serve outside Run for
// exactly this reason).
func RunActive() bool {
	return runActive.Load()
}

// Run executes one dtctl invocation in-process and returns its exit code.
// argv is the command line without the program name (os.Args[1:] shape).
//
// Run is the embedding seam for the service engine and for `dtctl serve`:
// unlike Execute it never terminates the process, and every invocation starts
// from a pristine command tree — flag values reset to declared defaults, and
// per-run tree mutations (command-profile masks, scope-preflight wraps)
// undone. Concurrent calls are safe and execute one at a time.
func Run(argv []string, opts RunOptions) int {
	runMu.Lock()
	defer runMu.Unlock()
	runActive.Store(true)
	defer runActive.Store(false)

	granted := AllCapabilities()
	if opts.Capabilities != nil {
		granted = *opts.Capabilities
	}
	prev := SetCapabilities(granted)
	defer SetCapabilities(prev)

	runBlocked = opts.BlockedCommands
	defer func() { runBlocked = nil }()

	cleanup, err := applyRunEnvironment(opts)
	if err != nil {
		// A malformed RunOptions is an embedding-caller bug, not a command
		// error — report it on the caller's stderr with a usage exit code.
		reportOptionsError(opts, err)
		return client.ExitUsageError
	}
	defer cleanup()

	restoreStdio, err := redirectStdio(opts.Stdout, opts.Stderr, opts.Stdin)
	if err != nil {
		reportOptionsError(opts, err)
		return client.ExitUsageError
	}
	defer restoreStdio()

	restorePristineTree()
	return executeArgs(argv)
}

// reportOptionsError surfaces a RunOptions problem on the invocation's stderr
// (falling back to the process stderr), without going through the redirected
// stream machinery that may itself be the thing that failed.
func reportOptionsError(opts RunOptions, err error) {
	w := io.Writer(os.Stderr)
	if opts.Stderr != nil {
		w = opts.Stderr
	}
	fmt.Fprintf(w, "Error: %v\n", err)
}

// pristineCommandState is the subset of cobra.Command that dtctl mutates
// between construction and execution. applyProfile overwrites RunE/Run/Args/
// Hidden/DisableFlagParsing to mask commands, and installScopePreflight wraps
// RunE; restoring these fields returns a command to its as-registered state.
type pristineCommandState struct {
	runE               func(*cobra.Command, []string) error
	run                func(*cobra.Command, []string)
	args               cobra.PositionalArgs
	hidden             bool
	disableFlagParsing bool
}

var (
	pristineOnce sync.Once
	pristineTree map[*cobra.Command]pristineCommandState
)

func capturePristineState(c *cobra.Command) pristineCommandState {
	return pristineCommandState{
		runE:               c.RunE,
		run:                c.Run,
		args:               c.Args,
		hidden:             c.Hidden,
		disableFlagParsing: c.DisableFlagParsing,
	}
}

// restorePristineTree returns the whole command tree to its as-registered
// state: the snapshot taken on first use (after all init() wiring, before any
// execution) is written back over every command, and all flag values return
// to their declared defaults. Each execution then applies its own per-run
// mutations (scope-preflight wraps, profile masks) from a clean slate, so
// nothing from one invocation — a --context override, a profile mask, an
// output format — can leak into the next.
func restorePristineTree() {
	pristineOnce.Do(func() {
		pristineTree = make(map[*cobra.Command]pristineCommandState)
		walkCommands(rootCmd, func(c *cobra.Command) {
			pristineTree[c] = capturePristineState(c)
		})
	})
	walkCommands(rootCmd, func(c *cobra.Command) {
		state, ok := pristineTree[c]
		if !ok {
			// A command registered after the first run (not a pattern dtctl
			// uses, but harmless): its current state becomes its pristine one.
			state = capturePristineState(c)
			pristineTree[c] = state
		}
		c.RunE = state.runE
		c.Run = state.run
		c.Args = state.args
		c.Hidden = state.hidden
		c.DisableFlagParsing = state.disableFlagParsing
		// No command sets IO writers at registration time, so pristine means
		// nil: cobra then resolves os.Stdout/os.Stderr dynamically at print
		// time. A caller-bound writer (tests do this) must not outlive its
		// invocation.
		c.SetOut(nil)
		c.SetErr(nil)
		c.SetIn(nil)
		resetFlagSet(c.Flags())
		resetFlagSet(c.PersistentFlags())
	})
	// Color decisions are cached per process but depend on per-run inputs
	// (--plain, NO_COLOR, TTY-ness of the current stdout).
	output.ResetColorCache()
}

// resetFlagSet returns every flag in fs to its declared default. Mirrors
// testutil.ResetCommandFlags (cmd/testutil/helpers.go), which stays separate
// so test helpers don't have to reach into the cmd package.
func resetFlagSet(fs *pflag.FlagSet) {
	fs.VisitAll(func(flag *pflag.Flag) {
		flag.Changed = false
		if sv, ok := flag.Value.(pflag.SliceValue); ok {
			// SliceValue.Set appends rather than replaces; use Replace to
			// restore the declared default. StringArray stores DefValue as
			// JSON; other slice types fall back to nil (empty) if the format
			// doesn't parse.
			var defaults []string
			if flag.DefValue != "[]" {
				_ = json.Unmarshal([]byte(flag.DefValue), &defaults)
			}
			_ = sv.Replace(defaults)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
	})
}
