# Davis CoPilot

<!-- NOTE: kept split from analyzers; grouped as 'Davis AI' on the docs.dynatrace.com page only. -->

<!-- SME: Overview - what this resource is in the Dynatrace platform, when to use it, and how it relates to neighboring resources (1-2 sentences). -->

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| get | copilot-skills | `dtctl get copilot-skills` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `davis-copilot:conversations:execute` |
| run | `davis-copilot:conversations:execute` |


## Output

<!-- SME: describe the returned shape (key fields, id/name conventions) and how -o json / -o wide differ. -->


## Examples

<!-- SME: 3-5 real invocations with sample output. -->

