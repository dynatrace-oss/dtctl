# dtctl Implementation Status

Last Updated: January 2026

## Overview

This document tracks the current implementation status of dtctl. For future planned features, see [FUTURE_FEATURES.md](FUTURE_FEATURES.md).

---

## Implemented Features ✅

### Core Infrastructure
- [x] Go module with Cobra CLI framework
- [x] Configuration management (YAML config, contexts, token storage)
- [x] Context safety levels (readonly, readwrite-mine, readwrite-all, dangerously-unrestricted)
- [x] HTTP client with retry, rate limiting, error handling
- [x] Output formatters: JSON, YAML, table, wide, CSV, chart, sparkline, barchart
- [x] Global flags: `--context`, `--output`, `--verbose`, `--dry-run`, `--chunk-size`, `--show-diff`
- [x] Shell completion (bash, zsh, fish)
- [x] Automatic pagination with `--chunk-size` (default 500)
- [x] User identity: `dtctl auth whoami` (via metadata API with JWT fallback)
- [x] OS keychain integration for secure token storage

### Verbs Implemented
- [x] `get` - List/retrieve resources
- [x] `describe` - Detailed resource info
- [x] `create` - Create from manifest
- [x] `delete` - Delete resources
- [x] `edit` - Edit in $EDITOR
- [x] `apply` - Create or update
- [x] `exec` - Execute workflows, analyzers, copilot, functions, SLOs
- [x] `logs` - View execution logs
- [x] `query` - Execute DQL queries
- [x] `wait` - Wait for conditions on resources (polling with exponential backoff)
- [x] `history` - Show version history (snapshots)
- [x] `restore` - Restore to previous version
- [x] `share/unshare` - Share dashboards and notebooks

### Resources

| Resource | get | describe | create | delete | edit | apply | exec | logs | share | history | restore | --mine |
|----------|-----|----------|--------|--------|------|-------|------|------|-------|---------|---------|--------|
| **workflow** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | ✅ | ✅ | ✅ |
| **execution** | ✅ | ✅ | - | - | - | - | - | ✅ | - | - | - | - |
| **dashboard** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | ✅ | ✅ | ✅ | ✅ |
| **notebook** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | ✅ | ✅ | ✅ | ✅ |
| **slo** | ✅ | ✅ | ✅ | ✅ | - | ✅ | ✅ | - | - | - | - | - |
| **slo-template** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **notification** | ✅ | ✅ | - | ✅ | - | - | - | - | - | - | - | - |
| **bucket** | ✅ | ✅ | ✅ | ✅ | - | ✅ | - | - | - | - | - | - |
| **settings** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | - | - | - | - |
| **app** | ✅ | ✅ | - | ✅ | - | - | - | - | - | - | - | - |
| **function** | ✅ | - | - | - | - | - | ✅ | - | - | - | - | - |
| **edgeconnect** | ✅ | ✅ | ✅ | ✅ | - | - | - | - | - | - | - | - |
| **user** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **group** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **analyzer** | ✅ | ✅ | - | - | - | - | ✅ | - | - | - | - | - |
| **copilot** | ✅ | - | - | - | - | - | ✅ | - | - | - | - | - |
| **lookup** | ✅ | ✅ | ✅ | ✅ | - | ✅ | - | - | - | - | - | - |

### DQL Query Features
- [x] Inline queries: `dtctl query "fetch logs | limit 10"`
- [x] File-based queries: `dtctl query -f query.dql`
- [x] Template variables: `--set key=value`
- [x] All output formats supported
- [x] Chart output for timeseries: `dtctl query "timeseries ..." -o chart`
- [x] Live mode with periodic updates: `--live`, `--interval`
- [x] Customizable chart dimensions: `--width`, `--height`, `--fullscreen`
- [x] Custom record/byte/scan limits

### SLO Features
- [x] List SLOs: `dtctl get slos`
- [x] Get SLO details: `dtctl describe slo <id>`
- [x] List SLO templates: `dtctl get slo-templates`
- [x] Create/update SLOs: `dtctl apply -f slo.yaml`
- [x] Evaluate SLOs: `dtctl exec slo <id>`
- [x] Evaluation with custom timeout: `--timeout`
- [x] Automatic polling with exponential backoff
- [x] Table, JSON, and YAML output formats

### Davis AI Features
- [x] List analyzers: `dtctl get analyzers`
- [x] Execute analyzer: `dtctl exec analyzer <name> -f input.json`
- [x] Chat with CoPilot: `dtctl exec copilot "question"` (streaming)
- [x] NL to DQL: `dtctl exec copilot nl2dql "show error logs"`
- [x] Document search: `dtctl exec copilot document-search "query"`

### Wait Features
- [x] Wait for DQL query conditions: `dtctl wait query`
- [x] Supported conditions: count=N, count-gte, count-gt, count-lte, count-lt, any, none
- [x] Exponential backoff strategy with configurable parameters
- [x] Custom timeout and max attempts
- [x] File-based queries with template variables: `--file`, `--set`
- [x] Quiet and verbose modes for output control
- [x] All DQL query options supported (timeframe, limits, locale, etc.)
- [x] Exit codes for different failure scenarios (timeout, max attempts, errors)
- [x] Output results in various formats when condition is met

### Build & Release
- [x] CI/CD with GitHub Actions (testing, linting, security)
- [x] GoReleaser for multi-platform binaries
- [x] Vulnerability scanning with govulncheck

---

## Planned Features

### CLI Features
- [ ] Watch mode (`--watch`)
- [ ] Standalone diff command
- [ ] Patch command
- [ ] Bulk operations (apply from directory)
- [ ] JSONPath output

### Resource Gaps
- [ ] Document trash (list/restore deleted)
- [ ] Function describe

---

## Future Planned Features 🔮

See [FUTURE_FEATURES.md](FUTURE_FEATURES.md) for the complete implementation plan including:
- Platform Management (environment info, license)
- State Management for Apps
- Grail Filter Segments
- Grail Fieldsets
- Grail Resource Store

---

## Quality & Infrastructure

### Distribution
- [x] Multi-platform binaries (Linux, macOS, Windows - AMD64/ARM64)
- [x] GitHub Releases
- [ ] Homebrew tap
- [ ] Container image

### Testing
- [x] Unit tests for core packages
- [x] Integration tests
- [x] E2E tests
- [ ] Improve test coverage (target: 80%+)

### Code Quality
- [x] Linting (golangci-lint)
- [x] Security scanning
- [x] CI/CD pipeline
- [ ] Split large command files for better maintainability

---

## Notes

- Classic environment (v1/v2) APIs are explicitly excluded per design
- Focus on platform APIs (v2 and newer) only
- kubectl naming conventions are followed (e.g., `exec` not `execute`)
