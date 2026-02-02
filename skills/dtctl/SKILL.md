---
name: dtctl
description: Use dtctl CLI tool for querying observability data in Dynatrace via DQL (logs, metrics, traces, ...) and to manage Dynatrace platform resources (workflows, dashboards, notebooks, SLOs, settings, buckets, lookup tables).
license: Apache-2.0
---

# dtctl Command Reference

## Syntax
```bash
dtctl <verb> <resource> [name/id] [flags]
```

**Verbs:** get, describe, create, edit, apply, delete, exec, query, logs, wait, history, restore, share/unshare, find, open

**Key Resources:** workflow (wf), dashboard (dash), notebook (nb), slo, bucket (bkt), lookup (lkup), settings, function (fn), intent, analyzer (az), copilot (cp)

**Global Flags:**
- `--context` - Switch environment
- `-o, --output` - json|yaml|table|wide|csv|chart|sparkline|barchart
- `--dry-run` - Preview without executing
- `--plain` - Machine-readable output

## Setup
```bash
# Configure context
dtctl config set-context prod --environment "https://abc.apps.dynatrace.com" --token-ref prod-token --safety-level readonly
dtctl config set-credentials prod-token --token "dt0s16.YOUR_TOKEN"
dtctl config use-context prod

# Safety levels: readonly | readwrite-mine | readwrite-all | dangerously-unrestricted
```

## Common Commands

### Workflows
```bash
dtctl get workflows --mine
dtctl edit workflow <id>
dtctl apply -f workflow.yaml --set env=prod
dtctl exec workflow <id> --wait --timeout 10m
dtctl logs wfe <execution-id> --follow
dtctl history workflow <id>
```

### Dashboards/Notebooks
```bash
dtctl get dashboards --mine
dtctl edit dashboard <id>
dtctl share dashboard <id> --user user@example.com --access read-write
dtctl history dashboard <id>
dtctl restore dashboard <id> 3
```

### DQL Queries
```bash
dtctl query 'fetch logs | filter status="ERROR" | limit 100'
dtctl query -f query.dql --set host=h-123 --set timerange=2h
dtctl query 'timeseries avg(dt.host.cpu.usage)' -o chart
dtctl wait query 'fetch spans | filter test_id == "test-123"' --for=count=1 --timeout 5m
```

**Wait conditions:** count=N, count-gte=N, count-gt=N, count-lte=N, count-lt=N, any, none

**Template syntax in .dql files:**
```dql
fetch logs
| filter host.name = "{{.host}}"
| filter timestamp > now() - {{.timerange | default "1h"}}
```

### Lookup Tables
```bash
dtctl create lookup -f data.csv --path /lookups/grail/pm/errors --lookup-field code
dtctl get lookups
dtctl get lookup /lookups/grail/pm/errors -o csv > backup.csv

# Use in DQL
dtctl query "fetch logs | lookup [load '/lookups/grail/pm/errors'], lookupField:status_code"
```

### Settings API
```bash
dtctl get settings-schemas | grep openpipeline
dtctl get settings --schema builtin:openpipeline.logs.pipelines
dtctl edit setting <object-id>
dtctl apply -f config.yaml --set env=prod
```

### SLOs
```bash
dtctl get slos
dtctl describe slo <id>
dtctl exec slo <id> -o json
dtctl apply -f slo.yaml
```

### Davis AI
```bash
dtctl get analyzers
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --query "timeseries avg(dt.host.cpu.usage)" -o chart
dtctl exec copilot "What caused the CPU spike?"
dtctl exec copilot nl2dql "show error logs from last hour"
```

### App Functions
```bash
dtctl get functions                                       # List all functions
dtctl get functions --app dynatrace.slack                 # Filter by app
dtctl get functions -o wide                               # Show descriptions
dtctl describe function dynatrace.slack/slack-send-message
dtctl exec function dynatrace.slack/slack-send-message --method POST --payload '{"channel":"#alerts","message":"test"}'
dtctl exec function dynatrace.automations/execute-dql-query --method POST --data @query.json
```

### App Intents
```bash
dtctl get intents                                         # List all intents
dtctl get intents --app dynatrace.distributedtracing      # Filter by app
dtctl describe intent dynatrace.distributedtracing/view-trace
dtctl find intents --data trace_id=abc123                 # Find matching intents
dtctl open intent dynatrace.distributedtracing/view-trace --data trace_id=abc123
dtctl open intent dynatrace.distributedtracing/view-trace --data trace_id=abc123 --browser
```

## Template Variables
```bash
# In YAML files: {{.variable}} or {{.variable | default "value"}}
dtctl apply -f workflow.yaml --set environment=prod --set owner=team-a
```

## Troubleshooting
```bash
dtctl auth whoami                          # Check auth
dtctl auth can-i create workflows          # Check permissions
dtctl config set-credentials <context> --token "dt0s16.NEW_TOKEN"
dtctl --help                               # Command help
```

**Name resolution:** Use IDs instead of names if ambiguous (`dtctl get dashboards` to find ID)
**Safety blocks:** Adjust context safety level or switch context
**Permissions:** Check token scopes at https://github.com/dynatrace-oss/dtctl/blob/main/docs/TOKEN_SCOPES.md
