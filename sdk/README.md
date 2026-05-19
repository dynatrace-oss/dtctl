# dtctl SDK

A shared Go module for building tools and integrations against the Dynatrace platform, extracted from [dtctl](https://github.com/dynatrace-oss/dtctl).

## Install

```bash
go get github.com/dynatrace-oss/dtctl/sdk@latest
```

## What's in the SDK

### Infrastructure

| Package | Description |
|---------|-------------|
| `sdk/httpclient` | HTTP client with retry, rate limiting, typed errors, pagination helpers |
| `sdk/auth` | Token type detection (API token vs OAuth/Bearer) |
| `sdk/urls` | Dynatrace environment URL validation and normalization |
| `sdk/credstore` | OS keyring and file-based credential storage |
| `sdk/agentmode` | AI agent environment detection and structured JSON envelope |

### API wrappers (`sdk/api/`)

Thin, typed Go clients for Dynatrace REST APIs. Each package covers one API surface with CRUD operations, pagination, and structured error handling.

| Package | API |
|---------|-----|
| `api/analyzer` | Query analyzer (DQL validation) |
| `api/appengine` | App Engine (functions) |
| `api/bucket` | Grail bucket management |
| `api/copilot` | Davis Copilot completions |
| `api/document` | Documents (dashboards, notebooks) + trash |
| `api/edgeconnect` | EdgeConnect management |
| `api/extension` | Extensions 2.0 lifecycle |
| `api/hub` | Dynatrace Hub items |
| `api/iam` | Account-level IAM (groups, users, policies, service users) |
| `api/livedebugger` | Live debugger sessions |
| `api/notification` | Notification/alerting configuration |
| `api/query` | DQL query execution |
| `api/segment` | Data segments |
| `api/settings` | Settings 2.0 objects |
| `api/slo` | Service Level Objectives |
| `api/workflow` | Workflow CRUD and execution |

## What goes into the SDK

- **Direct REST API wrappers.** One package per Dynatrace API surface, exposing typed Go structs and CRUD functions that map 1:1 to API endpoints.
- **Shared infrastructure.** HTTP client, auth, URL handling, credential storage -- anything two or more consumers would need.
- **No CLI concerns.** No Cobra, no Viper, no terminal formatting, no interactive prompts.

## What does NOT go into the SDK

- **Convenience wrappers that compose multiple APIs.** If a function calls API A then API B (e.g. `EnsureEnvironmentShare`), it belongs in the CLI.
- **Display/formatting logic.** Functions like `populateDisplayFields`, `GetRaw`, DQL-to-AST conversion, or anything that transforms data for human consumption.
- **Packages that wrap non-Dynatrace APIs** or aggregate multiple API calls into a higher-level operation (e.g. `anomalydetector`, `azureconnection`, `gcpconnection`).
- **CLI-specific types.** Structs with fields that only exist for display purposes (e.g. `VariablesDisplay`, computed summary fields).
- **Lookup/resolution helpers.** Name-to-ID resolution, interactive disambiguation, resource resolvers.

## Design principles

- **No global state.** Everything is constructed explicitly.
- **No file I/O.** The SDK never reads from files or stdin. Functions like `ReadFileOrStdin` and `ParseInputFromFile` belong in the CLI layer (`pkg/resources/`). SDK functions accept Go types, never file paths.
- **Minimal dependencies.** No Cobra, Viper, logrus, or OpenTelemetry.
- **One-way dependency.** CLI -> SDK, never SDK -> CLI.
- **Logging is injected.** The SDK accepts a `Logger` interface; it never imports a logging library.
- **Errors are typed.** Use `errors.Is`/`errors.As` reliably. API errors follow the format `"API error (<code>): <message>"`.

## Versioning

- `sdk/v0.x.y` -- no stability promise during v0.
- `sdk/v1.0.0` -- once at least two consumer CLIs have adopted the module.
