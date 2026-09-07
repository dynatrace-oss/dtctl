<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# Filter Segments

Grail filter segments are reusable, named filter definitions that logically structure observability data across the Dynatrace platform. They act as query-time context, where Grail evaluates only the segment includes relevant to the queried data type. dtctl provides full CRUD management and query-time integration for segments (see [DQL Queries](dql-queries) for using segments with `dtctl query`).

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | segment | `dtctl create segment` | Create resources from files | yes | write |
| delete | segment | `dtctl delete segment` | Delete resources | yes | delete |
| describe | segment | `dtctl describe segment` | Show details of a specific resource | no | read |
| edit | segment | `dtctl edit segment` | Edit a resource | yes | write |
| get | segments | `dtctl get segments` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `storage:filter-segments:delete` |
| read | `storage:filter-segments:read` |
| write | `storage:filter-segments:write` |


## Output

`get segments` prints a compact table; `-o wide` adds description and owner. `describe segment` shows a segment's includes, variables, and visibility, for example:

```
Name:          my-k8s-segment
UID:           abc123-def456
Description:   Filters data for Kubernetes cluster alpha
Public:        Yes
Owner:         user@example.invalid
Version:       3

Includes:
  DATA OBJECT         FILTER
  All data objects    k8s.cluster.name = "alpha"
  Logs                dt.system.bucket = "custom-logs"

Variables:
  Type:     query
  Value:    data record(ns="namespace-a"), record(ns="namespace-b")
```

## Examples

```bash
# List all segments
dtctl get segments

# Wide output (includes description and owner)
dtctl get segments -o wide

# JSON for scripting
dtctl get segments -o json

# Watch for changes in real time
dtctl get segments --watch
```

Describe a segment by UID or by name (interactive disambiguation if ambiguous):

```bash
dtctl describe segment abc123-def456
dtctl describe segment my-k8s-segment
```

Define a segment in YAML and create or apply it:

```yaml
name: my-k8s-segment
description: Filters data for Kubernetes cluster alpha
isPublic: true
includes:
  - dataObject: _all_data_object
    filter: 'k8s.cluster.name = "alpha"'
  - dataObject: logs
    filter: 'dt.system.bucket = "custom-logs"'
variables:
  type: query
  value: 'data record(ns="namespace-a"), record(ns="namespace-b")'
```

```bash
# Create (fails if the segment already exists)
dtctl create segment -f segment.yaml

# Apply (creates if new, updates if existing)
dtctl apply -f segment.yaml

# Dry-run to preview what would happen
dtctl create segment -f segment.yaml --dry-run
```

Edit a segment in your editor — this opens the segment YAML in `$EDITOR` and applies the changes on save; the optimistic locking version is handled automatically:

```bash
dtctl edit segment my-k8s-segment
```

Apply segments at query time to narrow results (AND-combined when multiple are specified):

```bash
# Apply a single segment, or by short form
dtctl query "fetch logs | limit 10" --segment my-segment-uid
dtctl query "fetch logs | limit 10" -S my-segment-uid

# Bind variables inline (URL-query style)
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001"

# Use a YAML file for complex cases with multiple segments and variables
dtctl query "fetch logs | limit 10" --segments-file segments.yaml
```

Delete a segment (dtctl prompts for confirmation in interactive mode; use `--plain` to skip it in CI):

```bash
dtctl delete segment abc123-def456
dtctl delete segment abc123-def456 -y
```

