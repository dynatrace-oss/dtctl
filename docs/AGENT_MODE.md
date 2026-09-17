<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# AI Agent Mode

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

Error codes are stable identifiers that agents can match on programmatically:

| Code | Meaning | What to do |
|---|---|---|
| `auth_required` | Not authenticated (HTTP 401) | Authenticate, then retry once |
| `permission_denied` | Authenticated but not allowed (HTTP 403) | Don't retry; report the missing permission |
| `insufficient_scope` | Token lacks the scopes this command needs | Re-create the token with the `missing` scopes in the envelope |
| `not_found` | Resource does not exist (HTTP 404) | Verify the ID with `dtctl get <resource>` |
| `conflict` | Concurrent or duplicate change (HTTP 409) | Re-read the resource, re-apply on top |
| `bad_request` | The API rejected the request shape (HTTP 400) | Fix the payload; don't retry unchanged |
| `rate_limited` | Too many requests (HTTP 429) | Back off, then retry |
| `server_error` | Dynatrace-side failure (HTTP 5xx) | Retry with backoff; escalate if persistent |
| `timeout` | The operation timed out client-side | Narrow the request (timeframe, limit) and retry |
| `safety_blocked` | The context's [safety level](CONFIGURATION.md#safety-levels) forbids this operation | Don't retry; ask a human to widen the level |
| `profile_blocked` | The active [command profile](CONFIGURATION.md#command-profiles) doesn't expose this command | Re-read `dtctl commands` and pick a supported path |
| `unsupported_in_service` | Host-only command, unavailable in server mode | Don't retry; the suggestion says why |
| `capability_disabled` | A host ability (plugin, alias, hook, editor, browser) isn't granted | Don't retry; use an in-process alternative |
| `hook_rejected` | A pre-apply hook rejected the resource | Fix the resource, or apply with `--no-hooks` |
| `validation_error` | Local input validation failed | Fix the file or flags |
| `unknown_command` | Unknown command or flag | Follow the "did you mean" suggestion; re-read `dtctl commands` |
| `context_error` | No active context, or the named context is missing | Select a context (`dtctl ctx <name>`) |
| `config_error` | The dtctl config could not be read or is invalid | Report it; needs human repair |
| `spill_file_not_found` | The spilled result file is gone | `dtctl inspect --list`, or re-run the query |
| `spill_file_unreadable` | The spill file exists but cannot be parsed | Re-run the query |
| `spill_file_wrong_context` | The spill file belongs to another context or tenant | Switch context, or re-query here |
| `inspect_unknown_field` | `--fields` named a column the file doesn't have | Use `dtctl inspect <path> --schema` |
| `inspect_bad_flags` | Incompatible `dtctl inspect` flags | Pick one row-access primitive per call |
| `error` | Unclassified failure | Read `message`; treat as non-retryable |

`dtctl query` additionally passes the DQL API's own error type through as the code
(lowercased), e.g. `unknown_data_object`. **Treat an unrecognised code as
`error`** -- read `message` and `suggestions` instead of branching on it.

### Query results: the `result.kind` discriminator

In agent mode, `dtctl query` results are self-describing: the `result` payload
carries a `kind` field so a consumer always branches on one discriminator,
regardless of how big the result was. There are three kinds:

| `result.kind` | When | Payload |
|---|---|---|
| `records` | small result, returned inline | the rows under `result.records` |
| `result-file` | large result spilled to a file (see [Result Spill](CONFIGURATION.md#result-spill)) | a manifest: `path`, `format`, `rows`, `bytes`, column stats, `sample_rows` |
| `summary-only` | large result but the rows could not be written to disk | the same manifest **minus `path`** |

On a `result-file` result the rows are on disk, so read them with `dtctl inspect
<path>` (see [COMMANDS.md](COMMANDS.md)) -- `--head`/`--tail`/`--page`/
`--fields` for bounded row access, `--jq '<program>'` to keep only the matching
rows (a streaming filter over the whole file; a large match set re-spills via the
same `--spill*` guard), `--schema`/`--stats` to re-derive the profile, `--list`
to recover a path that has aged out of context -- instead of re-querying Grail.
On a `summary-only` result the rows are not on disk, so `context.suggestions`
carries the right next step for *why* the spill degraded: a read-only filesystem
steers you to re-query with `--spill=never` and a bound (`| fields …` /
`| limit N`, or `--max-result-records N`) so the inline result stays small, while
a one-off write failure suggests retrying with an explicit `--spill-to <path>`.

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
MUST treat an unrecognised `result.kind` as opaque** -- don't parse `result`, fall
back to the human-readable `context` (which always carries `decided`, `total`,
`warnings`, and `suggestions`). When Grail sampled the result, the per-column
stats move into a `sample_stats` block (each column tagged `basis: "sample"`) so
sample-based figures can't be misread as population truth.

> The inline `kind: "records"` envelope is emitted on the spill-aware path
> whenever agent mode emits JSON -- including under `--spill=never`, which forces
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
# Minimal overview -- verbs, resources, and subcommands only (defaults to TOON)
dtctl commands

# Brief catalog -- adds mutating status, access levels, flag types, and scopes
dtctl commands --brief -o json

# Full catalog -- detailed command descriptions, flag defaults, and global flags
dtctl commands --full -o json

# Human-readable how-to guide in Markdown
dtctl commands howto
```

The bare `dtctl commands` overview is ideal for including in an agent's system prompt or initial context, giving it a complete map of available operations without consuming excessive tokens; step up to `--brief` or `--full` when more detail is needed.

## Environment Inventory

`dtctl inventory` probes the current context's environment and reports what data
actually exists there. Where `dtctl commands` (above) answers *"what can I
run?"*, `dtctl inventory` answers *"what is there to query?"* -- which makes it
the natural first call before exploratory DQL, for humans and AI agents alike.

Discovery is **read-only and budgeted**: it runs a small battery of DQL queries (4 by default) and stops with a partial inventory rather than overrunning its budget. Nothing is persisted.

```bash
# The environment inventory for the current context
dtctl inventory

# Structured output (full lists, no compaction)
dtctl inventory -o json
dtctl inventory -o yaml
```

### What it reports

- **Data objects** -- which Grail catalog objects are fetchable, and which are query-only (`metrics`, `smartscape.*`) so you aren't baited into `fetch` calls that cannot work. The hundreds of legacy `dt.entity.*` lookback views are collapsed to an `entityViews` count.
- **Buckets** and **filter segments** -- segments can be applied to queries with `-S <name>`.
- **Live entity-type census** -- entity type → count via `smartscapeNodes`, the current-state topology (not the `dt.entity.*` lookback views, which diverge from it).
- **Capabilities** -- spans, logs, RUM, k8s, cloud integrations, metric families, and anything you define yourself: each reported as **present**, **absent**, or **unknown**.

Example (synthetic):

```
Context:      example
Generated:    2026-01-02T03:04:05Z
Capabilities: hosts, logs, spans
Absent (what was checked)
  rum — user.events is in the catalog, but all its buckets are empty (0 records within retention)
Unknown (no verdict — not evidence of absence)
  genai — probe failed: scan limit exceeded
Entities:     K8S_POD:200 SERVICE:40 HOST:12
Data objects: logs, spans (+2 dt.entity.* lookback views)
Query-only:   metrics (no fetch — see notes)
Buckets:      default_logs, default_spans
Segments (apply with -S <name>)
  prod — production workloads
```

### Verdicts carry evidence

Every **absent** capability cites exactly what was checked (`no K8S_* entities in the live census`, `user.events is in the catalog, but all its buckets are empty`), so a negative finding is citable without anyone re-deriving it with fresh probes.

A capability whose check could not run -- a failed probe, an exhausted budget, an unavailable or truncated fact source -- is reported as **unknown** with the reason, never as absent. Unknown must not be read as absent: the capability may still exist.

Stream capabilities get liveness checking for free: bucket statistics (already fetched for the bucket list) reveal when a catalog object holds zero records within retention, so a tenant that never ingested RUM reports `rum` as absent even though `user.events` sits in its catalog.

### Customizing the capability set

dtctl ships a built-in, structural-only capability set (topology, signal streams, metric families). Your own definitions merge over it:

```bash
# Merge org-specific definitions over the built-in set (repeatable, later files win)
dtctl inventory --definitions ./our-capabilities.yaml

# Only your definitions, without the built-in set
dtctl inventory --definitions ./our-capabilities.yaml --no-builtin-definitions
```

Each capability is defined by *how* it is discovered -- exactly one of four fixed shapes, deliberately not an expression language:

| Shape | Present when | Evidence strength | Extra cost |
|-------|--------------|-------------------|------------|
| `dataObject` | the object is in the catalog and its buckets hold data | strong | none |
| `entityTypes` (globs) | a matching type has live entities in the census | strong | none |
| `metricKey` (glob) | a matching key is in the live metric catalog | strong | none (shared query) |
| `probe` + `window` | the DQL probe returns at least one row | **weak negative** | 1 query per run |

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: InventoryDefinitions
capabilities:
  # Managed postgres appears under hyperscaler entity types, not the generic one
  postgres:
    entityTypes: [DB_INSTANCE_POSTGRES, "*_DBFORPOSTGRESQL_*"]

  # Only visible in span attributes: probe with a capped, sampled scan
  genai:
    probe: 'fetch spans, from:now()-24h, samplingRatio:100 | filter isNotNull(gen_ai.system) | limit 1'
    window: 24h

  # Remove a built-in capability from the merged set
  aws-cloudwatch: null
```

Globs are case-sensitive: Smartscape entity types are UPPERCASE (`K8S_*`, `AZURE_*`), so a lowercase pattern silently never matches. Probe shapes must declare `window` -- the evidence window the probe covers -- because their negatives are weak: absence of events in a window is not absence of the capability.

See [docs/dev/examples/inventory-definitions.example.yaml](dev/examples/inventory-definitions.example.yaml) for the annotated format.

### Budgets and cost

Discovery is bounded on three axes; it stops with a partial inventory (and says so) rather than overrunning:

```bash
dtctl inventory --budget-queries 100     # max queries (default 100)
dtctl inventory --budget-seconds 300     # max cumulative query seconds (default 300)
dtctl inventory --scan-limit-gbytes 25   # scan cap applied to every probe (default 25)
```

The default battery is 4 queries: data-object catalog, buckets, entity census, and the metric catalog (only when a `metricKey` definition needs it). Probe-shaped definitions cost one query each. A consumption receipt (`discovery: {queries, seconds}`) is attached to every inventory.

### For AI agents

In agent mode (auto-detected, see [Auto-Detection](#auto-detection) above), the inventory arrives in the structured JSON envelope with suggestions attached -- sample a listed data object, cite absence evidence instead of re-probing. Absent and unknown capabilities are structured `{name, evidence}` pairs:

```json
"absent": [
  {"name": "rum", "evidence": "user.events is in the catalog, but all its buckets are empty (0 records within retention)"}
]
```

This is about the data in the environment, not the resources you manage -- for dashboards, workflows, SLOs, and the rest, use `dtctl get <resource>`.

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

## Implementation notes (for contributors)

- Agent envelope: `pkg/output/agent.go` (`AgentPrinter`, `Response`, `PrintError`)
- Per-command context enrichment: the `enrichAgent()` helper in `cmd/root.go`
- Environment auto-detection: `sdk/agentmode/`, wired into `pkg/aidetect/detect.go`
- Environment inventory discovery: `sdk/inventory/`

When adding support for a new AI agent environment variable, update **all** of:
code (`pkg/aidetect/detect.go`, `pkg/skills/installer.go`, `cmd/skills.go`),
tests (`pkg/aidetect/detect_test.go`, `pkg/skills/installer_test.go`,
`cmd/skills_test.go`), and docs (`README.md`, this file). See `AGENTS.md` in the
repo root for the full contributor checklist.
