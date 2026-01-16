# AI Agent Development Guide

This guide helps AI agents efficiently develop features for dtctl.

## What is dtctl?

A kubectl-inspired CLI for managing Dynatrace platform resources (dashboards, workflows, SLOs, etc.). Written in Go with Cobra CLI framework.

**Core Pattern**: `dtctl <verb> <resource> [flags]`

## Quick Start for Agents

1. **Understand the Design**: Read [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md) - Design Principles section (lines 17-110)
2. **Check What Exists**: See [docs/dev/IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md) for feature matrix
3. **Find Code Patterns**: Look at `pkg/resources/` for resource handler examples

## Architecture Overview

```text
cmd/               # Cobra commands (get, describe, create, delete, apply, exec, etc.)
pkg/
  ├── client/      # HTTP client with retry/rate limiting
  ├── config/      # Config & context management
  ├── resources/   # Resource handlers (one per API)
  ├── output/      # Formatters (table, JSON, YAML, charts)
  └── exec/        # DQL query execution
```

## Adding a New Resource

**Pattern**: Follow existing resources like `pkg/resources/slo/` or `pkg/resources/workflow/`

**Steps**:

1. Create `pkg/resources/<name>/<name>.go` with handler functions
2. Add commands to `cmd/get.go`, `cmd/describe.go`, etc.
3. Register resource type in resolver if needed
4. Add tests: `test/e2e/<name>_test.go`

**Handler Signature Example**:

```go
func GetResource(client *client.Client, id string) (interface{}, error)
func ListResources(client *client.Client, filters map[string]string) ([]interface{}, error)
func CreateResource(client *client.Client, manifest []byte) error
func DeleteResource(client *client.Client, id string) error
```

## Key Files & Patterns

### Command Structure (`cmd/*.go`)

- `root.go` - Global flags, config initialization
- `get.go` - List/retrieve resources
- `describe.go` - Detailed resource info
- `apply.go` - Create/update from YAML/JSON
- `query.go` - DQL query execution
- `exec.go` - Execute workflows/functions/analyzers

### Client (`pkg/client/client.go`)

- Handles auth, retries, rate limiting, pagination
- Base URL from context config
- Token from keyring or config

### Output (`pkg/output/printer.go`)

- `PrintTable()` - Human-readable ASCII tables
- `PrintJSON()` - Raw API response
- `PrintYAML()` - Reconstructed YAML
- Chart outputs for timeseries

### Configuration (`pkg/config/config.go`)

- Multi-context support (like kubeconfig)
- Stored at `~/.config/dtctl/config`
- Tokens in OS keyring

## Design Principles (Token-Optimized Summary)

1. **Verb-Noun Pattern**: Always `dtctl <verb> <resource>`
2. **No Leaky Abstractions**: Don't invent query flags - use DQL passthrough
3. **YAML Input, Multiple Outputs**: Accept YAML, output table/JSON/YAML/charts
4. **Name Resolution**: Handle ambiguous names interactively (disabled with `--plain`)
5. **AI-Native**: Support `--plain` flag for machine parsing (no colors, no prompts)
6. **Idempotent Apply**: POST if new, PUT if exists

## Common Tasks for Agents

### Task: Add GET support for new resource

```text
Files: cmd/get.go, pkg/resources/<name>/<name>.go
Pattern: Copy from pkg/resources/slo/slo.go
Register: Add to cmd/get.go with aliases
```

### Task: Add EXEC support (execution)

```text
Files: cmd/exec.go, pkg/exec/<type>.go
Pattern: See pkg/exec/workflow.go for polling pattern
```

### Task: Add DQL template feature

```text
Files: pkg/exec/dql.go, pkg/util/template/
Pattern: Use Go text/template, support --set flag
```

### Task: Fix output formatting

```text
Files: pkg/output/<format>.go
Test: Run `dtctl get <resource> -o <format>`
```

## Testing

- Unit: `*_test.go` files alongside code
- Integration: `test/integration/` (use `.integrationtests.env` for running integration tests)
- E2E: `test/e2e/*_test.go` (requires live environment)
- Run: `make test` or `go test ./...`

## API Endpoints

Dynatrace Platform APIs:

- Base: `https://<env>.apps.dynatrace.com/platform/`
- Docs: Each resource handler has API spec reference in comments

## 🚨 **CRITICAL: Mandatory Safety Checks** 🚨

**ALL commands that modify resources MUST include safety level checks.** This is non-negotiable for security.

### Which Commands Need Safety Checks?

✅ **ALWAYS ADD** safety checks to:
- `create` - Creates new resources (`OperationCreate`)
- `edit` - Modifies resources (`OperationUpdate`)
- `apply` - Creates/updates resources (`OperationUpdate`)
- `delete` - Deletes resources (`OperationDelete` or `OperationDeleteBucket`)
- `update` - Updates resources (`OperationUpdate`)

❌ **NEVER ADD** safety checks to:
- `get` - Read-only operation
- `describe` - Read-only operation
- `query` - Read-only DQL execution
- `logs` - Read-only log viewing
- `history` - Read-only version history

### Safety Check Pattern (REQUIRED)

**Place immediately after `LoadConfig()` and BEFORE any client operations:**

```go
// Load configuration
cfg, err := LoadConfig()
if err != nil {
    return err
}

// Safety check - REQUIRED for all mutating commands
checker, err := NewSafetyChecker(cfg)
if err != nil {
    return err
}
if err := checker.CheckError(safety.OperationXXX, safety.OwnershipUnknown); err != nil {
    return err
}

// Now proceed with operation...
c, err := NewClientFromConfig(cfg)
// ...
```

### Operation Types

Choose the correct operation type:

| Operation | Use For | Example |
|-----------|---------|---------|
| `safety.OperationCreate` | Creating new resources | `create workflow`, `create dashboard` |
| `safety.OperationUpdate` | Modifying existing resources | `edit workflow`, `apply` |
| `safety.OperationDelete` | Deleting resources | `delete workflow`, `delete dashboard` |
| `safety.OperationDeleteBucket` | Deleting buckets (data loss) | `delete bucket` |

### Required Import

Add to your command file:

```go
import (
    // ... other imports
    "github.com/dynatrace-oss/dtctl/pkg/safety"
)
```

### Dry-Run Exception

Skip safety checks in dry-run mode (no actual changes):

```go
if !dryRun {
    // Safety check
    checker, err := NewSafetyChecker(cfg)
    // ... rest of safety check
}
```

### Verification Checklist

Before submitting code with a new/modified command:

- [ ] Added `safety` package import
- [ ] Safety check placed after `LoadConfig()` and before operations
- [ ] Correct `Operation` type chosen
- [ ] Code compiles: `go build .`
- [ ] Tests pass: `go test ./pkg/safety/...`
- [ ] Manually tested with `readonly` context (should block)

### Examples

See these files for reference:
- `cmd/edit.go` - Edit commands with safety checks
- `cmd/create.go` - Create commands with safety checks  
- `cmd/apply.go` - Apply command with safety check
- `cmd/get.go` - Delete commands with safety checks

### Why This Matters

**Without safety checks**, users can accidentally modify production resources even when using `readonly` contexts. This defeats the entire purpose of context safety levels and can lead to:
- Accidental production changes
- Security violations
- Data loss
- Compliance issues

**Historical Context**: In January 2026, we discovered that `edit`, `apply`, and `create` commands were missing safety checks, allowing modifications in `readonly` contexts. This was a critical security bug that could have led to serious production incidents.

---

## Common Pitfalls

❌ **Don't** add query filters as CLI flags (e.g., `--filter-status`)  
✅ **Do** use DQL: `dtctl query "fetch logs | filter status == 'ERROR'"`

❌ **Don't** assume resource names are unique  
✅ **Do** implement disambiguation or require ID

❌ **Don't** print to stdout in library code  
✅ **Do** return data, let cmd/ handle output

❌ **Don't** skip safety checks on mutating commands  
✅ **Do** add safety checks to ALL create/edit/apply/delete/update commands

## Finding Examples

Best resource handler examples:

- Simple CRUD: `pkg/resources/bucket/`
- Complex with subresources: `pkg/resources/workflow/`
- Execution pattern: `pkg/exec/workflow.go`
- History/versioning: `pkg/resources/document/`

## Testing Your Changes

```bash
# Build
make build

# Run locally
./bin/dtctl --help

# Test against environment
export DTCTL_CONTEXT=dev  # Or use --context flag
./bin/dtctl get dashboards
```

## Documentation Updates

When adding features, update:

- `docs/dev/IMPLEMENTATION_STATUS.md` - Feature matrix
- `docs/dev/API_DESIGN.md` - If new resource type
- Command help strings in `cmd/*.go`

## Resources

- **Design**: [docs/dev/API_DESIGN.md](docs/dev/API_DESIGN.md)
- **Architecture**: [docs/dev/ARCHITECTURE.md](docs/dev/ARCHITECTURE.md)
- **Status**: [docs/dev/IMPLEMENTATION_STATUS.md](docs/dev/IMPLEMENTATION_STATUS.md)
- **Future Work**: [docs/dev/FUTURE_FEATURES.md](docs/dev/FUTURE_FEATURES.md)

---

**Token Budget Tip**: Read API_DESIGN.md Design Principles section first (most critical context). Skip reading full ARCHITECTURE.md unless making structural changes.
