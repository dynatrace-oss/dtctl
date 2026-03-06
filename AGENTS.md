# AI Agent Development Guide

kubectl-inspired CLI for Dynatrace (dashboards, workflows, SLOs, etc). Go + Cobra framework.

**Pattern**: `dtctl <verb> <resource> [flags]`

## Quick Start

1. Read [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md) Design Principles (lines 17-110)
2. Check [IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md) for feature matrix
3. Copy patterns from `pkg/resources/slo/` or `pkg/resources/workflow/`

## Architecture

```text
cmd/          # Cobra commands (get, describe, create, delete, apply, exec, ctx, doctor)
pkg/
  ├── client/    # HTTP client (auth, retry, rate limiting, pagination)
  ├── config/    # Multi-context config (~/.config/dtctl/config, keyring tokens)
  ├── resources/ # Resource handlers (one per API)
  ├── output/    # Formatters (table, JSON, YAML, charts, agent envelope)
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

## 🚨 **CRITICAL: Safety Checks** 🚨

**ALL mutating commands MUST include safety checks.** Non-negotiable for security.

### Required for These Commands

✅ `create`, `edit`, `apply`, `delete`, `update` (all modify resources)  
❌ `get`, `describe`, `query`, `logs`, `history`, `ctx`, `doctor` (read-only)

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
