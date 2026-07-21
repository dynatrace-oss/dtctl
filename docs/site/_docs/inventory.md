---
layout: docs
title: Environment Inventory
---

`dtctl inventory` probes the current context's environment and reports what data actually exists there. Where [`dtctl commands`]({{ '/docs/ai-agent-mode/#command-catalog' | relative_url }}) answers *"what can I run?"*, `dtctl inventory` answers *"what is there to query?"* — which makes it the natural first call before exploratory DQL, for humans and AI agents alike.

Discovery is **read-only and budgeted**: it runs a small battery of DQL queries (4 by default) and stops with a partial inventory rather than overrunning its budget. Nothing is persisted.

```bash
# The environment inventory for the current context
dtctl inventory

# Structured output (full lists, no compaction)
dtctl inventory -o json
dtctl inventory -o yaml
```

## What it reports

- **Data objects** — which Grail catalog objects are fetchable, and which are query-only (`metrics`, `smartscape.*`) so you aren't baited into `fetch` calls that cannot work. The hundreds of legacy `dt.entity.*` lookback views are collapsed to an `entityViews` count.
- **Buckets** and **filter segments** — segments can be applied to queries with `-S <name>`.
- **Live entity-type census** — entity type → count via `smartscapeNodes`, the current-state topology (not the `dt.entity.*` lookback views, which diverge from it).
- **Capabilities** — spans, logs, RUM, k8s, cloud integrations, metric families, and anything you define yourself: each reported as **present**, **absent**, or **unknown**.

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

## Verdicts carry evidence

Every **absent** capability cites exactly what was checked (`no K8S_* entities in the live census`, `user.events is in the catalog, but all its buckets are empty`), so a negative finding is citable without anyone re-deriving it with fresh probes.

A capability whose check could not run — a failed probe, an exhausted budget, an unavailable or truncated fact source — is reported as **unknown** with the reason, never as absent. Unknown must not be read as absent: the capability may still exist.

Stream capabilities get liveness checking for free: bucket statistics (already fetched for the bucket list) reveal when a catalog object holds zero records within retention, so a tenant that never ingested RUM reports `rum` as absent even though `user.events` sits in its catalog.

## Customizing the capability set

dtctl ships a built-in, structural-only capability set (topology, signal streams, metric families). Your own definitions merge over it:

```bash
# Merge org-specific definitions over the built-in set (repeatable, later files win)
dtctl inventory --definitions ./our-capabilities.yaml

# Only your definitions, without the built-in set
dtctl inventory --definitions ./our-capabilities.yaml --no-builtin-definitions
```

Each capability is defined by *how* it is discovered — exactly one of four fixed shapes, deliberately not an expression language:

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

Globs are case-sensitive: Smartscape entity types are UPPERCASE (`K8S_*`, `AZURE_*`), so a lowercase pattern silently never matches. Probe shapes must declare `window` — the evidence window the probe covers — because their negatives are weak: absence of events in a window is not absence of the capability.

See [`docs/dev/examples/inventory-definitions.example.yaml`](https://github.com/dynatrace-oss/dtctl/blob/main/docs/dev/examples/inventory-definitions.example.yaml) for the annotated format.

## Budgets and cost

Discovery is bounded on three axes; it stops with a partial inventory (and says so) rather than overrunning:

```bash
dtctl inventory --budget-queries 100     # max queries (default 100)
dtctl inventory --budget-seconds 300     # max cumulative query seconds (default 300)
dtctl inventory --scan-limit-gbytes 25   # scan cap applied to every probe (default 25)
```

The default battery is 4 queries: data-object catalog, buckets, entity census, and the metric catalog (only when a `metricKey` definition needs it). Probe-shaped definitions cost one query each. A consumption receipt (`discovery: {queries, seconds}`) is attached to every inventory.

## For AI agents

In [agent mode]({{ '/docs/ai-agent-mode/' | relative_url }}) (auto-detected), the inventory arrives in the structured JSON envelope with suggestions attached — sample a listed data object, cite absence evidence instead of re-probing. Absent and unknown capabilities are structured `{name, evidence}` pairs:

```json
"absent": [
  {"name": "rum", "evidence": "user.events is in the catalog, but all its buckets are empty (0 records within retention)"}
]
```

This is about the data in the environment, not the resources you manage — for dashboards, workflows, SLOs, and the rest, use `dtctl get <resource>`.
