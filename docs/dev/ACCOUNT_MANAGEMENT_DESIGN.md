# Account Management Design

**Status:** Design Proposal
**Created:** 2026-04-10
**Updated:** 2026-04-15
**Author:** dtctl team
**Reference:** [timstewart-dynatrace/dtiam](https://github.com/timstewart-dynatrace/dtiam) prototype
**Replaces:** `IAM_INTEGRATION_DESIGN.md`, `ACCOUNT_NAMESPACE_DESIGN.md`

## Overview

This document proposes extending dtctl with two account-level subcommand
namespaces: `dtctl iam` for identity and access management, and
`dtctl account` for subscriptions, cost, audit logs, and other account
administration. Both operate on the Dynatrace Account Management API plane
(`api.dynatrace.com`), which is fundamentally different from the environment
plane that dtctl uses today.

The design draws from the `dtiam` prototype, adapts it to dtctl's
architecture, and introduces account UUID auto-discovery via the IAM
service's `access-info` endpoint.

## Goals

1. **Single binary** -- users manage environments _and_ account-level resources from one tool
2. **Unified config** -- one config file, one context concept, no second tool
3. **Familiar UX** -- verb-noun grammar within both namespaces, same output formats, same `--agent` mode
4. **Auto-discovery** -- detect account UUID from the user's session when possible
5. **Automation-ready** -- client-credentials flow for CI/CD alongside interactive PKCE
6. **Incremental adoption** -- account features are additive; existing commands unchanged
7. **FinOps visibility** -- subscription cost, usage, and forecast from the CLI

## Non-Goals

- Replacing the Dynatrace web UI for IAM or account administration
- Supporting Dynatrace Classic (non-Platform) IAM APIs
- Implementing dtiam's template engine (dtctl's `apply -f` covers this)
- Breaking existing config files or commands
- Supporting Dynatrace Managed cluster-level APIs

---

## Background: Three API Planes

dtctl today operates on the **environment plane**. Account-level operations
use two additional service endpoints:

| Plane | Base URL (prod) | Identity Anchor | Purpose |
|-------|----------------|-----------------|---------|
| **Environment** | `https://{envID}.apps.dynatrace.com` | Environment ID | Dashboards, workflows, SLOs, settings, etc. |
| **Account Management API** | `https://api.dynatrace.com` | Account UUID | IAM, subscriptions, cost, audit logs |
| **IAM Service** | `https://iam.dynatrace.com` | User session | Login flow, environment discovery, access-info |

### API Base URLs by Tier

| Tier | Environment | Account Management API | IAM Service | SSO |
|------|-------------|----------------------|-------------|-----|
| **Production** | `{id}.apps.dynatrace.com` | `api.dynatrace.com` | `iam.dynatrace.com` | `sso.dynatrace.com` |
| **Hardening** | `{id}.sprint.apps.dynatracelabs.com` | `api-hardening.internal.dynatracelabs.com` | `iam-hardening.dynatracelabs.com` | `sso-sprint.dynatracelabs.com` |
| **Development** | `{id}.dev.apps.dynatracelabs.com` | `api-dev.internal.dynatracelabs.com` | `iam-dev.dynatracelabs.com` | `sso-dev.dynatracelabs.com` |

Auto-detection from the environment URL (same logic as `pkg/auth/`
`DetectEnvironment()`) determines which tier's URLs to use.

### Account Management API Surface

The Account Management API at `api.dynatrace.com` exposes these domains:

| Domain | Base Path | Auth Scope | Purpose |
|--------|-----------|------------|---------|
| **IAM** | `/iam/v1/accounts/{uuid}/...` | `account-idm-read/write` | Users, groups, policies, bindings, boundaries |
| **Subscriptions** | `/sub/v2/accounts/{uuid}/...` | `account-uac-read` | DPS subscriptions, usage, cost |
| **Cost allocation** | `/v1/subscriptions/{uuid}/cost-allocation` | `account-uac-read` | Cost breakdown by cost center / product |
| **Cost allocation mgmt** | `/v1/accounts/{uuid}/settings/...` | `account-uac-read/write` | Cost center and product CRUD |
| **Environments** | `/env/v2/accounts/{uuid}/environments` | `account-env-read` | List environments (v2, no management zones) |
| **Audit logs** | `/audit/v1/accounts/{uuid}` | `account-idm-read` | Account-level change audit trail |
| **Notifications** | `/v1/accounts/{uuid}/notifications` | (TBD) | Budget, cost, forecast, BYOK alerts |
| **Limits** | `/iam/v1/accounts/{uuid}/limits` | `account-idm-read` | Account resource quotas |
| **Reference data** | `/ref/v1/...` | (TBD) | Time zones, geographic regions |

### IAM Service: access-info Endpoint

The IAM service exposes a public endpoint that returns all accounts and
environments accessible to the currently authenticated user:

```
GET {iamBaseURL}/api/public/environment-access/access-info
Authorization: Bearer <user-token>
```

Response:

```json
{
  "accounts": [
    {
      "account": {
        "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
        "name": "Playground",
        "environments": [
          {
            "id": "wkf10640",
            "name": "Dynatrace Playground",
            "urlAlias": "playground",
            "scopes": ["PLATFORM", "DYNATRACE_CLASSIC"]
          }
        ]
      }
    },
    {
      "account": {
        "id": "b91c54d8-3e72-4f1a-9a3d-7c4e8f2a1b60",
        "name": "Dynatrace CA Capability",
        "environments": [
          {
            "id": "abc12345",
            "name": "ACME Production",
            "urlAlias": null,
            "scopes": ["PLATFORM", "DYNATRACE_CLASSIC"]
          }
        ]
      }
    }
  ]
}
```

Key observations:
- Returns **all** accounts and environments the user can access
- Each environment has `id`, `name`, `urlAlias`, and `scopes`
- `scopes` indicates capabilities: `PLATFORM` (new platform) and/or `DYNATRACE_CLASSIC`
- `name` can be `null` or empty for some environments
- `urlAlias` enables friendly URLs (e.g., `playground.apps.dynatrace.com`)
- This is the same data the Dynatrace web UI login flow uses
- **Enables auto-discovery of account UUID** from a known environment ID

**This endpoint fundamentally changes the "Is Account UUID Implicit from Tenant?"
answer from "No" to "Yes, if the user has a valid session."**

---

## Part 1: Shared Infrastructure

These decisions apply to both `dtctl iam` and `dtctl account` namespaces.

### Decision 1: Config Schema -- Optional `account-uuid`

The `Context` struct gains an **optional** `account-uuid` field. In the best
case, the account UUID is auto-discovered at runtime via the IAM
`access-info` endpoint (see Decision 2). However, auto-discovery requires a
valid environment URL **and** an authenticated session. Some users may:

- Not have an environment URL set (account-only administration)
- Operate in restricted networks where `iam.dynatrace.com` is unreachable
- Prefer explicit configuration over implicit discovery

For these cases, `account-uuid` can be set directly in the context:

```yaml
contexts:
  # Typical context: environment + auto-discovered account UUID
  - name: production
    context:
      environment: https://abc123.apps.dynatrace.com
      token-ref: prod-token
      safety-level: readwrite-all
      description: "Production environment"

  # Explicit account UUID (overrides auto-discovery when set)
  - name: production-explicit
    context:
      environment: https://abc123.apps.dynatrace.com
      account-uuid: b91c54d8-3e72-4f1a-9a3d-7c4e8f2a1b60
      token-ref: prod-token
      safety-level: readwrite-all
      description: "Production environment with explicit account"

  # Account-only context: no environment, only account-level operations
  - name: account-admin
    context:
      account-uuid: b91c54d8-3e72-4f1a-9a3d-7c4e8f2a1b60
      token-ref: account-admin-token
      safety-level: readwrite-all
      description: "Account administration (no environment)"
```

The `Context` struct adds one optional field:

```go
type Context struct {
    Environment     string      `yaml:"environment" table:"ENVIRONMENT"`
    AccountUUID     string      `yaml:"account-uuid,omitempty" table:"ACCOUNT-UUID,wide"`
    TokenRef        string      `yaml:"token-ref" table:"TOKEN-REF"`
    SafetyLevel     SafetyLevel `yaml:"safety-level,omitempty" table:"SAFETY-LEVEL"`
    Description     string      `yaml:"description,omitempty" table:"DESCRIPTION,wide"`
}
```

When `account-uuid` is set in the config, auto-discovery is skipped entirely.
When it is empty (the default), auto-discovery runs as described in Decision 2.

#### Account-Only Contexts

A context may omit `environment` and set only `account-uuid`. This enables
pure account-level administration (`dtctl iam`, `dtctl account`) without
needing a specific environment. Environment-level commands (`dtctl get`,
`dtctl describe`, etc.) return a clear error when the context has no
environment:

```
Error: context "account-admin" has no environment configured

Account-level commands (dtctl iam, dtctl account) work with this context.
For environment-level commands, switch to a context with an environment:
  dtctl ctx use <context-with-environment>
```

#### Setting `account-uuid` via CLI

```bash
# Set account UUID on an existing context
dtctl ctx set account-uuid b91c54d8-3e72-4f1a-9a3d-7c4e8f2a1b60

# Set it during context creation
dtctl ctx add account-admin \
  --account-uuid b91c54d8-3e72-4f1a-9a3d-7c4e8f2a1b60 \
  --token-ref account-admin-token \
  --safety-level readwrite-all

# Clear account UUID (fall back to auto-discovery)
dtctl ctx set account-uuid ""

# Auto-discover and persist the account UUID into the config
dtctl ctx discover-account --save
```

The `dtctl ctx discover-account --save` command queries the `access-info`
endpoint, resolves the account UUID for the current environment, and writes
it into the context config. This is a convenience for users who want the
speed of explicit config without manually looking up the UUID.

#### Environment Variables (Runtime Overrides)

For CI/CD or debugging, env vars override **both** config and auto-discovery:

| Variable | Description |
|----------|-------------|
| `DTCTL_ACCOUNT_UUID` | Override config and auto-discovered account UUID |
| `DTCTL_ACCOUNT_TOKEN` | Override account-level token (skips PKCE/client-cred flow) |

These are runtime overrides only -- they are never persisted to the config.

#### How Others Handle Account/Project Scoping

| CLI | Scoping concept | How it's set | Stored where |
|-----|----------------|--------------|-------------|
| **Azure CLI** | Tenant + Subscription | `az account set --subscription ID` | `~/.azure/azureProfile.json` |
| **gcloud** | Project | `gcloud config set project ID` | `~/.config/gcloud/properties` |
| **kubectl** | Cluster | Embedded in context | `~/.kube/config` |
| **dtctl** (proposed) | Account UUID | Auto-discovered or explicit `account-uuid` in context | Config file (optional) or runtime |

### Decision 2: Account UUID Auto-Discovery

The `access-info` endpoint enables automatic account UUID resolution.

#### Resolution Order

When an `iam` or `account` command needs the account UUID:

1. `DTCTL_ACCOUNT_UUID` environment variable (runtime override, highest priority)
2. `account-uuid` field in the current context's config (explicit config)
3. **Auto-discover** via `access-info` endpoint (requires `environment` to be set):
   a. Call `{iamBaseURL}/api/public/environment-access/access-info`
   b. Match the current context's environment ID against the response
   c. If exactly one account owns that environment, use it automatically
   d. If multiple accounts contain the environment (unlikely but possible),
      prompt interactively or fail in `--plain` mode
4. Error with setup instructions (including how to set `account-uuid` in config)

#### Auto-Discovery Implementation

```go
// pkg/client/account_discovery.go

// AccessInfoResponse represents the IAM access-info endpoint response
type AccessInfoResponse struct {
    Accounts []AccessInfoAccount `json:"accounts"`
}

// AccessInfoAccount represents one account entry in the access-info response
type AccessInfoAccount struct {
    Account AccessInfoAccountDetail `json:"account"`
}

// AccessInfoAccountDetail holds account details including environments
type AccessInfoAccountDetail struct {
    ID           string                    `json:"id"`
    Name         string                    `json:"name"`
    Environments []AccessInfoEnvironment   `json:"environments"`
}

// AccessInfoEnvironment represents an environment within an account
type AccessInfoEnvironment struct {
    ID       string   `json:"id"`
    Name     string   `json:"name"`
    URLAlias *string  `json:"urlAlias"`
    Scopes   []string `json:"scopes"`
}

// DiscoverAccountUUID finds the account UUID for a given environment ID
// by querying the IAM access-info endpoint.
func DiscoverAccountUUID(token string, iamBaseURL string, environmentID string) (string, string, error) {
    // Returns (accountUUID, accountName, error)
}
```

#### IAM Base URL Selection

```go
// iamBaseURLForTier returns the IAM service URL for the detected tier
func iamBaseURLForTier(env Environment) string {
    switch env {
    case EnvironmentProd:
        return "https://iam.dynatrace.com"
    case EnvironmentHard:
        return "https://iam-hardening.dynatracelabs.com"
    case EnvironmentDev:
        return "https://iam-dev.dynatracelabs.com"
    default:
        return "https://iam.dynatrace.com"
    }
}
```

#### UX for Auto-Discovery

```bash
# Auto-discovery happens transparently:
$ dtctl iam get groups
# stderr: Using account "Dynatrace CA Capability" (b91c54d8-...) for environment abc12345
# ... group listing follows ...

# Inspect which account maps to your environment:
$ dtctl ctx discover-account
Found account "Dynatrace CA Capability" (b91c54d8-...) for environment abc12345
```

#### `dtctl ctx discover-account` Command

Uses the `access-info` endpoint to show all accessible accounts and
identify the one matching the current environment:

```bash
$ dtctl ctx discover-account
Accounts accessible to you:

  ACCOUNT                          UUID                                    ENVIRONMENTS
  Playground                       f47ac10b-58cc-4372-a567-0e02b2c3d479    wkf10640 (Dynatrace Playground)
  Dynatrace CA Capability          b91c54d8-3e72-4f1a-9a3d-7c4e8f2a1b60    abc12345 (ACME Production) ← current
  Dynatrace IAS                    d3a7e6c1-4b92-4f58-8d1e-9c6b3a2f7e04    abc98765 xyz98765
  Demo live                        a2c4e6f8-1b3d-4a5c-8e7f-9d0b2c4a6e81    xyz12345 (Demo Live)

Current environment "abc12345" belongs to account "Dynatrace CA Capability" (b91c54d8-...)
```

This serves as a diagnostic tool -- users can see all accounts/environments
they have access to and verify the mapping is correct.

With `--save`, the discovered UUID is persisted to the current context's
`account-uuid` field. Without `--save`, it is displayed but not stored.

### Decision 3: Authentication

#### The `resource` Parameter Problem

dtctl's existing PKCE flow already uses the OAuth `resource` parameter,
set to the environment URL (e.g., `https://abc123.apps.dynatrace.com`).
This scopes the resulting token to that specific environment.

The Account Management API (`api.dynatrace.com`) requires a **different**
`resource` value: `urn:dtaccount:{accountUuid}`. This means a single OAuth
token **cannot** be used for both environment-level and account-level API
calls -- they are scoped to different resources.

The IAM service (`iam.dynatrace.com`) reuses the existing environment-scoped
token (it only needs the `openid` scope, which is already included in all
safety levels). No separate token is needed for `access-info` and similar
IAM service endpoints.

**Consequence: two tokens per context.** When an `iam` or `account` command
auto-discovers the account UUID, dtctl needs two tokens:

| Token | `resource` parameter | Used for |
|-------|---------------------|----------|
| **Environment token** (existing) | `https://{envID}.apps.dynatrace.com` | All existing dtctl commands, IAM service (`iam.dynatrace.com`) |
| **Account token** (new) | `urn:dtaccount:{accountUuid}` | Account Management API (`api.dynatrace.com`): IAM, subscriptions, cost, audit |

#### Path A: Interactive Users (PKCE)

Two separate PKCE flows are needed. The auth login command is extended to
handle both when an account UUID is discoverable:

```bash
# Login for environment only (existing behavior)
dtctl auth login

# Login for environment + account (when account UUID is discoverable)
dtctl auth login
# Step 1: PKCE flow with resource=https://abc123.apps.dynatrace.com (existing)
# Step 2: Auto-discover account UUID via access-info (or read from config)
# Step 3: PKCE flow with resource=urn:dtaccount:b91c54d8-... (new)
# Both tokens stored in keyring under different keys

# Login for account only (if you only need account-level commands,
# or if your context has no environment URL)
dtctl auth login --account-only
```

The second flow reuses the same SSO session (the user is already
authenticated in the browser from step 1), so in practice it's a
near-instant redirect rather than a second interactive login.

**Implementation in `pkg/auth/oauth_flow.go`:**

The `OAuthConfig` struct already has an `EnvironmentURL` field that maps to
the `resource` parameter. For account-level tokens, a new flow is created
with `resource` set to the account URN:

```go
// AccountOAuthConfig creates an OAuth config for the account management API
func AccountOAuthConfig(env Environment, safetyLevel config.SafetyLevel, accountUUID string) *OAuthConfig {
    base := OAuthConfigForEnvironment(env, safetyLevel)
    return &OAuthConfig{
        AuthURL:        base.AuthURL,
        TokenURL:       base.TokenURL,
        UserInfoURL:    base.UserInfoURL,
        ClientID:       base.ClientID,
        Scopes:         accountScopesForSafetyLevel(safetyLevel),
        Port:           base.Port,
        Environment:    env,
        SafetyLevel:    safetyLevel,
        EnvironmentURL: fmt.Sprintf("urn:dtaccount:%s", accountUUID), // resource parameter
    }
}
```

**Account-level scopes by safety level:**

| Safety Level | Account Scopes |
|-------------|---------------|
| `readonly` | `openid`, `account-idm-read`, `account-env-read`, `account-uac-read`, `iam:policies:read`, `iam:bindings:read`, `iam:boundaries:read`, `iam:effective-permissions:read` |
| `readwrite-mine` | Above + `account-idm-write` |
| `readwrite-all` | Above + `iam-policies-management`, `account-uac-write` |

**Caveat:** The built-in dtctl client ID (`dt0s12.dtctl-prod`) must be granted
these scopes by Dynatrace. If it is not, users get a clear OAuth error and are
directed to use their own credentials (Path B).

#### Path B: Automation / CI (Client Credentials)

Client-credentials tokens also require the `resource` parameter. The token
request must include `resource=urn:dtaccount:{accountUuid}`:

```
POST https://sso.dynatrace.com/sso/oauth2/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&client_id=dt0s01.XXXXX
&client_secret=dt0s01.XXXXX.YYYYY
&scope=account-idm-read account-idm-write iam-policies-management account-env-read
&resource=urn:dtaccount:3e4f5a6b-c7d8-4e9f-a1b2-c3d4e5f6a7b8
```

Config:

```yaml
tokens:
  - name: iam-service-cred
    type: oauth-client-credentials     # NEW token type
    client-id: dt0s01.XXXXX
    # client-secret stored in keyring under "dtctl:iam-service-cred"
```

For client-credentials, the account UUID is resolved at runtime (via
`DTCTL_ACCOUNT_UUID` env var or `access-info` auto-discovery) and used
as the `resource` parameter in the token request.

#### IAM Service Authentication (`iam.dynatrace.com`)

The IAM service endpoints (e.g., `access-info`) accept the existing
environment-scoped PKCE token -- no separate token or `resource` parameter
is needed. The `openid` scope (already included in all safety levels) is
sufficient. This means auto-discovery of the account UUID works without
any additional auth flow.

#### Token Resolution Order (for iam/account commands)

For commands targeting `api.dynatrace.com` (all `dtctl iam` and
`dtctl account` commands):

1. `DTCTL_ACCOUNT_TOKEN` environment variable
2. Cached account token in keyring (keyed by auto-discovered account UUID)
3. Auto-trigger account PKCE flow if environment token exists but no account token
4. Error: "No valid token for account-level operations"

For commands targeting `iam.dynatrace.com` (auto-discovery, `access-info`):

1. Use the existing environment token (already has `openid` scope)

#### Token Keyring Storage

Account tokens use a distinct keyring key to avoid collisions:

| Token type | Keyring key pattern | `resource` parameter |
|-----------|-------------------|---------------------|
| Environment (existing) | `oauth:{env}:{tokenRef}` | `https://{envID}.apps.dynatrace.com` |
| Account (PKCE, new) | `account-oauth:{accountUUID}:{tokenRef}` | `urn:dtaccount:{accountUUID}` |
| Account (client-cred, new) | `client-cred:{name}` | `urn:dtaccount:{accountUUID}` |

### Decision 4: HTTP Client Architecture

Extend the existing client with account-level capabilities rather than
creating a completely separate client:

```go
// pkg/client/account_client.go

// AccountClient handles requests to the Dynatrace Account Management API.
type AccountClient struct {
    http        *resty.Client
    accountUUID string
}

func (c *AccountClient) HTTP() *resty.Client { return c.http }
func (c *AccountClient) AccountUUID() string { return c.accountUUID }
```

```go
// pkg/client/client.go

// AccountClient returns a client configured for the account management API.
func (c *Client) AccountClient(accountUUID, token string) *AccountClient {
    return &AccountClient{
        http:        newRestyClient(accountBaseURL, token),
        accountUUID: accountUUID,
    }
}
```

Account Management API base URL auto-detection from the environment tier:

```go
func accountBaseURLForTier(env Environment) string {
    switch env {
    case EnvironmentProd:
        return "https://api.dynatrace.com"
    case EnvironmentHard:
        return "https://api-hardening.internal.dynatracelabs.com"
    case EnvironmentDev:
        return "https://api-dev.internal.dynatracelabs.com"
    default:
        return "https://api.dynatrace.com"
    }
}
```

IAM and account resource handlers both receive `*AccountClient`. This keeps
the separation clean and prevents accidentally sending environment-scoped
tokens to the account API.

### Decision 5: Keyring Token Storage

See Decision 3 for the full token keyring key patterns. Key points:

- Environment tokens and account tokens use **separate keyring entries**
  because they are minted with different `resource` parameters and cannot
  be interchanged.
- Client secrets for client-credentials tokens never appear in the YAML
  config file, consistent with dtctl's existing keyring-first approach.
- Account PKCE tokens are stored alongside environment tokens, keyed by
  account UUID, so multiple accounts can coexist.

### Decision 6: Agent Mode

The `--agent` / `-A` JSON envelope works transparently for both namespaces.
A `context.account` field is added for all account-level commands:

```json
{
  "ok": true,
  "result": [...],
  "context": {
    "verb": "iam-get",
    "resource": "groups",
    "account": "3e4f5a6b-c7d8-...",
    "suggestions": ["dtctl iam describe group 'DevOps Team'"]
  }
}
```

### Decision 7: Error UX for Common Failure Modes

#### No Account UUID Discoverable

```
Error: could not determine account UUID for context "production"

Auto-discovery via access-info failed: no valid session.

Set the account UUID explicitly:
  dtctl ctx set account-uuid YOUR_ACCOUNT_UUID

Or authenticate and let dtctl discover it:
  dtctl auth login
  dtctl ctx discover-account        (to verify the mapping)
  dtctl ctx discover-account --save (to persist it in the config)

Or override with an environment variable:
  export DTCTL_ACCOUNT_UUID=YOUR_ACCOUNT_UUID

Find your account UUID in:
  Dynatrace Account Management > Account settings
```

#### No Environment Configured (Account-Only Context)

When auto-discovery is attempted on a context without `environment`:

```
Error: could not auto-discover account UUID for context "account-admin"

No environment URL is configured in this context and no account-uuid is set.
Auto-discovery requires an environment URL to match against the access-info response.

Set the account UUID explicitly:
  dtctl ctx set account-uuid YOUR_ACCOUNT_UUID

Or switch to a context that has an environment URL:
  dtctl ctx use <context-with-environment>
```

#### Token Lacks Account-Level Scopes / Wrong Resource

```
Error: no account-level token available for context "production"

Account API calls require a token with resource=urn:dtaccount:b91c54d8-...
Your current token is scoped to the environment (https://abc123.apps.dynatrace.com).

Authenticate for account access:
  dtctl auth login              (will perform a second auth for account scope)
  dtctl auth login --account-only

Or pass a token directly:
  export DTCTL_ACCOUNT_TOKEN=dt0c01.XXXXX...
```

#### Account UUID Does Not Match Environment

Detected by `dtctl doctor` cross-checking the `access-info` response:

```
Warning: environment "abc123" not found in any accessible account

The access-info endpoint did not return an account containing this environment.
Check your permissions or run "dtctl ctx discover-account" to inspect.
```

---

## Part 2: `dtctl iam` -- Identity & Access Management

### Command Structure

```
dtctl iam get users
dtctl iam get groups
dtctl iam get policies --level environment [--level-id <envID>]
dtctl iam get policies --level account
dtctl iam get policies --level global
dtctl iam get bindings --level environment [--level-id <envID>] [--group NAME]
dtctl iam get boundaries --level environment [--level-id <envID>]
dtctl iam get environments
dtctl iam get service-users

dtctl iam describe user <email-or-uid>
dtctl iam describe group <name-or-uuid>
dtctl iam describe policy <name-or-uuid> --level environment [--level-id <envID>]
dtctl iam describe boundary <name-or-uuid> --level environment [--level-id <envID>]

dtctl iam create group --name "Team" [--description "..."]
dtctl iam create policy --name "viewer" --statement "ALLOW ..." --level environment [--level-id <envID>]
dtctl iam create binding --policy "viewer" --group "Team" --level environment [--level-id <envID>] [--boundary "prod"]
dtctl iam create boundary --name "prod" --zones "Production,Staging" --level environment [--level-id <envID>]
dtctl iam create service-user --name "CI Pipeline" [--add-to-group "Team"]

dtctl iam delete group <name-or-uuid>
dtctl iam delete policy <name-or-uuid> --level environment [--level-id <envID>]
dtctl iam delete binding --group <g> --policy <p> --level environment [--level-id <envID>]
dtctl iam delete user <email-or-uid>
dtctl iam delete service-user <name-or-uuid>

dtctl iam analyze user-permissions <email>
dtctl iam analyze permissions-matrix
dtctl iam analyze least-privilege

dtctl iam bulk add-users-to-group -f users.csv
dtctl iam bulk create-groups -f groups.yaml
dtctl iam bulk create-bindings -f bindings.yaml

dtctl iam whoami
```

### Grammar Deviation: Namespace-First vs Verb-First

This is a deliberate break from dtctl's core `<verb> <resource>` grammar.
The three-level depth matches how other CLIs handle IAM:

| CLI | Pattern | Levels |
|-----|---------|--------|
| AWS | `aws iam create-role` | 2 |
| gcloud | `gcloud iam roles create` | 3 |
| Azure | `az ad user create` | 3 |
| dtctl | `dtctl iam get groups` | 3 |

**Rationale:** Namespace isolation (IAM resources are account-level),
auth boundary (all `dtctl iam` require an account-level session), discoverability
(`dtctl iam --help` lists all IAM operations), and industry precedent.

### IAM-Specific Verbs: `analyze` and `bulk`

- **`analyze`** computes derived results (effective permissions, matrices) via
  client-side computation across multiple API calls. Not a `get` -- there is
  no "permissions-matrix" resource in the API.
- **`bulk`** is batch orchestration (CSV/YAML/JSON file processing with
  progress reporting), not a CRUD verb.

### Handling Existing `get users` / `get groups`

| Command | Scope | Shows |
|---------|-------|-------|
| `dtctl get users` | Environment | Users with access to the current environment |
| `dtctl iam get users` | Account | All users in the Dynatrace account |
| `dtctl get groups` | Environment | Groups visible from the current environment |
| `dtctl iam get groups` | Account | All groups in the account with full membership |

If an account UUID is discoverable, `dtctl get users` output includes a hint:
`Tip: Use "dtctl iam get users" for the full account-level view`.

### Policies, Bindings, and Permissions: Two Systems

Dynatrace IAM has **two separate systems** for granting access to groups:

| System | API | Scope Required | Use Case |
|--------|-----|---------------|----------|
| **Policy Bindings** | Policy Management API (`/repo/{level}/bindings/`) | `iam-policies-management` | Platform-style: custom policies with `ALLOW` statement syntax |
| **Permissions** | Permission Management API (`/groups/{uuid}/permissions`) | `account-idm-write` | Legacy: assign predefined permission names like `tenant-manage-security-problems` |

**dtctl focuses on the policy bindings system** as it is the current platform
approach. The key concept:

1. **Policies** are created at a **level** (`account` or `environment`) and
   contain a `statementQuery` with `ALLOW`/`DENY` syntax.
2. **Bindings** connect a policy to one or more groups at the same level.
   A binding says: "group X gets the permissions defined in policy Y for
   environment Z" (or for the entire account).
3. Bindings can optionally include **boundaries** to further restrict scope.

The `--level` flag is required on `create policy`, `create binding`,
`create boundary`, and their `delete` counterparts. Valid values:

| `--level` | `--level-id` | Meaning |
|-----------|-------------|----------|
| `account` | (defaults to account UUID from context) | Policy applies to all environments in the account |
| `environment` | Environment ID (defaults to current context's environment) | Policy applies to one specific environment |

`global` policies are managed by Dynatrace and cannot be created or deleted.

#### Create Policy: API Detail

```
POST /iam/v1/repo/{levelType}/{levelId}/policies
Auth: iam-policies-management

{
  "name": "event-writer",
  "description": "Allow event ingestion",
  "tags": [],
  "statementQuery": "ALLOW storage:events:write, storage:events:read;"
}

Response 201:
{
  "uuid": "c8f3d291-...",
  "name": "event-writer",
  "statementQuery": "ALLOW storage:events:write, storage:events:read;",
  "statements": [
    {"effect": "ALLOW", "permissions": ["storage:events:write", "storage:events:read"]}
  ]
}
```

#### Create Binding: API Detail

```
POST /iam/v1/repo/{levelType}/{levelId}/bindings/{policyUuid}
Auth: iam-policies-management

{
  "groups": ["e5a7b310-9c42-4d83-a618-2f9d04c7e5b1"],
  "boundaries": []  // optional
}

Response 204 (no body)
```

The binding endpoint appends to existing bindings -- it does not replace them.

#### Policy Validation

Before creating a policy, the statement can be validated:

```
POST /iam/v1/repo/{levelType}/{levelId}/policies/validation
```

dtctl should validate automatically and show clear errors for invalid
statements before submitting the create request.

### IAM API Endpoints

All under `https://api.dynatrace.com/iam/v1/accounts/{accountUUID}/`:

| Resource | Path |
|----------|------|
| Users | `/users` |
| Groups | `/groups`, `/groups/{uuid}` |
| Group members | `/groups/{uuid}/members` (GET) |
| Service Users | `/service-users`, `/service-users/{uid}` |
| Policies | `/repo/{levelType}/{levelId}/policies` |
| Bindings | `/repo/{levelType}/{levelId}/bindings/{policyUuid}` |
| Boundaries | `/repo/{levelType}/{levelId}/boundaries` |
| Permissions | `/groups/{uuid}/permissions` (legacy) |
| Environments | `/environments` |
| Limits | `/limits` |

Policy levels: `account/{accountUUID}`, `environment/{envID}`, `global/global`.

### Resource Handler Pattern

```
pkg/resources/iam/
    iam.go                    # Existing environment-scoped handlers
    account/                  # NEW: account-level IAM handlers
        groups.go
        users.go
        policies.go
        bindings.go
        boundaries.go
        environments.go
        service_users.go
        limits.go
        permissions.go        # analyze: effective perms, matrix, least-privilege
```

### IAM Safety Checks

```go
const (
    OperationIAMCreate  Operation = "iam-create"
    OperationIAMUpdate  Operation = "iam-update"
    OperationIAMDelete  Operation = "iam-delete"
)
```

| Safety Level | iam get/describe | iam create | iam delete | iam analyze |
|-------------|-----------------|------------|------------|-------------|
| `readonly` | yes | no | no | yes |
| `readwrite-mine` | yes | no | no | yes |
| `readwrite-all` | yes | yes | yes | yes |

`readwrite-mine` blocks all IAM mutations because IAM resources have no
meaningful `owner` field -- they are inherently shared, account-wide resources.

### Service Users: Concepts and Auto-Group Behavior

When you create a service user via the API, Dynatrace **automatically creates
a dedicated group** for that service user. The response includes:

```json
{
  "uid": "a4c8e2f6-3b7d-4a19-8e5c-d1f3a7b9c2e4",
  "email": "a4c8e2f6-...@service.sso.dynatrace.com",
  "name": "CI Pipeline",
  "surname": "SERVICE_IDENTITY",
  "description": "Event ingestion for CI",
  "createdAt": "2026-04-15T10:00:00Z",
  "groupUuid": "c7d9e1f3-a5b2-4c84-9d6e-f8a0b3c5d7e2"
}
```

The `groupUuid` is the auto-created group. To give the service user
permissions, you must **bind a policy to that group** (or add the service
user to an existing group that already has bindings).

### Recipe: Service User Setup (End-to-End)

This is the most common IAM automation use case: create a service user
with specific permissions for a particular environment.

#### Step-by-Step (Individual Commands)

```bash
# 1. Create a policy at the environment level
#    --level-id defaults to the current context's environment if omitted
dtctl iam create policy \
  --name "event-writer" \
  --description "Allow event ingestion" \
  --statement "ALLOW storage:events:write, storage:events:read;" \
  --level environment
# → Created policy "event-writer" (uuid: c8f3d291-...)

# 2. Create a group
dtctl iam create group \
  --name "Event Writers" \
  --description "Service accounts for event ingestion"
# → Created group "Event Writers" (uuid: e5a7b310-...)

# 3. Bind the policy to the group
#    This is the step that actually grants the permissions.
#    Uses the same --level as the policy.
dtctl iam create binding \
  --policy "event-writer" \
  --group "Event Writers" \
  --level environment
# → Bound policy "event-writer" to group "Event Writers" at environment level

# 4. Create a service user and add it to the group
dtctl iam create service-user \
  --name "CI Pipeline" \
  --description "Event ingestion for CI" \
  --add-to-group "Event Writers"
# → Created service user "CI Pipeline" (uid: a4c8e2f6-...)
# → Added to group "Event Writers"
```

#### Alternative: Bind Policy to Service User's Auto-Group

If you want a 1:1 mapping (one service user, one policy, no shared group):

```bash
# 1. Create the policy (same as above)
dtctl iam create policy \
  --name "event-writer" \
  --statement "ALLOW storage:events:write, storage:events:read;" \
  --level environment

# 2. Create the service user (without --add-to-group)
dtctl iam create service-user --name "CI Pipeline"
# → Created service user "CI Pipeline" (uid: a4c8e2f6-...)
# → Auto-created group: c7d9e1f3-...

# 3. Bind the policy to the auto-created group
dtctl iam create binding \
  --policy "event-writer" \
  --group c7d9e1f3-a5b2-4c84-9d6e-f8a0b3c5d7e2 \
  --level environment
```

#### Declarative: apply -f (Future)

Once `dtctl iam apply` is supported (Phase 4+), the entire setup can be
declared in a single YAML file:

```yaml
# service-user-setup.yaml
apiVersion: iam/v1
kind: Policy
metadata:
  name: event-writer
  level: environment
spec:
  description: Allow event ingestion
  statementQuery: |
    ALLOW storage:events:write, storage:events:read;
---
apiVersion: iam/v1
kind: Group
metadata:
  name: Event Writers
spec:
  description: Service accounts for event ingestion
  policyBindings:
    - policy: event-writer
      level: environment
---
apiVersion: iam/v1
kind: ServiceUser
metadata:
  name: CI Pipeline
spec:
  description: Event ingestion for CI
  groups:
    - Event Writers
```

```bash
dtctl iam apply -f service-user-setup.yaml
```

The apply command would resolve dependencies (create policy before binding)
and handle idempotency (skip if already exists with same config).

#### Auth Scope Requirements

This workflow requires two scopes:

| Operation | Scope |
|-----------|-------|
| Create/delete policies, bindings, boundaries | `iam-policies-management` |
| Create/delete groups, service users, manage membership | `account-idm-write` |

Both are included in the `readwrite-all` safety level.

---

## Part 3: `dtctl account` -- Account Administration

### Command Structure

```
# Subscriptions
dtctl account get subscriptions
dtctl account describe subscription <uuid>

# Usage & Cost
dtctl account get usage --subscription <uuid> [--env <id>] [--capability <key>]
dtctl account get cost --subscription <uuid> [--env <id>] [--capability <key>]
dtctl account get cost --subscription <uuid> --per-environment --from 2026-01-01 --to 2026-03-31
dtctl account get forecast

# Audit Logs
dtctl account get audit-logs [--from <time>] [--to <time>] [--filter <expr>]

# Notifications
dtctl account get notifications [--type BUDGET|COST|FORECAST|BYOK_REVOKED|BYOK_ACTIVATED]
                                [--severity SEVERE|WARN|INFO]

# Environments (account-level view)
dtctl account get environments

# Cost Allocation
dtctl account get cost-centers
dtctl account get products
dtctl account get cost-allocation --subscription <uuid> --env <id> --field COSTCENTER|PRODUCT
```

### Relationship to `dtctl iam`

`dtctl account` and `dtctl iam` are **siblings**, not parent-child:

```
dtctl
  ├── get / describe / create / delete / ...   (environment plane)
  ├── iam                                       (account plane: identity & access)
  └── account                                   (account plane: administration)
```

Both share the same auto-discovered account UUID, `AccountClient`, auth, and token
resolution. No config fields needed.

### What Belongs Where

| Namespace | Resources | Rationale |
|-----------|-----------|-----------|
| `dtctl iam` | Users, groups, service users, policies, bindings, boundaries, limits | Identity and access control |
| `dtctl account` | Subscriptions, cost, usage, audit logs, notifications, environments, cost allocation | Account administration and FinOps |
| Top-level | All existing environment-level resources | Environment plane (unchanged) |

### Subscription & Cost API

The highest-value addition. Provides subscription metadata, usage telemetry,
cost data, and forecasting -- all read-only.

| Endpoint | Method | Path |
|----------|--------|------|
| List subscriptions | GET | `/sub/v2/accounts/{uuid}/subscriptions` |
| Get subscription | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}` |
| Get usage | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/usage` |
| Get usage/env | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/environments/usage` |
| Get cost | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/cost` |
| Get cost/env | GET | `/sub/v3/accounts/{uuid}/subscriptions/{subUuid}/environments/cost` |
| Get forecast | GET | `/sub/v2/accounts/{uuid}/subscriptions/forecast` |

**Auth scope:** `account-uac-read` for all read operations.

**Subscription auto-selection:** When `--subscription` is required but not
provided and exactly one active subscription exists, use it automatically.

### Audit Logs

```bash
dtctl account get audit-logs                                 # default: last 24h
dtctl account get audit-logs --from 2026-04-01 --to 2026-04-10
dtctl account get audit-logs --filter "resource = 'POLICY' and eventType = 'DELETE'"
```

| Method | Path | Auth Scope |
|--------|------|------------|
| GET | `/audit/v1/accounts/{uuid}` | `account-idm-read` |

### Notifications

The notifications API uses POST for what is semantically a read operation.
The handler translates CLI flags into the POST request body transparently.

```bash
dtctl account get notifications --type BUDGET --severity SEVERE
```

### Environments (Account-Level)

The account `get environments` endpoint returns all environments with
their `id`, `name`, `active` status, and `url` (via the v2 API). The
`access-info` endpoint additionally provides `urlAlias` and `scopes`.

Both `dtctl iam get environments` and `dtctl account get environments` use
the same underlying data, with `iam` presenting a simplified view for policy
scoping context.

### Pagination Patterns

Different account-level endpoints use different pagination styles:

| Endpoint | Style | Notes |
|----------|-------|-------|
| Cost/env (`/sub/v3/`) | Cursor (`page-key`/`page-size`) | Standard dtctl pattern: don't combine |
| Cost allocation | Cursor (`page-key`) | Settings pattern: page token embeds ALL params |
| Cost centers/products | Offset (`page`/`page-size`) | Offset-based, different from most dtctl resources |
| Others | Not paginated | Single response |

### Account Safety Checks

All Phase 1 `dtctl account` commands are **read-only** and require no safety
checks. If cost allocation writes are added later, they follow the standard
pattern (`readwrite-all` required).

### Package Layout

```
pkg/resources/account/
    subscription.go      # SubscriptionHandler: List, Get
    usage.go             # UsageHandler: GetUsage, GetUsageByEnvironment
    cost.go              # CostHandler: GetCost, GetCostByEnvironment
    forecast.go          # ForecastHandler: GetForecast, GetEvents
    audit.go             # AuditHandler: List (with filters)
    notification.go      # NotificationHandler: List (with filters)
    environment.go       # EnvironmentHandler: List (shared with IAM)
    cost_allocation.go   # CostAllocationHandler: GetAllocation, GetCostCenters, GetProducts
```

---

## Part 4: `dtctl --help` Layout

```
Resource Commands:
  get         List resources
  describe    Show detailed resource information
  create      Create a resource
  delete      Delete a resource
  apply       Create or update resources from file
  edit        Edit a resource in your editor

Query & Execution:
  query       Execute DQL queries
  exec        Execute workflows, automations

Platform Administration:
  iam         Manage Identity and Access Management (account-level)
  account     View subscriptions, cost, usage, and audit logs (account-level)

Configuration:
  ctx         Manage contexts and configuration
  auth        Authentication management
  doctor      Check connectivity and configuration
```

---

## Part 5: Doctor Integration

Extend `dtctl doctor` to check account-level connectivity when an account
UUID is discoverable:

| Check | Description |
|-------|-------------|
| `account-api-reachable` | Verify `api.dynatrace.com` (or tier equivalent) is reachable |
| `iam-service-reachable` | Verify `iam.dynatrace.com` (or tier equivalent) is reachable |
| `account-scopes-valid` | Verify current token has account-level scopes |
| `account-environment-match` | Cross-check via `access-info` that the configured environment belongs to the configured account |

---

## Part 6: Migration from Standalone dtiam

A documented manual migration is sufficient for the prototype's small user
base. Include a section in `docs/MIGRATING_FROM_DTIAM.md` with:

1. Side-by-side field mapping table
2. Step-by-step commands to recreate the config
3. Verification: `dtctl iam get environments` should match `dtiam get environments`

An automated import command is not worth the investment for the current user
base.

### What to Port from dtiam (Logic, Not Code)

- **Resource handler logic** -- API paths, request/response parsing, name resolution
- **Permissions analysis** -- effective permissions calculator, matrix, least-privilege
- **Bulk operations** -- CSV/YAML/JSON file processing for batch management

### What dtctl Already Has Better

| dtiam feature | dtctl advantage |
|--------------|----------------|
| Separate config file | dtctl's config: keyring, safety levels, aliases |
| Custom HTTP client | dtctl's `pkg/client/`: retry, rate limiting, pagination |
| Output formatting | dtctl's `pkg/output/`: charts, agent envelope, color, golden tests |
| Global state singleton | dtctl passes config through function parameters |
| Template engine | dtctl's `apply -f` with YAML/JSON |

---

## Implementation Phases

### Phase 1: Foundation

**Goal:** Auth + auto-discovery + diagnostics.

- Add optional `account-uuid` field to `Context` struct in `pkg/config/`
- Implement `AccountClient` in `pkg/client/`
- Implement `access-info` client for auto-discovery via IAM service URLs
- Implement account UUID resolution order (env var → config → auto-discovery)
- Add `dtctl ctx discover-account` command (with `--save` to persist UUID)
- Add `dtctl ctx set account-uuid` / `dtctl ctx add --account-uuid` support
- Add client-credentials grant flow to `pkg/auth/`
- Extend PKCE scope lists with account-level scopes
- Add `DTCTL_ACCOUNT_UUID` / `DTCTL_ACCOUNT_TOKEN` env var override support
- Support account-only contexts (no `environment`, only `account-uuid`)
- Implement `dtctl iam whoami` as connectivity smoke test
- Extend `dtctl doctor` with account-level checks
- Add account-level keyring key patterns

### Phase 2: Read-Only IAM + Subscriptions

**Goal:** Full read access to IAM resources and core FinOps visibility.

IAM:
- `GroupHandler`, `UserHandler`, `PolicyHandler`, `BindingHandler`,
  `BoundaryHandler`, `EnvironmentHandler`, `ServiceUserHandler`, `LimitsHandler`
- Register `dtctl iam get/describe <resource>` commands

Account:
- `SubscriptionHandler` (List, Get)
- `UsageHandler`, `CostHandler`, `ForecastHandler`
- Register `dtctl account get subscriptions/usage/cost/forecast`
- Subscription auto-selection

Both:
- Golden tests for all resource types
- Agent mode context enrichment

### Phase 3: Audit, Notifications, IAM Mutations

IAM mutations:
- `iam create/delete group/policy/binding/boundary/service-user`
- Group/user membership management
- Safety checks with `OperationIAMCreate/Update/Delete`
- `--dry-run` support

Account read:
- `AuditHandler`, `NotificationHandler`
- `dtctl account get audit-logs`, `get notifications`

### Phase 4: Advanced Features

- `iam analyze user-permissions`, `permissions-matrix`, `least-privilege`
- `iam bulk add-users-to-group`, `create-groups`, `create-bindings`
- `iam export all`
- `account get cost-allocation`, `cost-centers`, `products`
- (Optional) cost center/product write operations

---

## Open Questions

1. **Built-in client ID scopes.** Does `dt0s12.dtctl-prod` have (or can it
   get) the account-level OAuth scopes? If not, client-credentials is the
   only path, which hurts interactive UX.

2. **Account API base URLs for non-prod tiers.** Production is
   `api.dynatrace.com`. What are the exact hardening/dev equivalents?
   (IAM service URLs are confirmed: `iam-hardening.dynatracelabs.com`,
   `iam-dev.dynatracelabs.com`.)

3. **`access-info` endpoint stability.** Is
   `iam.dynatrace.com/api/public/environment-access/access-info` a stable
   public API or an internal UI endpoint? If internal, auto-discovery is
   best-effort and we must not hard-depend on it.

4. **`access-info` authentication.** Does it work with client-credentials
   tokens or only user (PKCE) tokens? Client-credentials tokens represent
   a service user, not a person, so the access-info response may differ or
   not work at all. (Note: it works with the existing environment-scoped
   PKCE token since it only needs `openid` scope.)

5. **Rate limiting.** The account management API may have stricter rate limits
   than environment APIs. Need to test and configure retry behavior.

6. **Pagination on IAM endpoints.** Need to identify exact param names and
   whether page tokens embed filters.

7. **Platform tokens: out of scope.** Platform tokens are account-scoped
   credentials, not authorization policies. They should be a separate
   top-level resource or live under `dtctl account` -- tracked separately.

8. **`dtctl iam apply -f`?** Should IAM resources support the declarative
   apply pattern? Deferred to Phase 4+ with careful safety consideration.

9. **Dual PKCE flow UX.** The two-step login (environment token + account
   token) should feel like a single operation. The second flow reuses the
   SSO session, so it should be near-instant. Need to verify this works
   smoothly across all tier SSO endpoints and that the callback port doesn't
   conflict between the two flows.

---

## Appendix A: OAuth Scope and Resource Reference

### The `resource` Parameter

Dynatrace OAuth tokens are scoped by the `resource` parameter at mint time.
A single token cannot be used for both environment and account API calls.

| Target | `resource` value | Example |
|--------|-----------------|----------|
| Environment API | Environment URL | `https://abc123.apps.dynatrace.com` |
| Account Management API | Account URN | `urn:dtaccount:3e4f5a6b-c7d8-4e9f-a1b2-c3d4e5f6a7b8` |
| IAM Service | (none needed) | Uses environment token with `openid` scope |

### Account-Level Scopes

These scopes are requested in the OAuth flow with `resource=urn:dtaccount:{uuid}`:

| Scope | Purpose |
|-------|---------|
| `openid` | Required for all OAuth flows |
| `account-idm-read` | List/get groups, users, service users, limits |
| `account-idm-write` | Create/delete groups, users, service users |
| `account-env-read` | List environments in account |
| `account-uac-read` | Subscriptions, usage, cost, cost allocation |
| `account-uac-write` | Cost center/product management |
| `iam-policies-management` | Full policy, binding, and boundary CRUD |
| `iam:effective-permissions:read` | Effective permissions analysis |
| `iam:policies:read` | Read policies |
| `iam:bindings:read` | Read bindings |
| `iam:boundaries:read` | Read boundaries |

### Combined Account Scope Set (readonly)

```
openid, account-idm-read, account-env-read, account-uac-read,
iam:policies:read, iam:bindings:read, iam:boundaries:read,
iam:effective-permissions:read
```

These are **separate from** the environment-level scopes. A context with both
environment and account configured results in two tokens with two different
scope sets and two different `resource` values.

## Appendix B: dtiam Feature Matrix

| dtiam Feature | dtctl Phase | Notes |
|--------------|-------------|-------|
| get/describe groups, users, policies, bindings, boundaries | Phase 2 | |
| get environments | Phase 2 | |
| create/delete groups, users, policies, bindings, boundaries | Phase 3 | |
| group membership, boundary attach/detach | Phase 3 | |
| service user CRUD | Phase 3 | |
| analyze user-permissions, permissions-matrix | Phase 4 | |
| bulk operations, export | Phase 4 | |
| Template system | Not planned | `apply -f` covers this |
| Platform tokens CRUD | Separate design | Not IAM; separate resource |

## Appendix C: Full Account API Endpoint Reference

| Resource | Method | Path | Paginated | Phase |
|----------|--------|------|-----------|-------|
| List subscriptions | GET | `/sub/v2/accounts/{uuid}/subscriptions` | No | 2 |
| Get subscription | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}` | No | 2 |
| Get usage | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/usage` | No | 2 |
| Get usage/env | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/environments/usage` | No | 2 |
| Get cost | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/cost` | No | 2 |
| Get cost/env | GET | `/sub/v3/accounts/{uuid}/subscriptions/{subUuid}/environments/cost` | Yes (cursor) | 2 |
| Get forecast | GET | `/sub/v2/accounts/{uuid}/subscriptions/forecast` | No | 2 |
| Get events | GET | `/sub/v2/accounts/{uuid}/subscriptions/events` | No | 2 |
| List audit logs | GET | `/audit/v1/accounts/{uuid}` | No (limit) | 3 |
| List notifications | POST | `/v1/accounts/{uuid}/notifications` | Yes (offset) | 3 |
| List environments (v2) | GET | `/env/v2/accounts/{uuid}/environments` | No | 2 |
| Access info (IAM svc) | GET | `{iamBaseURL}/api/public/environment-access/access-info` | No | 1 |
| Get cost allocation | GET | `/v1/subscriptions/{uuid}/cost-allocation` | Yes (cursor) | 4 |
| List cost centers | GET | `/v1/accounts/{uuid}/settings/costcenters` | Yes (offset) | 4 |
| List products | GET | `/v1/accounts/{uuid}/settings/products` | Yes (offset) | 4 |

## Appendix D: What's Excluded (and Why)

| API / Feature | Reason |
|--------------|--------|
| Reference Data API (timezones, regions) | No actionable CLI use case |
| Platform Tokens CRUD | Credentials management, not IAM. Separate design. |
| Environment edit (name/timezone) | Low-value, infrequent. Web UI is sufficient. |
| Lens (Adoption & Environments dashboards) | Web-UI-only, no public API |
| SAML/SCIM configuration | Highly sensitive; web UI is the right interface |
| Contact/billing information | Account metadata; web UI only |
| IP allowlist configuration | Environment-level security; could be added separately |
