<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# SLOs

Service Level Objectives (SLOs) define reliability targets for your services. dtctl lets you list, create, evaluate, and manage SLOs directly from the command line, including starting from a built-in template.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | slo | `dtctl create slo` | Create resources from files | yes | write |
| delete | slo | `dtctl delete slo` | Delete resources | yes | delete |
| describe | slo | `dtctl describe slo` | Show details of a specific resource | no | read |
| exec | slo | `dtctl exec slo` | Execute queries, workflows, or functions | yes | run |
| get | slos | `dtctl get slos` | Display one or many resources | no | read |
| get | slo-templates | `dtctl get slo-templates` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `slo:objective-templates:read`, `slo:slos:read` |
| write | `slo:slos:write` |


## Output

`get slos` prints a compact table by default; `-o wide` adds target, warning, and current status columns. `describe slo` shows full details for one SLO, including its current status, error budget, and evaluation configuration. `exec slo` (evaluation) returns the current SLO value, error budget remaining, and evaluation status.

## Examples

```bash
# List all SLOs
dtctl get slos

# Filter by name
dtctl get slos --filter 'name~production'

# Wide output with target, warning, and current status
dtctl get slos -o wide

# JSON for scripting
dtctl get slos -o json

# Watch for changes in real time
dtctl get slos --watch
```

Describe a specific SLO:

```bash
dtctl describe slo slo-123
```

Use a built-in template as a starting point, or define SLOs in YAML directly:

```bash
# List available SLO templates
dtctl get slo-templates

# Create an SLO from a template (interactive, prompts for required fields)
dtctl create slo --from-template

# Create (fails if the SLO already exists)
dtctl create slo -f slo.yaml

# Apply (creates if new, updates if existing)
dtctl apply -f slo.yaml
```

Trigger an on-demand evaluation to check the current status and remaining error budget:

```bash
dtctl exec slo slo-123
```

This returns output like:

```
Name:           Checkout Availability
Status:         SUCCESS
SLO Value:      99.94%
Target:         99.90%
Warning:        99.95%
Error Budget:   0.04% remaining
Timeframe:      last 7 days
```

Delete an SLO (dtctl prompts for confirmation in interactive mode; use `--plain` to skip it in CI):

```bash
dtctl delete slo slo-123
```

