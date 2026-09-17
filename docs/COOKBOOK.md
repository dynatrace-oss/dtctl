# dtctl Cookbook

<!-- Migrated from the standalone docs site; SME to verify. -->

Worked, end-to-end examples that combine multiple dtctl flags and commands to solve
a specific problem. For the full flag-by-flag reference, see the generated command
catalog (`dtctl commands --full -o json`) or run `dtctl <verb> <resource> --help`.

## Watch Mode

All `get` commands support watch mode for real-time monitoring:

```bash
dtctl get workflows --watch                    # Watch all
dtctl get workflows --watch --interval 5s      # Custom interval
dtctl get workflows --watch --watch-only       # Only show changes
dtctl get dashboards --mine --watch            # Watch your own
```

## Dry Run

Preview changes before applying:

```bash
dtctl apply -f workflow.yaml --dry-run
dtctl create settings -f pipeline.yaml --schema ... --dry-run
dtctl delete workflow "Test Workflow" --dry-run
```

## Idempotent Applies

Use `--write-id` and `--id` to prevent duplicate resources on repeated runs:

```bash
# First apply: stamp the generated ID back into the source file
dtctl apply -f dashboard.yaml --write-id

# All future runs update the same resource
dtctl apply -f dashboard.yaml

# Forgot --write-id on the first run? Recover without creating another duplicate:
dtctl apply -f dashboard.yaml --write-id --id <id-from-first-run>

# CI/scripting: apply a template file to a known target resource
dtctl apply -f template.yaml --id $DASHBOARD_ID
```

`--write-id` is a no-op when the file already contains an `id` field.

## Pipeline Integration

```bash
# Count resources
dtctl get workflows -o json | jq '. | length'

# Extract IDs
dtctl get workflows -o json | jq -r '.[].id'

# Filter and export
dtctl query "fetch logs" -o csv > logs.csv
dtctl query "fetch logs" -o json | jq '.records[]'
```

## Environment Variables

```bash
export DTCTL_OUTPUT=json           # Default output format
export DTCTL_CONTEXT=production    # Default context
export EDITOR=vim                  # Editor for edit commands
export DTCTL_SPILL=never           # Result spill mode: auto|always|never
export DTCTL_SPILL_DIR=/mnt/scratch # Base directory for spilled query results
```

## Health Check

`dtctl doctor` runs a battery of checks that answer "why isn't this working?"
before you go digging further:

```bash
dtctl doctor    # Runs 6 checks: version, config, context, token, connectivity, auth
```

This is usually the first thing to run when a command fails unexpectedly, before
filing an issue or digging into `-vv` debug output. Context and credential setup
itself is covered in the configuration guide (`docs/CONFIGURATION.md`, once that
page lands); `doctor` is the tool that verifies that setup is actually working.
