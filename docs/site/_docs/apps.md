---
layout: docs
title: App Engine
---

Dynatrace App Engine lets you extend the platform with custom and built-in applications. dtctl provides commands to list, inspect, and delete apps, as well as discover and execute app functions and intents.

## Listing and Viewing Apps

```bash
# List all installed apps
dtctl get apps

# Describe a specific app
dtctl describe app app-123
```

## App Functions

App functions are server-side endpoints exposed by Dynatrace apps. You can discover, inspect, and execute them directly from the CLI.

### Discover Functions

```bash
# List all available functions across all apps
dtctl get functions

# List functions for a specific app with extra detail
dtctl get functions --app dynatrace.automations -o wide
```

### Describe a Function

```bash
# View function details including parameters and schema
dtctl describe function dynatrace.automations/execute-dql-query
```

### Execute a Function

`dtctl exec function` has **two modes**:

1. **App function** — invoke a function exposed by an installed app (`app-id/function-name`).
2. **Ad-hoc JavaScript** — run JavaScript directly, without deploying an app.

The mode is chosen by the flags you pass: if `--code` or `-f`/`--file` is
present, dtctl runs in **ad-hoc mode** (and ignores any `app-id/function-name`
argument, `--method`, and `--defer`). Otherwise it runs in **app-function mode**.

The command has aliases `fn` and `func`.

#### App Functions

```bash
# GET (default method, no payload)
dtctl exec function dynatrace.automations/execute-dql-query

# POST with an inline JSON payload
dtctl exec function dynatrace.automations/execute-dql-query \
  --method POST \
  --payload '{"query":"fetch logs | limit 5"}'

# Payload from a file, or from stdin with "-"
dtctl exec function dynatrace.automations/execute-dql-query --method POST --data payload.json
cat payload.json | dtctl exec function dynatrace.automations/execute-dql-query --method POST --data -

# Defer execution (async, for resumable functions) — returns a handle instead of the result
dtctl exec function dynatrace.automations/long-running --method POST --payload '{}' --defer
```

`--method` accepts `GET` (default), `POST`, `PUT`, `PATCH`, or `DELETE`.
`--payload` takes an inline JSON string; `--data` reads the same payload from a
file (or `-` for stdin). Pass at most one of the two.

**Payload discovery:** If you're unsure what fields a function expects, try executing it with an empty payload (`--payload '{}'`). The error response will typically list the required fields and their types.

#### Ad-hoc JavaScript

Run a function's worth of JavaScript without publishing an app — useful for
quick platform automation and for prototyping app functions:

```bash
# Inline code
dtctl exec function --code 'export default async function () { return "hello" }'

# Code from a file (use -f - to read the code from stdin)
dtctl exec function -f script.js

# Code from a file, with an input payload passed to the function
dtctl exec function -f script.js --payload '{"input":"data"}'
dtctl exec function -f script.js --data payload.json
```

In ad-hoc mode `--payload` / `--data` supply the input passed to the code; the
`--method` and `--defer` flags do not apply.

#### Flag Reference

| Flag | Mode | Purpose |
|------|------|---------|
| `--method` | app function | HTTP method: `GET` (default), `POST`, `PUT`, `PATCH`, `DELETE` |
| `--payload` | both | Inline JSON payload / input |
| `--data` | both | Read the payload / input from a file (`-` = stdin) |
| `--defer` | app function | Defer execution (async, for resumable functions) |
| `--code` | ad-hoc | JavaScript source to execute inline (selects ad-hoc mode) |
| `-f`, `--file` | ad-hoc | Read JavaScript source from a file (`-` = stdin; selects ad-hoc mode) |

### Common Functions

| App | Function | Description |
|-----|----------|-------------|
| `dynatrace.automations` | `execute-dql-query` | Run a DQL query |
| `dynatrace.automations` | `send-email` | Send an email notification |
| `dynatrace.automations` | `send-slack-message` | Post a message to Slack |
| `dynatrace.automations` | `create-jira-issue` | Create a Jira issue |

## App Intents

Intents provide a deep-linking mechanism to navigate into specific app views with contextual data.

### Discover Intents

```bash
# List all registered intents
dtctl get intents

# Describe a specific intent to see its parameters
dtctl describe intent dynatrace.distributedtracing/view-trace
```

### Find Matching Intents

```bash
# Find intents that accept a given set of data fields
dtctl find intents --data trace_id=abc123
```

### Generate URLs

```bash
# Generate a deep-link URL and open it in the browser
dtctl open intent dynatrace.distributedtracing/view-trace \
  --data trace_id=abc123 \
  --browser
```

**Use cases:** Deep linking from alert notifications to the relevant trace or dashboard, scripted navigation for runbooks, and building custom integrations that open Dynatrace views with pre-filled context.

## Deleting Apps

```bash
# Delete an app by ID
dtctl delete app app-123
```

## Required Scopes

| Scope | Used By |
|-------|---------|
| `app-engine:apps:run` | Listing, describing, and deleting apps |
| `app-engine:functions:run` | Executing app functions |
