---
name: dtctl
description: Investigate incidents, debug performance issues, analyze logs, and manage observability resources in Dynatrace using the dtctl CLI. Use this skill whenever the user asks about error rates, latency spikes, service health, crash-looping pods, web vitals, SLO status, open problems, root cause analysis, log patterns, trace analysis, or building dashboards — even if they don't mention Dynatrace by name. Also covers DQL queries, workflow management, notebook and dashboard creation, settings configuration, and any operations against a Dynatrace environment.
---

# Dynatrace Control with dtctl

Operate `dtctl`, the kubectl-style CLI for Dynatrace. Pattern: `dtctl <verb> <resource> [flags]`.

## Initialization

Run once to establish context, permissions, and the command catalog:

```bash
dtctl commands --brief -o json          # all commands, flags, resources + aliases (capabilities)
dtctl config current-context            # active context
dtctl config describe-context $(dtctl config current-context) --plain  # env URL + safety level
dtctl auth status --plain               # token type (OAuth vs API/platform) + safety level
```

Safety levels: `readonly`, `readwrite-mine`, `readwrite-all`, `dangerously-unrestricted`.

Don't use `dtctl auth whoami` to test connectivity — it needs an OAuth token with `app-engine:apps:run` and returns a spurious 403 for plain API or read-scoped tokens even when reads work. Confirm with a real `get`/`query`.

## DQL (required reading)

Before writing, modifying, or running any DQL (`dtctl query`, `dtctl wait query`, query files), consult `references/DQL-reference.md` and follow it over any assumption or memory.

```bash
dtctl query "fetch logs | filter status='ERROR' | limit 100" -o json --plain
dtctl query -f query.dql --set host=h-123 --set timerange=2h -o json --plain   # Go-template vars
dtctl wait query "fetch spans | filter test_id='test-123'" --for=count=1 --timeout 5m
dtctl query "timeseries avg(dt.host.cpu.usage)" -o chart --plain
```

dtctl not installed/working? See [references/troubleshooting.md](references/troubleshooting.md).

## Resources & verbs

Resources and aliases are discoverable via `dtctl commands` (run at init). They include: analyzer, anomaly-detector, app, aws/azure/gcp connection & monitoring, bucket, copilot-skill, dashboard, document, edgeconnect, extension, extension-config, function, group, intent, lookup, notebook, notification, sdk-version, segment, settings, settings-schema, slo, slo-template, trash, user, workflow, workflow-execution. **Use IDs, not names** — names may be ambiguous and fail.

| Verb | Example |
|------|---------|
| get / describe | `dtctl get workflows --mine` · `dtctl describe workflow <id>` |
| apply / edit / delete | `dtctl apply -f wf.yaml --set env=prod` · `dtctl delete workflow <id>` |
| exec | `dtctl exec function <id> --payload '{...}'` · `dtctl exec analyzer <id> --input '{...}'` (also workflow, copilot) |
| query / wait | `dtctl query "fetch logs \| limit 10"` · `dtctl wait query ... --for=any` |
| logs / history / restore | `dtctl logs workflow-execution <id>` · `dtctl restore dashboard <id> --version 3` |
| share / unshare | `dtctl share dashboard <id> --user a@example.com` |
| find / open | `dtctl find intents --data trace.id=abc` · `dtctl open intent <app/intent> --data k=v` |
| diff / verify | `dtctl diff -f wf.yaml` · `dtctl verify query 'fetch logs' --fail-on-warn` |

## Output for agents

`--agent`/`-A` is auto-detected in AI environments (implies `--plain`; opt out with `--no-agent`). It wraps output in `{ok, result, context}` (errors: `{ok:false, error:{code,message}}`, where `context` carries `total`, `has_more`, `suggestions`).

```bash
-o toon          # token-efficient structured output — prefer for agents
-o json|yaml|csv # other machine formats
-o jsonl|parquet # streaming / columnar export for large results (pipe to a file, query with DuckDB)
-o chart|sparkline|barchart   # time series
-o table|wide    # human-readable (table is the default)
--jq '.[].id'    # filter structured output (json|yaml|toon; other formats auto-promote to json)
```

Prefer `--agent` plus `-o toon` and `--jq` to cut tokens.

### Query results: branch on `result.kind`

In agent mode `dtctl query` defaults to `--spill=auto`: large results spill to a local file and return a summary instead of dumping rows into context. Never assume `result` is an array — branch on `result.kind`:

| `result.kind` | Meaning → action |
|---|---|
| `records` | rows inline under `result.records` → use directly |
| `result-file` | spilled: manifest with `path`, `format`, `rows`, `bytes`, column stats, `sample_rows` → read/filter the file locally, **don't re-query** |
| `summary-only` | rows couldn't be written — manifest minus `path` → use stats/sample, or follow the cause-aware `context.suggestions` (`--spill=never` + a bound, or `--spill-to <path>`) |

Treat an unknown `kind` as opaque and fall back to `context` (`decided`, `total`, `warnings`, `suggestions`). Sampled results put stats in a `sample_stats` block (`basis: "sample"`) — not population truth. Interrogate spilled files with local tools (`jq`, DuckDB), not by re-running the query.

```bash
dtctl query "fetch logs | limit 1000000" --agent     # auto-spills if large
dtctl query "fetch logs" --spill=never               # force every row inline
dtctl query "fetch logs" --spill-to ./out.jsonl      # explicit path: jsonl|json|csv|parquet
dtctl query "fetch logs" --spill=auto --spill-threshold 100KB
```

## Apply & templates

`dtctl apply` is idempotent: POST when new, PUT when the file has an `id`. YAML/DQL files support Go templates filled via `--set`:

```yaml
title: "{{.environment}} Deployment"
cron: "{{.schedule | default "0 0 * * *"}}"
```
`dtctl apply -f file.yaml --set environment=prod --set schedule="0 6 * * *"`

## Dashboards

Create/update: `dtctl apply -f dashboard.yaml`. Export for reference: `dtctl get dashboard <id> -o yaml --plain`. Full schema + visualizationSettings: [references/resources/dashboards.md](references/resources/dashboards.md).

```yaml
name: "Dashboard Name"
type: dashboard
content:
  settings:
    defaultTimeframe: { enabled: true, value: { from: now()-2h, to: now() } }
  layouts:
    "1": { x: 0, "y": 0, w: 12, h: 6 }    # 24-col grid (full=24); quote "y" (YAML bool)
  tiles:
    "1":
      title: "Tile"
      type: data                          # data | markdown
      query: "fetch logs | limit 10"
      visualization: lineChart            # singleValue|lineChart|areaChart|barChart|pieChart|table|honeycomb|scatterplot
      davis: { enabled: false, davisVisualization: { isAvailable: true } }
```

Gotchas: set `davis.enabled: false` on data tiles; `makeTimeseries` for log/span series, `timeseries` for metrics; `id` present → update, absent → create; the `version` warning on create is benign.

## Permissions & safety

- Verify before mutating: `dtctl auth can-i <verb> <resource>`. Scopes: [TOKEN_SCOPES.md](https://github.com/dynatrace-oss/dtctl/blob/main/docs/TOKEN_SCOPES.md).
- Destructive ops may be blocked by safety level — switch with `dtctl config use-context <name>`, or raise the level when creating the context.
- Prefer `get`/`describe` first; `--mine` scopes to resources you own; `--plain` for all machine consumption.

## More

[troubleshooting](references/troubleshooting.md) · [multi-tenant config](references/config-management.md) · [DQL](references/DQL-reference.md) · [notebooks](references/resources/notebooks.md) · [extensions](references/resources/extensions.md) · `dtctl --help`, `dtctl <command> --help`
