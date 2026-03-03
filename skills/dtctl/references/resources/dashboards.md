# Dashboards Resource

## List
```bash
dtctl get dashboards -o json --plain
```

## Metadata Schema (from `get`)
```json
{
  "id": "string",
  "name": "string (NOT title)",
  "type": "dashboard",
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
dtctl describe dashboard <id> --plain
dtctl get dashboards -o json --plain | jq '.[] | select(.name | test("keyword"; "i")) | "\(.id) | \(.name)"'
dtctl share dashboard <id> --user <email> --access read-write --plain
dtctl unshare dashboard <id> --all --plain

# Export existing dashboard as YAML template
dtctl get dashboard <id> -o yaml --plain > dashboard.yaml

# Create new (omit id field) or update existing (include id field)
dtctl apply -f dashboard.yaml --plain

# Preview without applying
dtctl apply -f dashboard.yaml --dry-run --plain

# Delete
dtctl delete dashboard <id> --plain
```

## Creating Dashboards

Write a YAML file and apply with `dtctl apply -f dashboard.yaml --plain`.

### Full YAML structure
```yaml
name: "Dashboard Name"
type: dashboard
content:
  annotations: []
  importedWithCode: false
  settings:
    defaultTimeframe:
      enabled: true
      value:
        from: now()-2h
        to: now()
  layouts:
    "1":                    # string key, must match a key in tiles
      x: 0                 # column position (0-23)
      "y": 0               # MUST quote "y" — YAML parses bare y as boolean
      w: 12                # width in grid columns
      h: 6                 # height in grid rows
  tiles:
    "1":
      title: "Tile Title"
      type: data            # "data" for DQL tiles, "markdown" for text
      query: |
        fetch logs | limit 10
      visualization: lineChart
      visualizationSettings:
        autoSelectVisualization: false
      davis:
        enabled: false
        davisVisualization:
          isAvailable: true
```

### Layout grid
- **24 columns** wide. Common widths: full=24, half=12, third=8, quarter=6.
- Tiles must not overlap. Rows extend vertically as needed.
- Tile height `h` in grid units (1 unit ≈ 40px). Typical: KPIs h:3, charts h:6, tables h:6.

### Tile types

**`type: data`** — DQL-powered tile. Requires `query` and `visualization`.

**`type: markdown`** — Static text/section headers. Uses `content` field:
```yaml
"1":
  title: ""
  type: markdown
  content: "## Section Title\nDescription text"
```

### Visualizations

| Value | Use |
|---|---|
| `singleValue` | KPI cards |
| `lineChart` | Time series trends |
| `areaChart` | Stacked time series |
| `barChart` | Categorical comparisons |
| `pieChart` | Proportional breakdowns |
| `table` | Multi-column data |
| `honeycomb` | Entity health grids |
| `scatterplot` | Two-variable correlation |

### visualizationSettings by type

**singleValue** — `recordField` must match the alias in the query:
```yaml
visualization: singleValue
visualizationSettings:
  autoSelectVisualization: false
  singleValue:
    showLabel: true
    label: "Errors"
    recordField: errors
```

**lineChart / areaChart / barChart**:
```yaml
visualization: areaChart
visualizationSettings:
  autoSelectVisualization: false
  chartSettings:
    legend:
      position: bottom      # top | bottom | right
      showLegend: true
    stacked: true            # for stacked area/bar
  axes:
    yAxis:
      label: "Count"
```

**table**:
```yaml
visualization: table
visualizationSettings:
  autoSelectVisualization: false
  table:
    linewrapEnabled: true
    columnWidths:
      '["content"]': 500    # key format: '["field_name"]'
    columnOrder:
      - '["field1"]'
      - '["field2"]'
```

### Thresholds (color rules)

Apply to tables, single values, or chart baselines. Comparators: `<=`, `>=`, `=`, `<`, `>`.
```yaml
visualizationSettings:
  thresholds:
    - field: errors
      id: 1
      isEnabled: true
      rules:
        - { color: { Default: "var(--dt-colors-charts-status-ideal-default, #2f6862)" }, comparator: "<=", id: 0, value: 100 }
        - { color: { Default: "var(--dt-colors-charts-status-warning-default, #eea53c)" }, comparator: "<=", id: 1, value: 500 }
        - { color: { Default: "var(--dt-colors-charts-status-critical-default, #c62239)" }, comparator: ">", id: 2, value: 500 }
```

Theme colors: green `#2f6862`, yellow `#eea53c`, red `#c62239`.

### Unit overrides (for duration fields)
```yaml
visualizationSettings:
  unitsOverrides:
    - identifier: p99        # must match query alias
      baseUnit: nanosecond   # nanosecond | millisecond | second
      displayUnit: null      # null = auto-format
      decimals: 1
```

## Choosing Visualizations

Pick the visualization based on data shape, not aesthetics:

| Data shape | Use | Avoid |
|---|---|---|
| Single aggregated number | `singleValue` | |
| Time series (makeTimeseries) | `lineChart` or `areaChart` (stacked) | `barChart` (hard to read over time) |
| Categorical proportions (few groups) | `pieChart` | `barChart` (renders poorly with string categories) |
| Categorical comparison (many groups) | `table` | `barChart` (axis labels overlap) |
| Multi-field rows (latencies, rates) | `table` | |
| Entity status grid | `honeycomb` | |

**Key rule**: `barChart` expects a numeric or time x-axis. If your x-axis is string categories (namespace, container name, service name), use `pieChart` for proportions or `table` for precise values.

## Testing Queries Before Deploying

**Always validate tile queries with `dtctl query` before embedding in a dashboard.** A broken query shows as an empty or errored tile with no useful feedback.

```bash
# Test the exact query you plan to use in a tile
dtctl query 'fetch logs, from:now()-30m | filter loglevel == "ERROR" | summarize cnt = count()' -o json --plain

# Verify time series output has the expected shape
dtctl query 'fetch logs, from:now()-30m | makeTimeseries cnt = count(), interval:2m, by:{k8s.container.name}' -o json --plain
```

Check that:
- The query returns records (not an empty `[]`)
- Field names/aliases match what `recordField`, `identifier`, or `field` reference in visualizationSettings
- Time series queries use `makeTimeseries` (not `summarize`) for charts

## DQL Patterns for Dashboard Tiles

### Named parameters are required for optional args

Many DQL functions require named parameters. Positional args cause `TOO_MANY_POSITIONAL_PARAMETERS_WITH_OPTIONS` errors.

```dql
-- WRONG: positional params
fieldsAdd short = substring(content, 0, 100)
fieldsAdd result = if(x > 10, "high", "low")

-- CORRECT: named params
fieldsAdd short = substring(content, from:0, to:100)
fieldsAdd result = if(x > 10, then:"high", else:"low")
```

### Use HTTP status codes for error rates on HTTP services

Span `status == "ERROR"` captures application-level errors (exceptions, gRPC failures) but is **not reliably populated for HTTP services** — it can return 0 errors even when services return thousands of 5xx responses. For HTTP services, always use `http.response.status_code`:

```dql
-- For HTTP services: use status codes (reliable)
fetch spans | summarize errors_5xx = countIf(http.response.status_code >= 500), total = count()
             | fieldsAdd error_rate = 100.0 * errors_5xx / total

-- For non-HTTP/gRPC services: span status may work
fetch spans | summarize errors = countIf(status == "ERROR")

-- Best coverage: combine both signals
fetch spans | summarize errors = countIf(status == "ERROR" or http.response.status_code >= 500), total = count()
             | fieldsAdd error_rate = 100.0 * errors / total
```

### countIf() inside makeTimeseries for error rate trends

```dql
fetch spans, from:now()-30m
| filter dt.entity.service == "SERVICE-xxx"
| makeTimeseries cnt_5xx = countIf(http.response.status_code >= 500), cnt_total = count(), interval:2m
```

### Smartscape IDs are not strings

`startsWith()` / `endsWith()` fail on smartscape ID fields with a `DATATYPE_MISMATCH` warning and return empty results. This is despite CoPilot claiming they work — tested and confirmed broken. Use `matchesValue()` with `toString()`:

```dql
-- WRONG: returns warning "should be a string, but was a smartscape id" + empty results
smartscapeEdges "*" | filter startsWith(source_id, "SERVICE-")

-- CORRECT: cast to string first, then use pattern matching
smartscapeEdges "*" | filter matchesValue(toString(source_id), "SERVICE-*")
```

## Thresholds vs Coloring Rules

The YAML `thresholds` format works on create/apply, but the UI may rewrite it to `coloring.colorRules` when you export. Both formats are valid:

```yaml
# Format 1: thresholds (recommended for authoring)
visualizationSettings:
  thresholds:
    - field: error_rate
      id: 1
      isEnabled: true
      rules:
        - { color: { Default: "#2f6862" }, comparator: "<=", id: 0, value: 1 }
        - { color: { Default: "#eea53c" }, comparator: "<=", id: 1, value: 5 }
        - { color: { Default: "#c62239" }, comparator: ">", id: 2, value: 5 }

# Format 2: coloring.colorRules (UI-exported format)
visualizationSettings:
  coloring:
    colorRules:
      - colorMode: custom-color
        comparator: ">"
        customColor:
          Default: "#c62239"
        field: error_rate
        value: 5
```

If round-tripping (export → edit → apply), keep whichever format the export uses.

## Important Notes
- Field is `.name` NOT `.title` (opposite of workflows).
- **Always quote `"y"`** in layout YAML to prevent boolean parsing.
- Always set `davis: { enabled: false, davisVisualization: { isAvailable: true } }` on data tiles.
- Use `makeTimeseries` for log/span-based time series in tile queries; `timeseries` for metrics.
- The `version` field warning on create is benign — can be ignored.
- No `id` field in YAML → creates new dashboard; with `id` → updates existing.
- To learn from an existing dashboard: `dtctl get dashboard <id> -o yaml --plain`.
