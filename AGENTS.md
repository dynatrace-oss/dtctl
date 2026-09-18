# AI Agent Development Guide

kubectl-inspired CLI for Dynatrace (dashboards, workflows, SLOs, etc). Go + Cobra framework.

**Pattern**: `dtctl <verb> <resource> [flags]`

## Quick Start

1. Read [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md) Design Principles (lines 17-110)
2. Check [IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md) for feature matrix
3. Copy patterns from `pkg/resources/slo/` or `pkg/resources/workflow/`

## Architecture

```text
cmd/          # Cobra commands (get, describe, create, delete, apply, exec, ctx, doctor, commands, plugin)
              # plus the embedding entrypoint: run.go (cmd.Run + per-invocation isolation),
              # session.go, capabilities.go, stdio.go, blocked.go (docs/dev/SERVICE_ENGINE_DESIGN.md)
pkg/
  ├── client/    # Shim over sdk/session's client; keeps CLI-only extras (typed errors, pagination, OTel injection) and pins the dtctl/<version> User-Agent
  ├── config/    # Shim over sdk/session — the config model/keyring implementation lives in the sdk
  ├── auth/      # Shim over sdk/session's OAuth machinery + CLI-owned scope-composition tables
  ├── safety/    # Shim over sdk/session's safety checker
  ├── plugin/    # kubectl-style exec plugin resolution/discovery (docs/dev/PLUGIN_CONVENTIONS.md)
  ├── resources/ # Resource handlers — thin CLI wrappers that delegate to sdk/api/
  ├── output/    # Formatters (table, JSON, YAML, charts, agent envelope, color control)
  ├── exec/      # DQL query execution
  ├── vfs/       # Virtual-filesystem seam: user-supplied file paths resolve against the host disk (CLI) or per-request virtual files (engine)
  ├── engine/    # Embeddable service engine: one dtctl command line per request — multi-tenant session, virtual files, CLI-identical output
  └── serve/     # `dtctl serve <protocol>` — reference servers over pkg/engine, one subcommand per protocol (`serve http` today; wired in main, outside the invocation lock). Experimental: registered only when DTCTL_EXPERIMENTAL_SERVE is set
sdk/            # Separate Go module (github.com/dynatrace-oss/dtctl/sdk)
  ├── session/     # The session layer (docs/dev/CONFIG_CONTRACT.md): config model + load/save, credential stores, OAuth flow/refresh + cross-process lock, client-from-context with parameterized User-Agent, safety semantics
  ├── api/         # Typed API wrappers (one package per Dynatrace API surface)
  │                #   apispec/ is the odd one out: it reads the environment's *own* API index and
  │                #   OpenAPI documents (docs/dev/GENERIC_API_ACCESS.md)
  ├── httpclient/  # HTTP client, response helpers, pagination, typed errors
  ├── auth/        # Token type detection
  ├── urls/        # Environment URL validation/normalization
  ├── agentmode/   # AI agent environment detection
  └── inventory/   # Environment data-inventory discovery over a caller-supplied DQL Runner
```

**SDK delegation pattern**: CLI resource handlers in `pkg/resources/` import types from `sdk/api/` (often via type aliases) and delegate HTTP calls to SDK functions. The `sdk/api/*` packages contain **no file I/O, no CLI concerns, no display logic**. File reading (e.g., `ReadFileOrStdin`, `ParseInputFromFile`) stays in `pkg/resources/`. (`sdk/session` is the deliberate exception on file I/O: it owns the config file and credential stores — that's its job.)

**Two modules, one release**: the root `go.mod` requires `dtctl/sdk` *and* `replace`s it with `./sdk`. Go ignores that `replace` downstream, so the `require` must always name a real `sdk/vX.Y.Z` tag — otherwise the root module cannot be imported at all (e.g. a service embedding `pkg/engine`), while every in-repo build stays green because the replace covers it. The require carries `// x-release-please-version` so release-please keeps it equal to the CLI version, and `.github/workflows/release.yml` mirrors each `vX.Y.Z` into `sdk/vX.Y.Z`. Never hand-edit that version or drop the annotation. *Guard*: `go test ./pkg/version/ -run TestSDKRequireIsResolvable`

**Plugins**: dtctl dispatches unknown commands to `dtctl-<name>` executables on PATH (kubectl semantics; built-ins always win). See [docs/dev/PLUGIN_CONVENTIONS.md](docs/dev/PLUGIN_CONVENTIONS.md) for the env contract (`DTCTL_CONTEXT`, `DTCTL_CONFIG`, `DTCTL_AGENT`, `DTCTL_PLAIN`, `DTCTL_CALLER_VERSION` — never tokens).

## Agent Output Mode

dtctl supports `--agent` / `-A` to wrap all output in a structured JSON envelope for AI agents:

```json
{"ok": true, "result": [...], "context": {"verb": "get", "resource": "workflow", "suggestions": [...]}}
```

- **Auto-detected** in AI agent environments (opt out with `--no-agent`)
- Implies `--plain` (no colors, no interactive prompts)
- Errors are also structured: `{"ok": false, "error": {"code": "not_found", "message": "..."}}`
- Implementation: `pkg/output/agent.go` (`AgentPrinter`, `Response`, `PrintError`)
- Per-command context enrichment via `enrichAgent()` helper in `cmd/root.go`
- **Command catalog**: `dtctl commands` gives a compact minimal overview (verbs, resources, subcommands only; defaults to TOON) — ideal for agent bootstrap. `--brief` adds mutating status, access levels, flag types, and scopes; `--full` emits the exhaustive catalog (descriptions, flag defaults, global flags, materialized scopes). Override the format with `-o json`/`-o yaml`

## Color Control

ANSI color output follows the [no-color.org](https://no-color.org/) standard:

```
Color enabled = NOT (NO_COLOR is set) AND NOT (--plain flag) AND (stdout is a TTY OR FORCE_COLOR=1)
```

- **`NO_COLOR`** env var: any non-empty value disables color
- **`FORCE_COLOR=1`** env var: overrides TTY detection to force color on (e.g., in CI with color-capable terminals)
- **`--plain`** flag: disables color (and interactive prompts)
- **Non-TTY** (piped output): color is disabled automatically
- Implementation: `pkg/output/styles.go` (`ColorEnabled()`, `Colorize()`, `ColorCode()`)
- Result is cached with `sync.Once`; use `ResetColorCache()` in tests

## Adding a Supported Agent

When adding a new AI agent to the skills system, update **all** of the following:

1. **Code**: `pkg/aidetect/detect.go` (env var), `pkg/skills/installer.go` (agent entry + format), `cmd/skills.go` (help text, `--for` flag)
2. **Tests**: `pkg/aidetect/detect_test.go`, `pkg/skills/installer_test.go`, `cmd/skills_test.go`
3. **Docs**: `README.md`, `docs/QUICK_START.md` (agent detection list), `docs/dev/API_DESIGN.md` (agent detection list), `docs/dev/IMPLEMENTATION_STATUS.md` (skills feature line)

> Releases are automated by release-please from conventional commits — do not hand-edit a changelog. Just make sure the commit/PR title is a proper conventional commit (e.g. `feat: detect <agent> sessions`).

## Adding a Resource

1. **SDK layer** (`sdk/api/<name>/`): Create typed API wrapper with CRUD functions using `httpclient.Client`. No file I/O, no display logic.
2. **CLI layer** (`pkg/resources/<name>/`): Create resource handler that delegates to SDK. Handle file reading, display fields, name resolution here. Read user-supplied paths through `pkg/vfs`, never `os` directly (see [Embedding Invariants](#-critical-embedding-invariants-)).
3. **Commands**: Add to `cmd/get.go`, `cmd/describe.go`, etc. Mutating verbs need a safety check; `-f`/`--file` flags go through `vfs`; no `os.Exit`, no ungated subprocess.
4. Register in resolver
5. Add tests: `sdk/api/<name>/*_test.go` (SDK unit tests) + `test/e2e/<name>_test.go` (E2E)
6. **Claim the coverage**: add the API's base path to `nativeCoverage` in `pkg/resources/api/coverage.go`, so `dtctl get apis` stops listing it as uncovered and `dtctl exec api` points callers at the new command. `Command` must be what a user would actually type (`dtctl query`, not `dtctl get query`). *Guard*: `go test ./cmd/ -run TestNativeCoverageNamesRealCommands`

**SDK handler signature** (in `sdk/api/<name>/`):
```go
func NewHandler(client *httpclient.Client) *Handler
func (h *Handler) Get(id string) (*Resource, error)
func (h *Handler) List(opts ListOptions) ([]Resource, error)
```

**CLI handler** (in `pkg/resources/<name>/`): imports SDK types (often via type alias) and wraps with file I/O, display fields, etc.

## Generic API Access

`dtctl get apis` / `describe api` / `exec api` cover the APIs dtctl does **not** wrap natively. Full rationale: [docs/dev/GENERIC_API_ACCESS.md](docs/dev/GENERIC_API_ACCESS.md). Four conventions to respect when touching them:

1. **dtctl mirrors the environment's API index and filters nothing.** A dtctl-side filter would have to hard-code which APIs to conceal, and in an open-source tool that list *is* the disclosure. Resolution consults only what the index returned — never synthesize a candidate path from a name and retry it, which would turn a name lookup into an existence oracle.
2. **The HTTP method never decides the safety gate.** `resapi.Classify` takes the stricter of the method floor and the specification's declared scope; POST's floor is `OperationRead`, because plenty of read-only endpoints are POSTs. Unresolvable → `OperationDelete`. There is deliberately no flag to assert an operation.
3. **`exec api` must never be the integration target.** It stays `Hidden` (present in `dtctl commands --full` via `unadvertisedResources` in `pkg/commands/listing.go`, absent from `--help` and the compact catalogs), it names the native command whenever one covers the path, and no command profile grants it.
4. **No committed artifact names a non-public API.** Help text, examples, golden files, and E2E fixtures are synthetic; live tests assert invariants of the mechanism, never a list of expected APIs.

## Design Principles

1. Verb-noun pattern: `dtctl <verb> <resource>`
2. No custom query flags - use DQL passthrough
3. YAML input, multiple outputs (table/JSON/YAML/charts)
4. Interactive name resolution (disable with `--plain`)
5. Idempotent apply (POST if new, PUT if exists)

## Common Tasks

| Task | Files | Pattern |
|------|-------|---------|
| Add GET | `cmd/get.go`, `pkg/resources/<name>/` | Copy `pkg/resources/slo/` |
| Add EXEC | `cmd/exec.go`, `pkg/exec/<type>.go` | See `pkg/exec/workflow.go` polling |
| Add DQL template | `pkg/exec/dql.go`, `pkg/util/template/` | Use `text/template`, `--set` flag |
| Fix output | `pkg/output/<format>.go` | Test: `dtctl get <resource> -o <format>` |
| Read a user file | `pkg/vfs/` | `vfs.ReadFile` / `vfs.ReadFileOrStdin` — never `os.ReadFile` |
| Add a serve protocol | `pkg/serve/<proto>.go` | Copy `pkg/serve/http.go`; register in `NewCommand()` |
| Wrap a new API natively | `pkg/resources/api/coverage.go` | Add the base path with the *runnable* command (see below) |
| Gate an irreversible endpoint | `pkg/resources/api/classify.go` | Add a `destructivePatterns` entry (docs/dev/GENERIC_API_ACCESS.md) |

**Tests**: `make test` or `go test ./...` • E2E: `test/e2e/` • Integration: `test/integration/`

## Golden (Snapshot) Tests

Output formatting is covered by golden-file tests that capture the exact output of every printer for every resource type. These tests **must be updated** whenever you change output formatting, add/remove struct fields, or add a new resource.

### When to run

- **After modifying** anything in `pkg/output/` or resource structs in `pkg/resources/*/` — run golden tests to check for regressions
- **After adding a new resource** — add test cases to `pkg/output/golden_test.go` using the real production struct

### Commands

```bash
# Run golden tests (will FAIL if output changed — this is intentional)
go test ./pkg/output/ -run TestGolden

# Update golden files after intentional changes (review the diff!)
make test-update-golden
# or: go test ./pkg/output/ -run TestGolden -update

# Run full suite (golden tests are included automatically)
make test
```

### Key files

| File | Purpose |
|------|---------|
| `pkg/output/golden_test.go` | Test cases — uses **real production structs** from `pkg/resources/*` |
| `pkg/output/testdata/golden/` | Golden files across get/, describe/, query/, errors/, empty/ |
| `cmd/testutil/golden.go` | `AssertGolden` / `AssertGoldenStripped` helpers, `-update` flag |
| `pkg/output/testdata/README.md` | Workflow documentation |

### Important rules

1. **Use real structs** — Import from `pkg/resources/*`, never create test-only duplicates. This ensures golden files automatically catch when fields are added/removed.
2. **Review diffs** — After `make test-update-golden`, always `git diff` the golden files to verify changes are intentional.
3. **Privacy** — All test data must be synthetic. No real names, env IDs, tokens, or emails. Use `@example.invalid` for emails (RFC 2606).
4. **CI** — Golden tests run automatically in GitHub Actions via `go test ./...` on every PR. No separate workflow needed.

## 🚨 **CRITICAL: Safety Checks** 🚨

**ALL mutating commands MUST include safety checks.** Non-negotiable for security.

### Required for These Commands

✅ `create`, `edit`, `apply`, `delete`, `update` (all modify resources)  
✅ `exec` — every subcommand, with the operation matching what the execution actually does: `OperationRead` for an analysis or a preview, `OperationCreate` for a run that creates state, `OperationDelete` for ad-hoc code (`exec function --code` can do anything), and for `exec api` the operation derived from the API's own specification. Use `SetupWithSafety(op)`, not the ungated `SetupClient()`. *Guard*: `go test ./cmd/ -run TestExec.*Readonly`  
❌ `get`, `describe`, `query`, `logs`, `history`, `ctx`, `doctor`, `commands` (read-only)

### Pattern (after `LoadConfig()`, before client ops)

```go
cfg, err := LoadConfig()
if err != nil { return err }

// Safety check - REQUIRED
checker, err := NewSafetyChecker(cfg)
if err != nil { return err }
if err := checker.CheckError(safety.OperationXXX, safety.OwnershipUnknown); err != nil {
    return err
}

c, err := NewClientFromConfig(cfg)
// ... proceed
```

**Operation types**: `OperationCreate`, `OperationUpdate`, `OperationDelete`, `OperationDeleteBucket`

**Skip in dry-run**:
```go
if !dryRun {
    checker, err := NewSafetyChecker(cfg)
    // ... safety check
}
```

**Verification**:
- [ ] Import `github.com/dynatrace-oss/dtctl/pkg/safety`
- [ ] Check after `LoadConfig()`, before operations
- [ ] Correct operation type
- [ ] Test with `readonly` context (should block)

**Examples**: [cmd/edit.go](cmd/edit.go), [cmd/create.go](cmd/create.go), [cmd/apply.go](cmd/apply.go)

## 🚨 **CRITICAL: Embedding Invariants** 🚨

dtctl runs as a one-shot CLI **and** as an in-process library, one invocation per
service request (`cmd.Run` → `pkg/engine` → `dtctl serve http`). Same command
tree, same code paths, same bytes on stdout — the only difference is where the
invocation's credentials, files, and streams come from.

Every rule below has a guard test, because every one of them was violated at
least once — including twice inside the PR that introduced the model. Full
rationale: [docs/dev/SERVICE_ENGINE_DESIGN.md](docs/dev/SERVICE_ENGINE_DESIGN.md).

### 1. User-supplied file paths go through `pkg/vfs` — never `os` directly

```go
content, err := os.ReadFile(pathFromFlag)        // ❌ reads the SERVER's disk
content, err := vfs.ReadFile(pathFromFlag)       // ✅
content, err := vfs.ReadFileOrStdin(pathFromFlag) // ✅ when "-" means stdin
```

The test is **whose file is it**: a path the *user named* (`-f`, `--file`,
`--data-file`, a query file, a file to `diff`, an `apply --write-id` writeback)
is request state and must go through the seam. A path *dtctl chose* (editor temp
file, config file, spill buffer) is host state and stays on `os` — but only when
a blocked command or an ungranted capability keeps a request from reaching it.

**The rule follows the path, not the package.** `cmd/` reading through the seam
and then handing the path to a `pkg/` helper that calls `os.ReadFile` is the
same hole one frame deeper. The guard scans `cmd/` *and* `pkg/`.

Never open `/dev/stdin` as a path — the seam swaps the `os.Stdin` *variable*, so
a path slips past it and doesn't exist on Windows. Use `os.Stdin` or
`vfs.ReadFileOrStdin`.

*Guard*: `go test ./cmd/ -run TestUserFilePathsGoThroughVFS`

### 2. No subprocess — and no host disk — without a capability gate

Spawning (`exec.Command`, `syscall.Exec`) belongs in one of the five gateway
files, each gated on a `cmd.Capabilities` field. Host-disk features outside the
vfs seam are gated the same way: result spilling needs `HostDiskSpill`, because a
spilled file outlives the request on the server and its path means nothing to the
caller. Embedded callers grant nothing, so those paths become structurally
unreachable rather than merely discouraged.

*Guard*: `go test ./cmd/ -run TestSubprocessSpawnsConfinedToGateways`

### 3. No `os.Exit` in command bodies

An in-process caller dies with it. Return an error — `*silentExitError` for a
specific exit code with no extra message.

*Guard*: `go test ./cmd/ -run TestNoOsExitOutsideExecute`

### 4. Credentials come from `LoadConfig()`, never from the process

A `Session` transparently swaps in a synthetic single-context config. Reaching
for `os.Getenv("DTCTL_TOKEN")`, the keyring, or a config path directly would
hand one tenant's request the host's credentials — a session has scrubbed all of
them precisely so that cannot happen.

### 5. Output stays byte-identical to the CLI

Print via `pkg/output`, `fmt.Print*`, or cobra — all resolve `os.Stdout`/
`os.Stderr` dynamically, so the stream seam catches them. Don't cache a writer
across invocations; don't make output depend on the host environment.

*Guard*: `go test ./pkg/engine/ -run TestEngineOutputEqualsCLI` (builds the real
binary and diffs CLI vs. engine stdout/stderr/exit code)

### 6. New top-level command? Decide if it belongs in a service

Add it to `unsupportedCommands` in `pkg/engine/policy.go` **with a reason** if it
manages host-local state (config, keyring, shell, installed tools, an
interactive session) or would nest the service in itself. The reason is
user-facing — it appears in the `unsupported_in_service` error's suggestions.

## Privacy

Never put customer names, employee names, usernames, or specific Dynatrace environment identifiers into the codebase, GitHub issues, PRs, release notes, or commits.

## Common Pitfalls

❌ **Don't** add query filters as CLI flags (e.g., `--filter-status`)  
✅ **Do** use DQL: `dtctl query 'fetch logs | filter status == "ERROR"'`

❌ **Don't** assume resource names are unique  
✅ **Do** implement disambiguation or require ID

❌ **Don't** print to stdout in library code  
✅ **Do** return data, let cmd/ handle output

❌ **Don't** skip safety checks on mutating commands  
✅ **Do** add safety checks to ALL create/edit/apply/delete/update/exec commands

❌ **Don't** script against `dtctl exec api` — an escape hatch that becomes the integration target has failed  
✅ **Do** add a native command for the API instead (`dtctl get apis --uncovered` is the backlog)

❌ **Don't** read a user-supplied path with `os.ReadFile` / `os.Open`  
✅ **Do** use `vfs.ReadFile` / `vfs.ReadFileOrStdin` (see Embedding Invariants)

❌ **Don't** call `exec.Command` or `os.Exit` from a command body  
✅ **Do** go through a capability gateway, and return errors instead of exiting

❌ **Don't** send `page-size` together with `next-page-key`/`page-key` on paginated requests  
❌ **Don't** drop filter/search params on subsequent pages — page tokens do NOT always preserve them  
❌ **Don't** send `schemaIds`/`scopes` with `nextPageKey` on Settings API — the page token embeds everything  
✅ **Do** use the pagination pattern below: only `page-size` goes in the if/else; filters go outside (except Settings API)

## Pagination Pattern (CRITICAL)

Dynatrace APIs **reject** requests that combine `page-size` with `next-page-key`/`page-key` (HTTP 400). However, **page tokens do NOT always preserve filter parameters** — the Document API is a confirmed example where the `filter` param is dropped on page 2+ if not resent. To be safe, **always resend filter/search params on every page request**, and only exclude `page-size` when a page key is present.

**Exception — Document API**: The Document API (`/platform/document/v1/documents`) does **not** reject `page-size` + `page-key`. It also does **not** embed the page size in the page token (defaulting to 20/page if `page-size` is omitted). For Document API endpoints, send `page-size` on every request alongside `page-key`. See `pkg/resources/document/document.go` for the reference implementation.

**Exception — Settings API**: The Settings API (`/platform/classic/environment-api/v2/settings/objects`) rejects ALL other query params when `nextPageKey` is present — not just `pageSize`, but also `schemaIds`, `scopes`, and `fields` (HTTP 400: "must not be used in combination with nextPageKey query parameter"). The page token embeds everything. For Settings API endpoints, send ONLY `nextPageKey` on page 2+. See `pkg/resources/settings/settings.go` for the reference implementation.

### Correct pattern (default — most APIs)

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    if nextPageKey != "" {
        req.SetQueryParam("page-key", nextPageKey)
    } else if chunkSize > 0 {
        req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
    }
    // Always send filter, regardless of pagination
    if filter != "" {
        req.SetQueryParam("filter", filter)
    }

    resp, err := req.Get("/platform/...")
    // ... handle response, break if no more pages
}
```

### Correct pattern (Document API — accepts page-size with page-key)

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    if nextPageKey != "" {
        req.SetQueryParam("page-key", nextPageKey)
    }
    // Document API: send page-size and filter on EVERY request
    if chunkSize > 0 {
        req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
    }
    if filter != "" {
        req.SetQueryParam("filter", filter)
    }

    resp, err := req.Get("/platform/document/v1/documents")
    // ... handle response, break if no more pages
}
```

### Correct pattern (Settings API — page token embeds ALL params)

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    // Settings API rejects ALL other params when nextPageKey is present
    // (pageSize, schemaIds, scopes are all embedded in the page token).
    if nextPageKey != "" {
        req.SetQueryParam("nextPageKey", nextPageKey)
    } else {
        if chunkSize > 0 {
            req.SetQueryParam("pageSize", fmt.Sprintf("%d", chunkSize))
        }
        if schemaID != "" {
            req.SetQueryParam("schemaIds", schemaID)
        }
        if scope != "" {
            req.SetQueryParam("scopes", scope)
        }
    }

    resp, err := req.Get("/platform/classic/environment-api/v2/settings/objects")
    // ... handle response, break if no more pages
}
```

### Wrong pattern 1: sending page-size with page-key (causes HTTP 400 on non-Document APIs)

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    // BUG: page-size is sent on EVERY request, including subsequent pages
    if chunkSize > 0 {
        req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
    }
    if filter != "" {
        req.SetQueryParam("filter", filter)
    }
    if nextPageKey != "" {
        req.SetQueryParam("page-key", nextPageKey)
    }

    resp, err := req.Get("/platform/...")
}
```

### Wrong pattern 2: dropping filter on page 2+ (causes unfiltered results)

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    if nextPageKey != "" {
        // BUG: only sends page-key, drops filter on subsequent pages
        req.SetQueryParam("page-key", nextPageKey)
    } else {
        if filter != "" {
            req.SetQueryParam("filter", filter)
        }
        if chunkSize > 0 {
            req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
        }
    }

    resp, err := req.Get("/platform/...")
}
```

### Test guard (required for paginated mock servers of non-Document APIs)

Every test mock server for a paginated endpoint (except Document API) **must** include a constraint guard that rejects the invalid combination, so the bug is caught in tests:

```go
// Simulate API constraint: page-size must not be combined with page-key
if r.URL.Query().Get("page-size") != "" && r.URL.Query().Get("page-key") != "" {
    w.WriteHeader(http.StatusBadRequest)
    w.Write([]byte(`{"error":{"code":400,"message":"Constraints violated."}}`))
    return
}
```

For Settings API mocks, the guard must also reject `schemaIds`, `scopes`, and `fields` with `nextPageKey`:

```go
// Simulate Settings API constraint: pageSize, schemaIds, scopes, and fields
// must NOT be combined with nextPageKey (all are embedded in the page token).
if r.URL.Query().Get("nextPageKey") != "" {
    for _, param := range []string{"pageSize", "schemaIds", "scopes", "fields"} {
        if r.URL.Query().Get(param) != "" {
            w.WriteHeader(http.StatusBadRequest)
            fmt.Fprintf(w, `{"error":{"code":400,"message":"Constraints violated."}}`)
            return
        }
    }
}
```

**Reference implementations**: `pkg/resources/document/document.go` (Document API pattern), `pkg/resources/settings/settings.go` (Settings API pattern), `pkg/resources/extension/extension.go` (default pattern)

## Code Examples

- Simple CRUD: `pkg/resources/bucket/`
- Complex with subresources: `pkg/resources/workflow/`
- Execution pattern: `pkg/exec/workflow.go`
- History/versioning: `pkg/resources/document/`

## Resources

- **Design**: [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md)
- **Architecture**: [docs/dev/ARCHITECTURE.md](docs/dev/ARCHITECTURE.md)
- **Status**: [docs/dev/IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md)
- **Embedding/service model**: [docs/dev/SERVICE_ENGINE_DESIGN.md](docs/dev/SERVICE_ENGINE_DESIGN.md)
- **API discovery + passthrough**: [docs/dev/GENERIC_API_ACCESS.md](docs/dev/GENERIC_API_ACCESS.md)
- **Future Work**: [docs/dev/FUTURE_FEATURES.md](docs/dev/FUTURE_FEATURES.md)

---

**Token Budget Tip**: Read API_DESIGN.md Design Principles section first (most critical context). Skip reading full ARCHITECTURE.md unless making structural changes.
