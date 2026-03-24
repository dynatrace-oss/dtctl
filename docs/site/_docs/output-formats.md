---
layout: docs
title: Output Formats
---

dtctl supports multiple output formats to suit different workflows -- from human-readable tables for interactive use to structured JSON for scripting and AI agents.

## Table (Default)

The default output format is a compact, human-readable table:

```bash
dtctl get workflows
```

```
ID            NAME                     STATE    TRIGGER     LAST RUN
wf-abc123     Daily Health Check       enabled  Schedule    2025-01-15 08:00
wf-def456     Incident Remediation     enabled  Event       2025-01-15 12:34
wf-ghi789     Weekly Report            enabled  Schedule    2025-01-13 06:00
```

## JSON

Output as JSON for scripting and piping to tools like `jq`:

```bash
dtctl get workflow wf-123 -o json

# Pipe to jq for field extraction
dtctl get workflows -o json | jq '.[].name'
```

## YAML

Output as YAML, useful for round-tripping with `dtctl apply`:

```bash
dtctl get workflow wf-123 -o yaml
```

## Wide

The wide format adds additional columns that are hidden in the default table view:

```bash
dtctl get workflows -o wide
```

```
ID            NAME                     STATE    TRIGGER     OWNER            LAST RUN            LAST STATUS
wf-abc123     Daily Health Check       enabled  Schedule    user@example.com 2025-01-15 08:00    SUCCESS
wf-def456     Incident Remediation     enabled  Event       user@example.com 2025-01-15 12:34    FAILED
```

## Describe

The `describe` command renders a vertical key-value view with full detail by default:

```bash
dtctl describe workflow wf-123
```

```
ID:          wf-abc123
Name:        Daily Health Check
State:       enabled
Trigger:     Schedule (0 8 * * *)
Owner:       user@example.com
Created:     2025-01-01 10:00:00
Modified:    2025-01-14 15:30:00
Tasks:       3
```

All `describe` subcommands support the `-o` / `--output` flag to get structured output:

```bash
# JSON output for scripting
dtctl describe workflow wf-123 -o json

# YAML output for round-tripping
dtctl describe slo my-slo -o yaml

# Agent mode envelope
dtctl describe dashboard my-dash -A
```

## CSV

Export as CSV for spreadsheets and data pipelines:

```bash
# Export workflows to a CSV file
dtctl get workflows -o csv > workflows.csv

# Export DQL query results as CSV
dtctl query 'fetch logs | filter status == "ERROR" | limit 100' -o csv > errors.csv
```

## Plain Mode

The `--plain` flag disables colors, progress indicators, and interactive prompts. This is useful for piping output or running in non-interactive environments:

```bash
dtctl get workflows --plain
```

Color output follows the [no-color.org](https://no-color.org/) standard:

- `--plain` flag disables color
- `NO_COLOR` environment variable disables color
- Non-TTY output (piped) disables color automatically
- `FORCE_COLOR=1` overrides TTY detection to force color on

## Command Catalog

dtctl can describe its own commands in machine-readable form:

```bash
# Full command catalog as JSON
dtctl commands -o json

# Brief catalog (compact, ideal for AI agent bootstrap)
dtctl commands --brief -o json

# Human-readable how-to guide in Markdown
dtctl commands howto
```

## Agent Mode

The `--agent` (or `-A`) flag wraps all output in a structured JSON envelope designed for AI agent consumption:

```bash
dtctl get workflows --agent
```

```json
{
  "ok": true,
  "result": [...],
  "context": {
    "verb": "get",
    "resource": "workflow",
    "suggestions": [...]
  }
}
```

Agent mode is auto-detected when running inside AI agent environments (GitHub Copilot, Claude Code, Cursor, OpenCode, and others). To opt out of auto-detection:

```bash
dtctl get workflows --no-agent
```

Agent mode implies `--plain` -- no colors and no interactive prompts. See [AI Agent Mode](ai-agent-mode) for full details.

## Pagination

List commands support server-side pagination with the `--chunk-size` flag:

```bash
# Fetch in chunks of 200
dtctl get workflows --chunk-size 200

# Default chunk size is 500
dtctl get workflows

# Disable chunking (fetch all at once)
dtctl get workflows --chunk-size=0
```

All pages are fetched automatically and combined into a single result set.
