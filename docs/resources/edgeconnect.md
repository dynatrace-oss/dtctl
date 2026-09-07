# EdgeConnect

<!-- Authored from the dtctl binary; SME to verify against the current dtctl binary. -->

EdgeConnect provides secure connectivity from the Dynatrace platform to endpoints inside a private network. dtctl lists and inspects EdgeConnect configurations and can create and delete them (aliases: `edgeconnect`, `ec`).

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | edgeconnect | `dtctl create edgeconnect` | Create resources from files | yes | write |
| delete | edgeconnect | `dtctl delete edgeconnect` | Delete resources | yes | delete |
| describe | edgeconnect | `dtctl describe edgeconnect` | Show details of a specific resource | no | read |
| get | edgeconnects | `dtctl get edgeconnects` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `app-engine:edge-connects:delete` |
| read | `app-engine:edge-connects:read` |
| write | `app-engine:edge-connects:write` |


## Output

`get edgeconnects` returns each configuration's ID, name, and host patterns; `describe edgeconnect <id>` adds the full definition. Use `-o json` for the complete record.


## Examples

```bash
# List all EdgeConnect configurations
dtctl get edgeconnects

# Describe one
dtctl describe edgeconnect <id>

# Create (name is RFC 1123 compliant, max 50 chars)
dtctl create edgeconnect --name my-edgeconnect --host-patterns "*.internal.example.com,db.example.com"

# Create from a definition file
dtctl create edgeconnect -f edgeconnect.yaml

# Delete (skip the confirmation prompt)
dtctl delete edgeconnect <id> --yes
```

