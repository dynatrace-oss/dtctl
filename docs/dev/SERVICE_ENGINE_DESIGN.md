# dtctl as a Service — Engine Design

**Status:** Implemented; `dtctl serve` is experimental and gated behind
`DTCTL_EXPERIMENTAL_SERVE` (see [Maturity](#maturity))
**Created:** 2026-08-13
**Audience:** anyone changing `cmd/`, adding a command, or reading a user-supplied file.

> **Implementation:** `cmd/run.go` (`Run`, `RunOptions`, tree isolation, serialization),
> `cmd/capabilities.go` (subprocess gate), `cmd/session.go` (per-request tenant),
> `cmd/stdio.go` (stream redirection), `cmd/blocked.go` (surface mask),
> `pkg/vfs/` (file seam), `pkg/engine/` (the embedding surface),
> `pkg/serve/` (`dtctl serve http`, reference server).

## Overview

dtctl runs as a one-shot CLI *and* as an in-process library that a service can
call once per request. The same command tree, the same code paths, the same
bytes on stdout — the only difference is where the invocation's credentials,
files, and streams come from.

The motivating use cases are AI agents and automation that already know how to
drive dtctl on a terminal: a Dynatrace Workflow action, an agent gateway, an
MCP server. Rather than growing a second, parallel API surface that drifts from
the CLI, an embedding host hands over **a command line** and gets back **what
the CLI would have printed**.

```go
res, err := engine.Execute(ctx, engine.Request{
    Command:        `apply -f workflow.yaml --write-id --agent`,
    EnvironmentURL: "https://abc12345.apps.dynatrace.com",
    Token:          tenantToken,
    Files:          map[string][]byte{"workflow.yaml": src},
})
// res.Stdout is byte-identical to local CLI output;
// res.Files["workflow.yaml"] now carries the stamped id.
```

## Goals

1. **The CLI is the API.** No second command surface, no second output format.
   Anything a user can type, a service can send.
2. **Byte-identical output.** An agent moving between local CLI and service must
   not be able to tell the difference. Enforced by a test, not by review.
3. **Multi-tenant per invocation.** Each request carries its own environment and
   token. Nothing about the host process's identity leaks into a request, and
   nothing about one request survives into the next.
4. **Host abilities are opt-in.** Subprocesses, editors, browsers, the host disk
   — all off by default for embedded callers, granted explicitly by the CLI.
5. **No new trust in the host.** A service must not be able to make dtctl read
   the server's disk, credentials, or environment on a tenant's behalf.

## Non-Goals

- **A security sandbox.** The engine is a *correctness* boundary inside one
  process, not an isolation boundary. It does not contain a hostile *command*
  string; it prevents an honest one from reaching host state. Untrusted callers
  belong behind a process or WASM boundary.
- **Per-request preemption.** A run that has started cannot be killed
  mid-flight; in-process Go code cannot be preempted safely.
- **Intra-process parallelism.** See "Serialization" below. Scale with more
  instances or processes.
- **Authentication.** `pkg/serve` has none of its own. A deployment puts its own
  authn/TLS/rate limiting in front, or embeds `pkg/engine` directly.

## The five seams

Each seam replaces one thing a CLI process normally takes from its environment.
All are per-invocation and restored afterwards.

| Seam | `RunOptions` field | Default (CLI) | Embedded |
|---|---|---|---|
| Command tree | — | pristine per run | same |
| Subprocesses | `Capabilities` | all granted | none granted |
| Credentials + config | `Session` | host config, contexts, keyring | request's URL + token, host scrubbed |
| Environment | `Env` | host environment | host + explicit per-request entries |
| Files | `FS` | host filesystem | request's virtual files (`vfs.MapFS`) |
| Streams | `Stdout`/`Stderr`/`Stdin` | real process stdio | caller's writers/reader |

Surface reduction is a sixth, orthogonal knob: `BlockedCommands` removes
host-only commands from the tree per invocation.

### Serialization

The command tree is **package state** — 277 command values wired by `init()`.
Invocations therefore serialize on a package mutex (`cmd.runMu`), and that
single fact is what makes the rest of the design safe: swapping process
streams, mutating environment variables, and installing a filesystem are all
process-wide operations that would be indefensible under concurrency.

Consequences to internalize before "optimizing" this:

- A server handles one dtctl command at a time per process. `dtctl serve http`
  says so in its help text.
- Parallelism comes from more instances or more processes, never more
  goroutines.
- A server must not be started *inside* an invocation — it would hold the lock
  for its lifetime and deadlock its own first request. This is why `main`
  dispatches `serve` before the command pipeline, and why `dtctl serve http`
  refuses to run when `cmd.RunActive()` reports an invocation in progress.

### Restriction has four independent axes

Do not conflate these; each answers a different question.

| Axis | Question | Mechanism | Agent error code |
|---|---|---|---|
| Safety level | What may this command *do* to the tenant? | `pkg/safety` checks | `safety_blocked` |
| Profile | Which commands *exist* for this caller? | `applyProfile` mask | `profile_blocked` |
| Environment | Which commands *make sense* here at all? | `BlockedCommands` mask | `unsupported_in_service` |
| Capability | Which *host abilities* may this process use? | `cmd.Capabilities` gates | `capability_disabled` |

The masks compose — profile and environment both apply to a run. The
environment mask additionally guards non-runnable parents, so a blocked
`config` cannot answer with its own help text and exit 0.

## Rules for contributors

These are the invariants the design rests on. Each has a guard test, because
each was violated at least once during development — including twice in the
implementing PR itself.

### 1. User-supplied file paths go through `pkg/vfs`

```go
// NO — reads the server's disk on a service request
content, err := os.ReadFile(flagValue)

// YES
content, err := vfs.ReadFile(flagValue)
content, err := vfs.ReadFileOrStdin(flagValue) // when "-" means stdin
```

Applies to every path a **user named**: `-f`, `--file`, `--data-file`, query
files, files to `diff`, and writebacks like `apply --write-id`. Under an embedded
invocation those files exist only in the request; a direct `os.ReadFile` silently
reads the *server's* filesystem instead — a broken feature and a traversal into
host state in one line.

The rule follows the path, not the package. `cmd/diff.go` read its `-f` flags
through the seam while the `pkg/diff` helper one call deeper used `os.ReadFile`,
which is the same hole with an extra stack frame — and `dtctl inspect` took a
path argument straight to `os.Open`. Both survived a `cmd/`-only guard; both are
why the guard now scans `pkg/` too.

Paths **dtctl chose itself** — editor round-trip temp files, the config file,
spill buffers — are host state and stay on `os`. That is the whole test: whose
file is it? Host state is only defensible while an embedded invocation cannot
reach it, so pair it with the thing that keeps it unreachable: a blocked command
(rule 6) or an ungranted capability (rule 2).

Also never open `/dev/stdin` as a path. The stream seam swaps the `os.Stdin`
*variable*; a path reaches the process's real fd 0, past the redirection, and
does not exist on Windows. Use `os.Stdin` (the variable) or
`vfs.ReadFileOrStdin`.

**Guard:** `TestUserFilePathsGoThroughVFS` (`cmd/vfs_guard_test.go`) fails on any
direct `os` file call under `cmd/` or `pkg/` outside an annotated allowlist,
where each entry names what keeps that path unreachable from a service request.

### 2. Never reach the host without a capability gate

Every path in `cmd/` that spawns or replaces the process goes through one of the
five gateway files, each gated on a `cmd.Capabilities` field. The same applies to
host-disk features that live outside the vfs seam: result spilling writes a file
the caller cannot read and that outlives the request, so it is gated on
`HostDiskSpill`. Embedded callers grant nothing, which makes those paths
structurally unreachable rather than merely discouraged.

A capability is the right tool when the ability is *conditional* on the host; a
blocked command (rule 6) is right when the whole command is meaningless without
one. `--spill` is the former (the rows come back inline instead); `inspect`, a
reader for spilled files, is the latter.

**Guard:** `TestSubprocessSpawnsConfinedToGateways` (`cmd/capabilities_test.go`)
confines `exec.Command`/`syscall.Exec` in `cmd/` to the gateway files.

### 3. Command bodies never call `os.Exit`

An in-process caller dies with it. Return an error — `*silentExitError` when you
need a specific exit code with no additional message.

**Guard:** `TestNoOsExitOutsideExecute` (`cmd/silent_exit_test.go`).

### 4. Credentials and config come from the invocation, not the process

Read them through `LoadConfig()`, which a `Session` transparently replaces with
a synthetic single-context config. Never reach for `os.Getenv("DTCTL_TOKEN")`,
the keyring, or a config path directly in a command — a session-backed request
has scrubbed all of them, and code that goes around `LoadConfig` would hand one
tenant's request the host's (or another tenant's) credentials.

### 5. Output goes to the process streams, and stays byte-identical

Print via `pkg/output`, `fmt.Print*`, or cobra — all of which resolve
`os.Stdout`/`os.Stderr` dynamically, so the stream seam catches them. Do not
cache a writer across invocations, and do not add host-environment-dependent
output (TTY probes are fine; they already resolve to "not a TTY" for a service).

**Guard:** `TestEngineOutputEqualsCLI` (`pkg/engine/equality_test.go`) builds the
real binary and diffs CLI vs. engine stdout/stderr/exit code.

### 6. Per-run mutation of the command tree must be restorable

If you mutate a command at runtime (hide it, wrap its `RunE`, flip
`DisableFlagParsing`), the pristine-tree snapshot in `cmd/run.go` must know
about the field, or the mutation leaks into the next invocation. Add the field
to the snapshot, and a leak test beside
`TestRunProfileMaskDoesNotLeakBetweenInvocations` (`cmd/run_test.go`).

### 7. A new top-level command decides whether it belongs in a service

Add it to `unsupportedCommands` in `pkg/engine/policy.go` with a reason if it
manages host-local state (config, keyring, shell, installed tools, an
interactive session) or would nest the service in itself. The reason string is
user-facing: it appears in the `unsupported_in_service` error's suggestions.

## `dtctl serve`

`serve` is a **parent** command; each protocol is a named subcommand. Today:

```
dtctl serve http     JSON over HTTP (POST /v1/execute, GET /healthz)
```

Bare `dtctl serve` prints help and exits 0 — naming the protocol is mandatory,
so no single protocol is the silent default and a future `dtctl serve mcp` lands
beside `http` rather than competing with an incumbent. An unrecognized protocol
name is an error with a non-zero exit, not a help dump, so a supervisor cannot
mistake a typo for a started server.

Adding a protocol: a new file in `pkg/serve/`, a `newXCommand()` constructor
registered in `NewCommand()`, and the same `cmd.RunActive()` guard in its
`RunE`. It calls `engine.Execute` per request and translates the result into its
own wire format — protocol servers hold no dtctl logic of their own.

## Deliberate divergences from the local CLI

The service is not a perfect mirror, and the differences are intentional:

- **Host-only commands are absent** (see the environment axis above).
- **AI-agent auto-detection is skipped** for session-backed runs. The host's
  environment must not shape a tenant's output format; envelopes are opt-in per
  request via `--agent`.
- **`DTCTL_PROFILE` and `DTCTL_OUTPUT` are scrubbed** along with the credential
  variables, for the same reason.
- **`Env` is not exposed over HTTP.** Arbitrary variables reach proxies,
  exporters, and other process-level behavior. Embedding hosts that need it use
  `engine.Request.Env` directly, in-process.

## Maturity

`pkg/engine` is the stable half: a Go caller opts into it at compile time, and
the isolation rules above are enforced by guard tests.

`dtctl serve` is **experimental** and registered only when
`DTCTL_EXPERIMENTAL_SERVE` is set (the `DTCTL_EXPERIMENTAL_ACCOUNT` convention;
`serve.Experimental()` and the gate in `main`). Without it the command does not
exist — no help entry, no catalog entry, an ordinary "unknown command". The gate
comes off when the items below are settled, because each one changes what an
operator can rely on:

- **No per-request deadline.** A started execution cannot be interrupted, and it
  holds the single invocation slot. One `wait` with a long timeout, or a `query`
  against a slow environment, stalls every queued request behind it.
- **No admission control.** Requests queue on the invocation mutex without a
  bound; there is no "server busy" answer and no way to shed load.
- **Unbounded commands are still reachable.** `query --live` never returns on
  its own. Deciding whether the service surface excludes such commands (as it
  excludes `inspect`) or the transport imposes deadlines is the open half of the
  question below.
- **Per-invocation tracing cost.** `tracing.Init`/shutdown runs per invocation
  with a 5s flush budget; in a long-lived server that belongs at process scope.

## Open questions

- **Instance-per-request via WASM.** A spike measured 20–47 ms per-instance
  overhead, which would remove the serialization constraint and give hard
  per-request deadlines. Not pursued yet; the current model is
  process/instance-level scaling.
- **Cancellation.** The context gates the *start* of an execution only. Hard
  deadlines need a process or WASM boundary.
- **Streaming.** `POST /v1/execute` buffers the whole output. Long-running
  commands (`--watch`, `logs -f`) are unavailable in the service surface; a
  streaming endpoint would need a different response shape.
