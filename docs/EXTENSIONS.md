# Extensions 2.0

dtctl provides full support for managing Dynatrace Extensions 2.0 — listing extensions, inspecting versions and feature sets, and managing monitoring configurations.

## Listing Extensions

```bash
# List all installed extensions
dtctl get extensions

# Filter by name (case-insensitive substring match)
dtctl get extensions --name "sql"

# Output as JSON
dtctl get extensions -o json
```

## Viewing Extension Versions

```bash
# List all versions of an extension
dtctl get extension com.dynatrace.extension.postgres

# JSON output (useful for scripting)
dtctl get extension com.dynatrace.extension.postgres -o yaml
```

## Describing an Extension

The `describe` command shows full details including versions, data sources, feature sets, variables, and monitoring configurations:

```bash
# Describe the active version
dtctl describe extension com.dynatrace.extension.postgres

# Describe a specific version
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3

# Get structured output for automation
dtctl describe extension com.dynatrace.extension.postgres -o json

# Output only the monitoring configuration JSON Schema
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3 --monitoring-configuration-schema

# Strip documentation/display noise from the schema (useful for scripting)
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3 --monitoring-configuration-schema --no-fluff

# List available ActiveGate groups for a specific version
dtctl describe extension com.dynatrace.extension.postgres --version 2.9.3 --active-gate-groups
```

## Installing Extensions

### Upload a Custom Extension

Upload a locally-built extension zip file:

```bash
dtctl create extension -f my-extension.zip

# Preview without applying
dtctl create extension -f my-extension.zip --dry-run
```

### Install from the Hub

Install an extension directly from the Dynatrace Hub catalog:

```bash
# Install the latest available version
dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring

# Install a specific version
dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring --version 1.2.3

# Preview without applying
dtctl create extension --hub-extension com.dynatrace.extension.host-monitoring --dry-run
```

> **Required scope**: `extensions:definitions:write`

## Monitoring Configurations

Monitoring configurations define how an extension collects data for a specific scope (e.g., a host or host group).

### Listing Configurations

```bash
# List all monitoring configs for an extension
dtctl get extension-configs com.dynatrace.extension.postgres

# Get a specific config by ID
dtctl get extension-config com.dynatrace.extension.postgres --config-id <id>
```

> **Note:** dtctl adds `type` and `extensionName` fields to monitoring configuration responses for internal resource detection. These fields are not present in the raw Dynatrace REST API response.

### Describing a Configuration

```bash
# Show full details of a monitoring configuration
dtctl describe extension-config com.dynatrace.extension.postgres --config-id <id>

# JSON output
dtctl describe extension-config com.dynatrace.extension.postgres --config-id <id> -o json
```

### Creating a Configuration

Create a YAML file (e.g., `config.yaml`):

```yaml
scope: HOST-ABC123
value:
  enabled: true
  description: "Production PostgreSQL monitoring"
  version: "2.9.3"
  featureSets:
    - default
```

Apply it:

```bash
# Create a new monitoring configuration
dtctl apply extension-config com.dynatrace.extension.postgres -f config.yaml

# Preview without applying
dtctl apply extension-config com.dynatrace.extension.postgres -f config.yaml --dry-run

# Override scope from the command line
dtctl apply extension-config com.dynatrace.extension.postgres -f config.yaml --scope HOST-XYZ789
```

### Updating a Configuration

To update, include the `objectId` in the file:

```yaml
objectId: existing-config-id
scope: HOST-ABC123
value:
  enabled: false
  description: "Updated - disabled for maintenance"
  version: "2.9.3"
```

```bash
dtctl apply extension-config com.dynatrace.extension.postgres -f config.yaml
```

### Using with Generic Apply

Extension configs can also be applied via `dtctl apply -f` by including a `type` field:

```yaml
type: extension_monitoring_config
extensionName: com.dynatrace.extension.postgres
scope: HOST-ABC123
value:
  enabled: true
  version: "2.9.3"
```

```bash
dtctl apply -f extension-config.yaml
```

## Using Template Variables

Templates let you reuse config files across environments:

```yaml
scope: "{{.host}}"
value:
  enabled: true
  description: "Monitoring for {{.env}}"
  version: "{{.version}}"
```

```bash
dtctl apply extension-config com.dynatrace.extension.postgres \
  -f config.yaml \
  --set host=HOST-ABC123 \
  --set env=production \
  --set version=2.9.3
```

## Aliases

All extension commands support short aliases:

| Command             | Aliases                                         |
| ------------------- | ----------------------------------------------- |
| `extensions`        | `extension`, `ext`, `exts`                      |
| `extension-configs` | `extension-config`, `ext-configs`, `ext-config` |

```bash
# These are equivalent
dtctl get extensions
dtctl get ext

dtctl describe ext com.dynatrace.extension.postgres
```

## Hub Catalog

The Dynatrace Hub catalog lets you browse available extensions before installing them. These commands are read-only.

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

| Command                  | Aliases                 |
| ------------------------ | ----------------------- |
| `hub-extensions`         | `hub-extension`         |
| `hub-extension-releases` | `hub-extension-release` |
