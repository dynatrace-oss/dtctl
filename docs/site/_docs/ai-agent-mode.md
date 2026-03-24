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
