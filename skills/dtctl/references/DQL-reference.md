# DQL Syntax Guide

## Cost-Optimized Patterns (Read First)

Grail bills per GiB **scanned** (uncompressed). A dashboard tile billed 10×/day × 365 makes every byte matter. Always apply these before writing a query — especially for dashboard/notebook tiles that auto-refresh.

### The five levers, in order of impact

1. **`timeseries` on metrics is free under DPS. Everything else is billed.** Prefer `timeseries` whenever the data already exists as a metric. Only fall back to `fetch logs | makeTimeseries` when the signal lives in log content.
2. **Always set an inline `from:` on `fetch`.** A `fetch logs` with no `from:` defaults to full Grail retention and can scan terabytes. Dashboard tiles should default to `from:now()-2h`.
3. **Filter on indexed fields first, before any transform.** Filters pushed down on `dt.system.bucket`, `dt.entity.*`, `k8s.namespace.name`, `loglevel`, `status` prune whole storage blocks. Filters using `lower()`, `upper()`, arithmetic, or on fields produced by `fieldsAdd`/`parse` do **not** push down.
4. **Cap with `scanLimitGBytes:` and `samplingRatio:`.** Hard ceiling + linear scan reduction. For sampled aggregates, multiply the result back (`value = value[] * samplingRatio`).
5. **Column-prune with `fieldsKeep` / `fieldsRemove`** after early filters, before transforms. Reduces bytes read per row.

### Good vs bad

**Bad — full-retention scan, transform defeats pushdown, sort before filter:**
```dql
fetch logs
| sort timestamp desc
| filter lower(k8s.namespace.name) == "payments"
| filter matchesValue(content, "*error*")
| summarize c = count(), by:{k8s.pod.name}
```

**Good — bounded, pushdown-friendly, column-pruned, sampled:**
```dql
fetch logs, from:now()-1h, scanLimitGBytes:50, samplingRatio:100
| filter dt.system.bucket == "default_logs"
| filter k8s.namespace.name == "payments"
| filter matchesPhrase(content, "error")
| fieldsKeep timestamp, content, k8s.pod.name
| makeTimeseries c = count(), by:{k8s.pod.name}, from:now()-1h
```

**Best — skip the log scan entirely by using a metric:**
```dql
timeseries c = sum(log.payments.errors), by:{k8s.pod.name}, from:now()-1h
```

### Canonical pipeline order

```
fetch <source>, from:<relative>, scanLimitGBytes:<n>, samplingRatio:<n>
| filter <indexed field equality / in() / matchesPhrase>    -- early, pushdown-friendly
| fieldsKeep <only fields you need>                          -- bytes-per-row reduction
| parse / fieldsAdd                                          -- transforms last
| summarize / makeTimeseries
| sort                                                       -- NEVER right after fetch
| limit                                                      -- after summarize, not before
```

### Anti-patterns to avoid

- `fetch logs` / `fetch events` / `fetch spans` / `fetch bizevents` **with no `from:`** — defaults to full retention.
- `filter lower(x) == "..."` — transform breaks index pushdown. Use case-sensitive equality, or normalize at ingest. **Measured ~9× scan amplification on production logs (46.6 GiB vs 5.3 GiB over the same 1h window).**
- `sort timestamp desc` immediately after `fetch` before any filter — forces full scan to sort.
- `limit N` **before** `summarize` — semantically truncates the aggregation to a partial sample. Note: on live Grail data, this pattern actually scans **less** than `summarize | limit N` because Grail short-circuits the scan when `limit` is early. Choose ordering based on intent, not scan cost.
- `matchesValue(content, "*foo*")` with leading wildcard — no token-index shortcut. Prefer `matchesPhrase(content, "foo")`.
- Negation filters (`!=`, `not`) — inclusive filters are faster. Rewrite as positive matches when possible.
- `fetch logs | makeTimeseries ...` when a metric already covers the signal. Use `timeseries` instead.
- Dashboard tiles without `scanLimitGBytes:` — a single misconfigured tile can dominate a tenant's DPS bill.
- Timeframes > 24h on `fetch logs` without `samplingRatio:` — sample first, then optionally widen.

See [Dynatrace DQL best practices](https://docs.dynatrace.com/docs/platform/grail/dynatrace-query-language/dql-best-practices), [Optimize dashboards running log queries](https://docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-analysis/lma-log-query-dashboard), and [DPS log query consumption](https://docs.dynatrace.com/docs/license/capabilities/log-management/dps-log-query).

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
