---
title: "Command Reference"
layout: docs
---

Complete reference for all dtctl commands, flags, and resource types.

## Command Syntax

```
dtctl [verb] [resource-type] [resource-name] [flags]
```

## Core Verbs

| Verb | Description |
|------|-------------|
| `get` | List or retrieve resources |
| `describe` | Show detailed information about a resource (supports `-o` for structured output) |
| `create` | Create a resource from file or arguments |
| `delete` | Delete resources |
| `edit` | Edit a resource interactively (YAML or JSON) |
| `apply` | Apply configuration from file (create or update) |
| `logs` | Print logs for a resource |
| `query` | Execute a DQL query |
| `exec` | Execute a workflow, function, analyzer, or CoPilot skill |
| `history` | Show version history (snapshots) of a document |
| `restore` | Restore a document to a previous version |
| `diff` | Show differences between local and remote resources |
| `enable` | Enable a cloud monitoring configuration (GCP/Azure) in one step |
| `share` | Share a document with users or groups |
| `unshare` | Remove sharing from a document |
| `verify` | Verify DQL query syntax |
| `alias` | Manage command aliases |
| `ctx` | Quick context management |
| `doctor` | Health check (config, context, token, connectivity, auth) |
| `commands` | Machine-readable command catalog for AI agents |

## Global Flags

```
--context string      Use a specific context
-o, --output string   Output format: json|yaml|csv|table|wide|chart|sparkline|barchart|braille
--plain               Plain output (no colors, no interactive prompts)
--no-headers          Omit headers in table output
-v, --verbose         Verbose output (-v for details, -vv for full HTTP debug)
--debug               Enable debug mode (equivalent to -vv)
--dry-run             Print what would be done without doing it
-A, --agent           Agent output mode (structured JSON envelope)
--no-agent            Disable auto-detected agent mode
-w, --watch           Watch for changes
--interval duration   Watch/live polling interval (default: 2s)
--watch-only          Only show changes, skip initial state
--chunk-size int      Page size for API requests (default: 500, 0=no pagination)
```

## Resource Types

dtctl supports both singular and plural resource names, plus short aliases.

| Resource | Aliases | Operations |
|----------|---------|------------|
| `workflows` | `workflow`, `wf` | get, describe, create, edit, delete, apply, exec, history, restore, diff, watch |
| `workflow-executions` | `wfe` | get, describe, logs |
| `wfe-task-result` | — | get |
| `dashboards` | `dashboard`, `dash`, `db` | get, describe, create, edit, delete, apply, share, unshare, history, restore, diff, watch |
| `notebooks` | `notebook`, `nb` | get, describe, create, edit, delete, apply, share, unshare, history, restore, diff, watch |
| `documents` | `document`, `doc` | get, describe, create, edit, delete, history, restore |
| `trash` | — | get, describe, restore, delete |
| `slos` | `slo` | get, describe, create, delete, apply, exec (evaluate), watch |
| `slo-templates` | `slo-template` | get, describe |
| `settings-schemas` | `settings-schema` | get, describe |
| `settings` | — | get, create, update, delete |
| `buckets` | `bucket` | get, describe, create, delete, apply, watch |
| `segments` | `segment`, `seg`, `filter-segments`, `filter-segment` | get, describe, create, edit, delete, apply, watch |
| `lookups` | `lookup` | get, describe, create, delete |
| `extensions` | `extension`, `ext`, `exts` | get, describe |
| `extension-configs` | `extension-config`, `ext-configs`, `ext-config` | get, describe, apply |
| `hub-extensions` | `hub-extension` | get, describe |
| `hub-extension-releases` | `hub-extension-release` | get |
| `apps` | `app` | get, describe, delete |
| `functions` | `function`, `func` | get, describe, exec |
| `intents` | `intent` | get, describe, find, open |
| `analyzers` | `analyzer` | get, exec |
| `copilot-skills` | — | get |
| `notifications` | `notification` | get, describe, delete, watch |
| `edgeconnects` | `edgeconnect`, `ec` | get, describe, create, delete, apply |
| `breakpoints` | `breakpoint` | get, describe, create, update, delete |

## Configuration Commands

```bash
# Context management
dtctl config set-context <name> --environment <url> --token-ref <ref>
dtctl config get-contexts
dtctl config use-context <name>
dtctl config current-context
dtctl config describe-context <name>
dtctl config delete-context <name>
dtctl config view

# Quick context switching
dtctl ctx                          # List contexts
dtctl ctx <name>                   # Switch context

# Credentials
dtctl config set-credentials <ref> --token <token>

# Per-project config
dtctl config init                  # Generate .dtctl.yaml template
dtctl config init --context <name> # Custom context name

# Preferences
dtctl config set preferences.editor vim
dtctl config set preferences.output json
```

## Authentication Commands

```bash
# OAuth login (recommended)
dtctl auth login --context <name> --environment <url>
dtctl auth logout
dtctl auth refresh

# User identity
dtctl auth whoami
dtctl auth whoami --id-only
dtctl auth whoami -o json
```

## Query Commands

```bash
# Inline query
dtctl query "fetch logs | limit 10"

# File-based query
dtctl query -f query.dql

# Stdin (heredoc)
dtctl query -f - <<'EOF'
fetch logs | filter status = "ERROR" | limit 100
EOF

# With template variables
dtctl query -f query.dql --set host=my-server --set limit=500

# Query parameters
dtctl query "..." --max-result-records 5000
dtctl query "..." --default-timeframe-start "2024-01-01T00:00:00Z"
dtctl query "..." --timezone "Europe/Paris"
dtctl query "..." --metadata                    # Include execution metadata
dtctl query "..." --live --interval 5s           # Live mode

# Filter segments
dtctl query "..." --segment my-segment-uid       # By UID or name (repeatable)
dtctl query "..." -S seg-1 -S seg-2              # Short form, AND-combined
dtctl query "..." -S "seg?var=val"               # Bind variables inline
dtctl query "..." --segments-file segments.yaml  # Segments with variables from file

# Verify query syntax
dtctl verify query "fetch logs | limit 10"
dtctl verify query -f query.dql --canonical --fail-on-warn
```

## Execution Commands

```bash
# Workflows
dtctl exec workflow <id-or-name> --wait --show-results
dtctl exec workflow <id> --params env=prod,severity=high

# SLO evaluation
dtctl exec slo <id>

# Davis Analyzers
dtctl exec analyzer <analyzer-id> --query "timeseries avg(dt.host.cpu.usage)"

# App Functions
dtctl exec function <app-id>/<function-name> --method POST --payload '{...}'

# Davis CoPilot
dtctl exec copilot "What is DQL?" --stream
dtctl exec copilot nl2dql "error logs from last hour"
dtctl exec copilot dql2nl "fetch logs | filter status='ERROR'"
dtctl exec copilot document-search "CPU analysis" --collections notebooks
```

## Diff Command

```bash
# Compare local file with remote resource
dtctl diff -f workflow.yaml

# Compare two local files
dtctl diff -f v1.yaml -f v2.yaml

# Compare two remote resources
dtctl diff workflow prod-workflow staging-workflow

# Output formats
dtctl diff -f dashboard.yaml --semantic          # Human-readable
dtctl diff -f workflow.yaml -o json-patch        # RFC 6902
dtctl diff -f dashboard.yaml --side-by-side      # Split-screen

# Options
dtctl diff -f workflow.yaml --ignore-metadata    # Skip timestamps/versions
dtctl diff -f dashboard.yaml --ignore-order      # Ignore array order
dtctl diff -f workflow.yaml --quiet              # Exit code only (CI/CD)
```

## Alias Commands

```bash
# Simple alias
dtctl alias set wf "get workflows"

# Parameterized alias
dtctl alias set logs-errors "query 'fetch logs | filter status=\$1 | limit 100'"

# Shell alias (prefix with !)
dtctl alias set wf-names "!dtctl get workflows -o json | jq -r '.workflows[].title'"

# Management
dtctl alias list
dtctl alias delete <name>
dtctl alias export -f aliases.yaml
dtctl alias import -f aliases.yaml
```

## Health Check

```bash
dtctl doctor    # Runs 6 checks: version, config, context, token, connectivity, auth
```

## Command Catalog

```bash
dtctl commands -o json            # Full catalog
dtctl commands --brief -o json    # Compact (no descriptions, no global flags)
dtctl commands workflow -o json   # Filter to specific resource
dtctl commands howto              # Generate Markdown how-to guide
```

## Common Patterns

### Watch Mode

All `get` commands support watch mode for real-time monitoring:

```bash
dtctl get workflows --watch                    # Watch all
dtctl get workflows --watch --interval 5s      # Custom interval
dtctl get workflows --watch --watch-only       # Only show changes
dtctl get dashboards --mine --watch            # Watch your own
```

### Dry Run

Preview changes before applying:

```bash
dtctl apply -f workflow.yaml --dry-run
dtctl create settings -f pipeline.yaml --schema ... --dry-run
dtctl delete workflow "Test Workflow" --dry-run
```

### Idempotent Applies

Use `--write-id` and `--id` to prevent duplicate resources on repeated runs:

```bash
# First apply: stamp the generated ID back into the source file
dtctl apply -f dashboard.yaml --write-id

# All future runs update the same resource
dtctl apply -f dashboard.yaml

# Forgot --write-id on the first run? Recover without creating another duplicate:
dtctl apply -f dashboard.yaml --write-id --id <id-from-first-run>

# CI/scripting: apply a template file to a known target resource
dtctl apply -f template.yaml --id $DASHBOARD_ID
```

`--write-id` is a no-op when the file already contains an `id` field.

### Pipeline Integration

```bash
# Count resources
dtctl get workflows -o json | jq '. | length'

# Extract IDs
dtctl get workflows -o json | jq -r '.[].id'

# Filter and export
dtctl query "fetch logs" -o csv > logs.csv
dtctl query "fetch logs" -o json | jq '.records[]'
```

### Environment Variables

```bash
export DTCTL_OUTPUT=json           # Default output format
export DTCTL_CONTEXT=production    # Default context
export EDITOR=vim                  # Editor for edit commands
```
