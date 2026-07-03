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

Workspace filters let you scope which monitored processes are eligible for breakpoints. This is important in large environments to avoid unnecessary overhead.

```bash
# Set a workspace filter to target a specific Kubernetes namespace
dtctl update breakpoint --filters k8s.namespace.name:prod
```

## Breakpoint Lifecycle

### Create a Breakpoint

```bash
# Create a breakpoint at a specific source location
dtctl create breakpoint --file com/example/MyService.java --line 42
```

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

# Delete all breakpoints at a specific source location
dtctl delete breakpoint --file com/example/MyService.java --line 42

# Delete all breakpoints in the workspace
dtctl delete breakpoint --all
```

## Decoded Snapshots

When a breakpoint is hit, the runtime captures a snapshot of local variables and the call stack. You can query these snapshots using DQL and have dtctl decode them automatically.

```bash
# Query snapshots and decode variable data
dtctl query "fetch application.snapshots | limit 10" --decode-snapshots
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
dtctl create breakpoint --file com/example/MyService.java --line 42 --dry-run

# Safety checks prevent accidental modifications in read-only contexts
```

## Example End-to-End Workflow

```bash
# 1. Log in with OAuth
dtctl auth login

# 2. Set workspace filters to target production
dtctl update breakpoint --filters k8s.namespace.name:prod

# 3. Create a breakpoint on a suspect line
dtctl create breakpoint --file com/example/PaymentService.java --line 87

# 4. List breakpoints to confirm
dtctl get breakpoints

# 5. Wait for the breakpoint to be hit, then query snapshots
dtctl query "fetch application.snapshots \
  | filter source.file == 'com/example/PaymentService.java' \
  | limit 5" --decode-snapshots

# 6. Inspect the decoded variables to diagnose the issue

# 7. Clean up — delete the breakpoint
dtctl delete breakpoint --file com/example/PaymentService.java --line 87
```
