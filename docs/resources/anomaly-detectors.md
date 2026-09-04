# Anomaly Detectors

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | anomaly-detector | `dtctl create anomaly-detector` | Create resources from files | yes | write |
| delete | anomaly-detector | `dtctl delete anomaly-detector` | Delete resources | yes | delete |
| describe | anomaly-detector | `dtctl describe anomaly-detector` | Show details of a specific resource | no | read |
| edit | anomaly-detector | `dtctl edit anomaly-detector` | Edit a resource | yes | write |
| get | anomaly-detectors | `dtctl get anomaly-detectors` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `settings:objects:read` |
| write | `settings:objects:write` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

