---
layout: docs
title: Analyzers
---

Analyzers perform statistical computations on your observability data —
forecasting, change-point detection, correlation, and anomaly detection. You
list the available analyzers, inspect an analyzer's input/output schema, run one
against a DQL timeseries, and validate input before executing.

## List Analyzers

```bash
# List all available analyzers
dtctl get analyzers

# Get a single analyzer's raw definition
dtctl get analyzer dt.statistics.GenericForecastAnalyzer

# Filter the list (DQL-style expression)
dtctl get analyzers --filter "name contains 'forecast'"

# Structured output
dtctl get analyzers -o json
```

## Describe an Analyzer

`describe` resolves an analyzer's JSON Schemas and renders the **required and
optional input fields** so you know exactly what to pass to `exec analyzer`.
Unlike `get analyzer`, which returns the raw definition, `describe` flattens the
input and result schemas into a readable table (and includes them in
JSON/YAML output).

```bash
# Describe an analyzer and see its input fields
dtctl describe analyzer dt.statistics.GenericForecastAnalyzer

# Print the analyzer's full markdown documentation
dtctl describe analyzer dt.statistics.GenericForecastAnalyzer --doc

# Structured output (includes inputSchema and resultSchema)
dtctl describe analyzer dt.statistics.GenericForecastAnalyzer -o json
```

Example output:

```text
Name:         dt.statistics.GenericForecastAnalyzer
Display Name: Forecast Analysis
Category:     Forecasting
Type:         AnalyzerFactory

Input (required):
  timeSeriesData   string    DQL timeseries query to forecast
  forecastHorizon  integer   Number of intervals to predict

Input (optional):
  coverageProbability  number   Confidence band width (e.g. 0.9)

Output:
  forecastQualityAssessment  object   Quality metrics for the forecast
  output                     array    Predicted values with confidence bands

  Run it:  dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --query <dql>
  Docs:    dtctl describe analyzer dt.statistics.GenericForecastAnalyzer --doc
```

> `analyzer`, `analyzers`, and `az` are interchangeable aliases across
> `get`, `describe`, `exec`, and `verify`.

## Execute an Analyzer

```bash
# Run a forecast analyzer with the DQL query shorthand
# (works for timeseries-based analyzers)
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --query "timeseries avg(dt.host.cpu.usage)"

# Provide the full input as inline JSON
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --input '{"query":"timeseries avg(dt.host.cpu.usage)"}'

# Provide input from a JSON file
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json

# Execute and wait for completion (default; --wait=false returns immediately)
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  -f input.json --wait --timeout 300

# Structured output
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json -o json
```

| Flag | Description |
|------|-------------|
| `--query` | DQL timeseries query shorthand (for timeseries analyzers) |
| `--input` | Inline JSON input |
| `-f`, `--file` | Read JSON input from a file |
| `--validate` | Validate input without executing |
| `--wait` | Wait for execution to complete (default `true`) |
| `--timeout` | Timeout in seconds when waiting (default `300`) |

## Validate Input

Validate an analyzer input against its schema without running it. This is the
same check `exec analyzer --validate` performs, exposed as its own verb.

```bash
# Validate input from a file
dtctl verify analyzer dt.statistics.GenericForecastAnalyzer -f input.json

# Validate inline JSON
dtctl verify analyzer dt.statistics.GenericForecastAnalyzer \
  --input '{"timeSeriesData":"timeseries avg(dt.host.cpu.usage)"}'

# Validate the DQL query shorthand
dtctl verify analyzer dt.statistics.GenericForecastAnalyzer \
  --query "timeseries avg(dt.host.cpu.usage)"

# Structured output
dtctl verify analyzer dt.statistics.GenericForecastAnalyzer -f input.json -o json
```

## Common Analyzers

| Analyzer | Description |
|----------|-------------|
| `dt.statistics.GenericForecastAnalyzer` | Predict future metric values based on historical trends |
| `dt.statistics.GenericChangePointAnalyzer` | Detect significant changes in metric behavior |
| `dt.statistics.GenericCorrelationAnalyzer` | Find correlations between metric time series |
| `dt.statistics.GenericAnomalyDetectionAnalyzer` | Identify anomalous metric patterns |

Use `dtctl get analyzers` to discover every analyzer available in your
environment, then `dtctl describe analyzer <name>` to see its inputs.

## Required Scopes

| Scope | Used By |
|-------|---------|
| `davis:analyzers:read` | Listing and describing analyzers |
| `davis:analyzers:execute` | Executing and validating analyzers |
