# Settings

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | settings | `dtctl create settings` | Create resources from files | yes | write |
| delete | settings | `dtctl delete settings` | Delete resources | yes | delete |
| describe | settings | `dtctl describe settings` | Show details of a specific resource | no | read |
| edit | setting | `dtctl edit setting` | Edit a resource | yes | write |
| get | settings | `dtctl get settings` | Display one or many resources | no | read |
| describe | settings-schema | `dtctl describe settings-schema` | Show details of a specific resource | no | read |
| get | settings-schemas | `dtctl get settings-schemas` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `app-settings:objects:read`, `settings:objects:read`, `settings:schemas:read` |
| write | `settings:objects:write` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

