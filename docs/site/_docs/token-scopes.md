---
layout: docs
title: Token Scopes
---

dtctl's [safety levels](configuration#safety-levels) are a client-side guardrail. The actual access control is determined by the scopes assigned to your Dynatrace platform token. This page documents the recommended scopes for each safety level and resource type.

## Quick Reference

| Safety Level | Use Case | Token Type |
|---|---|---|
| `readonly` | Monitoring, troubleshooting, auditing | Platform token with read scopes |
| `readwrite-mine` | Managing your own workflows and documents | Platform token with read + write scopes |
| `readwrite-all` | Team-wide resource management | Platform token with full write scopes |
| `dangerously-unrestricted` | Admin operations, data/bucket deletion | Platform token with all scopes |

## Scopes by Safety Level

### readonly

Read-only access for monitoring and troubleshooting. Approximately 30 scopes:

This level does not include the Live Debugger write scope `dev-obs:breakpoints:set`.

```
automation:workflows:read
automation:calendars:read
automation:rules:read
document:documents:read
document:documents:share
document:trash-documents:read
document:environment-shares:read
storage:events:read
storage:logs:read
storage:metrics:read
storage:entities:read
storage:bizevents:read
storage:spans:read
storage:system:read
storage:fieldsets:read
storage:buckets:read
storage:filter-segments:read
settings:objects:read
settings:schemas:read
app-engine:apps:read
app-engine:edge-connects:read
extension:extensions:read
hub:catalog:read
state:app-states:read
state:user-app-states:read
davis:analyzers:read
slo:read
openPipeline:configurations:read
iam:policies:read
iam:bindings:read
iam:groups:read
iam:users:read
iam:service-users:read
notification:notifications:read
```

### readwrite-mine

Adds write scopes for personal resources (your own workflows, documents, etc.):

```
# All readonly scopes above, plus:
automation:workflows:write
automation:workflows:run
automation:calendars:write
automation:rules:write
dev-obs:breakpoints:set
document:documents:write
document:documents:delete
document:trash-documents:delete
storage:filter-segments:write
storage:filter-segments:delete
settings:objects:write
extension:extensions:write
davis:analyzers:write
slo:write
openPipeline:configurations:write
notification:notifications:write
```

### readwrite-all

Full resource management across the environment. Does not include data or bucket deletion:

```
# All readwrite-mine scopes above, plus:
document:documents:admin
document:environment-shares:write
document:environment-shares:claim
app-engine:apps:install
app-engine:apps:delete
app-engine:edge-connects:write
app-engine:edge-connects:delete
hub:catalog:write
state:app-states:write
state:user-app-states:write
iam:policies:write
iam:bindings:write
iam:groups:write
iam:service-users:write
```

### dangerously-unrestricted

Full admin access including bucket management and data deletion:

```
# All readwrite-all scopes above, plus:
storage:buckets:write
storage:buckets:delete
storage:events:write
storage:logs:write
storage:metrics:write
storage:bizevents:write
storage:spans:write
storage:fieldsets:write
```

## Per-Resource Scope Reference

### Workflows

| Operation | Required Scope |
|---|---|
| List / Get / Describe | `automation:workflows:read` |
| Create / Update / Apply | `automation:workflows:write` |
| Execute / Run | `automation:workflows:run` |
| Calendar access | `automation:calendars:read`, `automation:calendars:write` |
| Event triggers | `automation:rules:read`, `automation:rules:write` |

### Documents and Dashboards

| Operation | Required Scope |
|---|---|
| List / Get / Describe | `document:documents:read` |
| Create / Update / Apply | `document:documents:write` |
| Delete | `document:documents:delete` |
| Admin (all documents) | `document:documents:admin` |
| Share management | `document:documents:share`, `document:environment-shares:write` |
| Claim shared documents | `document:environment-shares:claim` |
| Trash operations | `document:trash-documents:read`, `document:trash-documents:delete` |

### DQL and Grail Data

| Operation | Required Scope |
|---|---|
| Query logs | `storage:logs:read` |
| Query metrics | `storage:metrics:read` |
| Query events | `storage:events:read` |
| Query business events | `storage:bizevents:read` |
| Query entities | `storage:entities:read` |
| Query spans | `storage:spans:read` |
| System tables | `storage:system:read` |
| Field sets | `storage:fieldsets:read`, `storage:fieldsets:write` |
| Ingest / write data | `storage:logs:write`, `storage:events:write`, etc. |

### Bucket Management

| Operation | Required Scope |
|---|---|
| List / Describe buckets | `storage:buckets:read` |
| Create / Update buckets | `storage:buckets:write` |
| Delete buckets | `storage:buckets:delete` |

### Filter Segments

| Operation | Required Scope |
|---|---|
| List / Get / Describe | `storage:filter-segments:read` |
| Create / Update / Apply | `storage:filter-segments:write` |
| Delete | `storage:filter-segments:delete` |
| Use in queries (`--segment`) | `storage:filter-segments:read` |

### SLOs

| Operation | Required Scope |
|---|---|
| List / Get / Describe | `slo:read` |
| Create / Update / Delete | `slo:write` |

### Settings

| Operation | Required Scope |
|---|---|
| List / Get / Describe | `settings:objects:read` |
| Create / Update / Delete | `settings:objects:write` |
| Schema discovery | `settings:schemas:read` |

### Extensions

| Operation | Required Scope |
|---|---|
| List / Get / Describe | `extensions:definitions:read` |
| Upload custom extension, install from Hub | `extensions:definitions:write` |
| Read monitoring configurations | `extensions:configurations:read` |
| Create / Update / Delete monitoring configurations | `extensions:configurations:write` |
| Hub catalog (browse) | `extensions:definitions:read` |

### Davis AI

| Operation | Required Scope |
|---|---|
| List / Get analyzers | `davis:analyzers:read` |
| Create / Update analyzers | `davis:analyzers:write` |

### App Engine

| Operation | Required Scope |
|---|---|
| List / Get apps | `app-engine:apps:read` |
| Install apps | `app-engine:apps:install` |
| Delete apps | `app-engine:apps:delete` |
| Edge connects | `app-engine:edge-connects:read`, `app-engine:edge-connects:write`, `app-engine:edge-connects:delete` |

### Notifications

| Operation | Required Scope |
|---|---|
| List / Get | `notification:notifications:read` |
| Create / Update | `notification:notifications:write` |

### Live Debugger

| Operation | Required Scope |
|---|---|
| Set / update / delete breakpoints | `dev-obs:breakpoints:set` |

`dev-obs:breakpoints:set` is not included in the `readonly` safety level.

### OpenPipeline

| Operation | Required Scope |
|---|---|
| List / Get configurations | `openPipeline:configurations:read` |
| Update configurations | `openPipeline:configurations:write` |

### IAM (Identity & Access Management)

| Operation | Required Scope |
|---|---|
| List users | `iam:users:read` |
| List groups | `iam:groups:read`, `iam:groups:write` |
| List service users | `iam:service-users:read`, `iam:service-users:write` |
| View policies | `iam:policies:read`, `iam:policies:write` |
| View bindings | `iam:bindings:read`, `iam:bindings:write` |

> **Note:** IAM scopes (`iam:*`) are not available in all token creation UIs. They may require account-level token management or OAuth client credentials depending on your Dynatrace deployment.
