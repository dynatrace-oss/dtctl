# Changelog

All notable changes to the dt-cli-sdk will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

- `sdk/inventory` — Environment data-inventory discovery engine: capabilities present/absent/unknown with cited evidence, live entity census, buckets, data-object catalog partition, and a budgeted discovery battery. Execution-agnostic — every query runs through a caller-supplied `Runner` interface, so backend services can embed discovery with their own DQL client. Definitions parsing is byte-based (`ParseDefinitions`); file loading stays in the CLI layer.

- `sdk/api/query` — `ExecuteRequest.PollingPromiseSeconds` field maps to the new `pollingPromiseSeconds` body parameter on `query:execute`, instructing the backend to auto-cancel a running query if the client does not poll within the specified number of seconds. `Handler.ExecuteAndPoll` defaults the field to 5 seconds when the caller leaves it unset; a caller-supplied non-zero value is preserved.

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
