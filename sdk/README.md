# dt-cli-sdk

A shared Go library of Dynatrace CLI primitives, hosted as a second module inside the [dtctl](https://github.com/dynatrace-oss/dtctl) repository.

## Install

```bash
go get github.com/dynatrace-oss/dtctl/sdk@latest
```

## Packages

| Package | Description |
|---------|-------------|
| `sdk/urls` | Dynatrace environment URL validation and suggestion |
| `sdk/auth` | Token type detection (API token vs OAuth/Bearer) |
| `sdk/httpclient` | HTTP client with retry, typed errors, and pagination |
| `sdk/agentmode` | AI agent detection and structured JSON envelope |
| `sdk/credstore` | OS keyring and file-based credential storage |

## Design Principles

- **No global state.** Everything is constructed explicitly.
- **Minimal dependencies.** No Cobra, Viper, logrus, or OpenTelemetry.
- **One-way dependency.** CLI → SDK, never SDK → CLI.
- **Logging is injected.** The SDK accepts a `Logger` interface; it never imports a logging library.
- **Errors are typed.** Use `errors.Is`/`errors.As` reliably.

## Versioning

- `sdk/v0.x.y` — no stability promise during v0.
- `sdk/v1.0.0` — once at least two consumer CLIs have adopted Tier 1 packages.
