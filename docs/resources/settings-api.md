<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# Settings

The Dynatrace Settings API provides a unified way to manage configuration objects across the platform. It is the primary mechanism for configuring OpenPipeline, built-in settings, and many other Dynatrace features. Direct OpenPipeline commands have been removed; all OpenPipeline configuration goes through this Settings API using `builtin:openpipeline.*` schemas.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | settings | `dtctl create settings` | Create resources from files | yes | write |
| delete | settings | `dtctl delete settings` | Delete resources | yes | delete |
| describe | settings | `dtctl describe settings` | Show details of a specific resource | no | read |
| edit | setting | `dtctl edit setting` | Edit a resource | yes | write |
| get | settings | `dtctl get settings` | Display one or many resources | no | read |
| describe | settings-schema | `dtctl describe settings-schema` | Show details of a specific resource | no | read |
| get | settings-schemas | `dtctl get settings-schemas` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `app-settings:objects:read`, `settings:objects:read`, `settings:schemas:read` |
| write | `settings:objects:write` |


## Output

`get settings` returns an array of settings objects for the given schema; each includes an object ID and a version identifier used for optimistic locking on update. `-o yaml`/`-o json` are the practical formats here since objects are usually round-tripped through export/edit/apply. `describe settings-schema` shows a schema's fields and constraints.

## Examples

Discover schemas, then inspect one:

```bash
# List all settings schemas, or filter by name
dtctl get settings-schemas
dtctl get settings-schemas --name "openpipeline"

# Describe a specific schema to see its fields and constraints
dtctl describe settings-schema builtin:openpipeline.logs.pipelines
```

List objects for a schema:

```bash
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment -o json
```

Create a settings object from a YAML file:

```bash
# Create
dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment

# Preview changes without applying
dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment --dry-run

# Template variables for environment-specific values
dtctl create settings -f pipeline.yaml --schema builtin:openpipeline.logs.pipelines --scope environment \
  --set env=production --set retention=90
```

Update via export/edit/apply. The version is handled automatically when the file was retrieved via `dtctl get`:

```bash
dtctl get settings --schema builtin:openpipeline.logs.pipelines --scope environment -o yaml > pipeline.yaml
# edit pipeline.yaml
dtctl apply -f pipeline.yaml
```

Bulk update: the array returned by `get` can be edited and applied back directly. Each item is applied individually, so if some fail the rest still succeed and a summary error reports the failures:

```bash
dtctl get settings --schema builtin:rum.web.enablement -o yaml > rum-settings.yaml
# edit rum-settings.yaml
dtctl apply -f rum-settings.yaml
```

Delete a settings object by its object ID:

```bash
dtctl delete settings <object-id>
```

Deploy the same settings configuration across environments with template variables:

```bash
dtctl create settings -f pipeline-template.yaml \
  --schema builtin:openpipeline.logs.pipelines --scope environment \
  --set env=staging

dtctl ctx use production
dtctl create settings -f pipeline-template.yaml \
  --schema builtin:openpipeline.logs.pipelines --scope environment \
  --set env=production
```

