# Live Debugger

<!-- SME: already covered by the standalone guide at docs/LIVE_DEBUGGER.md; see that file for the full walkthrough (breakpoints, snapshots, decoding). Overview below should stay a short pointer, not a re-migration of that content. -->

See [Live Debugger](../LIVE_DEBUGGER.md) for the full guide to managing breakpoints and snapshots with dtctl.

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

See [Live Debugger](../LIVE_DEBUGGER.md).

## Examples

See [Live Debugger](../LIVE_DEBUGGER.md) for full examples.

