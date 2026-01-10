# dtctl Quick Start Guide

This guide provides practical examples for using dtctl to manage your Dynatrace environment. It covers configuration, common workflows, and all resource types with hands-on examples.

> **Note**: This guide assumes dtctl is already installed. If you need to build or install dtctl, see [INSTALLATION.md](INSTALLATION.md) first.

## Table of Contents

1. [Configuration](#configuration)
2. [Workflows](#workflows)
3. [Dashboards & Notebooks](#dashboards--notebooks)
4. [DQL Queries](#dql-queries)
5. [Service Level Objectives (SLOs)](#service-level-objectives-slos)
6. [Settings](#settings)
7. [Notifications](#notifications)
8. [Grail Buckets](#grail-buckets)
9. [OpenPipeline](#openpipeline)
10. [App Engine](#app-engine)
11. [EdgeConnect](#edgeconnect)
12. [Davis AI](#davis-ai)
13. [Output Formats](#output-formats)
14. [Tips & Tricks](#tips--tricks)
15. [Troubleshooting](#troubleshooting)

---

## Configuration

### Initial Setup

Set up your first Dynatrace environment:

```bash
# Create a context with your environment details
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token

# Store your platform token securely
dtctl config set-credentials my-token \
  --token "dt0s16.XXXXXXXXXXXXXXXXXXXXXXXX"

# Verify your configuration
dtctl config view
```

**Creating a Platform Token:**

To create a platform token in Dynatrace:
1. Navigate to **Identity & Access Management > Access Tokens**
2. Select **Generate new token** and choose **Platform token**
3. Give it a descriptive name (e.g., "dtctl-token")
4. Add the required scopes based on what you'll manage (see below)
5. Copy the token immediately - it's only shown once!

For detailed instructions, see [Dynatrace Platform Tokens documentation](https://docs.dynatrace.com/docs/manage/identity-access-management/access-tokens-and-oauth-clients/platform-tokens).

**Required Token Scopes by Resource:**
- **Workflows**: `automation:workflows:read`, `automation:workflows:write`, `automation:workflows:execute`
- **Documents** (dashboards/notebooks): `document:documents:read`, `document:documents:write`
- **DQL Queries**: `storage:logs:read`, `storage:events:read`, `storage:metrics:read`, `storage:buckets:read`
- **SLOs**: `slo:read`, `slo:write`
- **Settings**: `settings:objects:read`, `settings:objects:write`, `settings:schemas:read`
- **Grail Buckets**: `storage:buckets:read`, `storage:buckets:write`
- **OpenPipeline**: `openpipeline:configurations:read`, `openpipeline:configurations:write`
- **Davis Analyzers**: `davis:analyzers:read`, `davis:analyzers:execute`
- **Davis CoPilot**: `davis-copilot:conversations:execute`

### Multiple Environments

Manage multiple Dynatrace environments easily:

```bash
# Set up dev environment
dtctl config set-context dev \
  --environment "https://dev.apps.dynatrace.com" \
  --token-ref dev-token

dtctl config set-credentials dev-token \
  --token "dt0s16.DEV_TOKEN_HERE"

# Set up prod environment
dtctl config set-context prod \
  --environment "https://prod.apps.dynatrace.com" \
  --token-ref prod-token

dtctl config set-credentials prod-token \
  --token "dt0s16.PROD_TOKEN_HERE"

# List all contexts
dtctl config get-contexts

# Switch between environments
dtctl config use-context dev
dtctl config use-context prod

# Check current context
dtctl config current-context
```

### One-Time Context Override

Use a different context without switching:

```bash
# Execute a command in prod while dev is active
dtctl get workflows --context prod
```

### Current User Identity

View information about the currently authenticated user:

```bash
# View current user info
dtctl auth whoami

# Output:
# User ID:     621321d-1231-dsad-652321829b50
# User Name:   John Doe
# Email:       john.doe@example.com
# Context:     prod
# Environment: https://abc12345.apps.dynatrace.com

# Get just the user ID (useful for scripting)
dtctl auth whoami --id-only

# Output as JSON
dtctl auth whoami -o json
```

**Note:** The `whoami` command requires the `app-engine:apps:run` scope for full user details. If that scope is unavailable, it falls back to extracting the user ID from the JWT token.

---

## Workflows

Workflows automate tasks and integrate with Dynatrace monitoring.

### List and View Workflows

```bash
# List all workflows
dtctl get workflows

# List in table format with more details
dtctl get workflows -o wide

# Get a specific workflow by ID
dtctl get workflow workflow-123

# View detailed information
dtctl describe workflow workflow-123

# Describe by name (with fuzzy matching)
dtctl describe workflow "My Workflow"

# Output as JSON for processing
dtctl get workflow workflow-123 -o json
```

### Edit Workflows

Edit workflows directly in your preferred editor:

```bash
# Edit in YAML format (default)
dtctl edit workflow workflow-123

# Edit by name
dtctl edit workflow "My Workflow"

# Edit in JSON format
dtctl edit workflow workflow-123 --format=json

# Set your preferred editor
export EDITOR=vim
# or
dtctl config set preferences.editor vim
```

### Create Workflows

Create new workflows from YAML or JSON files:

```bash
# Create from a file
dtctl create workflow -f my-workflow.yaml

# Apply (create or update if exists)
dtctl apply -f my-workflow.yaml
```

**Example workflow file** (`my-workflow.yaml`):

```yaml
title: Daily Health Check
description: Runs a health check every day at 9 AM
trigger:
  schedule:
    rule: "0 9 * * *"
    timezone: "UTC"
tasks:
  check_errors:
    action: dynatrace.automations:run-javascript
    input:
      script: |
        export default async function () {
          console.log("Running health check...");
          return { status: "ok" };
        }
```

### Execute Workflows

Run workflows on-demand:

```bash
# Execute a workflow
dtctl exec workflow workflow-123

# Execute with parameters
dtctl exec workflow workflow-123 \
  --params environment=production \
  --params severity=high

# Execute and wait for completion
dtctl exec workflow workflow-123 --wait

# Execute with custom timeout
dtctl exec workflow workflow-123 --wait --timeout 10m
```

### View Executions

Monitor workflow executions:

```bash
# List all recent executions
dtctl get workflow-executions

# List executions for a specific workflow
dtctl get workflow-executions -w workflow-123

# Get details of a specific execution
dtctl describe workflow-execution exec-456
# or use short alias
dtctl describe wfe exec-456

# View execution logs
dtctl logs workflow-execution exec-456
# or
dtctl logs wfe exec-456

# Stream logs in real-time
dtctl logs wfe exec-456 --follow

# View logs for all tasks
dtctl logs wfe exec-456 --all

# View logs for a specific task
dtctl logs wfe exec-456 --task check_errors
```

### Delete Workflows

```bash
# Delete by ID
dtctl delete workflow workflow-123

# Delete by name (prompts for confirmation)
dtctl delete workflow "Old Workflow"

# Skip confirmation prompt
dtctl delete workflow "Old Workflow" -y
```

### Version History

View and restore previous versions of workflows:

```bash
# View version history
dtctl history workflow workflow-123
dtctl history workflow "My Workflow"

# Output as JSON
dtctl history workflow workflow-123 -o json
```

### Restore Previous Versions

Restore a workflow to a previous version:

```bash
# Restore to a specific version
dtctl restore workflow workflow-123 5

# Restore by name
dtctl restore workflow "My Workflow" 3

# Skip confirmation prompt
dtctl restore workflow "My Workflow" 3 --force
```

---

## Dashboards & Notebooks

Dashboards provide visual monitoring views, while notebooks enable interactive data exploration.

### List and View Documents

```bash
# List all dashboards
dtctl get dashboards

# List all notebooks
dtctl get notebooks

# Filter by name
dtctl get dashboards --name "production"
dtctl get notebooks --name "analysis"

# List only your own dashboards/notebooks
dtctl get dashboards --mine
dtctl get notebooks --mine

# Combine filters
dtctl get dashboards --mine --name "production"

# Get a specific document by ID
dtctl get dashboard dash-123
dtctl get notebook nb-456

# Describe by name
dtctl describe dashboard "Production Overview"
dtctl describe notebook "Weekly Analysis"
```

### Edit Documents

```bash
# Edit a dashboard in YAML (default)
dtctl edit dashboard dash-123

# Edit by name
dtctl edit dashboard "Production Overview"

# Edit in JSON format
dtctl edit notebook nb-456 --format=json
```

### Create and Apply Documents

Both `create` and `apply` work with dashboards and notebooks:

```bash
# Create a new dashboard (always creates new)
dtctl create dashboard -f dashboard.yaml

# Apply a dashboard (creates if new, updates if exists)
dtctl apply -f dashboard.yaml

# Both commands show tile count and URL:
# Dashboard "My Dashboard" (abc-123) created successfully [18 tiles]
# URL: https://env.apps.dynatrace.com/ui/document/v0/#/dashboards/abc-123
```

**When to use which:**
- **`create`**: Use when you want to create a new resource. Fails if the ID already exists.
- **`apply`**: Use for declarative management. Creates new resources or updates existing ones based on the ID in the file.

Both commands validate the document structure and warn about issues:
```bash
# If structure is wrong, you'll see warnings:
# Warning: dashboard content has no 'tiles' field - dashboard may be empty
```

### Round-Trip Export/Import

Export a dashboard and re-import it (works directly without modifications):

```bash
# Export existing dashboard
dtctl get dashboard abc-123 -o yaml > dashboard.yaml

# Re-apply to same or different environment
dtctl apply -f dashboard.yaml

# dtctl automatically handles the content structure
```

**Example dashboard** (`dashboard.yaml`):

```yaml
type: dashboard
name: Production Monitoring
content:
  tiles:
    - name: Response Time
      tileType: DATA_EXPLORER
      queries:
        - query: "timeseries avg(dt.service.request.response_time)"
```

### Share Documents

Share dashboards and notebooks with users and groups:

```bash
# Share with a user (read access by default)
dtctl share dashboard dash-123 --user user@example.com

# Share with write access
dtctl share dashboard dash-123 \
  --user user@example.com \
  --access read-write

# Share with a group
dtctl share notebook nb-456 --group "Platform Team"

# View sharing information
dtctl describe dashboard dash-123

# Remove user access
dtctl unshare dashboard dash-123 --user user@example.com

# Remove all shares
dtctl unshare dashboard dash-123 --all
```

### Version History (Snapshots)

View and restore previous versions of dashboards and notebooks:

```bash
# View version history
dtctl history dashboard dash-123
dtctl history notebook nb-456

# View history by name
dtctl history dashboard "Production Overview"
dtctl history notebook "Weekly Analysis"

# Output as JSON
dtctl history dashboard dash-123 -o json
```

### Restore Previous Versions

Restore a document to a previous snapshot version:

```bash
# Restore to a specific version
dtctl restore dashboard dash-123 5
dtctl restore notebook nb-456 3

# Restore by name
dtctl restore dashboard "Production Overview" 5

# Skip confirmation prompt
dtctl restore notebook "Weekly Analysis" 3 --force
```

**Notes:**
- Snapshots are created when documents are updated with the `create-snapshot` option
- Maximum 50 snapshots per document (oldest auto-deleted when exceeded)
- Snapshots auto-delete after 30 days
- Only the document owner can restore snapshots
- Restoring automatically creates a snapshot of the current state before restoring

### Delete Documents

```bash
# Delete a dashboard (moves to trash)
dtctl delete dashboard dash-123

# Delete by name
dtctl delete notebook "Old Analysis"

# Skip confirmation
dtctl delete dashboard dash-123 -y
```

---

## DQL Queries

Execute Dynatrace Query Language (DQL) queries to fetch logs, metrics, events, and more.

### Simple Queries

```bash
# Execute an inline query
dtctl query "fetch logs | limit 10"

# Filter logs by status
dtctl query "fetch logs | filter status='ERROR' | limit 100"

# Query recent events
dtctl query "fetch events | filter event.type='CUSTOM_ALERT' | limit 50"

# Summarize data
dtctl query "fetch logs | summarize count(), by: {status} | sort count desc"
```

### File-Based Queries

Store complex queries in files for reusability:

```bash
# Execute from file
dtctl query -f queries/errors.dql

# Save output to file
dtctl query -f queries/errors.dql -o json > results.json
```

### Stdin Input (Avoid Shell Escaping)

For queries with special characters like quotes, use stdin to avoid shell escaping issues:

```bash
# Heredoc syntax (recommended for complex queries)
dtctl query -f - -o json <<'EOF'
metrics
| filter startsWith(metric.key, "dt")
| summarize count(), by: {metric.key}
| fieldsKeep metric.key
| limit 10
EOF

# Pipe from a file
cat query.dql | dtctl query -o json

# Pipe from echo (simple cases)
echo 'fetch logs | filter status="ERROR"' | dtctl query -o table
```

**Tip:** Using single-quoted heredocs (`<<'EOF'`) preserves all special characters exactly as written—no escaping needed.

**Example query file** (`queries/errors.dql`):

```dql
fetch logs
| filter status = 'ERROR'
| filter timestamp > now() - 1h
| summarize count(), by: {log.source}
| sort count desc
| limit 10
```

### Template Queries

Use templates with variables for flexible queries:

```bash
# Query with variable substitution
dtctl query -f queries/logs-by-host.dql --set host=my-server

# Override multiple variables
dtctl query -f queries/logs-by-host.dql \
  --set host=my-server \
  --set timerange=24h \
  --set limit=500
```

**Example template** (`queries/logs-by-host.dql`):

```dql
fetch logs
| filter host = "{{.host}}"
| filter timestamp > now() - {{.timerange | default "1h"}}
| limit {{.limit | default 100}}
```

**Template syntax:**
- `{{.variable}}` - Reference a variable
- `{{.variable | default "value"}}` - Provide default value

### Output Formats

```bash
# Table format (default, human-readable)
dtctl query "fetch logs | limit 5" -o table

# JSON format (for processing)
dtctl query "fetch logs | limit 5" -o json

# YAML format
dtctl query "fetch logs | limit 5" -o yaml

# CSV format (for spreadsheets and data export)
dtctl query "fetch logs | limit 5" -o csv

# Export to CSV file
dtctl query "fetch logs" -o csv > logs.csv
```

### Large Dataset Downloads

By default, DQL queries are limited to 1000 records. Use query limit flags to download larger datasets:

```bash
# Increase result limit to 5000 records
dtctl query "fetch logs" --max-result-records 5000 -o csv > logs.csv

# Download up to 15000 records
dtctl query "fetch logs | limit 15000" --max-result-records 15000 -o csv > logs.csv

# Set result size limit in bytes (100MB)
dtctl query "fetch logs" \
  --max-result-records 10000 \
  --max-result-bytes 104857600 \
  -o csv > large_export.csv

# Set scan limit in gigabytes
dtctl query "fetch logs" \
  --max-result-records 10000 \
  --default-scan-limit-gbytes 5.0 \
  -o csv > large_export.csv

# Combine with filters for targeted exports
dtctl query "fetch logs | filter status='ERROR'" \
  --max-result-records 5000 \
  -o csv > error_logs.csv
```

**Query Limit Parameters:**
- `--max-result-records`: Maximum number of result records to return (default: 1000)
- `--max-result-bytes`: Maximum result size in bytes (default: varies by environment)
- `--default-scan-limit-gbytes`: Scan limit in gigabytes (default: varies by environment)

**Query Execution Parameters:**
- `--default-sampling-ratio`: Sampling ratio for query results (normalized to power of 10 ≤ 100000)
- `--fetch-timeout-seconds`: Time limit for fetching data in seconds
- `--enable-preview`: Request preview results if available within timeout
- `--enforce-query-consumption-limit`: Enforce query consumption limit
- `--include-types`: Include type information in query results

**Timeframe Parameters:**
- `--default-timeframe-start`: Query timeframe start timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T12:10:04.123Z')
- `--default-timeframe-end`: Query timeframe end timestamp (ISO-8601/RFC3339, e.g., '2022-04-20T13:10:04.123Z')

**Localization Parameters:**
- `--locale`: Query locale (e.g., 'en_US', 'de_DE')
- `--timezone`: Query timezone (e.g., 'UTC', 'Europe/Paris', 'America/New_York')

**Note:** All parameters are sent in the DQL query request body and work with both immediate responses and long-running queries that require polling.

**Advanced Query Examples:**

```bash
# Query with specific timeframe
dtctl query "fetch logs" \
  --default-timeframe-start "2024-01-01T00:00:00Z" \
  --default-timeframe-end "2024-01-02T00:00:00Z" \
  -o csv

# Query with timezone and locale
dtctl query "fetch logs" \
  --timezone "Europe/Paris" \
  --locale "fr_FR" \
  -o json

# Query with sampling for large datasets
dtctl query "fetch logs" \
  --default-sampling-ratio 10 \
  --max-result-records 10000 \
  -o csv

# Query with preview mode (faster results)
dtctl query "fetch logs" \
  --enable-preview \
  -o table

# Query with type information included
dtctl query "fetch logs" \
  --include-types \
  -o json
```

**Tip:** Use CSV output with increased limits for:
- Exporting data for analysis in Excel or Google Sheets
- Creating backups of log data
- Feeding data into external analysis tools
- Generating reports from DQL query results

### Query Warnings

DQL queries may return warnings (e.g., scan limits reached, results truncated). These warnings are printed to **stderr**, keeping stdout clean for data processing.

```bash
# Warnings appear on stderr, data on stdout
dtctl query "fetch spans, from: -10d | summarize count()"
# Warning: Your execution was stopped after 500 gigabytes of data were scanned...
# map[count():194414758]

# Pipe data normally - warnings don't interfere
dtctl query "fetch logs | limit 100" -o json | jq '.records[0]'

# Suppress warnings entirely
dtctl query "fetch spans | summarize count()" 2>/dev/null

# Save data to file (warnings still visible in terminal)
dtctl query "fetch logs" -o csv > logs.csv

# Save data and warnings separately
dtctl query "fetch logs" -o json > data.json 2> warnings.txt

# Discard warnings, save only data
dtctl query "fetch logs" -o csv 2>/dev/null > clean_data.csv
```

**Common warnings:**
- **SCAN_LIMIT_GBYTES**: Query stopped after scanning the default limit. Use `--default-scan-limit-gbytes` to adjust.
- **RESULT_TRUNCATED**: Results exceeded the limit. Use `--max-result-records` to increase.

---

## Service Level Objectives (SLOs)

SLOs define and track service reliability targets.

### List and View SLOs

```bash
# List all SLOs
dtctl get slos

# Filter by name
dtctl get slos --filter 'name~production'

# Get a specific SLO
dtctl get slo slo-123

# Detailed view
dtctl describe slo slo-123
```

### SLO Templates

Use templates to quickly create SLOs:

```bash
# List available templates
dtctl get slo-templates

# View template details
dtctl describe slo-template template-456

# Create SLO from template
dtctl create slo \
  --from-template template-456 \
  --name "API Availability" \
  --target 99.9
```

### Create and Apply SLOs

```bash
# Create from file
dtctl create slo -f slo-definition.yaml

# Apply (create or update)
dtctl apply -f slo-definition.yaml
```

**Example SLO** (`slo-definition.yaml`):

```yaml
name: API Response Time
description: 95% of requests should complete within 500ms
target: 95.0
warning: 97.0
evaluationType: AGGREGATE
filter: type("SERVICE") AND entityName.equals("my-api")
metricExpression: "(100)*(builtin:service.response.time:splitBy():sort(value(avg,descending)):limit(10):avg:partition(\"latency\",value(\"good\",lt(500))))/(builtin:service.requestCount.total:splitBy():sort(value(avg,descending)):limit(10):avg)"
```

### Evaluate SLOs

Evaluate SLOs to get current status, values, and error budget for each criterion:

```bash
# Evaluate SLO performance
dtctl exec slo slo-123

# Evaluate with custom timeout (default: 30 seconds)
dtctl exec slo slo-123 --timeout 60

# Output as JSON for analysis
dtctl exec slo slo-123 -o json

# Extract error budget from results
dtctl exec slo slo-123 -o json | jq '.evaluationResults[].errorBudget'

# View in table format (default)
dtctl exec slo slo-123
```

### Delete SLOs

```bash
# Delete an SLO
dtctl delete slo slo-123

# Skip confirmation
dtctl delete slo slo-123 -y
```

---

## Settings

Settings control various Dynatrace platform configurations.

### List Settings Schemas

Settings are organized by schemas:

```bash
# List all available settings schemas
dtctl get settings-schemas

# Get a specific schema definition
dtctl get settings-schema builtin:health-experience.cloud-alert

# View schema details with validation rules
dtctl describe settings-schema builtin:health-experience.cloud-alert
```

### List and View Settings

```bash
# List settings for a specific schema
dtctl get settings --schema builtin:alerting.profile

# Filter by scope
dtctl get settings \
  --schema builtin:anomaly-detection.infrastructure \
  --scope environment

# Get a specific settings object
dtctl get settings object-789

# Detailed view
dtctl describe settings object-789
```

### Create and Apply Settings

```bash
# Create settings from file
dtctl create settings \
  -f alerting-profile.yaml \
  --schema builtin:alerting.profile \
  --scope environment

# Apply (create or update)
dtctl apply -f alerting-profile.yaml
```

**Example settings file** (`alerting-profile.yaml`):

```yaml
schemaId: builtin:alerting.profile
scope: environment
value:
  name: "Production Alerts"
  rules:
    - severityLevel: ERROR
      enabled: true
    - severityLevel: WARNING
      enabled: false
```

### Delete Settings

```bash
# Delete a settings object
dtctl delete settings object-789
```

---

## Notifications

View and manage event notifications.

### List Notifications

```bash
# List all notifications
dtctl get notifications

# Filter by type
dtctl get notifications --type EMAIL

# Get a specific notification
dtctl get notification notif-123

# Detailed view
dtctl describe notification notif-123
```

### Delete Notifications

```bash
# Delete a notification
dtctl delete notification notif-123
```

---

## Grail Buckets

Grail buckets provide scalable log and event storage.

### List and View Buckets

```bash
# List all buckets
dtctl get buckets

# Get a specific bucket
dtctl get bucket logs-production

# Detailed view with configuration
dtctl describe bucket logs-production
```

### Create and Apply Buckets

```bash
# Create a bucket from file
dtctl create bucket -f bucket-config.yaml

# Apply (create or update)
dtctl apply -f bucket-config.yaml
```

**Example bucket configuration** (`bucket-config.yaml`):

```yaml
bucketName: logs-production
displayName: Production Logs
table: logs
retentionDays: 35
status: active
```

### Delete Buckets

```bash
# Delete a bucket
dtctl delete bucket logs-staging

# Skip confirmation
dtctl delete bucket logs-staging -y
```

---

## OpenPipeline

OpenPipeline processes and routes observability data.

### View Pipeline Configurations

```bash
# List all pipelines
dtctl get openpipelines

# Get a specific pipeline (by type)
dtctl get openpipeline logs

# Detailed view with processing rules
dtctl describe openpipeline logs
```

**Note:** Pipeline editing is typically done through the Dynatrace UI. Use `describe` to view current configurations.

---

## App Engine

Manage Dynatrace apps and functions.

### List and View Apps

```bash
# List all apps
dtctl get apps

# Filter by name
dtctl get apps --name "monitoring"

# Get a specific app
dtctl get app app-123

# Detailed view
dtctl describe app app-123
```

### Delete Apps

```bash
# Delete an app
dtctl delete app app-123

# Skip confirmation
dtctl delete app app-123 -y
```

---

## EdgeConnect

EdgeConnect provides secure connectivity for ActiveGates.

### List and View EdgeConnect

```bash
# List all EdgeConnect configurations
dtctl get edgeconnects

# Get a specific configuration
dtctl get edgeconnect ec-123

# Detailed view
dtctl describe edgeconnect ec-123
```

### Create and Apply EdgeConnect

```bash
# Create from file
dtctl create edgeconnect -f edgeconnect-config.yaml

# Apply (create or update)
dtctl apply -f edgeconnect-config.yaml
```

**Example configuration** (`edgeconnect-config.yaml`):

```yaml
name: "Production EdgeConnect"
hostPatterns:
  - "*.example.com"
  - "api.production.net"
oauthClientId: "client-id"
oauthClientSecret: "client-secret"
```

### Delete EdgeConnect

```bash
# Delete a configuration
dtctl delete edgeconnect ec-123
```

---

## Davis AI

Davis AI provides predictive analytics (Analyzers) and generative AI assistance (CoPilot).

### Davis Analyzers

Analyzers perform statistical analysis on time series data for forecasting, anomaly detection, and correlation analysis.

#### List and View Analyzers

```bash
# List all available analyzers
dtctl get analyzers

# Filter analyzers by name
dtctl get analyzers --filter "name contains 'forecast'"

# Get a specific analyzer definition
dtctl get analyzer dt.statistics.GenericForecastAnalyzer

# View analyzer details as JSON
dtctl get analyzer dt.statistics.GenericForecastAnalyzer -o json
```

#### Execute Analyzers

Run analyzers to perform statistical analysis:

```bash
# Execute with a DQL query (shorthand for timeseries analyzers)
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --query "timeseries avg(dt.host.cpu.usage)"

# Execute with inline JSON input
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --input '{"timeSeriesData":"timeseries avg(dt.host.cpu.usage)","forecastHorizon":50}'

# Execute from input file
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f forecast-input.json

# Validate input without executing
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  -f forecast-input.json --validate

# Output result as JSON
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --query "timeseries avg(dt.host.cpu.usage)" -o json
```

**Example analyzer input file** (`forecast-input.json`):

```json
{
  "timeSeriesData": "timeseries avg(dt.host.cpu.usage)",
  "forecastHorizon": 100,
  "generalParameters": {
    "timeframe": {
      "startTime": "now-7d",
      "endTime": "now"
    }
  }
}
```

#### Common Analyzers

| Analyzer | Description |
|----------|-------------|
| `dt.statistics.GenericForecastAnalyzer` | Time series forecasting |
| `dt.statistics.ChangePointAnalyzer` | Detect changes in time series |
| `dt.statistics.CorrelationAnalyzer` | Find correlations between metrics |
| `dt.statistics.TimeSeriesCharacteristicAnalyzer` | Analyze time series properties |
| `dt.statistics.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer` | Static threshold anomaly detection |

### Davis CoPilot

CoPilot provides AI-powered assistance for understanding your Dynatrace environment.

#### List CoPilot Skills

```bash
# List available CoPilot skills
dtctl get copilot-skills

# Output:
# NAME
# conversation
# nl2dql
# dql2nl
# documentSearch
```

#### Chat with CoPilot

```bash
# Ask a question
dtctl exec copilot "What is DQL?"

# Ask about your environment
dtctl exec copilot "What caused the CPU spike on my production hosts?"

# Read question from file
dtctl exec copilot -f question.txt

# Stream response in real-time (shows tokens as they arrive)
dtctl exec copilot "Explain the recent errors in my environment" --stream

# Provide additional context
dtctl exec copilot "Analyze this issue" \
  --context "Error logs show connection timeouts to database"

# Disable Dynatrace documentation retrieval
dtctl exec copilot "What is an SLO?" --no-docs

# Add formatting instructions
dtctl exec copilot "List the top 5 error types" \
  --instruction "Format as a numbered list with counts"
```

#### CoPilot Use Cases

```bash
# Get help writing DQL queries
dtctl exec copilot "Write a DQL query to find all ERROR logs from the last hour"

# Understand existing queries
dtctl exec copilot "Explain this query: fetch logs | filter status='ERROR' | summarize count()"

# Troubleshoot issues
dtctl exec copilot "Why might my service response time be increasing?"

# Learn about Dynatrace features
dtctl exec copilot "How do I set up an SLO for API availability?"
```

#### NL to DQL (Natural Language to DQL)

Generate DQL queries from natural language descriptions:

```bash
# Generate a DQL query from natural language
dtctl exec copilot nl2dql "show me error logs from the last hour"
# Output: fetch logs | filter status = "ERROR" | filter timestamp > now() - 1h

# More complex queries
dtctl exec copilot nl2dql "find hosts with CPU usage above 80%"
dtctl exec copilot nl2dql "count logs by severity for the last 24 hours"

# Read prompt from file
dtctl exec copilot nl2dql -f prompt.txt

# Get full response with messageToken (for feedback)
dtctl exec copilot nl2dql "show recent errors" -o json
```

#### DQL to NL (Explain DQL Queries)

Get natural language explanations of DQL queries:

```bash
# Explain a DQL query
dtctl exec copilot dql2nl "fetch logs | filter status='ERROR' | summarize count(), by:{host}"
# Output:
# Summary: Count error logs grouped by host
# Explanation: This query fetches logs, filters for ERROR status, and counts them by host.

# Explain a complex query
dtctl exec copilot dql2nl "timeseries avg(dt.host.cpu.usage), by:{dt.entity.host} | filter avg > 80"

# Read query from file
dtctl exec copilot dql2nl -f query.dql

# Get full response as JSON
dtctl exec copilot dql2nl "fetch logs | limit 10" -o json
```

#### Document Search

Find relevant notebooks and dashboards:

```bash
# Search for documents about CPU analysis
dtctl exec copilot document-search "CPU performance analysis" --collections notebooks

# Search across multiple collections
dtctl exec copilot document-search "error monitoring" --collections dashboards,notebooks

# Exclude specific documents from results
dtctl exec copilot document-search "performance" --exclude doc-123,doc-456

# Output as JSON for processing
dtctl exec copilot document-search "kubernetes" --collections notebooks -o json
```

### Required Token Scopes

For Davis AI features, your platform token needs:
- **Analyzers**: `davis:analyzers:read`, `davis:analyzers:execute`
- **CoPilot Chat**: `davis-copilot:conversations:execute`
- **NL to DQL**: `davis-copilot:nl2dql:execute`
- **DQL to NL**: `davis-copilot:dql2nl:execute`
- **Document Search**: `davis-copilot:document-search:execute`

---

## Output Formats

All `get` and `query` commands support multiple output formats.

### Table Format (Default)

Human-readable table output:

```bash
dtctl get workflows

# Output:
# ID            TITLE              OWNER          UPDATED
# wf-123        Health Check       me             2h ago
# wf-456        Alert Handler      team-sre       1d ago
```

### JSON Format

Machine-readable JSON:

```bash
dtctl get workflow wf-123 -o json

# Output:
# {
#   "id": "wf-123",
#   "title": "Health Check",
#   "owner": "me",
#   ...
# }

# Pretty-print with jq
dtctl get workflows -o json | jq '.'
```

### YAML Format

Kubernetes-style YAML:

```bash
dtctl get workflow wf-123 -o yaml

# Output:
# id: wf-123
# title: Health Check
# owner: me
# ...
```

### Wide Format

Table with additional columns:

```bash
dtctl get workflows -o wide

# Shows more details in table format
```

### CSV Format

Spreadsheet-compatible comma-separated values output:

```bash
# Export workflows to CSV
dtctl get workflows -o csv > workflows.csv

# Export DQL query results to CSV
dtctl query "fetch logs | limit 100" -o csv > logs.csv

# Download large datasets (up to 10000 records)
dtctl query "fetch logs" --max-result-records 5000 -o csv > large_export.csv

# Import into Excel, Google Sheets, or other tools
```

**CSV Features:**
- Proper escaping for special characters (commas, quotes, newlines)
- Alphabetically sorted columns for consistency
- Handles missing values gracefully
- Compatible with all spreadsheet applications
- Perfect for data export and offline analysis

### Plain Output

No colors, no interactive prompts (for scripts):

```bash
dtctl get workflows --plain
```

### Pagination (--chunk-size)

Like kubectl, dtctl automatically paginates through large result sets:

```bash
# Default: fetch all results in chunks of 500 (like kubectl)
dtctl get notebooks

# Disable chunking (return only first page from API)
dtctl get notebooks --chunk-size=0

# Use smaller chunks (useful for slow connections)
dtctl get notebooks --chunk-size=100
```

---

## Tips & Tricks

### Name Resolution

Use resource names instead of memorizing IDs:

```bash
# Works with any command that accepts an ID
dtctl describe workflow "My Workflow"
dtctl edit dashboard "Production Overview"
dtctl delete notebook "Old Analysis"

# If multiple resources match, you'll be prompted to select
# Use --plain to require exact matches only
```

### Shell Completion

Enable tab completion for faster workflows:

**Bash:**
```bash
source <(dtctl completion bash)

# Make it permanent:
sudo mkdir -p /etc/bash_completion.d
dtctl completion bash | sudo tee /etc/bash_completion.d/dtctl > /dev/null
```

**Zsh:**
```bash
mkdir -p ~/.zsh/completions
dtctl completion zsh > ~/.zsh/completions/_dtctl
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
rm -f ~/.zcompdump* && autoload -U compinit && compinit
```

**Fish:**
```bash
mkdir -p ~/.config/fish/completions
dtctl completion fish > ~/.config/fish/completions/dtctl.fish
```

### Query Libraries

Organize your DQL queries in a directory:

```bash
# Create a directory for your queries (using XDG data home)
mkdir -p ~/.local/share/dtctl/queries

# Create reusable queries
cat > ~/.local/share/dtctl/queries/errors-last-hour.dql <<EOF
fetch logs
| filter status = 'ERROR'
| filter timestamp > now() - 1h
| limit {{.limit | default 100}}
EOF

# Use them easily
dtctl query -f ~/.local/share/dtctl/queries/errors-last-hour.dql
```

**Note**: dtctl follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html) and adapts to platform conventions:

**Linux:**
- Config: `$XDG_CONFIG_HOME/dtctl` (default: `~/.config/dtctl`)
- Data: `$XDG_DATA_HOME/dtctl` (default: `~/.local/share/dtctl`)
- Cache: `$XDG_CACHE_HOME/dtctl` (default: `~/.cache/dtctl`)

**macOS:**
- Config: `~/Library/Application Support/dtctl`
- Data: `~/Library/Application Support/dtctl`
- Cache: `~/Library/Caches/dtctl`

**Windows:**
- Config: `%LOCALAPPDATA%\dtctl`
- Data: `%LOCALAPPDATA%\dtctl`
- Cache: `%LOCALAPPDATA%\dtctl`

### Export and Backup

Backup your resources regularly:

```bash
# Export all workflows
dtctl get workflows -o yaml > workflows-backup.yaml

# Export all dashboards
dtctl get dashboards -o json > dashboards-backup.json

# Export with timestamp
dtctl get workflows -o yaml > "workflows-$(date +%Y%m%d).yaml"
```

### Dry Run

Preview changes before applying:

```bash
# See what would be created/updated (shows create vs update, validates structure)
dtctl apply -f workflow.yaml --dry-run

# For dashboards/notebooks, dry-run shows:
# - Whether it will create or update
# - Document name and ID
# - Tile/section count
# - Structure validation warnings
dtctl apply -f dashboard.yaml --dry-run

# Example output:
# Dry run: would create dashboard
#   Name: SRE Service Health Overview
#   Tiles: 18
#
# Document structure validated successfully

# If there are issues, you'll see warnings:
# Warning: detected double-nested content (.content.content) - using inner content
# Warning: dashboard content has no 'tiles' field - dashboard may be empty

# See what would be deleted
dtctl delete workflow "Test Workflow" --dry-run
```

### Show Diff

See exactly what changes when updating resources:

```bash
# Show diff when updating a dashboard
dtctl apply -f dashboard.yaml --show-diff

# Output shows:
# --- existing dashboard
# +++ new dashboard
# - "title": "Old Title"
# + "title": "New Title"
```

### Verbose Output

Debug issues with verbose mode:

```bash
# See API calls and responses (auth headers redacted)
dtctl get workflows -v

# Full debug output including auth headers (use with caution!)
dtctl get workflows -vv
```

### Environment Variables

Set default preferences:

```bash
# Set default output format
export DTCTL_OUTPUT=json

# Set default context
export DTCTL_CONTEXT=production

# Override with flags
dtctl get workflows -o yaml
```

### Pipeline Commands

Combine dtctl with standard Unix tools:

```bash
# Count workflows
dtctl get workflows -o json | jq '. | length'

# Find workflows by owner
dtctl get workflows -o json | jq '.[] | select(.owner=="me")'

# Extract workflow IDs
dtctl get workflows -o json | jq -r '.[].id'

# Filter and format
dtctl query "fetch logs | limit 100" -o json | \
  jq '.records[] | select(.status=="ERROR")'
```

### Large Dataset Exports

Export large datasets from DQL queries for offline analysis:

```bash
# Export up to 5000 records to CSV
dtctl query "fetch logs | filter status='ERROR'" \
  --max-result-records 5000 \
  -o csv > error_logs.csv

# Export multiple datasets with timestamps
dtctl query "fetch logs" --max-result-records 10000 -o csv > "logs-$(date +%Y%m%d-%H%M%S).csv"

# Process large CSV exports with Unix tools
dtctl query "fetch logs" --max-result-records 5000 -o csv | \
  grep "ERROR" | \
  wc -l

# Split large exports into smaller files
dtctl query "fetch logs" --max-result-records 10000 -o csv | \
  split -l 1000 - logs_part_

# Import into databases
dtctl query "fetch logs" --max-result-records 5000 -o csv > logs.csv
# Then use database import tools:
# psql -c "\COPY logs FROM 'logs.csv' CSV HEADER"
# mysql -e "LOAD DATA LOCAL INFILE 'logs.csv' INTO TABLE logs FIELDS TERMINATED BY ',' ENCLOSED BY '\"' IGNORE 1 ROWS"
```

**Performance Tips:**
- Use filters in your DQL query to reduce dataset size
- Request only the columns you need
- Consider time-based filtering for incremental exports
- CSV format is more compact than JSON for large datasets

---

## Troubleshooting

### "config file not found"

This means you haven't set up your configuration yet. Run:

```bash
dtctl config set-context my-env \
  --environment "https://YOUR_ENV.apps.dynatrace.com" \
  --token-ref my-token

dtctl config set-credentials my-token --token "dt0s16.YOUR_TOKEN"
```

### "failed to execute workflow" or "failed to list workflows"

Check:
1. Your token has the correct permissions
2. Your environment URL is correct
3. You're using the right context

Enable verbose mode to see HTTP request/response details:
```bash
dtctl get workflows -v
```

The `-v` flag enables debug logging and shows detailed HTTP interactions with the API.

### Platform Token Permissions

For the core features, your platform token needs:
- **Workflows**: `automation:workflows:read`, `automation:workflows:write`, `automation:workflows:execute`
- **Documents** (dashboards/notebooks): `document:documents:read`, `document:documents:write`
- **DQL Queries**: `storage:logs:read`, `storage:events:read`, `storage:metrics:read`, `storage:buckets:read`
- **SLOs**: `slo:read`, `slo:write`
- **Settings**: `settings:objects:read`, `settings:objects:write`, `settings:schemas:read`
- **Grail Buckets**: `storage:buckets:read`, `storage:buckets:write`
- **OpenPipeline**: `openpipeline:configurations:read`, `openpipeline:configurations:write`
- **Davis Analyzers**: `davis:analyzers:read`, `davis:analyzers:execute`
- **Davis CoPilot**: `davis-copilot:conversations:execute`

---

## Next Steps

- **API Reference**: See [API_DESIGN.md](API_DESIGN.md) for complete command reference
- **Architecture**: Read [ARCHITECTURE.md](ARCHITECTURE.md) to understand how dtctl works
- **Design Principles**: Check [DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) for design philosophy
- **Implementation Status**: View [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) for roadmap

## Getting Help

```bash
# General help
dtctl --help

# Command-specific help
dtctl get --help
dtctl query --help

# Resource-specific help
dtctl get workflows --help
```

Enable verbose mode with `-v` to see detailed HTTP request/response logs for debugging API issues.

For issues and feature requests, visit the [GitHub repository](https://github.com/dynatrace/dtctl).
