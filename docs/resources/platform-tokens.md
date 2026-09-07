<!-- Authored from the dtctl binary (cmd/account_token.go); SME to verify against the current dtctl binary. -->

# Platform Tokens

Platform tokens (`dt0s16.*`) authenticate against the Dynatrace platform. dtctl manages them through the **account** command group via the Account Management API, so these commands operate at the account level (not a single environment) and require account authentication (`dtctl account login`).

## Supported operations

| Operation | Command syntax | Description |
| --- | --- | --- |
| list | `dtctl account list token` (alias `tokens`) | List all platform tokens for the account |
| create | `dtctl account create token --name <name> --scope <scope>` | Create a new platform token |
| delete | `dtctl account delete token <tokenId>` (alias `revoke`) | Delete (revoke) a platform token by its ID |

## Flags

Flags for `account create token`:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--name` | string | (required) | Token name |
| `--scope` | string array | (required) | Token scope; repeat or comma/space/newline-separate for multiple |
| `--expires` | string | `90d` | Token lifetime (for example `30d`, `720h`) |
| `--expires-at` | string | | Exact expiration date in RFC3339 (mutually exclusive with `--expires`) |
| `--user-uuid` | string | current user | User UUID the token belongs to; auto-resolved from the account token's JWT subject if omitted |
| `--resource` | string array | current environment | Environment URL(s) the token is scoped to |
| `--tag` | string array | | Token tag; may be specified multiple times |

`--dry-run` previews `create` and `delete` without making changes.

## Authentication

These commands operate against the account, so they need account authentication rather than a per-environment context:

```bash
dtctl account login
```

## Output

<!-- SME: unverified inference - the output shape below was NOT captured from a live run; verify against the binary. -->

`account list token` returns each token's ID, name, scopes, owner, expiry, and tags. Use `-o json` for the full structured record. `create` prints the new token value once (store it immediately; it cannot be retrieved again).

## Examples

```bash
# List all platform tokens
dtctl account list token

# List as JSON
dtctl account list token -o json

# Create a token (90-day default expiry)
dtctl account create token --name ci-pipeline --scope account-idm-read

# Multiple scopes (repeat or comma-separate)
dtctl account create token --name ci-pipeline --scope account-idm-read,storage:buckets:read

# Custom expiry (relative, or an exact RFC3339 date)
dtctl account create token --name ci-pipeline --scope account-idm-read --expires 30d
dtctl account create token --name ci-pipeline --scope account-idm-read --expires-at 2026-10-01T00:00:00Z

# Preview without creating
dtctl account create token --name ci-pipeline --scope account-idm-read --dry-run

# Delete (revoke) a token by ID
dtctl account delete token <tokenId>
```
