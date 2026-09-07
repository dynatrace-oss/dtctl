# Users & Groups

<!-- Authored from the dtctl binary; SME to verify against the current dtctl binary. -->

Users and groups are the IAM identities in your Dynatrace account. dtctl provides read-only access: list users and groups and inspect individual ones (aliases: `user`/`users`, `group`/`groups`). Managing membership and permissions is done through Dynatrace account management.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| describe | group | `dtctl describe group` | Show details of a specific resource | no | read |
| get | groups | `dtctl get groups` | Display one or many resources | no | read |
| describe | user | `dtctl describe user` | Show details of a specific resource | no | read |
| get | users | `dtctl get users` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| read | `iam:groups:read`, `iam:users:read` |


## Output

`get users` / `get groups` return each identity's UUID and name (email for users); `describe user <uuid>` / `describe group <uuid>` add the full record, including group membership. Use `-o json` for the complete structure.


## Examples

```bash
# List all IAM users
dtctl get users

# Describe a specific user
dtctl describe user <user-uuid>

# List all IAM groups
dtctl get groups

# Describe a specific group
dtctl describe group <group-uuid>
```

