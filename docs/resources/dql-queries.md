<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# DQL Queries

<!-- NOTE: DQL is the `query`/`exec` verb, not a CRUD resource - author prose + examples; no per-resource ops to generate. -->

## Overview

dtctl provides a powerful interface for executing Dynatrace Query Language (DQL) queries directly from your terminal. Run ad-hoc queries inline, load them from files, use template variables, and stream live results. Large results can spill to a local file with a compact summary returned in their place, and `dtctl inspect` reads a spilled file's rows locally without re-querying Grail. `dtctl wait query` polls a query until a record-count condition is met, and `dtctl verify query` validates syntax without executing. See [Filter Segments](filter-segments) for applying segments at query time.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| query | — | `dtctl query` | Execute a DQL query against Grail | no | read |
| wait | query | `dtctl wait query` | Poll a DQL query with exponential backoff until a record-count condition is met | no | read |
| inspect | — | `dtctl inspect` | Inspect a spilled query-result file locally (rows, schema, stats) without re-querying Grail | no | read |
| verify | query | `dtctl verify query` | Verify DQL query syntax, optionally returning the canonical form | no | read |

## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). `query` additionally takes a large set of query-tuning flags:

| Flag | Purpose |
| --- | --- |
| `--default-timeframe-start` / `--default-timeframe-end` | Default query timeframe (ISO-8601/RFC3339) used when the query omits one |
| `--timezone` | Query timezone (e.g. `UTC`, `Europe/Paris`) |
| `--locale` | Query locale (e.g. `en_US`, `de_DE`) |
| `--default-sampling-ratio` | Sampling ratio for faster approximate results (normalized to a power of 10) |
| `--enable-preview` | Return preview results if available within the timeout |
| `--fetch-timeout-seconds` | Time limit for fetching data (seconds) |
| `--enforce-query-consumption-limit` | Enforce the tenant query consumption limit |
| `--no-progress` | Disable the live progress bar shown on stderr for long queries |
| `--max-result-records` | Maximum number of result records |
| `--max-result-bytes` | Maximum result payload size in bytes |
| `--default-scan-limit-gbytes` | Cap on how much data Grail scans (GB) |
| `--metadata`, `-M` | Include execution metadata (bare = all, or `=field1,field2`) |
| `--include-types` | Include DQL column type information |
| `--include-contributions` | Include Grail bucket contribution information |
| `--client-context` | Set the `dt-client-context` request header (caller intent) |
| `--width` / `--height` / `--fullscreen` | Terminal chart dimensions (chart output formats) |
| `--decode-snapshots` | Decode Live Debugger snapshot payloads (see [Live Debugger](../LIVE_DEBUGGER.md)) |
| `--spill*` | Spill large results to a file (see Output below) |
| `-S` / `--segment`, `-V` / `--segment-var`, `--segments-file` | Apply filter segments (see [Filter Segments](filter-segments)) |
| `--live` / `--interval` | Live mode with periodic refresh |
| `--set` | Set a template variable (`key=value`) |
| `-f` / `--file` | Read the query from a file (`-` for stdin) |

`wait query` accepts the standard query-tuning flags above, plus:

| Flag | Purpose |
| --- | --- |
| `--for` | Condition to wait for (required): `count=N`, `count-gte=N`, `count-gt=N`, `count-lte=N`, `count-lt=N`, `any`, `none` |
| `--timeout` | Maximum time to wait (default `5m`, `0` = unlimited) |
| `--max-attempts` | Maximum number of attempts (`0` = unlimited) |
| `--initial-delay` | Delay before the first attempt |
| `--min-interval` / `--max-interval` | Backoff bounds (defaults `1s` / `10s`) |
| `--backoff-multiplier` | Exponential backoff multiplier (must be `> 1.0`, default `2`) |
| `-q, --quiet` | Suppress progress messages |

`inspect` takes its own row-access and re-derivation flags: `--head`, `--tail`, `--page`/`--offset`/`--limit`, `--fields`, `--jq`, `--schema`, `--stats`, `--sample`, `--list`.

## Required token scopes

| Operation | Required scope |
| --- | --- |
| Query logs | `storage:logs:read` |
| Query metrics | `storage:metrics:read` |
| Query events | `storage:events:read` |
| Query business events | `storage:bizevents:read` |
| Query entities | `storage:entities:read` |
| Query spans | `storage:spans:read` |
| System tables | `storage:system:read` |
| Field sets | `storage:fieldsets:read` |
| Use filter segments (`--segment`) | `storage:filter-segments:read` |

The specific scope required depends on what the query's `fetch`/`timeseries` clause targets; dtctl does not require a single blanket scope for `query`/`wait`/`inspect`/`verify`.

## Output

Control how results are displayed with `-o`: `table` (default), `json`, `yaml`, `csv`, or a terminal chart format (`chart`, `sparkline`, `barchart`, `braille`).

```bash
dtctl query "fetch logs | limit 10"          # default table output
dtctl query "fetch logs | limit 10" -o json  # for scripting and piping to jq
dtctl query "fetch logs | limit 10" -o csv   # for spreadsheets and data tools
```

**Spilling large results.** A large result is a context-window hazard for AI agents: tens of thousands of rows serialised into a model's context can cost millions of tokens. Instead of returning the rows, `dtctl query` can spill the full result to a local file and return a compact summary in its place — per-column stats (type, nulls, distinct/top-K, min/max, mean), a few sample rows, and the file path.

```bash
# Tri-state control. Bare --spill = always spill.
dtctl query "fetch logs" --spill            # always spill
dtctl query "fetch logs" --spill=auto       # spill only above the threshold
dtctl query "fetch logs" --spill=never      # force rows inline (the default for a bare command)

# Choose the destination or format
dtctl query "fetch logs" --spill-to ./out.jsonl    # explicit file (implies --spill; format from extension)
dtctl query "fetch logs" --spill --spill-format parquet # jsonl (default), json, csv, or parquet
dtctl query "fetch logs" --spill=auto --spill-threshold 100KB  # size that triggers a spill (default 50KB)
```

Defaults: `never` for a bare command (so the `... -o csv > out.csv` pipeline path is untouched), `auto` in agent mode. Files go to the OS user cache dir, partitioned by context, written atomically and pruned after a 24h TTL. Spill is also configurable via a `spill:` config section and the `DTCTL_SPILL` / `DTCTL_SPILL_DIR` environment variables.

Once a result has spilled, `dtctl inspect` interrogates the file locally without re-querying:

```bash
dtctl inspect ./out.jsonl --head 20                       # first 20 rows
dtctl inspect ./out.jsonl --page --offset 1000 --limit 50 # a window deep in the result
dtctl inspect ./out.jsonl --jq 'select(.status == 500)'   # keep only matching rows (streaming, whole-file)
dtctl inspect ./out.jsonl --schema                        # re-derive columns + types + null counts
dtctl inspect --list                                      # lost the path? list spilled files in this context
```

`--jq` on `inspect` is a full-file filter run per record (like `jq` over NDJSON); it is not a query engine, so push aggregate questions back into DQL and re-query.

**Progress indicator.** Long-running queries execute asynchronously and are polled until they finish; a live progress bar is drawn on stderr by default for interactive terminals (`--no-progress` disables it), and is automatically suppressed for non-interactive output, under `--plain`, and in agent mode.

## Examples

Simple inline queries:

```bash
dtctl query "fetch logs | limit 10"
dtctl query "timeseries avg(dt.host.cpu.usage)"
```

File-based and stdin queries, which avoid shell-escaping issues:

```bash
dtctl query -f queries/errors.dql

dtctl query <<'EOF'
fetch logs
| filter loglevel == "ERROR"
| summarize count = count(), by: {dt.entity.service}
| sort count desc
| limit 20
EOF
```

Template queries with `--set`:

```bash
dtctl query "fetch logs | filter environment == '{{ .env }}' | limit {{ .n }}" \
  --set env=production --set n=50
```

Large dataset downloads:

```bash
dtctl query "fetch logs" --max-result-records 100000
dtctl query "fetch logs" --default-scan-limit-gbytes 500
```

Filter segments at query time (AND-combined when multiple are given):

```bash
dtctl query "fetch logs | limit 10" --segment my-segment-uid
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001"
dtctl query "fetch logs | limit 10" --segments-file segments.yaml
```

Timeframe, sampling, and metadata:

```bash
dtctl query "fetch logs | limit 10" \
  --default-timeframe-start "2024-01-01T00:00:00Z" \
  --default-timeframe-end   "2024-01-02T00:00:00Z"

dtctl query "fetch logs" --default-sampling-ratio 1000
dtctl query "fetch logs | limit 10" --metadata=scannedRecords,scannedBytes,executionTimeMilliseconds
```

Live mode, streaming results at a regular interval:

```bash
dtctl query 'fetch logs | filter loglevel == "ERROR" | sort timestamp desc | limit 10' \
  --live --interval 5s
```

Waiting for a query condition (tests and CI/CD, waiting for instrumented data to land):

```bash
# Wait until exactly one matching span arrives
dtctl wait query "fetch spans | filter test_id == 'test-123'" --for=count=1

# Wait for any error logs, up to 2 minutes
dtctl wait query 'fetch logs | filter status == "ERROR"' --for=any --timeout 2m

# Capture the result once the condition is met
dtctl wait query "..." --for=count=1 -o json > result.json
```

Exit codes for `wait query`: `0` condition met, `1` timeout, `2` max attempts exceeded, `3` query execution error, `4` invalid condition syntax, `5` invalid arguments.

Verifying query syntax without executing (useful in CI/CD and pre-commit hooks):

```bash
dtctl verify query "fetch logs | limit 10"
dtctl verify query -f queries/errors.dql
dtctl verify query "fetch logs | limit 10" --canonical
dtctl verify query -f queries/errors.dql --fail-on-warn
```

```bash
# In a CI pipeline: verify all .dql files before deploying
for f in queries/*.dql; do
  dtctl verify query -f "$f" --fail-on-warn || exit 1
done
```

Cancelling a running query: press `Ctrl+C` (or send `SIGTERM`) at any time. dtctl sends a best-effort `query:cancel` request to Grail so the backend stops executing, then exits, printing a confirmation or failure message to stderr.
