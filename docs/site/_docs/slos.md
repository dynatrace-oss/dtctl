---
layout: docs
title: "SLOs"
---

Service Level Objectives (SLOs) define reliability targets for your services. dtctl lets you list, create, evaluate, and manage SLOs directly from the command line.

## Listing SLOs

```bash
# List all SLOs
dtctl get slos

# Filter by name
dtctl get slos --filter 'name~production'

# Wide output with target, warning, and current status
dtctl get slos -o wide

# JSON or YAML for scripting
dtctl get slos -o json
```

## Describing an SLO

Get full details for a specific SLO, including its current status, error budget, and evaluation configuration:

```bash
dtctl describe slo slo-123
```

## Templates

Dynatrace provides built-in SLO templates for common use cases. Use them as a starting point:

```bash
# List available SLO templates
dtctl get slo-templates

# Create an SLO from a template (interactive — prompts for required fields)
dtctl create slo --from-template
```

## Creating and Applying SLOs

Define SLOs in YAML and create or update them:

```bash
# Create (fails if the SLO already exists)
dtctl create slo -f slo.yaml

# Apply (creates if new, updates if existing — idempotent)
dtctl apply -f slo.yaml
```

### Example SLO YAML

```yaml
name: Service Availability
description: 97% of requests are successful over the last 7 days
criteria:
  - timeframeFrom: -7d
    target: 97.0
    warning: 98.0
customSli:
  indicator: |
    timeseries { total=sum(dt.service.request.count), failures=sum(dt.service.request.failure_count) }
    , by: { dt.smartscape.service }
    | fieldsAdd sli=(((total[]-failures[])/total[])*(100))
    | fieldsAdd dt.smartscape.entity.name = getNodeName(dt.smartscape.service)
    | fieldsRemove total, failures, smartscapeIds
tags:
  - slo-type:availability
```

| Field               | Description                                              |
|---------------------|----------------------------------------------------------|
| `evaluationType`    | `AGGREGATE` (single value over the timeframe) or `PER_INTERVAL` (evaluated per interval) |
| `target`            | The SLO target percentage (e.g., `99.9`)                 |
| `warning`           | Warning threshold — alerts before the target is breached |
| `filter`            | Entity selector to scope the SLO                         |
| `metricExpression`  | Metric expression that computes the SLO value            |
| `timeframe`         | Evaluation window (e.g., `"-1w"`, `"-30d"`)              |

## Evaluating an SLO

Trigger an on-demand evaluation to check the current status and remaining error budget:

```bash
dtctl exec slo slo-123
```

This returns the current SLO value, error budget remaining, and evaluation status:

```
Name:           Checkout Availability
Status:         SUCCESS
SLO Value:      99.94%
Target:         99.90%
Warning:        99.95%
Error Budget:   0.04% remaining
Timeframe:      last 7 days
```

## Watch Mode

Monitor SLOs in real time:

```bash
dtctl get slos --watch
```

Press `Ctrl+C` to stop watching.

## Deleting an SLO

```bash
dtctl delete slo slo-123
```

dtctl prompts for confirmation in interactive mode. Use `--plain` to skip the prompt in scripts and CI pipelines.
