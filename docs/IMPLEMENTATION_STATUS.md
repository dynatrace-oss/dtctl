# dtctl Implementation Status

## Overview

This document tracks implemented features and planned work for dtctl.

---

## Implemented Features ✅

### Core Infrastructure
- [x] Go module with Cobra CLI framework
- [x] Configuration management (YAML config, contexts, token storage)
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
- [x] `exec` - Execute workflows, analyzers, copilot, functions
- [x] `logs` - View execution logs
- [x] `query` - Execute DQL queries
- [x] `history` - Show version history (snapshots)
- [x] `restore` - Restore to previous version
- [x] `share/unshare` - Share dashboards and notebooks

### Resources

| Resource | get | describe | create | delete | edit | apply | exec | logs | share | history | restore | --mine |
|----------|-----|----------|--------|--------|------|-------|------|------|-------|---------|---------|--------|
| **workflow** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | ✅ | ✅ | - |
| **execution** | ✅ | ✅ | - | - | - | - | - | ✅ | - | - | - | - |
| **dashboard** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | ✅ | ✅ | ✅ | ✅ |
| **notebook** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | - | - | ✅ | ✅ | ✅ | ✅ |
| **settings** | ✅ | ✅ | ✅ | ✅ | - | ✅ | - | - | - | - | - | - |
| **settings-schema** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **slo** | ✅ | ✅ | ✅ | ✅ | - | ✅ | - | - | - | - | - | - |
| **slo-template** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **notification** | ✅ | ✅ | - | ✅ | - | - | - | - | - | - | - | - |
| **bucket** | ✅ | ✅ | ✅ | ✅ | - | ✅ | - | - | - | - | - | - |
| **openpipeline** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **app** | ✅ | ✅ | - | ✅ | - | - | - | - | - | - | - | - |
| **function** | ✅ | - | - | - | - | - | ✅ | - | - | - | - | - |
| **edgeconnect** | ✅ | ✅ | ✅ | ✅ | - | - | - | - | - | - | - | - |
| **user** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **group** | ✅ | ✅ | - | - | - | - | - | - | - | - | - | - |
| **analyzer** | ✅ | ✅ | - | - | - | - | ✅ | - | - | - | - | - |
| **copilot** | ✅ | - | - | - | - | - | ✅ | - | - | - | - | - |

### DQL Query Features
- [x] Inline queries: `dtctl query "fetch logs | limit 10"`
- [x] File-based queries: `dtctl query -f query.dql`
- [x] Template variables: `--set key=value`
- [x] All output formats supported
- [x] Chart output for timeseries: `dtctl query "timeseries ..." -o chart`
- [x] Live mode with periodic updates: `--live`, `--interval`
- [x] Customizable chart dimensions: `--width`, `--height`, `--fullscreen`
- [x] Custom record/byte/scan limits

### Davis AI Features
- [x] List analyzers: `dtctl get analyzers`
- [x] Execute analyzer: `dtctl exec analyzer <name> -f input.json`
- [x] Chat with CoPilot: `dtctl exec copilot "question"` (streaming)
- [x] NL to DQL: `dtctl exec copilot nl2dql "show error logs"`
- [x] Document search: `dtctl exec copilot document-search "query"`

### Build & Release
- [x] CI/CD with GitHub Actions (testing, linting, security)
- [x] GoReleaser for multi-platform binaries
- [x] Vulnerability scanning with govulncheck

---

## Planned Features

### CLI Features
- [ ] Label selectors (`-l env=prod`)
- [ ] Watch mode (`--watch`)
- [ ] Standalone diff command
- [ ] Patch command
- [ ] Bulk operations (apply from directory)
- [ ] JSONPath output

### Resource Gaps
- [ ] Document trash (list/restore deleted)
- [ ] Settings edit
- [ ] Function describe
- [ ] Workflow `--mine` filter

### Distribution
- [ ] Homebrew tap
- [ ] Container image

---

## Quality Improvements

### Test Coverage
- [ ] pkg/client tests (target: 90%)
- [ ] pkg/config tests (target: 90%)
- [ ] pkg/resources tests (target: 70%)

### Code Structure
- [ ] Split large command files (get.go, create.go, describe.go)
- [ ] Extract common client initialization
- [ ] Define ResourceHandler interface

### Security
- [ ] Input validation for editor paths
- [ ] File path sanitization

---

## Notes

- Classic environment (v1/v2) APIs are explicitly excluded per design
- kubectl naming conventions are followed (e.g., `exec` not `execute`)
