# AI Agent Development Guide

kubectl-inspired CLI for Dynatrace (dashboards, workflows, SLOs, etc). Go + Cobra framework.

**Pattern**: `dtctl <verb> <resource> [flags]`

## Quick Start

1. Read [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md) Design Principles (lines 17-110)
2. Check [IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md) for feature matrix
3. Copy patterns from `pkg/resources/slo/` or `pkg/resources/workflow/`

## Architecture

```text
cmd/          # Cobra commands (get, describe, create, delete, apply, exec, ctx, doctor, commands)
pkg/
  ├── client/    # HTTP client (auth, retry, rate limiting, pagination)
  ├── config/    # Multi-context config (~/.config/dtctl/config, keyring tokens)
  ├── resources/ # Resource handlers (one per API)
  ├── output/    # Formatters (table, JSON, YAML, charts, agent envelope, color control)
  └── exec/      # DQL query execution
```

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
- **Command catalog**: `dtctl commands --brief -o json` provides a machine-readable listing of all available commands, flags, and resource types — ideal for agent bootstrap

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
3. **Docs**: `README.md`, `CHANGELOG.md`, `docs/QUICK_START.md` (agent detection list), `docs/dev/API_DESIGN.md` (agent detection list), `docs/dev/IMPLEMENTATION_STATUS.md` (skills feature line)

## Adding a Resource

1. Create `pkg/resources/<name>/<name>.go` with Get/List/Create/Delete functions
2. Add to `cmd/get.go`, `cmd/describe.go`, etc.
3. Register in resolver
4. Add tests: `test/e2e/<name>_test.go`

**Handler signature**:
```go
func GetResource(client *client.Client, id string) (interface{}, error)
func ListResources(client *client.Client, filters map[string]string) ([]interface{}, error)
```

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
| `pkg/output/testdata/golden/` | Golden files (49 files across get/, describe/, query/, errors/, empty/) |
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
✅ **Do** add safety checks to ALL create/edit/apply/delete/update commands

## Code Examples

- Simple CRUD: `pkg/resources/bucket/`
- Complex with subresources: `pkg/resources/workflow/`
- Execution pattern: `pkg/exec/workflow.go`
- History/versioning: `pkg/resources/document/`

## Resources

- **Design**: [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md)
- **Architecture**: [docs/dev/ARCHITECTURE.md](docs/dev/ARCHITECTURE.md)
- **Status**: [docs/dev/IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md)
- **Future Work**: [docs/dev/FUTURE_FEATURES.md](docs/dev/FUTURE_FEATURES.md)

---

**Token Budget Tip**: Read API_DESIGN.md Design Principles section first (most critical context). Skip reading full ARCHITECTURE.md unless making structural changes.
