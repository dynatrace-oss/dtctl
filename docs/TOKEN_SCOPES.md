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

For creating platform tokens, see [Dynatrace Platform Tokens documentation](https://docs.dynatrace.com/docs/manage/identity-access-management/access-tokens-and-oauth-clients/platform-tokens).

## Recommended Scopes by Safety Level

### `readonly`

Read-only access for production monitoring and troubleshooting.

```
document:documents:read,
automation:workflows:read,
slo:slos:read,
settings:schemas:read,
settings:objects:read,
storage:logs:read,
storage:events:read,
storage:metrics:read,
storage:spans:read,
storage:bizevents:read,
storage:entities:read,
storage:smartscape:read,
storage:system:read,
storage:security.events:read,
storage:application.snapshots:read,
storage:user.events:read,
storage:user.sessions:read,
storage:user.replays:read,
storage:buckets:read,
storage:bucket-definitions:read,
storage:fieldsets:read,
storage:fieldset-definitions:read,
storage:files:read,
storage:filter-segments:read,
iam:users:read,
iam:groups:read,
notifications:read,
vulnerabilities:read,
davis:analyzers:read,
app-engine:apps:run,
email:emails:send
```

### `readwrite-mine`

Create and manage your own resources in sandbox/development environments.

```
document:documents:read,
document:documents:write,
automation:workflows:read,
automation:workflows:write,
automation:workflows:execute,
slo:slos:read,
slo:slos:write,
settings:schemas:read,
settings:objects:read,
settings:objects:write,
storage:logs:read,
storage:events:read,
storage:metrics:read,
storage:spans:read,
storage:bizevents:read,
storage:entities:read,
storage:smartscape:read,
storage:system:read,
storage:security.events:read,
storage:buckets:read,
storage:bucket-definitions:read,
storage:files:read,
storage:files:write,
storage:filter-segments:read,
storage:filter-segments:write,
davis:analyzers:read,
davis:analyzers:execute,
davis-copilot:conversations:execute,
app-engine:apps:run,
app-engine:functions:run,
email:emails:send
```

### `readwrite-all`

Full resource management for team environments (no data deletion).

```
document:documents:read,
document:documents:write,
automation:workflows:read,
automation:workflows:write,
automation:workflows:execute,
slo:slos:read,
slo:slos:write,
settings:schemas:read,
settings:objects:read,
settings:objects:write,
storage:logs:read,
storage:logs:write,
storage:events:read,
storage:events:write,
storage:metrics:read,
storage:metrics:write,
storage:spans:read,
storage:bizevents:read,
storage:entities:read,
storage:smartscape:read,
storage:system:read,
storage:security.events:read,
storage:application.snapshots:read,
storage:user.events:read,
storage:user.sessions:read,
storage:user.replays:read,
storage:buckets:read,
storage:buckets:write,
storage:bucket-definitions:read,
storage:fieldsets:read,
storage:fieldset-definitions:read,
storage:files:read,
storage:files:write,
storage:filter-segments:read,
storage:filter-segments:write,
iam:users:read,
iam:groups:read,
notifications:read,
vulnerabilities:read,
davis:analyzers:read,
davis:analyzers:execute,
davis-copilot:conversations:execute,
davis-copilot:nl2dql:execute,
davis-copilot:dql2nl:execute,
davis-copilot:document-search:execute,
app-engine:apps:install,
app-engine:apps:run,
app-engine:apps:delete,
app-engine:functions:run,
app-engine:edge-connects:read,
app-engine:edge-connects:write,
email:emails:send
```

### `dangerously-unrestricted`

Full admin access including data deletion and bucket management.

```
document:documents:read,
document:documents:write,
automation:workflows:read,
automation:workflows:write,
automation:workflows:execute,
slo:slos:read,
slo:slos:write,
settings:schemas:read,
settings:objects:read,
settings:objects:write,
settings:objects:admin,
storage:logs:read,
storage:logs:write,
storage:events:read,
storage:events:write,
storage:metrics:read,
storage:metrics:write,
storage:spans:read,
storage:bizevents:read,
storage:entities:read,
storage:smartscape:read,
storage:system:read,
storage:security.events:read,
storage:application.snapshots:read,
storage:user.events:read,
storage:user.sessions:read,
storage:user.replays:read,
storage:buckets:read,
storage:buckets:write,
storage:bucket-definitions:read,
storage:bucket-definitions:write,
storage:bucket-definitions:delete,
storage:bucket-definitions:truncate,
storage:fieldsets:read,
storage:fieldset-definitions:read,
storage:fieldset-definitions:write,
storage:files:read,
storage:files:write,
storage:files:delete,
storage:filter-segments:read,
storage:filter-segments:write,
storage:filter-segments:share,
storage:filter-segments:delete,
storage:filter-segments:admin,
storage:records:delete,
iam:users:read,
iam:groups:read,
iam:policies:read,
notifications:read,
notifications:write,
vulnerabilities:read,
davis:analyzers:read,
davis:analyzers:execute,
davis-copilot:conversations:execute,
davis-copilot:nl2dql:execute,
davis-copilot:dql2nl:execute,
davis-copilot:document-search:execute,
app-engine:apps:install,
app-engine:apps:run,
app-engine:apps:delete,
app-engine:functions:run,
app-engine:edge-connects:read,
app-engine:edge-connects:write,
email:emails:send
```

---

## Quick Reference by Resource Type

### Workflows
| Scope | Description |
|-------|-------------|
| `automation:workflows:read` | Read workflow definitions |
| `automation:workflows:write` | Create, update, delete workflows |
| `automation:workflows:execute` | Execute workflows |

### Documents (Dashboards & Notebooks)
| Scope | Description |
|-------|-------------|
| `document:documents:read` | Read dashboards and notebooks |
| `document:documents:write` | Create, update, delete documents |

### DQL Queries & Grail Data
| Scope | Description |
|-------|-------------|
| `storage:logs:read` | Read logs |
| `storage:logs:write` | Write logs |
| `storage:events:read` | Read events |
| `storage:events:write` | Write events |
| `storage:metrics:read` | Read metrics |
| `storage:metrics:write` | Write metrics |
| `storage:spans:read` | Read spans |
| `storage:bizevents:read` | Read business events |
| `storage:entities:read` | Read entities |
| `storage:smartscape:read` | Read topology |
| `storage:system:read` | Read system tables |
| `storage:security.events:read` | Read security events |
| `storage:application.snapshots:read` | Read app snapshots |
| `storage:user.events:read` | Read user events |
| `storage:user.sessions:read` | Read user sessions |
| `storage:user.replays:read` | Read session replays |
| `storage:fieldsets:read` | Read fieldsets |
| `storage:fieldset-definitions:read` | Read fieldset schemas |
| `storage:fieldset-definitions:write` | Write fieldset schemas |
| `storage:files:read` | Read files/lookup tables |
| `storage:files:write` | Write files/lookup tables |
| `storage:files:delete` | Delete files/lookup tables |
| `storage:filter-segments:read` | Read filter segments |
| `storage:filter-segments:write` | Write filter segments |
| `storage:filter-segments:share` | Share filter segments |
| `storage:filter-segments:delete` | Delete own filter segments |
| `storage:filter-segments:admin` | Admin all filter segments |
| `storage:records:delete` | Delete records in Grail |

### Bucket Management
| Scope | Description |
|-------|-------------|
| `storage:buckets:read` | Read from buckets |
| `storage:bucket-definitions:read` | Read bucket definitions |
| `storage:bucket-definitions:write` | Create/update bucket definitions |
| `storage:bucket-definitions:delete` | Delete bucket definitions |
| `storage:bucket-definitions:truncate` | Truncate bucket data |

### SLOs
| Scope | Description |
|-------|-------------|
| `slo:slos:read` | Read SLOs |
| `slo:slos:write` | Create, update, delete, evaluate SLOs |

### Settings API
| Scope | Description |
|-------|-------------|
| `settings:schemas:read` | Read settings schemas |
| `settings:objects:read` | Read settings objects |
| `settings:objects:write` | Create, update, delete settings |
| `settings:objects:admin` | Admin access for ownership |

### Davis AI
| Scope | Description |
|-------|-------------|
| `davis:analyzers:read` | View analyzers |
| `davis:analyzers:execute` | Execute analyzers |
| `davis-copilot:conversations:execute` | CoPilot chat |
| `davis-copilot:nl2dql:execute` | Natural language to DQL |
| `davis-copilot:dql2nl:execute` | DQL to natural language |
| `davis-copilot:document-search:execute` | Document search |

### App Engine
| Scope | Description |
|-------|-------------|
| `app-engine:apps:install` | Install/update apps |
| `app-engine:apps:run` | List/run apps, user metadata |
| `app-engine:apps:delete` | Uninstall apps |
| `app-engine:functions:run` | Execute functions |
| `app-engine:edge-connects:read` | Read EdgeConnect |
| `app-engine:edge-connects:write` | Manage EdgeConnect |

### IAM

> **Note**: The `iam:users:read` and `iam:groups:read` scopes may not be available in all token management UIs (e.g., the platform token page). If unavailable, user and group listing features will not work with that token type.

| Scope | Description |
|-------|-------------|
| `iam:users:read` | Read users |
| `iam:groups:read` | Read groups |
| `iam:policies:read` | Read policies |

### Other
| Scope | Description |
|-------|-------------|
| `notifications:read` | Read notifications |
| `notifications:write` | Create/update notifications |
| `vulnerabilities:read` | Read vulnerabilities |
