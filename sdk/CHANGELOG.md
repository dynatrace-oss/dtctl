# Changelog

All notable changes to the dt-cli-sdk will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.2.0] - 2026-05-16

### Added

- `sdk/api/` — Typed Go clients for 16 Dynatrace REST APIs, each with CRUD operations, pagination, and structured error handling:
  `analyzer`, `appengine`, `bucket`, `copilot`, `document`, `edgeconnect`, `extension`, `hub`, `iam`, `livedebugger`, `notification`, `query`, `segment`, `settings`, `slo`, `workflow`.

### Changed

- `sdk/httpclient` — Logger is now wired through the HTTP client; callers can inject a `Logger` to get request-level debug output.

### Dependencies

- Bump go-resty to v2.17.2, godbus to v5.2.2, go-keyring to v0.2.8.
- Bump golang.org/x/net.

## [0.1.0] - 2026-05-09

### Added

- `sdk/urls` — Dynatrace environment URL validation and correction.
- `sdk/auth` — Token type detection and classification.
- `sdk/httpclient` — HTTP client with retry, typed errors, and pagination.
- `sdk/agentmode` — AI agent detection and structured JSON envelope.
- `sdk/credstore` — OS keyring and file-based credential storage.
