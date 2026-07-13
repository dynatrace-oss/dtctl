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
| `wait` | Poll a DQL query until a record-count condition is met (tests/CI) |
| `inspect` | Inspect a spilled query-result file locally (rows, schema, stats) without re-querying Grail |
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

# Quick context switching (shortcuts without the "config" prefix)
dtctl ctx                          # List contexts
dtctl ctx <name>                   # Switch context
dtctl ctx current                  # Show current context name
dtctl ctx describe <name>          # Show details of a context
dtctl ctx set <name> --environment <url> [--token-ref <ref>]  # Create/update a context and switch to it
dtctl ctx delete <name>            # Delete a context
dtctl ctx token [<name>]           # Print the resolved token for a context

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

# Session status: token presence, expiry, refresh token, granted scopes
dtctl auth status
dtctl auth status -o json

# User identity
dtctl auth whoami
dtctl auth whoami --id-only
dtctl auth whoami -o json
```

`auth status` reports the OAuth session state for the current context — whether an
access token is stored, when it expires, whether a refresh token is present (so
the CLI can auto-refresh), and the granted scopes. For platform (non-OAuth)
tokens it reports the auth type and skips the OAuth-specific fields.

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

# Spill a large result to a file, return a summary (see dql-queries#spilling-large-results-to-a-file)
dtctl query "..." --spill                         # always spill (bare flag)
dtctl query "..." --spill=auto --spill-threshold 100KB  # spill only above the size
dtctl query "..." --spill-to ./out.jsonl          # explicit destination; --spill-format jsonl|json|csv|parquet

# Filter segments
dtctl query "..." --segment my-segment-uid       # By UID or name (repeatable)
dtctl query "..." -S seg-1 -S seg-2              # Short form, AND-combined
dtctl query "..." -S "seg?var=val"               # Bind variables inline
dtctl query "..." --segments-file segments.yaml  # Segments with variables from file

# Verify query syntax
dtctl verify query "fetch logs | limit 10"
dtctl verify query -f query.dql --canonical --fail-on-warn
```

## Inspect Commands

`dtctl inspect` reads a query-result file that `dtctl query` spilled to disk (see
[Spilling Large Results](dql-queries#spilling-large-results-to-a-file)) — without
re-querying Grail and without pulling the whole result back into context. Its
primary capability is **row access**, which the spill summary never carried.
Choose exactly one primitive per call; it is not a query engine (no SQL, no
GROUP BY, no dtctl predicate language — push aggregates back into DQL).

```bash
# Row access
dtctl inspect q-7f3a9c.jsonl --head 20            # first N rows
dtctl inspect q-7f3a9c.jsonl --tail 10            # last N rows
dtctl inspect q-7f3a9c.jsonl --page --offset 1000 --limit 50   # a paginated window (result order)
dtctl inspect q-7f3a9c.jsonl --head 20 --fields timestamp,content  # column projection (composable)

# Keep only the matching rows — a streaming jq filter over the WHOLE file
dtctl inspect q-7f3a9c.jsonl --jq 'select(.status == 500)'         # every matching row
dtctl inspect q-7f3a9c.jsonl --jq 'select(.status == 500)' --head 20  # first 20 matches (window bounds it)
dtctl inspect q-7f3a9c.jsonl --jq '{host, timestamp}'             # reshape each row to an object

# Re-derive the summary for a file whose manifest is out of context
dtctl inspect q-7f3a9c.jsonl --schema             # columns + types + null counts
dtctl inspect q-7f3a9c.jsonl --stats              # per-column profile (or --stats=col,col)
dtctl inspect q-7f3a9c.jsonl --sample 5           # N representative (leading) rows

# Recover a lost file handle
dtctl inspect --list                              # spilled files in the active context, with provenance
```

Reads jsonl, json, csv, and parquet. Honours the shared `--spill*` flags: an
oversized inspect window — or a `--jq` filter that matches a large number of rows —
re-spills to a new managed file instead of flooding output. It lists/reads only
the active context's partition and refuses a file that belongs to a different
context or tenant.

`--jq` on `inspect` is a **full-file** filter: unlike elsewhere in dtctl (where
`--jq` post-processes the in-memory result), here it is run **per record** over
the whole spilled file — like `jq` over an NDJSON file — and collects the objects
it emits. It composes with a single row-access window (`--head`/`--tail`/`--page`,
which bounds the matches) and with `--fields` (which projects them), but is
mutually exclusive with `--schema`/`--stats`/`--sample`. The program must emit
objects; for free-form scalar extraction run `jq` over the file yourself.

## Wait Commands

`dtctl wait query` polls a DQL query with exponential backoff until a record-count
condition is met — built for tests and CI/CD that must wait for data to land. See
[Waiting for Query Conditions](dql-queries#waiting-for-query-conditions) for the
full condition list, polling controls, and exit codes.

```bash
# Wait for exactly one matching record (default timeout 5m)
dtctl wait query "fetch spans | filter test_id == 'test-123'" --for=count=1

# Wait for any error logs, with a custom timeout
dtctl wait query "fetch logs | filter status == 'ERROR'" --for=any --timeout 2m

# Conditions: count=N | count-gte=N | count-gt=N | count-lte=N | count-lt=N | any | none
# Polling:    --timeout --max-attempts --initial-delay --min-interval --max-interval --backoff-multiplier
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
dtctl commands                    # Minimal overview: verbs, resources, subcommands (TOON default)
dtctl commands --brief -o json    # Compact: + mutating/access/scopes + flag types
dtctl commands --full -o json     # Full catalog: descriptions, flag defaults, global flags
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
export DTCTL_SPILL=never           # Result spill mode: auto|always|never
export DTCTL_SPILL_DIR=/mnt/scratch # Base directory for spilled query results
```
