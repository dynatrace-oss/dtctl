# DQL Syntax Guide

## Cost-Optimized DQL

For the authoritative DQL optimization guide (command order, filter pushdown, cardinality, timeframe sizing, anti-patterns), see [`dt-dql-essentials/references/optimization.md`](https://github.com/Dynatrace/dynatrace-for-ai/blob/main/skills/dt-dql-essentials/references/optimization.md) in the `dynatrace-for-ai` skills. Consult that guide **before writing DQL for dashboards, notebooks, or workflows**.

### dtctl-specific cost affordances

Two items not covered upstream:

- **Scan-cost knobs on `fetch`** — `scanLimitGBytes:<n>` caps the bytes the query will read (hard ceiling against runaway scans); `samplingRatio:<n>` linearly reduces scanned data on long-window log queries (multiply aggregates back by the ratio). Prefer both on billable `fetch logs/events/spans/bizevents` in tile queries.
- **Cost lint / rewrite via dtctl** — run `dtctl verify query '<dql>' --cost-lint` to flag cost anti-patterns on any query (use `--strict-cost` in CI to fail on findings; `--rewrite-cost` to print a safely rewritten form). The same flags work on `dtctl exec copilot nl2dql` and `dtctl apply` (which walks every tile query in a dashboard/notebook YAML).

One calibration worth knowing: `limit N | summarize` is **semantically** distinct from `summarize | limit N` (partial sample vs full aggregate), not a cost anti-pattern — Grail short-circuits the scan when `limit` appears early. Choose ordering by intent.

## Copy These Templates Exactly

**Filter multiple values:**
```dql
filter in(loglevel, array("ERROR", "WARN", "SEVERE"))
```

**Aggregation with grouping:**
```dql
summarize cnt = count(), by:{loglevel}
```

**String length:**
```dql
fieldsAdd len = stringLength(content)
```

**Entity fields:**
```dql
fetch dt.entity.service
| fields id, entity.name
```

**Format timestamp:**
```dql
fieldsAdd ts = formatTimestamp(timestamp, format:"yyyy-MM-dd HH:mm:ss")
```

## Data Sources

```dql
fetch logs, from:now()-1h           -- Log records
fetch events                        -- System events
fetch bizevents                     -- Business events
fetch spans                         -- Trace spans
fetch dt.entity.service             -- Entities (service, host, etc.)
fetch security.events               -- Security/vulnerability events
smartscapeEdges "*"                 -- Entity relationships (calls, runs_on, etc.)
smartscapeNodes SERVICE             -- Entity graph nodes
timeseries avg(dt.host.cpu.usage)   -- Metrics (NOT fetch metrics)
```

## Essential Patterns

### Filter and select
```dql
fetch logs, from:now()-1h
| filter loglevel == "ERROR"
| fields timestamp, content, loglevel
| sort timestamp desc
| limit 100
```

### Aggregate with grouping (alias required for sort)
```dql
fetch logs, from:now()-2h
| summarize cnt = count(), by:{loglevel}
| sort cnt desc
```

### Multiple values
```dql
filter loglevel == "ERROR" or loglevel == "WARN" or loglevel == "SEVERE"
-- OR --
filter in(loglevel, array("ERROR", "WARN", "SEVERE"))
```

### Metrics (timeseries command, NOT fetch)
```dql
timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}, from:now()-6h, interval:5m
```

### Log time-series (makeTimeseries, NOT summarize)
```dql
fetch logs, from:now()-4h
| filter loglevel == "ERROR"
| makeTimeseries cnt = count(), interval:10m, by:{k8s.namespace.name}
```

### Entity search
```dql
fetch dt.entity.service
| filter contains(entity.name, "payment") or startsWith(entity.name, "api-")
| fields id, entity.name
```

### Array expansion (after expand, use brackets)
```dql
fetch spans
| filter isNotNull(span.events)
| expand span.events
| filter span.events[span_event.name] == "exception"
| fields span.events[exception.message], span.events[exception.type]
```
Note: After `expand arr`, access fields via `arr[field]` NOT `arr.field`

### String functions
```dql
filter contains(content, "timeout") or contains(content, "connection refused")
filter endsWith(log.source, ".log")
filter startsWith(name, "api-")
```

### Absolute timestamps
```dql
fetch events, from:"2025-01-01T00:00:00Z", to:"2025-01-02T00:00:00Z"
-- OR in filter --
filter timestamp >= toTimestamp("2025-01-01T00:00:00Z")
```

### Computed fields
```dql
fetch logs
| fieldsAdd msg_len = stringLength(content)
| fieldsAdd time_str = formatTimestamp(timestamp, format:"yyyy-MM-dd HH:mm:ss")
| fields timestamp, time_str, msg_len, content
```

### Business events aggregation
```dql
fetch bizevents, from:now()-1h
| summarize total = count(), sum_amt = sum(amount), avg_amt = avg(amount), by:{event.type}
```

### Field escaping (hyphens/special chars)
```dql
filter `error-code` == "404"
```

### Security vulnerabilities
```dql
fetch security.events
| filter event.type == "VULNERABILITY_STATE_REPORT_EVENT"
| filter vulnerability.resolution.status == "OPEN"
| sort vulnerability.risk.score desc
```

### Smartscape relationships
```dql
smartscapeEdges "*"
| filter type == "calls"
| fields source_id, target_id, type
| limit 100
```

## Key Functions

| Function | Usage |
|----------|-------|
| `count()` | `cnt = count()` |
| `sum(field)` | `total = sum(amount)` |
| `avg(field)` | `average = avg(duration)` |
| `contains(str, sub)` | `contains(content, "error")` |
| `startsWith(str, pre)` | `startsWith(name, "api-")` |
| `endsWith(str, suf)` | `endsWith(source, ".log")` |
| `lower(str)` | `lower(loglevel) == "error"` |
| `in(val, arr)` | `in(level, array("A","B"))` |
| `stringLength(str)` | `stringLength(content)` |
| `formatTimestamp(ts, format:f)` | `formatTimestamp(timestamp, format:"HH:mm")` |
| `toTimestamp(str)` | `toTimestamp("2025-01-01T00:00:00Z")` |
| `isNotNull(field)` | `isNotNull(span.events)` |
| `matchesValue(str, pattern)` | `matchesValue(name, "*payment*")` |
| `countIf(condition)` | `errors = countIf(loglevel == "ERROR")` |
| `countDistinct(field)` | `unique_hosts = countDistinct(dt.entity.host)` |
| `percentile(field, pct)` | `p95 = percentile(duration, 95)` |
