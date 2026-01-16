# Token Scopes Reference

This document lists the Dynatrace platform token scopes required for each safety level. Copy the scope list for your desired safety level when creating a token in Dynatrace.

> **Note**: Safety levels are client-side only. The token scopes you configure in Dynatrace are what actually controls access. Configure your tokens with the minimum required scopes for your use case.
>
> **Ownership checks are also client-side**: The `readwrite-mine` safety level and `--mine` flag work by comparing the resource owner ID with your user ID locally. The Dynatrace API does not enforce ownership restrictions—if your token has write access, you can modify any resource that is shared with a user. The ownership check is a convenience feature to prevent accidental modifications to shared resources.

## Quick Reference

| Safety Level | Use Case | Token Type |
|--------------|----------|------------|
| `readonly` | Production monitoring, troubleshooting | Read-only token |
| `readwrite-mine` | Personal development, sandbox | Standard token |
| `readwrite-all` | Team environments, administration | Standard token |
| `dangerously-unrestricted` | Dev environments, bucket management | Full access token |

---

## Scopes for `readonly`

Use this for production monitoring contexts where no modifications should be possible.

```
automation:workflows:read,
automation:calendars:read,
automation:rules:read,
document:documents:read,
document:trash.documents:read,
storage:logs:read,
storage:events:read,
storage:metrics:read,
storage:spans:read,
storage:entities:read,
storage:buckets:read,
storage:files:read,
slo:read,
settings:objects:read,
davis:analyzers:read,
app-engine:apps:run
```

**Capabilities**: List and view all resources, run DQL queries, view SLO status.

---

## Scopes for `readwrite-mine`

Use this for personal development where you create and manage your own resources.

```
automation:workflows:read,
automation:workflows:write,
automation:workflows:run,
automation:calendars:read,
automation:rules:read,
document:documents:read,
document:documents:write,
document:documents:delete,
document:trash.documents:read,
document:trash.documents:delete,
storage:logs:read,
storage:events:read,
storage:metrics:read,
storage:spans:read,
storage:entities:read,
storage:buckets:read,
storage:files:read,
storage:files:write,
slo:read,
slo:write,
settings:objects:read,
settings:objects:write,
davis:analyzers:read,
davis:analyzers:execute,
davis-copilot:conversations:execute,
app-engine:apps:run
```

**Capabilities**: All read operations, plus create/update/delete your own workflows, dashboards, notebooks, SLOs, and lookup tables.

---

## Scopes for `readwrite-all`

Use this for team environments and production administration.

```
automation:workflows:read,
automation:workflows:write,
automation:workflows:run,
automation:calendars:read,
automation:calendars:write,
automation:rules:read,
automation:rules:write,
document:documents:read,
document:documents:write,
document:documents:delete,
document:trash.documents:read,
document:trash.documents:delete,
storage:logs:read,
storage:events:read,
storage:metrics:read,
storage:spans:read,
storage:entities:read,
storage:buckets:read,
storage:buckets:write,
storage:files:read,
storage:files:write,
slo:read,
slo:write,
settings:objects:read,
settings:objects:write,
davis:analyzers:read,
davis:analyzers:execute,
davis-copilot:conversations:execute,
app-engine:apps:run
```

**Capabilities**: Full resource management except Grail bucket deletion.

---

## Scopes for `dangerously-unrestricted`

Use this only for development environments where you need full control including data deletion.

```
automation:workflows:read,
automation:workflows:write,
automation:workflows:run,
automation:calendars:read,
automation:calendars:write,
automation:rules:read,
automation:rules:write,
document:documents:read,
document:documents:write,
document:documents:delete,
document:trash.documents:read,
document:trash.documents:delete,
storage:logs:read,
storage:events:read,
storage:metrics:read,
storage:spans:read,
storage:entities:read,
storage:buckets:read,
storage:buckets:write,
storage:files:read,
storage:files:write,
storage:files:delete,
slo:read,
slo:write,
settings:objects:read,
settings:objects:write,
davis:analyzers:read,
davis:analyzers:execute,
davis-copilot:conversations:execute,
app-engine:apps:run
```

**Capabilities**: Everything, including Grail bucket deletion and lookup table deletion.

---

## Scope Details by Resource

### Workflows

| Operation | Scope |
|-----------|-------|
| List, describe, get | `automation:workflows:read` |
| Create, edit, delete | `automation:workflows:write` |
| Execute | `automation:workflows:run` |

### Dashboards & Notebooks

| Operation | Scope |
|-----------|-------|
| List, describe, get | `document:documents:read` |
| Create, edit | `document:documents:write` |
| Delete (move to trash) | `document:documents:delete` |
| View trash | `document:trash.documents:read` |
| Empty trash | `document:trash.documents:delete` |

### DQL Queries

| Operation | Scope |
|-----------|-------|
| Query logs | `storage:logs:read` |
| Query events | `storage:events:read` |
| Query metrics | `storage:metrics:read` |
| Query spans/traces | `storage:spans:read` |
| Query entities | `storage:entities:read` |

### SLOs

| Operation | Scope |
|-----------|-------|
| List, describe, get | `slo:read` |
| Create, edit, delete, evaluate | `slo:write` |

### Grail Buckets

| Operation | Scope |
|-----------|-------|
| List, describe | `storage:buckets:read` |
| Create, update, delete | `storage:buckets:write` |

### Lookup Tables

| Operation | Scope |
|-----------|-------|
| List, describe, get | `storage:files:read` |
| Create, update | `storage:files:write` |
| Delete | `storage:files:delete` |

### Settings & OpenPipeline

| Operation | Scope |
|-----------|-------|
| List schemas, get objects | `settings:objects:read` |
| Create, update, delete | `settings:objects:write` |

### Davis AI

| Operation | Scope |
|-----------|-------|
| List analyzers | `davis:analyzers:read` |
| Execute analyzers | `davis:analyzers:execute` |
| CoPilot chat, nl2dql, dql2nl | `davis-copilot:conversations:execute` |

### User Identity

| Operation | Scope |
|-----------|-------|
| `dtctl auth whoami` | `app-engine:apps:run` |

---

## Creating a Token in Dynatrace

1. Navigate to **Account Management > Identity & Access Management > OAuth clients**
2. Click **Create client**
3. Give it a name (e.g., "dtctl-readonly", "dtctl-dev")
4. Copy the appropriate scope list from above
5. Paste into the scopes field
6. Generate and copy the client secret

For detailed instructions, see [Dynatrace OAuth clients documentation](https://docs.dynatrace.com/docs/manage/identity-access-management/access-tokens-and-oauth-clients/oauth-clients).

---

## See Also

- [Context Safety Levels](dev/context-safety-levels.md) - How safety levels work
- [Quick Start](QUICK_START.md) - Getting started with dtctl
