# Spec: `dtctl commands` — Machine-Readable Command Catalog

**Status**: Shipped (PR #62, merged 2026-03-06)
**Priority**: P1
**Effort**: Medium (2-3 days)
**Impact**: Enables automated tool registration for AI agents and MCP servers

---

## Problem

AI coding agents (Claude Code, Cursor, GitHub Copilot, etc.) need to know what dtctl can do — what commands exist, what parameters they accept, what types are expected. Currently, agents rely on reading `AGENTS.md` or the user manually describing commands. Discovery happens through trial and error or by reading help text one command at a time.

---

## Design

### New command: `commands`

Simple, self-explanatory — matches the pattern used by Heroku CLI and oclif-based tools. No ambiguity about what it does.

```
dtctl commands                        # Full JSON listing of all commands
dtctl commands --brief                # Minimal listing (reduced token count)
dtctl commands workflows              # Commands for a single resource type
dtctl commands -o yaml                # YAML output
dtctl commands howto                  # Usage-focused reference document
```

**Naming rationale**:
- `commands` — straightforward, describes exactly what it outputs. Established pattern (Heroku CLI, oclif framework).
- `--brief` instead of `--compact` — clearer intent ("give me the short version").
- Positional arg instead of a flag — `dtctl commands workflows` is more natural than `dtctl commands --scope workflows`. Matches kubectl patterns (`kubectl api-resources`, `kubectl explain pods`). Accepts resource names or aliases (`wf`, `dash`).
- `-o` — reuses dtctl's standard output flag. Only `json` (default) and `yaml` are supported; other formats (table, chart, etc.) are not meaningful here.
- `howto` subcommand — action-oriented; outputs an LLM-optimized usage guide.

### Output structure

The output describes dtctl's verb-noun command model:

```json
{
  "schema_version": 1,
  "tool": "dtctl",
  "version": "0.13.0",
  "description": "kubectl-inspired CLI for the Dynatrace platform",
  "command_model": "verb-noun",
  "global_flags": {
    "--output": { "type": "string", "default": "table", "values": ["table", "wide", "json", "yaml", "csv", "chart", "sparkline", "barchart", "braille"] },
    "--agent": { "type": "boolean", "default": false, "description": "Enable agent output mode with response envelope" },
    "--plain": { "type": "boolean", "default": false, "description": "Machine-readable output (no colors, no prompts)" },
    "--dry-run": { "type": "boolean", "default": false, "description": "Preview changes without applying" },
    "--context": { "type": "string", "description": "Override the current config context" },
    "--chunk-size": { "type": "integer", "default": 500, "description": "Pagination chunk size (0 for all)" }
  },
  "verbs": {
    "get": {
      "description": "List or get resources",
      "mutating": false,
      "resources": ["workflows", "dashboards", "notebooks", "slos", "settings", "settings-schemas", "buckets", "apps", "functions", "intents", "users", "groups", "notifications", "edgeconnect", "workflow-executions", "copilot-skills", "sdk-versions", "slo-templates", "azure-connections", "azure-monitoring", "gcp-connections", "gcp-monitoring"],
      "flags": { "--mine": "boolean" }
    },
    "describe": {
      "description": "Show detailed resource information",
      "mutating": false,
      "resources": ["workflow", "dashboard", "notebook", "slo", "settings-schema", "bucket", "app", "function", "intent", "edgeconnect", "workflow-execution", "azure-connection", "azure-monitoring", "gcp-connection", "gcp-monitoring"],
      "required_args": ["id-or-name"]
    },
    "apply": {
      "description": "Create or update resources from file (declarative, idempotent)",
      "mutating": true,
      "safety_operation": "OperationCreate",
      "flags": {
        "-f/--file": { "type": "string", "required": true, "description": "YAML/JSON file path" },
        "--set": { "type": "string[]", "description": "Template variables (key=value)" },
        "--show-diff": { "type": "boolean", "description": "Show diff before applying" }
      },
      "supported_resources": ["workflow", "dashboard", "notebook", "slo", "settings", "bucket", "lookup-table", "notification", "azure-connection", "azure-monitoring", "gcp-connection", "gcp-monitoring"]
    },
    "create": {
      "description": "Create a resource from file or flags",
      "mutating": true,
      "safety_operation": "OperationCreate",
      "flags": {
        "-f/--file": { "type": "string", "description": "YAML/JSON file path" }
      },
      "resources": ["workflow", "dashboard", "notebook", "slo", "bucket", "lookup-table", "edgeconnect", "settings", "azure-connection", "azure-monitoring", "gcp-connection", "gcp-monitoring"]
    },
    "edit": {
      "description": "Edit a resource in $EDITOR",
      "mutating": true,
      "safety_operation": "OperationUpdate",
      "resources": ["workflow", "dashboard", "notebook", "settings"]
    },
    "delete": {
      "description": "Delete resources",
      "mutating": true,
      "safety_operation": "OperationDelete",
      "resources": ["workflow", "dashboard", "notebook", "slo", "settings", "bucket", "lookup-table", "notification", "edgeconnect", "azure-connection", "azure-monitoring", "gcp-connection", "gcp-monitoring"]
    },
    "diff": {
      "description": "Compare local vs remote resource state",
      "mutating": false,
      "modes": ["local-vs-remote", "file-vs-file", "resource-vs-resource", "stdin-vs-remote"]
    },
    "query": {
      "description": "Execute DQL queries",
      "mutating": false,
      "flags": {
        "--query": { "type": "string", "description": "Inline DQL query" },
        "-f/--file": { "type": "string", "description": "DQL query from file" },
        "--set": { "type": "string[]", "description": "Template variables" },
        "--live": { "type": "boolean", "description": "Live-updating query results" }
      }
    },
    "wait": {
      "description": "Wait for a DQL condition to be met",
      "mutating": false,
      "flags": {
        "--query": { "type": "string", "required": true },
        "--condition": { "type": "string", "required": true, "description": "e.g., 'count > 0', 'status == COMPLETED'" },
        "--timeout": { "type": "duration", "default": "5m" },
        "--interval": { "type": "duration", "default": "10s" }
      }
    },
    "watch": {
      "description": "Real-time resource monitoring with change detection",
      "mutating": false,
      "flags": {
        "--interval": { "type": "duration", "default": "5s" },
        "--watch-only": { "type": "boolean", "description": "Show only changes, not full list" }
      }
    },
    "exec": {
      "description": "Execute resources (run workflows, DQL queries, functions, analyzers, copilot)",
      "mutating": true,
      "safety_operation": "OperationCreate",
      "subcommands": {
        "workflow": { "description": "Run a workflow", "required_args": ["id-or-name"] },
        "dql": { "description": "Execute a DQL query" },
        "function": { "description": "Execute an app function", "required_args": ["app-id/function-name"] },
        "analyzer": { "description": "Execute a Davis AI analyzer", "required_args": ["analyzer-name"] },
        "copilot": { "description": "Davis CoPilot chat", "subcommands": ["nl2dql", "dql2nl", "document-search"] }
      }
    },
    "history": {
      "description": "Show version history",
      "mutating": false,
      "resources": ["workflow", "dashboard", "notebook"]
    },
    "restore": {
      "description": "Restore a previous version",
      "mutating": true,
      "safety_operation": "OperationUpdate",
      "resources": ["workflow", "dashboard", "notebook"],
      "required_args": ["id-or-name", "version"]
    },
    "share": {
      "description": "Share a resource with users or groups",
      "mutating": true,
      "safety_operation": "OperationUpdate",
      "resources": ["dashboard", "notebook"]
    },
    "unshare": {
      "description": "Remove sharing from a resource",
      "mutating": true,
      "safety_operation": "OperationUpdate",
      "resources": ["dashboard", "notebook"]
    },
    "doctor": {
      "description": "Health check (config, auth, connectivity)",
      "mutating": false
    },
    "ctx": {
      "description": "Quick context switching",
      "mutating": false,
      "subcommands": ["list", "use", "current"]
    }
  },
  "resource_aliases": {
    "wf": "workflows",
    "dash": "dashboards",
    "db": "dashboards",
    "nb": "notebooks",
    "bkt": "buckets",
    "ec": "edgeconnect",
    "fn": "functions",
    "func": "functions"
  },
  "time_formats": {
    "relative": ["1h", "30m", "7d", "5min"],
    "absolute": "RFC3339 (e.g., 2024-01-15T10:00:00Z)",
    "unix": "Unix timestamp (e.g., 1705312800)"
  },
  "patterns": [
    "Use 'dtctl apply -f' for idempotent resource management",
    "Use 'dtctl diff' before 'dtctl apply' to preview changes",
    "Use 'dtctl query' for ad-hoc DQL queries, not resource-specific flags",
    "Use '--dry-run' to validate apply operations without executing",
    "Use '--agent' for JSON output with operational metadata",
    "Use 'dtctl wait' in CI/CD to poll for conditions",
    "Always specify '--context' in automation scripts"
  ],
  "antipatterns": [
    "Don't use 'dtctl create' followed by 'dtctl edit' — use 'dtctl apply -f' instead",
    "Don't parse table output — use '-o json' or '--agent'",
    "Don't hardcode resource IDs — use 'dtctl get' to discover them",
    "Don't skip 'dtctl diff' before 'dtctl apply' in production contexts"
  ]
}
```

### Key schema fields

- **`schema_version`** — integer, incremented on breaking changes to the schema structure. Lets consumers detect incompatibilities without parsing the entire document.
- **`mutating`** — boolean per verb. Derived from dtctl's safety system (`OperationCreate`, `OperationUpdate`, `OperationDelete`). Agents can use this to assess risk before running a command.
- **`safety_operation`** — the safety operation type for mutating commands. Maps directly to dtctl's 4-tier safety levels (a command blocked at `readonly` will have `safety_operation` set; a read-only verb won't).
- **`time_formats`** — in the JSON schema (not just in `howto`) so agents consuming the structured output have this without a second call.

### Brief mode

`--brief` strips descriptions, examples, patterns, antipatterns, time_formats, and safety_operation. Reduces token count by ~60%:

```json
{
  "schema_version": 1,
  "tool": "dtctl",
  "version": "0.13.0",
  "command_model": "verb-noun",
  "verbs": {
    "get": { "mutating": false, "resources": ["workflows", "dashboards", "notebooks", "slos", "settings", ...] },
    "apply": { "mutating": true, "flags": { "-f": "string (required)", "--set": "string[]", "--show-diff": "boolean" } },
    "query": { "mutating": false, "flags": { "--query": "string", "-f": "string", "--set": "string[]" } },
    "delete": { "mutating": true, "resources": ["workflow", "dashboard", "notebook", ...] },
    ...
  },
  "aliases": { "wf": "workflows", "dash": "dashboards", ... }
}
```

Note: `mutating` is preserved in brief mode — it's small, and agents always need it.

### Positional resource filter

`dtctl commands workflows` (or `dtctl commands wf`) outputs the full schema but filtered to only verbs that operate on workflows. The filter accepts resource names and aliases, resolved through the same alias system used by the rest of dtctl.

```bash
dtctl commands workflows    # Only verbs that apply to workflows
dtctl commands wf           # Same, using alias
dtctl commands get          # Only the 'get' verb (filter by verb name)
```

When a positional arg matches both a verb and a resource, verb takes priority (consistent with dtctl's command resolution).

### `dtctl commands howto`

Outputs a markdown document optimized for LLM context. Structure:

```markdown
# dtctl Quick Reference

## Command Model
dtctl uses a verb-noun pattern: `dtctl <verb> <resource> [flags]`

## Common Workflows

### Deploying a workflow
1. `dtctl apply -f workflow.yaml --dry-run --show-diff` — preview
2. `dtctl apply -f workflow.yaml` — apply
3. `dtctl describe workflow <id>` — verify

### Querying data
1. `dtctl query --query "fetch logs | filter status == 'ERROR' | limit 10"`
2. `dtctl query --query "..." --output chart` — visualize

### CI/CD pipeline
1. `dtctl apply -f resource.yaml --plain` — deploy
2. `dtctl wait --query "..." --condition "count > 0" --timeout 5m` — wait for condition
3. `dtctl verify query --query "..."` — validate DQL syntax (exit code 0/1)

## Safety Levels
dtctl contexts have safety levels that restrict mutating commands:
- `readonly` — blocks all create/update/delete
- `readwrite-mine` — allows modifying own resources only
- `readwrite-all` — allows modifying all resources (default)
- `dangerously-unrestricted` — allows all operations including bucket deletion

## Time Formats
- Relative: 1h, 30m, 7d, 5min
- Absolute: RFC3339 (2024-01-15T10:00:00Z)
- Unix: 1705312800

## Output Formats
table, wide, json, yaml, csv, chart, sparkline, barchart, braille
```

### Future enhancement: `--help` interception in agent mode

When `--agent` is active, `--help` on any command could return a scoped JSON schema for that command subtree instead of unstructured text. This removes a discovery round-trip — agents naturally try `--help`, and getting JSON back lets them parse it without regex:

```bash
# With --agent active:
dtctl get workflows --help    # Returns JSON schema scoped to 'get workflows'
dtctl apply --help            # Returns JSON schema scoped to 'apply'
```

This is a natural extension of the `commands` feature and should be implemented after the base `commands` command ships. It is tracked separately to keep this spec focused.

---

## Implementation Plan

All steps completed in PR #62.

### Step 1: Create cmd/commands.go (0.5 day) -- Done

1. Add `commandsCmd` with `howto` subcommand
2. Default behavior (no subcommand): output JSON command listing
3. Accept optional positional arg for resource/verb filtering
4. Support `-o json` (default) and `-o yaml`
5. Register under root command
6. Add short/long descriptions following the help text spec

### Step 2: Build command listing generator (1 day) -- Done

1. Create `pkg/commands/listing.go`
2. Walk the Cobra command tree (`rootCmd.Commands()`) to extract:
   - Command names, aliases, descriptions
   - Flags with types, defaults, required status
   - ValidArgs and resource lists
3. Categorize into verb-noun structure
4. Annotate each verb with `mutating` (derived from whether the command handler calls `NewSafetyChecker`) and `safety_operation`
5. Include `schema_version`, `time_formats`, `resource_aliases`
6. Output as JSON or YAML to stdout
7. `--brief` flag strips verbose fields (descriptions, patterns, antipatterns, time_formats, safety_operation) but preserves `mutating`
8. Positional arg filters to a specific resource or verb

### Step 3: Build howto generator (0.5 day) -- Done

1. Create `pkg/commands/howto.go`
2. Static markdown template with dynamic sections (version, resource list, safety levels)
3. Include workflows, time formats, output formats, patterns

### Step 4: Tests (0.5 day) -- Done

1. Golden test for command listing output (must not change unexpectedly)
2. Golden test for brief listing
3. Golden test for filtered listing (`dtctl commands workflows`)
4. Golden test for howto output
5. Test that listing is valid JSON
6. Test that all registered commands appear in listing
7. Test that `mutating` is `true` for all commands that call `NewSafetyChecker`
8. Test that resource aliases resolve correctly in filtering

---

## Acceptance Criteria

- [x] `dtctl commands` outputs valid JSON describing all commands with `schema_version`
- [x] Every verb has a `mutating` boolean; mutating verbs include `safety_operation`
- [x] `dtctl commands --brief` is ≤40% the size of full output and preserves `mutating`
- [x] `dtctl commands workflows` outputs listing filtered to workflow-related verbs only
- [x] `dtctl commands wf` resolves alias and produces same output as `dtctl commands workflows`
- [x] `dtctl commands -o yaml` outputs YAML
- [x] `dtctl commands howto` outputs markdown with workflows, time formats, safety levels, patterns
- [x] Listing dynamically reflects registered commands (adding a new resource auto-includes it)
- [x] `time_formats` is present in the JSON schema (not only in howto)
- [x] Golden tests prevent listing format regressions

---

## Important: Implementation naming

**In the dtctl codebase, do NOT reference pup or any competing CLI.** The naming in this spec (`commands`, `howto`, `--brief`, `mutating`, `patterns`, `antipatterns`, `command_model`) is original to dtctl. The concept of a CLI exposing its own command structure as machine-readable data is a general pattern (cf. Heroku CLI's `commands`, kubectl's `api-resources`, Terraform's `providers schema`).

## References (this comparison repo only)

- Inspiration: pup's agent schema command — dtctl's design diverges with `commands` top-level command, `howto` subcommand, positional resource filtering, `mutating`/`safety_operation` annotations, and verb-noun listing structure
- Heroku CLI: `heroku commands --json` pattern
- dtctl's command tree: `dtctl/cmd/root.go` (root command), `dtctl/cmd/get.go` (registers resources)
- dtctl's safety system: `dtctl/pkg/safety/` (operation types, 4-tier levels)
- dtctl's AI detection: `dtctl/pkg/aidetect/detect.go`
