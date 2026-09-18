---
layout: docs
title: "Filter Segments"
---

Grail filter segments are reusable, named filter definitions that logically structure observability data across the Dynatrace platform. They act as query-time context — where Grail evaluates only the segment includes relevant to the queried data type. dtctl provides full CRUD management and query-time integration for segments.

## Listing Segments

```bash
# List all segments
dtctl get segments

# Wide output (includes description and owner)
dtctl get segments -o wide

# JSON or YAML for scripting
dtctl get segments -o json
```

## Describing a Segment

Get full details for a specific segment, including its includes, variables, and visibility:

```bash
# By UID
dtctl describe segment abc123-def456

# By name (interactive disambiguation if ambiguous)
dtctl describe segment my-k8s-segment
```

Example output:

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

## Creating and Applying Segments

Define a segment in YAML and create or update it:

```bash
# Create (fails if the segment already exists)
dtctl create segment -f segment.yaml

# Apply (creates if new, updates if existing — idempotent)
dtctl apply -f segment.yaml

# Dry-run to preview what would happen
dtctl create segment -f segment.yaml --dry-run
```

### Example Segment YAML

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

| Field         | Description                                                                 |
|---------------|-----------------------------------------------------------------------------|
| `name`        | Human-readable segment name                                                 |
| `description` | Optional description                                                        |
| `isPublic`    | `true` for environment-wide visibility, `false` for owner-only (default)    |
| `includes`    | Array of filter rules. Each specifies a `dataObject` and a `filter` expression. Includes are OR-combined within a segment. |
| `dataObject`  | Data type to filter: `logs`, `spans`, `events`, `metrics`, `entities`, or `_all_data_object` for all |
| `filter`      | DQL filter expression applied to the data object. dtctl automatically converts DQL to the API's internal JSON AST format on create/update, and converts back to DQL on get/describe. You can also provide a raw JSON AST string (starting with `{`) which will be passed through unchanged. |
| `variables`   | Optional variable definition with `type` and `value` fields                 |

## Editing a Segment

Open a segment in your editor, modify it, and save to update:

```bash
dtctl edit segment my-k8s-segment
```

This opens the segment YAML in `$EDITOR`, then applies the changes on save. The optimistic locking version is handled automatically.

## Using Segments in Queries

Segments can be applied at query time to filter results. See [DQL Queries](dql-queries) for full details.

```bash
# Apply a single segment
dtctl query "fetch logs | limit 10" --segment my-segment-uid

# Apply multiple segments (AND-combined per Grail semantics)
dtctl query "fetch logs | limit 10" --segment seg-1 --segment seg-2

# Bind variables inline (URL-query style)
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001"

# Multiple values for a variable
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001,HOST-002"

# Multiple variables on one segment
dtctl query "fetch logs | limit 10" -S "my-segment?host=HOST-001&ns=production"

# Use a YAML file for complex cases
dtctl query "fetch logs | limit 10" --segments-file segments.yaml

# By name (resolved via API)
dtctl query "fetch logs | limit 10" --segment my-k8s-segment
```

## Watch Mode

Monitor segments in real time:

```bash
dtctl get segments --watch
```

Press `Ctrl+C` to stop watching.

## Deleting a Segment

```bash
# Delete with confirmation prompt
dtctl delete segment abc123-def456

# Skip confirmation
dtctl delete segment abc123-def456 -y
```

dtctl prompts for confirmation in interactive mode. Use `--plain` to skip the prompt in scripts and CI pipelines.

## Aliases

Segments support multiple aliases for convenience:

| Context    | Primary      | Aliases                                  |
|------------|--------------|------------------------------------------|
| `get`      | `segments`   | `segment`, `seg`, `filter-segments`, `filter-segment` |
| `describe` | `segment`    | `seg`, `filter-segment`                  |
| `create`   | `segment`    | `seg`, `filter-segment`                  |
| `edit`     | `segment`    | `seg`, `filter-segment`                  |
| `delete`   | `segment`    | `segments`, `seg`, `filter-segment`, `filter-segments` |
