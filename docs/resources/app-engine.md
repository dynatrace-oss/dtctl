# App Engine

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| delete | app | `dtctl delete app` | Delete resources | yes | delete |
| describe | app | `dtctl describe app` | Show details of a specific resource | no | read |
| get | apps | `dtctl get apps` | Display one or many resources | no | read |
| describe | function | `dtctl describe function` | Show details of a specific resource | no | read |
| exec | function | `dtctl exec function` | Execute queries, workflows, or functions | yes | run |
| get | functions | `dtctl get functions` | Display one or many resources | no | read |
| describe | intent | `dtctl describe intent` | Show details of a specific resource | no | read |
| find | intents | `dtctl find intents` | Find resources based on criteria | no | read |
| get | intents | `dtctl get intents` | Display one or many resources | no | read |
| open | intent | `dtctl open intent` | Open resources in browser | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `app-engine:apps:delete` |
| read | `app-engine:apps:run` |
| run | `app-engine:functions:run` |
| write | `app-engine:apps:install` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

