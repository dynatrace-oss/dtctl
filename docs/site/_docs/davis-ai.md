---
layout: docs
title: Davis AI
---

Dynatrace Davis AI provides intelligent analytics and conversational AI capabilities. dtctl exposes two major Davis features: **Analyzers** for statistical analysis and **CoPilot** for conversational AI interactions.

## Davis Analyzers

Davis analyzers perform statistical computations on your observability data — forecasting, change-point detection, correlation, and anomaly detection.

### List Analyzers

```bash
# List all available Davis analyzers
dtctl get analyzers
```

### Execute an Analyzer

```bash
# Run a forecast analyzer with a DQL timeseries query
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --query "timeseries avg(dt.host.cpu.usage)"

# Use a preset shortcut instead of naming the analyzer explicitly
dtctl analyze forecast \
  --query "timeseries avg(dt.host.cpu.usage)"

# Run the anomaly-detection preset
dtctl analyze anomaly \
  --query "timeseries avg(dt.host.cpu.usage)"

# Execute an analyzer using input from a file
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --file analyzer-input.yaml

# Validate analyzer input without executing
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --query "timeseries avg(dt.host.cpu.usage)" \
  --validate
```

### Common Analyzers

| Analyzer | Description |
|----------|-------------|
| `dt.statistics.GenericForecastAnalyzer` | Predict future metric values based on historical trends |
| `dt.statistics.GenericChangePointAnalyzer` | Detect significant changes in metric behavior |
| `dt.statistics.GenericCorrelationAnalyzer` | Find correlations between metric time series |
| `dt.statistics.GenericAnomalyDetectionAnalyzer` | Identify anomalous metric patterns |

Preset shortcuts are available via `dtctl analyze forecast`, `dtctl analyze anomaly`, `dtctl analyze change-point`, and `dtctl analyze correlation`.

## Davis CoPilot

Davis CoPilot is Dynatrace's conversational AI assistant. dtctl lets you interact with CoPilot from the terminal, including chat, natural-language-to-DQL translation, and document search.

### List CoPilot Skills

```bash
# List all available CoPilot skills
dtctl get copilot-skills
```

### Chat

```bash
# Ask a question with streaming output
dtctl exec copilot "What is DQL?" --stream

# Provide additional context
dtctl exec copilot "Why is my service slow?" \
  --context "Service: payment-api, Environment: production"

# Add custom instructions to guide the response
dtctl exec copilot "Summarize recent incidents" \
  --instructions "Focus on infrastructure-related issues only"

# Disable documentation lookup for faster responses
dtctl exec copilot "Explain gRPC" --no-docs
```

### Natural Language to DQL

```bash
# Convert a natural language question to a DQL query
dtctl exec copilot nl2dql "show me error logs from the last hour"
```

### DQL to Natural Language

```bash
# Explain a DQL query in plain English
dtctl exec copilot dql2nl \
  "fetch logs | filter status='ERROR' | summarize count(), by:{host}"
```

### Document Search

```bash
# Search across Dynatrace documents using natural language
dtctl exec copilot document-search "CPU performance analysis" \
  --collections notebooks
```

## Required Scopes

| Scope | Used By |
|-------|---------|
| `davis:analyzers:execute` | Executing Davis analyzers |
| `davis:copilot:execute` | CoPilot chat, NL2DQL, DQL2NL, document search |
