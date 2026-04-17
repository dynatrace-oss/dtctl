---
layout: docs
title: Extensions 2.0
---

Extensions 2.0 provide monitoring capabilities for technologies not covered by OneAgent out of the box. dtctl lets you list, inspect, and configure extensions and their monitoring configurations.

## Listing Extensions

```bash
# List all extensions
dtctl get extensions

# Filter by name
dtctl get extensions --name "sql"

# Using the alias
dtctl get ext --name "postgres"
```

## Viewing Extension Versions

Each extension can have multiple versions installed. View available versions for a specific extension:

```bash
dtctl get extension com.dynatrace.extension.postgres
```

## Describing an Extension

Get full details about a specific extension version, including its configuration schema and metadata:

```bash
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3
```

Retrieve the monitoring configuration JSON Schema for a version:

```bash
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3 --monitoring-configuration-schema

# Strip documentation/display noise for scripting
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3 --monitoring-configuration-schema --no-fluff
```

List the ActiveGate groups available for a specific version:

```bash
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3 --active-gate-groups
```

## Installing Extensions

### Upload a Custom Extension

```bash
dtctl create extension -f my-extension.zip

# Preview without applying
dtctl create extension -f my-extension.zip --dry-run
```

### Install from the Hub

```bash
# Latest version
dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring

# Specific version
dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring --version 1.2.3
```

> **Required scope**: `extensions:definitions:write`

## Monitoring Configurations

Monitoring configurations define how an extension collects data -- which endpoints to monitor, credentials, polling intervals, and feature flags.

### Listing Monitoring Configurations

```bash
# List all monitoring configs for an extension
dtctl get extension-configs --extension com.dynatrace.extension.postgres

# Using the alias
dtctl get ext-configs --extension com.dynatrace.extension.postgres

# Describe a specific monitoring config
dtctl describe extension-config <config-id>
```

### Creating Monitoring Configurations

Define a monitoring configuration in YAML and create it:

{% raw %}
```yaml
# postgres-monitoring.yaml
scope: HOST_GROUP-ABC123
description: Production PostgreSQL monitoring
version: "2.9.3"
featureSets:
  - postgresql
postgresql:
  endpoints:
    - host: db-prod-01.internal
      port: 5432
      authentication:
        scheme: basic
        username: monitoring
        password: "{{ .db_password }}"
      databases:
        - name: app_production
          collectMetrics: true
```
{% endraw %}

```bash
dtctl create extension-config -f postgres-monitoring.yaml \
  --extension com.dynatrace.extension.postgres
```

### Applying with Options

```bash
# Apply with scope
dtctl apply -f postgres-monitoring.yaml --scope HOST_GROUP-ABC123

# Dry-run to validate
dtctl apply -f postgres-monitoring.yaml --dry-run

# Use template variables for environment-specific values
dtctl apply -f postgres-monitoring.yaml \
  --set db_password=secret123 \
  --set env=production
```

### Generic Apply

Extension configurations can also be applied using the generic `dtctl apply -f` command when the YAML file includes a `type` field:

```yaml
# postgres-config.yaml
type: extension-config
extension: com.dynatrace.extension.postgres
spec:
  scope: HOST_GROUP-ABC123
  description: Production PostgreSQL monitoring
  version: "2.9.3"
  featureSets:
    - postgresql
  postgresql:
    endpoints:
      - host: db-prod-01.internal
        port: 5432
```

```bash
dtctl apply -f postgres-config.yaml
```

## Template Variables for Multi-Environment Deployment

Use Go template syntax and the `--set` flag to deploy the same configuration across environments:

{% raw %}
```yaml
# extension-template.yaml
scope: "{{ .host_group }}"
description: "{{ .env | title }} PostgreSQL monitoring"
version: "2.9.3"
featureSets:
  - postgresql
postgresql:
  endpoints:
    - host: "{{ .db_host }}"
      port: 5432
      authentication:
        scheme: basic
        username: monitoring
        password: "{{ .db_password }}"
```

```bash
# Deploy to staging
dtctl create extension-config -f extension-template.yaml \
  --extension com.dynatrace.extension.postgres \
  --set env=staging \
  --set host_group=HOST_GROUP-STAGING \
  --set db_host=db-staging.internal \
  --set db_password=staging-pass

# Deploy to production
dtctl ctx use production
dtctl create extension-config -f extension-template.yaml \
  --extension com.dynatrace.extension.postgres \
  --set env=production \
  --set host_group=HOST_GROUP-PROD \
  --set db_host=db-prod-01.internal \
  --set db_password=prod-pass
```
{% endraw %}

## Aliases

| Full Name | Alias |
|-----------|-------|
| `extensions` | `ext` |
| `extension-configs` | `ext-configs` |

## Example Configurations

### SQL Server Extension

{% raw %}
```yaml
scope: HOST_GROUP-SQL01
description: SQL Server monitoring
version: "2.5.1"
featureSets:
  - sqlserver
sqlServer:
  endpoints:
    - host: sql-prod.internal
      port: 1433
      authentication:
        scheme: basic
        username: dt_monitor
        password: "{{ .sql_password }}"
      databases:
        - name: OrdersDB
```
{% endraw %}

### SNMP Extension

{% raw %}
```yaml
scope: HOST_GROUP-NETWORK
description: Network device monitoring
version: "2.3.0"
featureSets:
  - snmpDefault
snmp:
  devices:
    - address: 10.0.1.1
      snmpVersion: v3
      authentication:
        username: dtmonitor
        authProtocol: SHA
        authPassword: "{{ .snmp_auth }}"
        privProtocol: AES
        privPassword: "{{ .snmp_priv }}"
```
{% endraw %}

## Hub Catalog

The Dynatrace Hub catalog lets you browse available extensions before installing them. These commands are read-only and do not require write scopes.

### Browsing Hub Extensions

```bash
# List all available extensions in the Hub
dtctl get hub-extensions

# Filter by keyword (case-insensitive, matches name, ID, or description)
dtctl get hub-extensions --filter kafka

# Wide output (includes description)
dtctl get hub-extensions -o wide

# Get a specific Hub extension by ID
dtctl get hub-extensions com.dynatrace.extension.host-monitoring

# Describe a Hub extension
dtctl describe hub-extensions com.dynatrace.extension.host-monitoring
```

### Viewing Extension Releases

```bash
# List all releases for an extension
dtctl get hub-extension-releases com.dynatrace.extension.host-monitoring

# Output as JSON
dtctl get hub-extension-releases com.dynatrace.extension.host-monitoring -o json
```

### Aliases

| Full Name | Alias |
|-----------|-------|
| `hub-extensions` | `hub-extension` |
| `hub-extension-releases` | `hub-extension-release` |
