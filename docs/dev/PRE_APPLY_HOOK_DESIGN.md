# Pre-Apply Hook Design

**Status:** Design Proposal
**Created:** 2026-04-02
**Author:** dtctl team

## Overview

Pre-apply hooks let users run an external command to validate (or reject) a
resource before `dtctl apply` sends it to the Dynatrace API. The hook receives
the **processed JSON on stdin** (after YAML-to-JSON conversion and template
rendering) and the **resource type and source filename as positional parameters
($1 and $2)**. The source filename is informational only — hooks must always
read content from stdin, not from the file. A non-zero exit code aborts the
apply.

This enables validation workflows that dtctl itself does not need to know about:
JSON Schema checks, OPA/Rego policies, team naming conventions, custom linters
written in any language. It also provides a clean integration point for a future
server-side `dtctl verify` command -- the hook mechanism remains the same, only
the hook command changes.

## Goals

1. **External validation** -- Run any command to validate resource content before apply
2. **Language-agnostic** -- Works with node, python, bash, jq, OPA, or any stdin-reading tool
3. **Processed content** -- Hook sees the final JSON that would be sent to the API, not raw YAML
4. **Clear contract** -- Exit code, stdin, args, stderr -- nothing ambiguous
5. **Escapable** -- `--no-hooks` flag to bypass when needed

## Non-Goals

- Post-apply hooks (wrapping dtctl in a script covers this)
- Content mutation (hooks validate, they don't transform)
- Multiple hooks or hook chains (single command; users chain with `&&` or a dispatcher)
- Built-in validators (the whole point is to keep validation external)
- Hook discovery or marketplace

---

## User Experience

### Configuration

Hooks are configured in the preferences section (global) or per-context (scoped):

```yaml
# Global hook -- applies to all contexts
preferences:
  hooks:
    pre-apply: 'node ./scripts/validate.js "$1" "$2"'

# Per-context hook -- overrides global for this context
contexts:
  - name: production
    context:
      environment: https://xyz.apps.dynatrace.com
      token-ref: prod-token
      safety-level: readwrite-mine
      hooks:
        pre-apply: 'python3 ./policy/strict-check.py "$1" "$2"'
```

Per-context hooks take precedence over global hooks. This allows stricter
validation for production while keeping development lightweight.

### Basic Usage

```bash
# Normal apply -- hook runs automatically
dtctl apply -f dashboard.yaml
# Hook runs via: sh -c '<command>' -- dashboard dashboard.yaml
# $1 = resource type, $2 = filename (available as positional params), stdin = processed JSON
# Hook exits 0 → apply proceeds
# Hook exits non-zero → apply aborted, stderr shown to user

# Skip hook
dtctl apply -f dashboard.yaml --no-hooks

# Dry-run -- hook still runs (validation is useful even in dry-run)
dtctl apply -f dashboard.yaml --dry-run
```

### Writing a Hook

A hook is any executable that:
1. Reads JSON from stdin (the processed resource content — **not** from the file)
2. Receives resource type (`$1`) and source filename (`$2`) as positional arguments
3. Exits 0 to allow, non-zero to reject
4. Writes validation errors to stderr

> **Important:** The resource content is always delivered via **stdin** as processed
> JSON (after YAML→JSON conversion and template rendering). The source filename
> (`$2`) is the original path passed to `dtctl apply -f` — it is provided for
> context and logging only. Do **not** read from `$2`; the file may contain raw
> YAML or unresolved templates that differ from the processed JSON on stdin.

**Minimal example (bash + jq):**

```bash
#!/bin/bash
# validate.sh -- require dashboards to have a title
resource_type="$1"
source_file="$2"  # informational only — read content from stdin, not this file

if [ "$resource_type" = "dashboard" ]; then
  title=$(cat | jq -r '.title // empty')
  if [ -z "$title" ]; then
    echo "Error: dashboard must have a title" >&2
    exit 1
  fi
fi
```

**Node.js example:**

```js
#!/usr/bin/env node
// validate.js
const fs = require('fs');
const [resourceType, sourceFile] = process.argv.slice(2);
// Read processed JSON from stdin — do NOT read sourceFile directly
const input = JSON.parse(fs.readFileSync('/dev/stdin', 'utf8'));

const errors = [];

if (resourceType === 'workflow') {
  if (!input.title) errors.push('workflow must have a title');
  if (!input.tasks || input.tasks.length === 0) errors.push('workflow must have at least one task');
}

if (resourceType === 'dashboard') {
  if (!input.title) errors.push('dashboard must have a title');
}

if (errors.length > 0) {
  errors.forEach(e => console.error(`Error: ${e}`));
  process.exit(1);
}
```

**OPA/Rego example:**

```bash
#!/bin/bash
# opa-validate.sh -- validate using OPA policy
resource_type="$1"
cat | opa eval -I -d "./policies/${resource_type}.rego" 'data.policy.deny' --format raw | \
  grep -q '[]' || { echo "Policy violation" >&2; exit 1; }
```

### Output on Failure

When a hook rejects a resource, dtctl shows the hook's stderr output:

```
$ dtctl apply -f bad-dashboard.yaml
Error: pre-apply hook rejected the resource

Hook stderr:
  Error: dashboard must have a title
  Error: dashboard must have at least one tile

Hook command: node ./scripts/validate.js "$1" "$2"
Exit code: 1
```

In agent mode (`--agent`), the error is structured:

```json
{
  "ok": false,
  "error": {
    "code": "hook_rejected",
    "message": "pre-apply hook rejected the resource",
    "details": {
      "hook": "node ./scripts/validate.js \"$1\" \"$2\"",
      "exit_code": 1,
      "stderr": "Error: dashboard must have a title\nError: dashboard must have at least one tile"
    }
  }
}
```

### Error Cases

```bash
# Hook command not found
$ dtctl apply -f dashboard.yaml
Error: pre-apply hook failed to execute: exec: "validate.sh": executable file not found in $PATH

# Hook times out (default: 30s)
$ dtctl apply -f dashboard.yaml
Error: pre-apply hook timed out after 30s

# Hook crashes (segfault, etc.)
$ dtctl apply -f dashboard.yaml
Error: pre-apply hook failed: signal: segmentation fault
```

---

## Technical Design

### Config Changes

Add a `Hooks` struct to both `Preferences` (global) and `Context` (per-context):

```go
// pkg/config/config.go

type Hooks struct {
    PreApply string `yaml:"pre-apply,omitempty"`
}

type Preferences struct {
    Output string `yaml:"output,omitempty"`
    Editor string `yaml:"editor,omitempty"`
    Hooks  Hooks  `yaml:"hooks,omitempty"`
}

type Context struct {
    Environment string      `yaml:"environment"             table:"ENVIRONMENT"`
    TokenRef    string      `yaml:"token-ref"               table:"TOKEN-REF"`
    SafetyLevel SafetyLevel `yaml:"safety-level,omitempty"  table:"SAFETY-LEVEL"`
    Description string      `yaml:"description,omitempty"   table:"DESCRIPTION,wide"`
    Hooks       Hooks       `yaml:"hooks,omitempty"`
}
```

### Hook Resolution

Per-context hooks take precedence over global hooks:

```go
// pkg/config/config.go

func (c *Config) GetPreApplyHook() string {
    // Per-context hook wins
    if ctx, err := c.CurrentContextObj(); err == nil {
        if ctx.Context.Hooks.PreApply != "" {
            return ctx.Context.Hooks.PreApply
        }
    }
    // Fall back to global
    return c.Preferences.Hooks.PreApply
}
```

### Hook Executor

A standalone package that knows nothing about dtctl internals -- it just runs
a command with stdin/args and returns the result:

```go
// pkg/hook/hook.go
package hook

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
    "time"
)

const DefaultTimeout = 30 * time.Second

// Result holds the outcome of a hook execution
type Result struct {
    ExitCode int
    Stderr   string
}

// RunPreApply executes the pre-apply hook command.
// The command is run via "sh -c" with resource type and source file as
// positional parameters. Processed JSON is piped to stdin.
// sourceFile is the original filename (informational only — the hook must
// read content from stdin, not from this path).
func RunPreApply(ctx context.Context, command string, resourceType string, sourceFile string, jsonData []byte) (*Result, error) {
    if command == "" {
        return &Result{ExitCode: 0}, nil
    }

    ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
    defer cancel()

    // Pass resource type and source file as positional parameters.
    // Inside the hook: $1 = resource type, $2 = source file (available but not
    // appended to the command — the hook references them explicitly if needed).
    // Note: $2 is the original filename for context/logging only. The actual
    // resource content is always on stdin (processed JSON).
    cmd := exec.CommandContext(ctx, "sh", "-c", command, "--", resourceType, sourceFile)
    cmd.Stdin = bytes.NewReader(jsonData)

    var stderr bytes.Buffer
    cmd.Stderr = &stderr

    err := cmd.Run()
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return nil, fmt.Errorf("pre-apply hook timed out after %s", DefaultTimeout)
        }
        if exitErr, ok := err.(*exec.ExitError); ok {
            return &Result{
                ExitCode: exitErr.ExitCode(),
                Stderr:   stderr.String(),
            }, nil
        }
        return nil, fmt.Errorf("pre-apply hook failed to execute: %w", err)
    }

    return &Result{ExitCode: 0, Stderr: stderr.String()}, nil
}
```

### Integration into the Apply Flow

The hook runs inside `Applier.Apply()`, after template rendering and resource
type detection, before dry-run branching or API dispatch. This is the point
where the processed JSON and resource type are both available.

```go
// pkg/apply/applier.go

type Applier struct {
    client        *client.Client
    baseURL       string
    safetyChecker *safety.Checker
    currentUserID string
    preApplyHook  string   // hook command (empty = no hook)
    sourceFile    string   // original filename for hook context
}

// WithPreApplyHook sets the pre-apply hook command
func (a *Applier) WithPreApplyHook(command string) *Applier {
    a.preApplyHook = command
    return a
}

// WithSourceFile sets the original filename (passed to hook as context)
func (a *Applier) WithSourceFile(filename string) *Applier {
    a.sourceFile = filename
    return a
}

func (a *Applier) Apply(fileData []byte, opts ApplyOptions) ([]ApplyResult, error) {
    // ... existing: ValidateAndConvert, RenderTemplate ...

    resourceType, err := detectResourceType(jsonData)
    if err != nil {
        return nil, err
    }

    // --- Pre-apply hook ---
    if !opts.NoHooks && a.preApplyHook != "" {
        result, err := hook.RunPreApply(
            context.Background(),
            a.preApplyHook,
            string(resourceType),
            a.sourceFile,
            jsonData,
        )
        if err != nil {
            return nil, err
        }
        if result.ExitCode != 0 {
            return nil, &HookRejectedError{
                Command:  a.preApplyHook,
                ExitCode: result.ExitCode,
                Stderr:   result.Stderr,
            }
        }
    }

    // ... existing: dry-run, dispatch ...
}
```

### HookRejectedError

A typed error so the command layer can format it appropriately:

```go
// pkg/apply/errors.go

type HookRejectedError struct {
    Command  string
    ExitCode int
    Stderr   string
}

func (e *HookRejectedError) Error() string {
    msg := "pre-apply hook rejected the resource"
    if e.Stderr != "" {
        msg += "\n\nHook stderr:\n"
        // Indent each line of stderr
        for _, line := range strings.Split(strings.TrimSpace(e.Stderr), "\n") {
            msg += "  " + line + "\n"
        }
    }
    msg += fmt.Sprintf("\nHook command: %s\nExit code: %d", e.Command, e.ExitCode)
    return msg
}
```

### Command Layer Changes

```go
// cmd/apply.go

func init() {
    // ... existing flags ...
    applyCmd.Flags().Bool("no-hooks", false, "skip pre-apply hooks")
}

// In RunE:
noHooks, _ := cmd.Flags().GetBool("no-hooks")

applier := apply.NewApplier(c)

// Configure hook
if !noHooks {
    if hookCmd := cfg.GetPreApplyHook(); hookCmd != "" {
        applier = applier.WithPreApplyHook(hookCmd).WithSourceFile(file)
    }
}
```

### ApplyOptions Change

```go
type ApplyOptions struct {
    TemplateVars map[string]interface{}
    DryRun       bool
    Force        bool
    ShowDiff     bool
    NoHooks      bool  // skip pre-apply hooks
}
```

---

## Execution Contract

| Aspect | Behavior |
|--------|----------|
| **Invocation** | `sh -c '<command>' -- <resource-type> <source-file>` |
| **$1** | Resource type (e.g., `dashboard`, `workflow`) |
| **$2** | Source filename from `dtctl apply -f` (informational; do NOT read this file) |
| **Stdin** | Processed JSON (after YAML→JSON + template rendering) — **always read content from stdin** |
| **Stdout** | Ignored (hook can print debug info without affecting dtctl) |
| **Stderr** | Shown to user on failure |
| **Exit 0** | Proceed with apply |
| **Exit non-zero** | Abort apply, show stderr |
| **Timeout** | 30 seconds (hardcoded, reasonable for any validation) |
| **Working directory** | Inherited from dtctl process |
| **Environment** | Inherited from dtctl process |
| **`--no-hooks`** | Skips hook entirely |
| **`--dry-run`** | Hook still runs (validation is useful even in preview) |
| **`--agent` mode** | Structured error with `code: "hook_rejected"` |

---

## Config Precedence

```
per-context hooks.pre-apply  →  preferences.hooks.pre-apply  →  (no hook)
```

This allows:
- A global default hook for all environments
- A stricter hook for production contexts
- An empty per-context hook to disable the global hook for a specific context

To explicitly disable the global hook for a context, set it to an empty string
or the special value `"none"`:

```yaml
contexts:
  - name: development
    context:
      environment: https://dev.dynatrace.com
      token-ref: dev-token
      hooks:
        pre-apply: "none"  # disables global hook for this context
```

---

## Use Cases

### 1. Team Naming Conventions

```bash
#!/bin/bash
# check-naming.sh -- enforce team naming conventions
resource_type="$1"
input=$(cat)

name=$(echo "$input" | jq -r '.title // .name // empty')
if [ -n "$name" ] && ! echo "$name" | grep -qE '^\[team-'; then
  echo "Error: $resource_type name must start with [team-*] prefix, got: $name" >&2
  exit 1
fi
```

### 2. JSON Schema Validation

```bash
#!/bin/bash
# schema-validate.sh -- validate against JSON Schema
resource_type="$1"
schema="./schemas/${resource_type}.json"

if [ -f "$schema" ]; then
  cat | ajv validate -s "$schema" --errors=text 2>&1 || exit 1
fi
# No schema for this type -- allow
```

### 3. OPA Policy Check

```bash
#!/bin/bash
# opa-check.sh
resource_type="$1"
input=$(cat)

result=$(echo "$input" | opa eval -I -d ./policies/ \
  "data.dtctl.${resource_type}.deny" --format json)

violations=$(echo "$result" | jq -r '.result[0].expressions[0].value[]')
if [ -n "$violations" ]; then
  echo "Policy violations:" >&2
  echo "$violations" | while read -r v; do echo "  - $v" >&2; done
  exit 1
fi
```

### 4. CI/CD Pipeline Validation

```yaml
# .dtctl.yaml (project-local config)
apiVersion: v1
kind: Config
current-context: ci
contexts:
  - name: ci
    context:
      environment: https://ci.dynatrace.com
      token-ref: ci-token
      hooks:
        pre-apply: "./ci/validate.sh"
```

### 5. Production Guardrails

```yaml
contexts:
  - name: production
    context:
      environment: https://prod.apps.dynatrace.com
      token-ref: prod-token
      safety-level: readwrite-mine
      hooks:
        pre-apply: "python3 ./policy/prod-guardrails.py"
  - name: staging
    context:
      environment: https://staging.apps.dynatrace.com
      token-ref: staging-token
      # No hook -- anything goes in staging
```

---

## Testing Strategy

### Unit Tests

```go
// pkg/hook/hook_test.go

func TestRunPreApply_Success(t *testing.T) {
    // Hook that exits 0
    result, err := hook.RunPreApply(ctx, "cat > /dev/null", "dashboard", "test.yaml", []byte(`{"title":"test"}`))
    require.NoError(t, err)
    require.Equal(t, 0, result.ExitCode)
}

func TestRunPreApply_Rejected(t *testing.T) {
    // Hook that exits 1 with stderr
    result, err := hook.RunPreApply(ctx, "echo 'bad' >&2 && exit 1", "dashboard", "test.yaml", []byte(`{}`))
    require.NoError(t, err) // err is nil; rejection is in result
    require.Equal(t, 1, result.ExitCode)
    require.Contains(t, result.Stderr, "bad")
}

func TestRunPreApply_Timeout(t *testing.T) {
    // Hook that hangs
    _, err := hook.RunPreApply(ctxWithShortTimeout, "sleep 999", "dashboard", "test.yaml", []byte(`{}`))
    require.ErrorContains(t, err, "timed out")
}

func TestRunPreApply_NotFound(t *testing.T) {
    // Command doesn't exist
    _, err := hook.RunPreApply(ctx, "nonexistent-binary", "dashboard", "test.yaml", []byte(`{}`))
    require.ErrorContains(t, err, "failed to execute")
}

func TestRunPreApply_EmptyCommand(t *testing.T) {
    // No hook configured -- always succeeds
    result, err := hook.RunPreApply(ctx, "", "dashboard", "test.yaml", []byte(`{}`))
    require.NoError(t, err)
    require.Equal(t, 0, result.ExitCode)
}

func TestRunPreApply_ReceivesJSON(t *testing.T) {
    // Verify the hook receives the JSON on stdin
    result, err := hook.RunPreApply(ctx,
        `python3 -c "import sys,json; d=json.load(sys.stdin); assert d['title']=='test'"`,
        "dashboard", "test.yaml", []byte(`{"title":"test"}`))
    require.NoError(t, err)
    require.Equal(t, 0, result.ExitCode)
}

func TestRunPreApply_ReceivesArgs(t *testing.T) {
    // Verify the hook receives resource type and filename as args
    result, err := hook.RunPreApply(ctx,
        `test "$1" = "workflow" && test "$2" = "my-wf.yaml"`,
        "workflow", "my-wf.yaml", []byte(`{}`))
    require.NoError(t, err)
    require.Equal(t, 0, result.ExitCode)
}
```

### Integration Tests

```go
// pkg/apply/applier_hook_test.go

func TestApply_HookRejects(t *testing.T) {
    applier := apply.NewApplier(mockClient).
        WithPreApplyHook("exit 1").
        WithSourceFile("test.yaml")

    _, err := applier.Apply(validWorkflowJSON, apply.ApplyOptions{})
    require.Error(t, err)

    var hookErr *apply.HookRejectedError
    require.ErrorAs(t, err, &hookErr)
    require.Equal(t, 1, hookErr.ExitCode)
}

func TestApply_HookAllows(t *testing.T) {
    applier := apply.NewApplier(mockClient).
        WithPreApplyHook("cat > /dev/null").
        WithSourceFile("test.yaml")

    results, err := applier.Apply(validWorkflowJSON, apply.ApplyOptions{})
    require.NoError(t, err)
    require.Len(t, results, 1)
}

func TestApply_NoHooksFlag(t *testing.T) {
    applier := apply.NewApplier(mockClient).
        WithPreApplyHook("exit 1").  // would reject
        WithSourceFile("test.yaml")

    results, err := applier.Apply(validWorkflowJSON, apply.ApplyOptions{NoHooks: true})
    require.NoError(t, err) // hook was skipped
    require.Len(t, results, 1)
}

func TestApply_HookRunsOnDryRun(t *testing.T) {
    hookRan := false
    // Use a hook that creates a marker file to verify it ran
    applier := apply.NewApplier(mockClient).
        WithPreApplyHook("exit 1").
        WithSourceFile("test.yaml")

    _, err := applier.Apply(validWorkflowJSON, apply.ApplyOptions{DryRun: true})
    require.Error(t, err) // hook still runs and rejects
}
```

### Config Tests

```go
// pkg/config/config_test.go

func TestGetPreApplyHook_ContextOverridesGlobal(t *testing.T) {
    cfg := &Config{
        Preferences: Preferences{
            Hooks: Hooks{PreApply: "global-hook"},
        },
        CurrentContext: "prod",
        Contexts: []NamedContext{{
            Name: "prod",
            Context: Context{
                Hooks: Hooks{PreApply: "prod-hook"},
            },
        }},
    }
    require.Equal(t, "prod-hook", cfg.GetPreApplyHook())
}

func TestGetPreApplyHook_FallsBackToGlobal(t *testing.T) {
    cfg := &Config{
        Preferences: Preferences{
            Hooks: Hooks{PreApply: "global-hook"},
        },
        CurrentContext: "dev",
        Contexts: []NamedContext{{
            Name: "dev",
            Context: Context{}, // no hook
        }},
    }
    require.Equal(t, "global-hook", cfg.GetPreApplyHook())
}

func TestGetPreApplyHook_NoneDisablesGlobal(t *testing.T) {
    cfg := &Config{
        Preferences: Preferences{
            Hooks: Hooks{PreApply: "global-hook"},
        },
        CurrentContext: "dev",
        Contexts: []NamedContext{{
            Name: "dev",
            Context: Context{
                Hooks: Hooks{PreApply: "none"},
            },
        }},
    }
    // "none" is treated as no hook
    require.Equal(t, "", cfg.GetPreApplyHook())
}
```

---

## Alternatives Considered

### 1. Shell aliases wrapping dtctl apply

Users can already define `!` aliases like `validate-apply: "!./validate.sh $1 && dtctl apply -f $1"`.

**Pros:** Zero implementation work.
**Cons:** Loses all apply flags (`--set`, `--dry-run`, `--show-diff`), no
resource-type awareness, fragile quoting, users have to know about it.
**Decision:** Not sufficient. Hooks need to intercept the *processed* content
inside the apply flow, which aliases can't do.

### 2. Plugin system with Go interfaces

A formal plugin mechanism where validators implement a Go interface and are
loaded dynamically.

**Pros:** Type-safe, fast.
**Cons:** Massive complexity, Go-only, version coupling, CGo plugin limitations,
cross-compilation pain.
**Decision:** Way too complex. A stdin/stdout contract works with any language.

### 3. Pipe-based mutation (hook transforms content)

Hook receives JSON on stdin, returns modified JSON on stdout. dtctl uses the
output instead of the input.

**Pros:** More powerful -- hooks can normalize, inject defaults, strip fields.
**Cons:** Mutation is hard to reason about ("why did my dashboard get extra
fields?"), debugging is painful, breaks the mental model of "file = what gets
applied".
**Decision:** Validate-only. If users want transformation, they can build a
pipeline *before* dtctl: `transform.sh < raw.yaml | dtctl apply -f -` (once
stdin support is added).

### 4. Multiple hook commands (chain)

Support a list of hooks that run in sequence.

**Pros:** Composable -- schema check + policy check + naming check.
**Cons:** Adds complexity for ordering, partial failure semantics, timeout
allocation. A single dispatcher script achieves the same thing.
**Decision:** Single command. Users compose with `&&` or write a dispatcher:

```yaml
hooks:
  pre-apply: "./hooks/run-all.sh"
```

```bash
#!/bin/bash
# hooks/run-all.sh -- dispatcher
input=$(cat)
echo "$input" | ./hooks/schema.sh "$@" && \
echo "$input" | ./hooks/policy.sh "$@" && \
echo "$input" | ./hooks/naming.sh "$@"
```

### 5. Post-apply hooks

Run a command after successful apply (for notifications, audit logging, etc.).

**Pros:** Complete lifecycle coverage.
**Cons:** Everything a post-apply hook does can be done by wrapping dtctl in a
script: `dtctl apply -f foo.yaml && notify.sh`. Pre-apply hooks are different --
they need to intercept the *processed* content inside the apply flow, which
can't be done externally.
**Decision:** Defer. Can be added later without breaking changes if needed.

---

## Implementation Checklist

1. Add `Hooks` struct to `pkg/config/config.go` (in `Preferences` and `Context`)
2. Add `GetPreApplyHook()` method to `Config` with context-over-global precedence
3. Create `pkg/hook/hook.go` with `RunPreApply()`
4. Create `pkg/hook/hook_test.go`
5. Add `HookRejectedError` to `pkg/apply/`
6. Add `preApplyHook`/`sourceFile` fields and builder methods to `Applier`
7. Wire hook execution into `Applier.Apply()` after type detection
8. Add `NoHooks` to `ApplyOptions`
9. Add `--no-hooks` flag to `cmd/apply.go`
10. Wire config → applier in `cmd/apply.go` RunE
11. Handle `HookRejectedError` in agent mode output
12. Add tests for config, hook executor, and applier integration
13. Update `docs/dev/IMPLEMENTATION_STATUS.md`

---

## Future Enhancements

These are explicitly out of scope for the initial implementation:

- **Configurable timeout** -- `hooks.pre-apply-timeout: 60s` in config
- **Hook for other commands** -- `pre-create`, `pre-delete`, `pre-edit` (same pattern, add when needed)
- **Server-side validation** -- `dtctl verify <resource> -f` that calls a Dynatrace API; completely orthogonal to hooks
- **Stdin apply support** -- `cat dashboard.yaml | dtctl apply -f -` would need special handling for the filename arg (pass `-` or `<stdin>`)
- **Hook metrics** -- Track hook execution time, pass/fail rate for debugging slow pipelines

---

## References

- git hooks: https://git-scm.com/docs/githooks
- kubectl admission webhooks (inspiration for the "validate but don't mutate" pattern): https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/
- OPA (Open Policy Agent): https://www.openpolicyagent.org/
