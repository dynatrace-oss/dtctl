---
layout: docs
title: Workflows
---

Dynatrace Automation workflows let you orchestrate multi-step processes — from scheduled health checks to incident remediation. dtctl gives you full lifecycle management: list, inspect, create, execute, and monitor workflows from your terminal.

## Listing and Viewing Workflows

```bash
# List all workflows (compact table)
dtctl get workflows

# Wide output with additional columns (owner, trigger type, last run)
dtctl get workflows -o wide

# Output as JSON or YAML for scripting
dtctl get workflows -o json
dtctl get workflows -o yaml
```

To inspect a single workflow in detail:

```bash
# By name (interactive disambiguation if multiple match)
dtctl describe workflow "My Workflow"

# By ID (exact match)
dtctl describe workflow workflow-123
```

## Editing a Workflow

Open a workflow in your `$EDITOR`, make changes, and save to update it in place:

```bash
dtctl edit workflow workflow-123
```

dtctl downloads the current definition, opens it in your editor, and applies the diff on save — similar to `kubectl edit`.

## Creating and Applying Workflows

Create a workflow from a YAML file:

```bash
# Create (fails if the workflow already exists)
dtctl create workflow -f my-workflow.yaml

# Apply (creates if new, updates if existing — idempotent)
dtctl apply -f my-workflow.yaml

# First apply: stamp the generated ID back into the file so future applies update in place
dtctl apply -f my-workflow.yaml --write-id

# Forgot --write-id on the first run? Recover without creating another duplicate:
dtctl apply -f my-workflow.yaml --write-id --id <id-from-first-run>

# CI/scripting: apply a template to a specific known workflow
dtctl apply -f my-workflow.yaml --id $WORKFLOW_ID
```

### Example Workflow YAML

```yaml
title: Daily Health Check
description: Runs a health check every morning and posts results to Slack
owner: Operations
trigger:
  schedule:
    rule: "0 8 * * *"   # Every day at 08:00 UTC
    timezone: UTC
    isActive: true
tasks:
  check_health:
    name: check_health
    action: dynatrace.automations:run-javascript
    description: Run health check script
    input:
      script: |
        import { execution } from '@dynatrace-sdk/automation-utils';
        export default async function ({ executionId }) {
          const exe = await execution(executionId);
          const params = exe.params;
          console.log(`Running health check for env: ${params.env || 'default'}`);
          return { status: 'healthy', checkedAt: new Date().toISOString() };
        }
    position:
      x: 0
      y: 1
  notify_slack:
    name: notify_slack
    action: dynatrace.slack:slack-send-message
    description: Post results to Slack
    input:
      channel: "#ops-alerts"
      message: "Health check completed: {% raw %}{{ result('check_health').status }}{% endraw %}"
    conditions:
      states:
        check_health: OK
    position:
      x: 0
      y: 2
```

## Executing Workflows

Trigger a workflow execution on demand:

```bash
# Fire and forget
dtctl exec workflow workflow-123

# Pass parameters and wait for completion
dtctl exec workflow workflow-123 --params env=prod --wait

# Wait and display task results when finished
dtctl exec workflow workflow-123 --wait --show-results
```

The `--wait` flag polls the execution until it reaches a terminal state (success, error, or cancelled). `--show-results` prints the output of each task.

## Viewing Executions

```bash
# List recent workflow executions
dtctl get workflow-executions

# Short alias
dtctl get wfe

# Describe a specific execution (status, timing, task breakdown)
dtctl describe wfe exec-456

# Stream execution logs in real time
dtctl logs wfe exec-456 --follow
```

## Task Results

Retrieve the output of a specific task within an execution:

```bash
dtctl get wfe-task-result exec-456 --task my_task

# JSON output for programmatic consumption
dtctl get wfe-task-result exec-456 --task my_task -o json
```

## Watch Mode

Monitor workflows in real time. dtctl highlights additions, modifications, and deletions as they happen:

```bash
dtctl get workflows --watch
```

Press `Ctrl+C` to stop watching.

## Version History

View and restore previous versions of a workflow:

```bash
# List version history
dtctl history workflow workflow-123

# Restore version 5
dtctl restore workflow workflow-123 5
```

## Deleting Workflows

```bash
# Delete by ID
dtctl delete workflow workflow-123

# Delete by name
dtctl delete workflow "Daily Health Check"
```

Deletion is permanent. dtctl prompts for confirmation in interactive mode; use `--plain` to skip the prompt (e.g., in CI pipelines).
