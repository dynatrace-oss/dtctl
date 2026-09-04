# Filter Segments

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | segment | `dtctl create segment` | Create resources from files | yes | write |
| delete | segment | `dtctl delete segment` | Delete resources | yes | delete |
| describe | segment | `dtctl describe segment` | Show details of a specific resource | no | read |
| edit | segment | `dtctl edit segment` | Edit a resource | yes | write |
| get | segments | `dtctl get segments` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `storage:filter-segments:delete` |
| read | `storage:filter-segments:read` |
| write | `storage:filter-segments:write` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

