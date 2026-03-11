# Notebooks Resource

## List
```bash
dtctl get notebooks -o json --plain
```

## Metadata Schema (from `get`)
```json
{
  "id": "string (UUID)",
  "name": "string",
  "type": "notebook",
  "owner": "string (user UUID)",
  "isPrivate": boolean,
  "version": number,
  "modificationInfo": {
    "createdBy": "string",
    "createdTime": "ISO 8601",
    "lastModifiedBy": "string",
    "lastModifiedTime": "ISO 8601"
  }
}
```

## Operations
```bash
dtctl describe notebook <id> --plain
dtctl get notebooks -o json --plain | jq '.[] | select(.name | test("keyword"; "i")) | "\(.id) | \(.name)"'

# Export existing notebook as YAML template
dtctl get notebook <id> -o yaml --plain > notebook.yaml

# Create new (omit id field) or update existing (include id field)
dtctl apply -f notebook.yaml --plain

# Delete
dtctl delete notebook <id> --plain
```

## Creating Notebooks

Write a YAML file and apply with `dtctl apply -f notebook.yaml --plain`.

### Full YAML structure

**Use the `sections` format.** dtctl recognizes `content.sections[]` and reports section count on create. A `content.cells[]` format exists in the API but dtctl warns "notebook content has no 'sections' field" and may produce empty notebooks.

```yaml
name: "Notebook Name"
type: notebook
content:
  defaultSegments: []
  defaultTimeframe:
    from: now()-2h
    to: now()
  sections:
    - id: header-001
      markdown: |-
        # Notebook Title
        Description text here.
      type: markdown
    - id: query-001
      showTitle: true
      drilldownPath: []
      filterSegments: []
      previousFilterSegments: []
      state:
        davis:
          davisVisualization:
            isAvailable: true
          includeLogs: true
        input:
          timeframe:
            from: now()-2h
            to: now()
          value: |-
            fetch logs, from:now()-2h
            | filter loglevel == "ERROR"
            | summarize error_count = count(), by:{k8s.namespace.name}
            | sort error_count desc
            | limit 10
        querySettings:
          defaultSamplingRatio: 10
          defaultScanLimitGbytes: 500
          enableSampling: false
          maxResultMegaBytes: 1
          maxResultRecords: 1000
```

### Section types

**Markdown section** — for headers, notes, and narrative text:
```yaml
- id: section-01
  markdown: |-
    ## Section Header
    Explanation of findings.
  type: markdown
```

**DQL query section** — executes a query with optional visualization:
```yaml
- id: query-01
  showTitle: true
  drilldownPath: []
  filterSegments: []
  previousFilterSegments: []
  state:
    davis:
      davisVisualization:
        isAvailable: true
      includeLogs: true
    input:
      timeframe:
        from: now()-30m
        to: now()
      value: |-
        fetch logs, from:now()-30m
        | filter loglevel == "ERROR"
        | makeTimeseries cnt = count(), interval:2m, by:{k8s.container.name}
    querySettings:
      defaultSamplingRatio: 10
      defaultScanLimitGbytes: 500
      enableSampling: false
      maxResultMegaBytes: 1
      maxResultRecords: 1000
    visualization:
      type: areaChart
      chartSettings:
        legend:
          position: bottom
          showLegend: true
```

### Visualization settings

Visualization goes under `state.visualization` (NOT `visualizationSettings` like dashboards).

```yaml
# Table (default if no visualization specified)
state:
  visualization:
    type: table

# Area chart (stacked time series)
state:
  visualization:
    type: areaChart
    chartSettings:
      legend: { position: bottom, showLegend: true }
      stacked: true

# Line chart
state:
  visualization:
    type: lineChart
    chartSettings:
      legend: { position: bottom, showLegend: true }

# Single value
state:
  visualization:
    type: singleValue
    singleValue:
      showLabel: true
      label: "Error Count"
      recordField: errors

# Pie chart
state:
  visualization:
    type: pieChart
```

Available types: `table`, `lineChart`, `areaChart`, `barChart`, `pieChart`, `singleValue`, `honeycomb`, `scatterplot`.

## Notebooks vs Dashboards

Use the right tool for the job:

| Aspect | Notebook | Dashboard |
|--------|----------|-----------|
| **Layout** | Linear, top-to-bottom | Grid-based (24 columns) |
| **Purpose** | Investigation, analysis, runbooks | Monitoring, at-a-glance status |
| **DQL location** | `state.input.value` | `query` |
| **Viz config location** | `state.visualization` | `visualizationSettings` |
| **Davis config** | `state.davis` (per section) | `davis` (per tile) |
| **Query settings** | `state.querySettings` (per section) | `querySettings` (per tile, auto-added) |
| **Timeframe** | `content.defaultTimeframe` + per-section `state.input.timeframe` | `content.settings.defaultTimeframe` |
| **Layout config** | None needed (linear) | `content.layouts` with x/y/w/h |
| **Result caching** | Can store `state.result` | No result storage |

**When to use notebooks**: RCA investigations, runbook documentation, step-by-step analysis with markdown narrative between queries.

**When to use dashboards**: Operational monitoring, KPI displays, alerting overviews with multiple tiles visible simultaneously.

## DQL Patterns — Same Gotchas as Dashboards

All DQL issues that affect dashboards also affect notebooks. See [dashboards.md](dashboards.md) for full details. Key reminders:

- **Named params required**: `substring(str, from:0, to:100)`, `if(cond, then:val, else:val)`
- **HTTP 5xx for error rates**: `countIf(http.response.status_code >= 500)` is more reliable than `countIf(status == "ERROR")` for HTTP services
- **Smartscape IDs**: Use `matchesValue(toString(source_id), "SERVICE-*")`, not `startsWith(source_id, "SERVICE-")`
- **Time series**: Use `makeTimeseries` for logs/spans, `timeseries` for metrics
- **Test queries first**: Run with `dtctl query '...' -o json --plain` before embedding in a notebook

## Authoring Tips

1. **Alternate markdown and query sections** — explain what each query investigates before showing it
2. **Use unique section IDs** — any string works (`header-001`, `query-latency`, `note-findings`)
3. **Set per-section timeframes** — override `state.input.timeframe` for queries that need different windows
4. **Keep `querySettings` consistent** — copy the standard block to avoid unexpected sampling or limits
5. **Export before editing** — use `dtctl get notebook <id> -o yaml --plain` to see the current state, especially after manual UI edits add `state.result` blocks

## Important Notes
- Field is `.name` for notebooks (same as dashboards).
- No `id` field in YAML → creates new; with `id` → updates existing.
- The `version` field warning on create is benign.
- Deleted notebooks go to trash (can be restored).
- Sections are rendered top-to-bottom in the order they appear in the YAML array.
- The `state.result` block (cached query output) is optional — omit it when authoring; the UI populates it on execution.
