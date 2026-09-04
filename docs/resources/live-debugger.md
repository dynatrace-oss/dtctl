# Live Debugger

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | breakpoint | `dtctl create breakpoint` | Create resources from files | yes | write |
| delete | breakpoint | `dtctl delete breakpoint` | Delete resources | yes | delete |
| describe | breakpoint | `dtctl describe breakpoint` | Show details of a specific resource | no | read |
| get | breakpoints | `dtctl get breakpoints` | Display one or many resources | no | read |
| update | breakpoint | `dtctl update breakpoint` | Update resources | yes | write |
| get | snapshots | `dtctl get snapshots` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `dev-obs:breakpoints:set` |
| read | `dev-obs:breakpoints:set` |
| write | `dev-obs:breakpoints:set` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

