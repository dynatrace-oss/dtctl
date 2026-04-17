---
layout: docs
title: "DQL Queries"
---

dtctl provides a powerful interface for executing Dynatrace Query Language (DQL) queries directly from your terminal. Run ad-hoc queries inline, load them from files, use template variables, and stream live results.

## Simple Inline Queries

Pass a DQL query string directly as an argument:

```bash
dtctl query "fetch logs | limit 10"

dtctl query "fetch spans | limit 10"

dtctl query "fetch events | limit 10"

dtctl query "timeseries avg(dt.host.cpu.usage)"
```

## File-Based Queries

Store complex queries in `.dql` files and execute them with `-f`:

```bash
dtctl query -f queries/errors.dql
```

This keeps queries version-controlled and avoids shell-escaping issues.

## Stdin Input

Pipe queries or use heredocs to avoid shell quoting problems entirely:

```bash
# Heredoc
dtctl query <<'EOF'
fetch logs
| filter loglevel == "ERROR"
| summarize count = count(), by: {dt.entity.service}
| sort count desc
| limit 20
EOF

# Pipe from a file
cat queries/errors.dql | dtctl query

# Pipe from another command
echo 'fetch logs | limit 5' | dtctl query
```

### PowerShell Quoting

On Windows PowerShell, use here-strings to avoid escaping issues:

```powershell
# PowerShell here-string
dtctl query @'
fetch logs
| filter loglevel == "ERROR"
| limit 10
'@
```

## Template Queries

Use Go template syntax with `--set` to parameterize queries:

{% raw %}
```bash
dtctl query "fetch logs | filter environment == '{{ .env }}' | limit {{ .n }}" \
  --set env=production --set n=50
```
{% endraw %}

Template variables work with both inline queries and file-based queries:

```bash
dtctl query -f queries/service-errors.dql --set service=checkout --set hours=24
```

## Output Formats

Control how results are displayed:

```bash
# Default table output
dtctl query "fetch logs | limit 10"

# JSON (for scripting and piping to jq)
dtctl query "fetch logs | limit 10" -o json

# YAML
dtctl query "fetch logs | limit 10" -o yaml

# CSV (for spreadsheets and data tools)
dtctl query "fetch logs | limit 10" -o csv
```

## Large Dataset Downloads

Control result size limits for bulk data extraction:

```bash
# Increase max records returned (default varies by query type)
dtctl query "fetch logs" --max-result-records 100000

# Increase max result payload size
dtctl query "fetch logs" --max-result-bytes 52428800

# Control how much data Grail scans (in GB)
dtctl query "fetch logs" --default-scan-limit-gbytes 500
```

## Filter Segments

Apply [filter segments](segments) at query time to narrow results to specific data subsets. Segments are AND-combined when multiple are specified. See [Filter Segments](segments) for how to manage segments.

```bash
# Apply a single segment by UID
dtctl query "fetch logs | limit 10" --segment abc123-def456

# Apply a segment by name (resolved via API)
dtctl query "fetch logs | limit 10" --segment my-k8s-segment

# Multiple segments (AND-combined per Grail semantics)
dtctl query "fetch logs | limit 10" --segment seg-1 --segment seg-2

# Short form
dtctl query "fetch logs | limit 10" -S my-segment-uid
```

### Segment Variables

Some segments require variable bindings (e.g., a ready-made `k8s.namespace.name` segment needs a namespace value). Bind variables inline using URL-query-style syntax on `-S`:

```bash
# Bind a single variable
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001"

# Multiple values for a variable (comma-separated)
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001,HOST-002"

# Multiple variables on one segment
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001&ns=production"

# Works with segment names (resolved before variable binding)
dtctl query "fetch logs | limit 10" \
  -S "[READY-MADE] k8s.namespace.name?k8s.namespace.name=astroshop"
```

The format is `SEGMENT?var=value&var2=value1,value2` where `SEGMENT` is a UID or name, `?` separates the ID from variables, `&` separates multiple variables, and `,` separates multiple values.

You can also use `--segment-var` / `-V` to override variables from `--segments-file`:

```bash
# Override a file-defined variable
dtctl query "fetch logs" --segments-file segments.yaml -V "seg-1:host=HOST-NEW"
```

For complex multi-segment configurations with many variables, use a YAML file:

```bash
dtctl query "fetch logs | limit 10" --segments-file segments.yaml
```

```yaml
# segments.yaml
- id: simple-segment-uid

- id: segment-with-variables
  variables:
    - name: host
      values: [HOST-0000000001, HOST-0000000002]

- id: segment-with-namespace
  variables:
    - name: ns
      values: [production, staging]
```

Both `--segment` and `--segments-file` can be combined. If the same segment ID appears in both, the file entry wins (it may carry variables). Variables from `-V` take precedence over file variables for the same name. A maximum of 10 segments per query is enforced client-side.

## Additional Parameters

Fine-tune query execution with these options:

```bash
# Specify a time frame
dtctl query "fetch logs | limit 10" --timeframe "now()-2h"

# Set timezone and locale
dtctl query "fetch logs | limit 10" --timezone "America/New_York" --locale "en_US"

# Enable sampling for faster results on large datasets
dtctl query "fetch logs" --sampling-ratio 0.1

# Fetch execution metadata alongside results
dtctl query "fetch logs | limit 10" --metadata

# Preview mode (faster, approximate results)
dtctl query "fetch logs | limit 10" --preview
```

## Live Mode

Stream query results at a regular interval:

```bash
# Re-run every 5 seconds
dtctl query "fetch logs | filter loglevel == 'ERROR' | sort timestamp desc | limit 10" \
  --live --interval 5s
```

Press `Ctrl+C` to stop live mode.

## Query Warnings

DQL may emit warnings (e.g., result truncation, deprecated syntax). These are printed to **stderr** so they don't interfere with piped output:

```bash
# Warnings appear on stderr, results on stdout
dtctl query "fetch logs" -o json > results.json
# Any warnings are still visible in the terminal
```

## Query Verification

Validate DQL queries without executing them — useful for CI/CD pipelines and pre-commit hooks.

```bash
# Verify a query is syntactically valid
dtctl verify query "fetch logs | limit 10"

# Verify from a file
dtctl verify query -f queries/errors.dql

# Return the canonical (normalized) form of the query
dtctl verify query "fetch logs | limit 10" --canonical

# Treat warnings as errors (non-zero exit code)
dtctl verify query -f queries/errors.dql --fail-on-warn
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | Query is valid |
| `1`  | Query has syntax or semantic errors |
| `2`  | Query is valid but has warnings (with `--fail-on-warn`) |

### CI/CD Integration

```bash
# In a CI pipeline — verify all .dql files before deploying
for f in queries/*.dql; do
  dtctl verify query -f "$f" --fail-on-warn || exit 1
done
```
