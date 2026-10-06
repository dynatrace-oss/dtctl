# Per-invocation command trees and state

dtctl runs as a one-shot CLI and as an in-process library (`cmd.Run`,
`pkg/engine`, `dtctl serve`). The library path serializes invocations on a
mutex, because an invocation owns the whole process while it runs: the command
tree is a set of package-level `cobra.Command` values wired by `init()`, flag
values are package variables, and the per-run state — streams, environment,
filesystem, session, capabilities — is swapped process-wide for the duration of
the run. See [SERVICE_ENGINE_DESIGN.md](SERVICE_ENGINE_DESIGN.md)
("Serialization").

A host that evaluates many invocations in one process, most of them waiting on a
platform round trip, needs each invocation to carry a tree and state of its own
instead. This document describes the pieces that make that possible. They are
behavior-neutral for the CLI and for the serialized engine: every accessor
falls back to the process-level default when an invocation carries nothing of
its own.

## The command tree

Every command is a constructor, `func newXCmd() *cobra.Command`, that carries
the command's whole wiring: flags, required marks, hooks and stability tier. A
flag specific to one command binds to a variable local to its constructor and
captured by the command's `RunE`.

The CLI's tree is still the one `init()` wires, built from those constructors
(`var getBucketsCmd = newGetBucketsCmd()`, attached to its parent in `init()`).
`newCommandTree` (`cmd/root_factory.go`) builds a second, complete, freshly
allocated tree from the same constructors, so nothing one tree's flags parse is
visible to another.

Flags whose storage is shared with code outside the constructor — the root
persistent flags, `--dry-run`, and `get`'s `--limit` and `--fields` — bind to
storage on the invocation. That storage is **passed to the constructors**: a
command that is being built has no context yet to find the invocation through.

The fresh tree leaves out two things on purpose. Shell-completion registrations
are kept by cobra in a process-wide map that is never pruned, so registering
them per tree would retain every tree for the life of the process. And
development-tier commands are attached to the singleton's parents and cannot be
grafted onto a fresh tree.

`TestNewCommandTreeMatchesSingleton` holds the two trees together: the same
commands, flags, required marks, hooks and stability tiers. Wiring added to an
`init()` instead of a constructor fails it.

## Invocation state on the context

An `invocation` (`cmd/invocation.go`) carries one run's streams, environment
overlay, filesystem, session, blocked commands, capabilities, root flag values
and tracing context. `Run` attaches it to the context it threads into the
command tree, cobra hands that context to every command, and a command reaches
it with `cmdContext(cmd)`. Nothing is keyed by goroutine, so goroutines a command
starts inherit the invocation along with the context they are given.

| State | Accessor | Falls back to |
|---|---|---|
| stdout, stderr, stdin | `currentStdout`, `currentStderr`, `currentStdin` | `os.Stdout`, `os.Stderr`, `os.Stdin` |
| environment | `getenv`, `lookupEnv`, `withInvocationEnv` | the process environment |
| user-supplied files, `-` | `vfsEnv` (a `vfs.Env`) | the installed `vfs` and `os.Stdin` |
| session, blocked commands, capabilities | `currentSession`, `currentBlocked`, `currentCaps` | the package-level values `Run` maintains |
| root flag values | `curFlags` | `gFlags`, where the singleton tree binds them |

An invocation that has ended is invisible to the accessors. cobra keeps the
context a serialized `Run` bound the singleton tree to, so a later direct call to
a command would otherwise find the finished invocation's session and streams.

Library helpers that used to write to the process streams or read the process
filesystem take them explicitly, and `cmd` hands them the invocation's:
`output.NewPrinterWithOpts` (`Writer`, `Notice`), `output.NewProgressReporterTo`,
`prompt.ConfirmWith`, `exec.DQLExecutor.WithStreams` and `WithVFS`,
`apply.Applier.WithStderr` and `WithVFS`, `vfs.Env`, and `session.Config.WithEnv`.
Each existing entry point delegates with the process defaults.

## The deadline

On a tree of its own the request's context is the only deadline. The SDK builds
its client with a six-minute timeout and nearly every resource handler passes
`context.Background()`, so a context deadline would otherwise reach almost no
request. `bindClientContext` therefore joins the invocation's context to every
request whose own context cannot end. A request whose context *can* end is left
alone: it is either derived from the command's context already, or it is a bound
the SDK set on purpose — the query execute outlives a cancellation by a short
grace so that a query it started can still be cancelled on the backend.

Nothing preempts in-process code between requests: a command that loops between
calls runs on until its next call.

## Rules for request-path code

1. **No package-level mutable state** on a request path, including wiring done in
   `init()`. Guard: `TestNewCommandTreeMatchesSingleton`.
2. **No writes to the process streams.** In `cmd/` write to `currentStdout(ctx)` /
   `currentStderr(ctx)` or the command's own writers; a `pkg/` type takes its
   writers from its caller. Guards: `TestNoProcessStreamWritesOnRequestPaths`,
   `TestRequestPathsInjectTheirStreams`.
3. **A command reads the tree it runs from**, not the singleton, and takes its
   per-run state from the context, never from a package variable.

## Adding a command

1. Write `newXCmd()`: flags bound to locals, stability tier and hooks inside.
2. Keep `var xCmd = newXCmd()` and attach it to its parent in `init()`.
3. Add it to `newCommandTree` in `cmd/root_factory.go`.
4. In the body, start from `cmdContext(cmd)`; print through `currentStdout(ctx)`
   and `currentStderr(ctx)`; build printers, executors and appliers with the
   helpers in `cmd/invocation.go`.

`TestNewCommandTreeMatchesSingleton` fails if steps 2 and 3 disagree.
