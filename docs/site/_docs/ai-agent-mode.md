---
layout: docs
title: AI Agent Mode
---

dtctl provides first-class support for AI coding agents with a structured JSON output mode, automatic environment detection, and a machine-readable command catalog.

## Overview

The `--agent` (or `-A`) flag wraps all dtctl output in a structured JSON envelope:

```bash
dtctl get workflows --agent
```

This makes it straightforward for AI agents to parse responses, handle errors, and discover follow-up actions without scraping human-readable text.

## Response Format

### Successful responses

```json
{
  "ok": true,
  "result": [
    {
      "id": "wf-abc123",
      "name": "Daily Health Check",
      "state": "enabled"
    }
  ],
  "context": {
    "verb": "get",
    "resource": "workflow",
    "suggestions": [
      "dtctl describe workflow wf-abc123",
      "dtctl exec workflow wf-abc123"
    ]
  }
}
```

### Error responses

```json
{
  "ok": false,
  "error": {
    "code": "auth_required",
    "message": "No valid authentication found. Run 'dtctl auth login' or configure a token.",
    "suggestions": [
      "dtctl auth login --context my-env --environment https://abc12345.apps.dynatrace.com",
      "dtctl config set-credentials my-token --token <your-token>"
    ]
  }
}
```

Error codes are stable identifiers that agents can match on programmatically (e.g. `auth_required`, `not_found`, `forbidden`, `rate_limited`).

### Query results: the `result.kind` discriminator

In agent mode, `dtctl query` results are self-describing: the `result` payload
carries a `kind` field so a consumer always branches on one discriminator,
regardless of how big the result was. There are three kinds:

| `result.kind` | When | Payload |
|---|---|---|
| `records` | small result, returned inline | the rows under `result.records` |
| `result-file` | large result [spilled to a file](dql-queries#spilling-large-results-to-a-file) | a manifest: `path`, `format`, `rows`, `bytes`, column stats, `sample_rows` |
| `summary-only` | large result but the rows could not be written to disk | the same manifest **minus `path`** |

On a `summary-only` result the rows are not on disk, so `context.suggestions`
carries the right next step for *why* the spill degraded: a read-only filesystem
steers you to re-query with `--spill=never` and a bound (`| fields …` / `| limit N`,
or `--max-result-records N`) so the inline result stays small, while a one-off
write failure suggests retrying with an explicit `--spill-to <path>`.

```json
{
  "ok": true,
  "envelope_version": 1,
  "result": {
    "kind": "result-file",
    "path": "~/Library/Caches/dtctl/results/prod/q-7f3a9c.jsonl",
    "format": "jsonl",
    "rows": 84213,
    "columns": [ { "name": "status", "type": "long", "nulls": 0, "min": 500, "max": 599 } ],
    "sample_rows": [ /* first few rows */ ]
  },
  "context": {
    "verb": "query", "resource": "logs", "total": 84213,
    "decided": "spilled", "threshold_bytes": 51200, "measured_bytes": 16804000
  }
}
```

The envelope carries `envelope_version` for forward compatibility. **A consumer
MUST treat an unrecognised `result.kind` as opaque** — don't parse `result`, fall
back to the human-readable `context` (which always carries `decided`, `total`,
`warnings`, and `suggestions`). When Grail sampled the result, the per-column
stats move into a `sample_stats` block (each column tagged `basis: "sample"`) so
sample-based figures can't be misread as population truth.

> The inline `kind: "records"` envelope is emitted on the spill-aware path
> whenever agent mode emits JSON — including under `--spill=never`, which forces
> every row inline regardless of size but still as a `kind: "records"` envelope
> (never a human table). Explicit non-JSON output (`-o toon/csv/yaml`) and `--jq`
> transforms keep their requested shape and fall through to the plain
> `{ "records": …, "metadata": … }` output.

## Auto-Detection

dtctl automatically enables agent mode when it detects it is running inside a known AI agent environment. Detection is based on the presence of specific environment variables:

| Environment Variable | Agent |
|---|---|
| `CLAUDECODE` | Claude Code |
| `OPENCODE` | OpenCode |
| `GITHUB_COPILOT` | GitHub Copilot |
| `CURSOR_AGENT` | Cursor |
| `KIRO` | Kiro |
| `JUNIE` | Junie |
| `OPENCLAW` | OpenClaw |
| `CODEIUM_AGENT` | Codeium / Windsurf |
| `TABNINE_AGENT` | Tabnine |
| `AMAZON_Q` | Amazon Q |

When auto-detected, agent mode is enabled without requiring the `--agent` flag.

### Opting out

To disable auto-detection and get normal human-readable output:

```bash
dtctl get workflows --no-agent
```

## Behavior

Agent mode implies `--plain`:

- No ANSI colors in output
- No interactive prompts (e.g. name disambiguation)
- No progress spinners or animations

This ensures output is always machine-parseable.

## Command Catalog

AI agents can bootstrap their knowledge of dtctl using the built-in command catalog:

```bash
# Brief catalog -- compact listing of commands, flags, and resource types
dtctl commands --brief -o json

# Full catalog -- detailed command descriptions and flag documentation
dtctl commands -o json

# Human-readable how-to guide in Markdown
dtctl commands howto
```

The brief catalog is ideal for including in an agent's system prompt or initial context, giving it a complete map of available operations without consuming excessive tokens.

## Tips and Tricks

### Name resolution

When agent mode is active, interactive name disambiguation is disabled. Use exact IDs instead of display names to avoid ambiguity:

```bash
# Prefer IDs in agent mode
dtctl describe workflow wf-abc123

# Names may fail if multiple resources share the same name
dtctl describe workflow "Daily Health Check"
```

All `describe` subcommands support agent mode, returning the full resource object in the JSON envelope:

```bash
dtctl describe workflow wf-abc123 --agent
dtctl describe slo my-slo -A
dtctl describe dashboard my-dash -o json -A
```

### Dry-run

Use `--dry-run` to preview mutating operations without making changes:

```bash
dtctl apply -f workflow.yaml --dry-run
```

### Diff

Use `--diff` to see what would change before applying:

```bash
dtctl apply -f workflow.yaml --diff
```

### Verbose output

Use `-v` or `--verbose` for additional debugging information:

```bash
dtctl get workflows -v --agent
```

### Environment variables

Configure dtctl without interactive commands:

```bash
export DTCTL_ENVIRONMENT="https://abc12345.apps.dynatrace.com"
export DTCTL_TOKEN="dt0s16.XXXXXXXX.YYYYYYYY"
dtctl get workflows --agent
```

### Pipeline commands

Chain dtctl commands with standard Unix tools:

```bash
# Get all workflow IDs, then describe each one
dtctl get workflows -o json --agent | jq -r '.result[].id' | xargs -I{} dtctl describe workflow {} --agent

# Export query results for processing
dtctl query 'fetch logs | filter status == "ERROR" | limit 10' -o json --agent | jq '.result'
```
