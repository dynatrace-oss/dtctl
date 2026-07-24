---
layout: docs
title: Live Debugger
---

Dynatrace Live Debugger lets you set non-breaking breakpoints on running applications, capture variable snapshots, and inspect them without stopping your services. dtctl provides full lifecycle management for breakpoints and supports decoding captured snapshots via DQL.

## Overview

Key capabilities:

- **Breakpoints** — Create, list, update, and delete non-breaking breakpoints on live applications
- **Workspace filters** — Scope breakpoints to specific Kubernetes namespaces, hosts, or process groups
- **Snapshot decoding** — Query and decode captured variable snapshots using DQL

## Prerequisites

Live Debugger requires OAuth authentication. Ensure you are logged in before using these commands:

```bash
dtctl auth login
```

## Configure Workspace Filters

Workspace filters scope which monitored processes are eligible for breakpoints, which is important in large environments to avoid unnecessary overhead. Filters are workspace-scoped: a single filter set applies to **every** breakpoint in the workspace. Because of this, changing the filters re-scopes not only breakpoints you create afterwards but **all existing breakpoints** in the workspace as well.

You can set filters in the same step as creating a breakpoint (see [Create a Breakpoint](#create-a-breakpoint)), so a separate filter command is not required. Use the standalone command when you want to set or change filters without creating a breakpoint:

```bash
# Set workspace filters (e.g. target a specific Kubernetes namespace)
dtctl update breakpoint --filters k8s.namespace.name:prod
```

Because changing filters re-scopes existing breakpoints, `dtctl` counts the active breakpoints that would be affected and asks you to confirm before applying the change:

```text
This will change the workspace filters for 3 active breakpoints. Continue? [y/N]:
```

Pass `--yes` (`-y`) to skip the prompt, e.g. in scripts:

```bash
dtctl update breakpoint --filters k8s.namespace.name:prod --yes
```

> **Note:** Breakpoints that have auto-disabled (for example after reaching their hit limit) are not counted. In non-interactive contexts (`--plain` or auto-detected agent mode) the change proceeds without prompting.

## Breakpoint Lifecycle

### Create a Breakpoint

```bash
# Create a breakpoint at a specific source location (file:line)
dtctl create breakpoint com/example/MyService.java:42

# Set workspace filters and create the breakpoint in one step
dtctl create breakpoint com/example/MyService.java:42 --filters k8s.namespace.name:prod
```

When `--filters` changes the workspace filters, it also re-scopes existing breakpoints (see [Configure Workspace Filters](#configure-workspace-filters)), so you are prompted to confirm unless you pass `--yes`.

### List Breakpoints

```bash
# List all breakpoints
dtctl get breakpoints
```

### Describe a Breakpoint

```bash
# View full details of a breakpoint including hit count and status
dtctl describe breakpoint bp-abc123
```

### Update a Breakpoint

```bash
# Add a conditional expression to a breakpoint
dtctl update breakpoint bp-abc123 --condition "userId != null"

# Disable a breakpoint without deleting it
dtctl update breakpoint bp-abc123 --enabled=false
```

### Delete Breakpoints

```bash
# Delete a single breakpoint by ID
dtctl delete breakpoint bp-abc123

# Delete all breakpoints at a specific source location (file:line)
dtctl delete breakpoint com/example/MyService.java:42

# Delete all breakpoints in the workspace
dtctl delete breakpoint --all
```

## Decoded Snapshots

When a breakpoint is hit, the runtime captures a snapshot of local variables and the call stack. You can query these snapshots using DQL and have dtctl decode them automatically.

```bash
# Query snapshots and decode variable data
dtctl query "fetch application.snapshots | limit 10" --decode-snapshots

# Filter snapshots by a specific breakpoint using its immutable ID
# The breakpoint ID is shown in: dtctl create breakpoint / dtctl get breakpoints / dtctl describe breakpoint
dtctl query "fetch application.snapshots | filter breakpoint.id == toUid(\"00000000000000000000000000096294\")" --decode-snapshots
```

### Full vs Simplified Decoding

By default, `--decode-snapshots` (equivalent to `--decode-snapshots=simplified`) produces a simplified view that shows variable names and values in a human-readable format. For the full decoded tree (including nested objects and type annotations), use `--decode-snapshots=full`:

```bash
# Full decoding with complete object graphs
dtctl query "fetch application.snapshots | limit 5" \
  --decode-snapshots=full
```

## Safety and Dry-Run

Live Debugger commands that modify state (create, update, delete) support safety checks and dry-run mode:

```bash
# Preview what would be created without actually creating it
dtctl create breakpoint com/example/MyService.java:42 --dry-run

# Safety checks prevent accidental modifications in read-only contexts
```

## Example End-to-End Workflow

```bash
# 1. Log in with OAuth
dtctl auth login

# 2. Create a breakpoint on a suspect line, setting workspace filters in the same step
dtctl create breakpoint com/example/PaymentService.java:87 --filters k8s.namespace.name:prod

# 3. List breakpoints to confirm
dtctl get breakpoints

# 4. Wait for the breakpoint to be hit, then query snapshots
dtctl query "fetch application.snapshots \
  | filter source.file == 'com/example/PaymentService.java' \
  | limit 5" --decode-snapshots

# 5. Inspect the decoded variables to diagnose the issue

# 6. Clean up — delete the breakpoint
dtctl delete breakpoint com/example/PaymentService.java:87
```
