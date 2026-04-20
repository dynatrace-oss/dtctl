---
layout: docs
title: "Dashboards & Notebooks"
---

Dynatrace dashboards and notebooks are managed as documents. dtctl supports the full lifecycle: list, view, create, edit, share, version-track, and delete — for both resource types.

## Listing Documents

```bash
# List all dashboards
dtctl get dashboards

# List all notebooks
dtctl get notebooks

# Filter by name substring
dtctl get dashboards --name "Production"

# Show only your own dashboards
dtctl get dashboards --mine

# Wide output with owner, tile count, and last modified date
dtctl get dashboards -o wide
```

## Describing a Dashboard

```bash
# By name (interactive disambiguation if multiple match)
dtctl describe dashboard "Production Overview"

# By ID
dtctl describe dashboard dash-123
```

The describe view shows metadata (owner, sharing, version), a tile summary, and the dashboard URL in Dynatrace.

## Editing a Dashboard

Open a dashboard in your `$EDITOR`:

```bash
dtctl edit dashboard dash-123
```

On save, dtctl computes the diff and updates only the changed fields.

## Creating and Applying Dashboards

```bash
# Create (fails if the document ID already exists)
dtctl create dashboard -f dashboard.yaml

# Apply (creates if new, updates if existing — idempotent)
dtctl apply -f dashboard.yaml

# First apply: stamp the generated ID back into the file so future applies update in place
dtctl apply -f dashboard.yaml --write-id

# Forgot --write-id on the first run? Recover without creating another duplicate:
dtctl apply -f dashboard.yaml --write-id --id <id-from-first-run>

# CI/scripting: apply a template to a specific known dashboard
dtctl apply -f dashboard.yaml --id $DASHBOARD_ID
```

On success, dtctl prints the tile count and a direct URL to the dashboard in Dynatrace.

### Round-Trip Export / Import

Export a dashboard to YAML, modify it, and re-apply:

```bash
# Export
dtctl get dashboard abc-123 -o yaml > dashboard.yaml

# Edit locally
$EDITOR dashboard.yaml

# Re-import
dtctl apply -f dashboard.yaml
```

### Example Dashboard YAML

```yaml
kind: dashboard
name: Production Overview
description: Key metrics for production services
tiles:
  - title: Error Rate
    type: data
    query: >
      timeseries avg(dt.service.request.failure_rate),
      filter: environment == "production"
    visualization: lineChart
    position:
      x: 0
      y: 0
      w: 6
      h: 4
  - title: Active Users
    type: data
    query: >
      timeseries count(dt.rum.user_session.count),
      filter: application == "webapp"
    visualization: singleValue
    position:
      x: 6
      y: 0
      w: 3
      h: 4
  - title: Deployment Log
    type: markdown
    content: |
      ## Recent Deployments
      Check the [deployment tracker](#) for details.
    position:
      x: 9
      y: 0
      w: 3
      h: 4
```

## Sharing

Control access to dashboards and notebooks:

```bash
# Grant read-write access to a user
dtctl share dashboard dash-123 --user user@example.com --access read-write

# Grant read-only access
dtctl share dashboard dash-123 --user viewer@example.com --access read

# Revoke access
dtctl unshare dashboard dash-123 --user user@example.com
```

## Version History (Snapshots)

Dynatrace keeps document snapshots. View and restore previous versions:

```bash
# List all versions
dtctl history dashboard dash-123

# Restore a specific version
dtctl restore dashboard dash-123 5
```

## Watch Mode

Monitor dashboards in real time — additions, modifications, and deletions are highlighted:

```bash
dtctl get dashboards --watch
```

## Deleting Dashboards

Deleting a dashboard moves it to the trash, where it is retained for 30 days before permanent removal:

```bash
dtctl delete dashboard dash-123
```

### Trash Management

```bash
# List items in the trash
dtctl get trash

# Inspect a trashed document
dtctl describe trash dash-123

# Restore a document from the trash
dtctl restore trash dash-123

# Permanently delete (bypasses the 30-day retention)
dtctl delete trash dash-123 --permanent
```
