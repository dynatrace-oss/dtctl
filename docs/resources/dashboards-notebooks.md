# Dashboards & Notebooks

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | dashboard | `dtctl create dashboard` | Create resources from files | yes | write |
| delete | dashboard | `dtctl delete dashboard` | Delete resources | yes | delete |
| describe | dashboard | `dtctl describe dashboard` | Show details of a specific resource | no | read |
| edit | dashboard | `dtctl edit dashboard` | Edit a resource | yes | write |
| get | dashboards | `dtctl get dashboards` | Display one or many resources | no | read |
| history | dashboard | `dtctl history dashboard` | Show version history of resources | no | read |
| restore | dashboard | `dtctl restore dashboard` | Restore resources to a previous version | yes | write |
| share | dashboard | `dtctl share dashboard` | Share documents with users or groups | yes | write |
| unshare | dashboard | `dtctl unshare dashboard` | Remove sharing from documents | yes | write |
| create | notebook | `dtctl create notebook` | Create resources from files | yes | write |
| delete | notebook | `dtctl delete notebook` | Delete resources | yes | delete |
| describe | notebook | `dtctl describe notebook` | Show details of a specific resource | no | read |
| edit | notebook | `dtctl edit notebook` | Edit a resource | yes | write |
| get | notebooks | `dtctl get notebooks` | Display one or many resources | no | read |
| history | notebook | `dtctl history notebook` | Show version history of resources | no | read |
| restore | notebook | `dtctl restore notebook` | Restore resources to a previous version | yes | write |
| share | notebook | `dtctl share notebook` | Share documents with users or groups | yes | write |
| unshare | notebook | `dtctl unshare notebook` | Remove sharing from documents | yes | write |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `document:documents:delete` |
| read | `document:documents:read` |
| write | `document:documents:write` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

