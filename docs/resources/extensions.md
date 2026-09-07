# Extensions

<!-- NOTE: hub-extensions folded in here per the plan. -->

<!-- SME: already covered by the standalone guide at docs/EXTENSIONS.md; see that file for the full walkthrough (install, configure, hub-extensions). Overview below should stay a short pointer, not a re-migration of that content. -->

See [Extensions](../EXTENSIONS.md) for the full guide to managing extensions, extension configurations, and Hub extensions with dtctl.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | extension | `dtctl create extension` | Create resources from files | yes | write |
| describe | extension | `dtctl describe extension` | Show details of a specific resource | no | read |
| download | extension | `dtctl download extension` | Download raw resource artifacts | no | read |
| get | extensions | `dtctl get extensions` | Display one or many resources | no | read |
| apply | extension-config | `dtctl apply extension-config` | Apply a configuration to create or update resources | yes | write |
| describe | extension-config | `dtctl describe extension-config` | Show details of a specific resource | no | read |
| get | extension-configs | `dtctl get extension-configs` | Display one or many resources | no | read |
| describe | hub-extensions | `dtctl describe hub-extensions` | Show details of a specific resource | no | read |
| get | hub-extensions | `dtctl get hub-extensions` | Display one or many resources | no | read |
| get | hub-extension-releases | `dtctl get hub-extension-releases` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `extensions:configurations:read`, `extensions:definitions:read`, `hub:catalog:read` |
| write | `extensions:configurations:write`, `extensions:definitions:write` |


## Output

See [Extensions](../EXTENSIONS.md).

## Examples

See [Extensions](../EXTENSIONS.md) for full examples.

