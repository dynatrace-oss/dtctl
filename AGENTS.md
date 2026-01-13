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
- Integration: `test/integration/`
- E2E: `test/e2e/*_test.go` (requires live environment)
- Run: `make test` or `go test ./...`

## API Endpoints

Dynatrace Platform APIs:

- Base: `https://<env>.apps.dynatrace.com/platform/`
- Docs: Each resource handler has API spec reference in comments

## Common Pitfalls

❌ **Don't** add query filters as CLI flags (e.g., `--filter-status`)  
✅ **Do** use DQL: `dtctl query "fetch logs | filter status == 'ERROR'"`

❌ **Don't** assume resource names are unique  
✅ **Do** implement disambiguation or require ID

❌ **Don't** print to stdout in library code  
✅ **Do** return data, let cmd/ handle output

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
