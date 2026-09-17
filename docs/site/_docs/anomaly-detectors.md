---
layout: docs
title: "Anomaly Detectors"
---

Custom anomaly detectors are Davis AI configurations that continuously monitor time series data and trigger Davis events when anomalous behavior is detected. They use the `builtin:davis.anomaly-detectors` Settings schema. dtctl provides full CRUD management with a human-friendly flattened YAML format.

## Listing Anomaly Detectors

```bash
# List all custom anomaly detectors
dtctl get anomaly-detectors

# List only enabled detectors
dtctl get anomaly-detectors --enabled

# List only disabled detectors
dtctl get anomaly-detectors --enabled=false

# Wide output (includes object IDs and descriptions)
dtctl get anomaly-detectors -o wide

# JSON or YAML for scripting
dtctl get anomaly-detectors -o json
```

## Describing an Anomaly Detector

Get full details including analyzer configuration, event template, and recent problems triggered by the detector:

```bash
# By object ID
dtctl describe anomaly-detector vu9U3hXa3q0AAAA

# By title (interactive disambiguation if ambiguous)
dtctl describe anomaly-detector "Aurora cluster CPU utilization"
```

Example output:

```
Title:                 Aurora cluster CPU utilization
Object ID:             vu9U3hXa3q0AAAA
Enabled:               true
Source:                 Clouds
Description:           Monitors Aurora cluster CPU utilization

Analyzer:
  Type:                Static Threshold
  Alert Condition:     ABOVE 90
  Sliding Window:      3 violating samples in 5 minutes
  De-alerting Samples: 5
  Missing Data Alert:  false

Query:
  timeseries cpu=avg(cloud.aws.rds.CPUUtilization), interval:1m

Event Template:
  event.type:          PERFORMANCE_EVENT
  event.name:          Aurora cluster high CPU
  event.description:   CPU utilization is high

Recent Problems (last 7 days):
  DISPLAY ID        STATUS    START                 CATEGORY
  P-2603120042      CLOSED    2026-03-28 14:22:00   PERFORMANCE
  (1 problem in the last 7 days)
```

The recent problems section cross-references Davis problems via DQL, giving immediate operational context for each detector.

## Creating and Applying Anomaly Detectors

Define a detector in YAML and create or update it:

```bash
# Create (fails if a detector with the same title already exists)
dtctl create anomaly-detector -f detector.yaml

# Apply (creates if new, updates if existing -- idempotent)
dtctl apply -f detector.yaml

# Create with template variables
dtctl create anomaly-detector -f detector.yaml --set threshold=95

# Dry-run to preview what would be sent
dtctl create anomaly-detector -f detector.yaml --dry-run
```

### Flattened YAML Format (Recommended)

The flattened format is human-friendly and recommended for version-controlled configurations:

```yaml
title: "High CPU on production hosts"
description: "Alert when CPU exceeds threshold on prod hosts"
enabled: true
source: "dtctl"
analyzer:
  name: dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer
  input:
    query: "timeseries cpu=avg(dt.host.cpu.usage), by:{dt.entity.host}, interval:1m"
    threshold: "90"
    alertCondition: ABOVE
    violatingSamples: "3"
    slidingWindow: "5"
    dealertingSamples: "5"
    alertOnMissingData: "false"
eventTemplate:
  event.type: PERFORMANCE_EVENT
  event.name: "High CPU on {dims:dt.entity.host}"
  event.description: "CPU usage exceeded threshold"
```

| Field | Description |
|-------|-------------|
| `title` | Display name for the detector (required) |
| `description` | Optional description |
| `enabled` | Whether the detector is active (defaults to `true`) |
| `source` | What created it (defaults to `"dtctl"` when omitted) |
| `analyzer.name` | Detection algorithm (see Analyzer Types below) |
| `analyzer.input` | Key-value pairs for the analyzer configuration |
| `eventTemplate` | Key-value pairs defining the triggered Davis event |
| `executionSettings.actor` | UUID of the identity whose permissions the detector query runs with (defaults to the authenticated user) |

### Analyzer Types

| Analyzer | Description |
|----------|-------------|
| `dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer` | Fires when a metric crosses a fixed value |
| `dt.statistics.ui.anomaly_detection.AutoAdaptiveAnomalyDetectionAnalyzer` | Fires when a metric deviates from a learned baseline |

### Common Analyzer Input Fields

| Field | Description |
|-------|-------------|
| `query` | DQL timeseries query to evaluate |
| `threshold` | Static threshold value (static threshold only) |
| `alertCondition` | `ABOVE` or `BELOW` |
| `slidingWindow` | Evaluation window in minutes |
| `violatingSamples` | Number of violating samples to trigger |
| `dealertingSamples` | Number of non-violating samples to clear |
| `alertOnMissingData` | Whether to alert when data is missing (`"true"` / `"false"`) |
| `numberOfSignalFluctuations` | Signal fluctuation threshold (auto-adaptive only) |

### Raw Settings API Format

The native Settings API format is also accepted for interoperability with Dynatrace API docs and export/import workflows:

```yaml
schemaId: builtin:davis.anomaly-detectors
scope: environment
value:
  title: "High CPU on production hosts"
  enabled: true
  analyzer:
    name: dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer
    input:
      - key: query
        value: "timeseries cpu=avg(dt.host.cpu.usage), interval:1m"
      - key: threshold
        value: "90"
      - key: alertCondition
        value: ABOVE
  eventTemplate:
    properties:
      - key: event.type
        value: PERFORMANCE_EVENT
      - key: event.name
        value: "High CPU detected"
```

### Validation and Schema Defaults

The `builtin:davis.anomaly-detectors` schema declares several fields non-nullable
*without* a server-side default, so a definition that simply omits them is
rejected with `Must not be null` on a field the author never wrote. dtctl fills
them in from the schema defaults before sending the request:

| Field | Filled with |
|-------|-------------|
| `description` | `""` |
| `enabled` | `true` |
| `source` | `"dtctl"` |
| `analyzer.input` | `[]` |
| `eventTemplate.properties` | `[]` |
| `executionSettings` | `{}` |
| `executionSettings.actor` | the authenticated identity (`dtctl auth whoami --id-only`) |

An `actor` you set yourself always wins, and updating an existing detector keeps
the actor it already runs as when the definition omits one.

`--dry-run` validates the definition against the schema in the environment
without persisting anything, and prints the exact payload that would be sent:

```bash
dtctl create anomaly-detector -f detector.yaml --dry-run
dtctl apply -f detector.yaml --dry-run
```

A definition the environment rejects fails the dry run and names the offending
fields:

```text
invalid anomaly detector:
  - analyzer: Parameter 'threshold' must be provided.
```

If validation cannot be performed (no reachable environment, missing scope), the
dry run reports `schema validation skipped` and falls back to local checks only.

## Editing an Anomaly Detector

Open a detector in your editor, modify it, and save to update:

```bash
dtctl edit anomaly-detector "High CPU on production hosts"
```

This fetches the detector, converts it to the flattened YAML format, opens it in `$EDITOR`, then applies the changes on save. Optimistic locking is handled automatically via the Settings API schema version.

## Watch Mode

Monitor anomaly detectors in real time:

```bash
dtctl get anomaly-detectors --watch
```

Press `Ctrl+C` to stop watching.

## Deleting an Anomaly Detector

```bash
# Delete with confirmation prompt
dtctl delete anomaly-detector "High CPU on production hosts"

# Delete by object ID
dtctl delete anomaly-detector vu9U3hXa3q0AAAA

# Skip confirmation
dtctl delete anomaly-detector vu9U3hXa3q0AAAA -y
```

dtctl prompts for confirmation in interactive mode. Use `--plain` to skip the prompt in scripts and CI pipelines.

## Aliases

Anomaly detectors support the `ad` alias for quick access:

| Context    | Primary               | Aliases              |
|------------|-----------------------|----------------------|
| `get`      | `anomaly-detectors`   | `anomaly-detector`, `ad` |
| `describe` | `anomaly-detector`    | `ad`                 |
| `create`   | `anomaly-detector`    | `ad`                 |
| `edit`     | `anomaly-detector`    | `ad`                 |
| `delete`   | `anomaly-detector`    | `ad`                 |
