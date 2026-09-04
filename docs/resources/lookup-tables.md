# Lookup Tables

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | lookup | `dtctl create lookup` | Create resources from files | yes | write |
| delete | lookup | `dtctl delete lookup` | Delete resources | yes | delete |
| describe | lookup | `dtctl describe lookup` | Show details of a specific resource | no | read |
| get | lookups | `dtctl get lookups` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `storage:files:delete` |
| read | `storage:files:read` |
| write | `storage:files:write` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

