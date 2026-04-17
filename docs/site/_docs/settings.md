---
layout: docs
title: Settings API
---

The Dynatrace Settings API provides a unified way to manage configuration objects across the platform. This is the primary mechanism for configuring OpenPipeline, built-in settings, and many other Dynatrace features.

## Settings Schemas

Schemas define the structure and validation rules for settings objects. List available schemas to discover what can be configured:

```bash
# List all settings schemas
dtctl get settings-schemas

# Filter schemas by name
dtctl get settings-schemas --name "openpipeline"

# Describe a specific schema to see its fields and constraints
dtctl describe settings-schema builtin:openpipeline.logs.pipelines
```

### Common OpenPipeline Schemas

| Schema ID | Purpose |
|-----------|---------|
| `builtin:openpipeline.logs.pipelines` | Log processing pipelines |
| `builtin:openpipeline.logs.ingest-sources` | Log ingest source configuration |
| `builtin:openpipeline.logs.routing` | Log routing rules |
| `builtin:openpipeline.logs.security-context` | Security context for log pipelines |
| `builtin:openpipeline.events.pipelines` | Event processing pipelines |
| `builtin:openpipeline.events.sdlc` | SDLC event pipelines |
| `builtin:openpipeline.business-events.pipelines` | Business event pipelines |

## Listing Settings Objects

```bash
# List objects for a specific schema
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment

# Output as JSON for scripting
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment -o json

# Output as YAML
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment -o yaml
```

## Creating Settings Objects

Create settings objects from a YAML file, specifying the schema and scope:

```bash
# Create a settings object
dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment

# Preview changes without applying
dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --dry-run

# Use template variables for environment-specific values
dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment \
  --set env=production --set retention=90
```

### Example Pipeline YAML

```yaml
customId: my-log-pipeline
displayName: Production Log Pipeline
processing:
  - name: extract-severity
    description: Extract severity from log message
    processor:
      type: dql
      dqlScript: |
        parse content, "LD 'severity=' WORD:severity"
  - name: normalize-timestamp
    description: Normalize timestamp format
    processor:
      type: dql
      dqlScript: |
        fieldsAdd timestamp = toTimestamp(raw_timestamp)
storage:
  catch_all:
    bucketName: default_logs
    enabled: true
  custom:
    - bucketName: logs-production
      enabled: true
      matcher: severity == "ERROR" or severity == "CRITICAL"
routing:
  catch_all:
    pipelineId: default
    enabled: true
```

## Updating Settings Objects

Settings objects use optimistic locking to prevent conflicting updates. When you retrieve an object, it includes a version identifier. You must provide this version when updating:

```bash
# Get current object (includes version in metadata)
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment -o yaml > pipeline.yaml

# Edit the file, then apply the update
dtctl apply -f pipeline.yaml
```

The version is automatically handled when using `dtctl apply` with a file that was previously retrieved via `dtctl get`.

## Deleting Settings Objects

```bash
# Delete a settings object by its object ID
dtctl delete settings <object-id>
```

## OpenPipeline Configuration Workflow

A typical workflow for configuring OpenPipeline via the Settings API:

1. **Discover schemas** -- list available OpenPipeline schemas to find the one you need:
   ```bash
   dtctl get settings-schemas --name "openpipeline"
   ```

2. **Inspect the schema** -- understand required fields and constraints:
   ```bash
   dtctl describe settings-schema builtin:openpipeline.logs.pipelines
   ```

3. **Check existing configuration** -- see what is already configured:
   ```bash
   dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment -o yaml
   ```

4. **Prepare your YAML** -- write the settings object definition.

5. **Dry-run** -- validate your configuration before applying:
   ```bash
   dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --dry-run
   ```

6. **Apply** -- create or update the settings object:
   ```bash
   dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment
   ```

7. **Verify** -- confirm the settings were applied:
   ```bash
   dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment
   ```

## Multi-Environment Deployment

Use template variables to deploy the same settings configuration across multiple environments:

{% raw %}
```yaml
# pipeline-template.yaml
customId: "{{ .env }}-log-pipeline"
displayName: "{{ .env | title }} Log Pipeline"
storage:
  catch_all:
    bucketName: "logs-{{ .env }}"
    enabled: true
  custom:
    - bucketName: "logs-{{ .env }}-errors"
      enabled: true
      matcher: severity == "ERROR"
```

```bash
# Deploy to staging
dtctl create settings -f pipeline-template.yaml \
  --schema builtin:openpipeline.logs.pipelines --scope environment \
  --set env=staging

# Deploy to production
dtctl ctx use production
dtctl create settings -f pipeline-template.yaml \
  --schema builtin:openpipeline.logs.pipelines --scope environment \
  --set env=production
```
{% endraw %}

## Migration from OpenPipeline Commands

> **Note:** Direct OpenPipeline commands (`dtctl get openpipeline`, etc.) have been removed. All OpenPipeline configuration is now managed through the Settings API using the `builtin:openpipeline.*` schemas. This provides a consistent interface and supports features like dry-run, template variables, and multi-environment deployment.

## Required Scopes

| Operation | Required Scope |
|-----------|---------------|
| List / Get / Describe | `settings:objects:read` |
| Create / Update / Delete | `settings:objects:write` |
