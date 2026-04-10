# dtctl Implementation Status

Last Updated: March 2026

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
- [x] Global flags: `--context`, `--output`, `--verbose`, `--debug`, `--dry-run`, `--chunk-size`, `--show-diff`, `--agent`, `--no-agent`
- [x] Shell completion (bash, zsh, fish)
- [x] Automatic pagination with `--chunk-size` (default 500)
- [x] User identity: `dtctl auth whoami` (via metadata API with JWT fallback)
- [x] OS keychain integration for secure token storage
- [x] Command aliases: simple, parameterized ($1-$9), and shell aliases (with import/export)
- [x] AI agent detection in User-Agent header for telemetry
- [x] Agent output envelope (`--agent` / `-A`) with auto-detection, structured errors, and per-command context enrichment
- [x] Enhanced error messages with contextual troubleshooting suggestions
- [x] Machine-readable command catalog (`dtctl commands`) for AI agent bootstrap
- [x] [NO_COLOR](https://no-color.org/) standard: color disabled when piped, `NO_COLOR` env var, `FORCE_COLOR=1` override
- [x] Consistent help text: all parent verb commands have `Long` descriptions and Cobra `Example` fields

### Verbs Implemented
- [x] `get` - List/retrieve resources
- [x] `describe` - Detailed resource info (all subcommands support `-o json|yaml|toon|csv` and agent mode)
- [x] `create` - Create from manifest
- [x] `delete` - Delete resources
- [x] `edit` - Edit in $EDITOR
- [x] `apply` - Create or update
- [x] `diff` - Compare resources (local vs remote, file vs file, resource vs resource)
- [x] `exec` - Execute workflows, analyzers, copilot, functions, SLOs
- [x] `logs` - View execution logs
- [x] `query` - Execute DQL queries
- [x] `wait` - Wait for conditions on resources (polling with exponential backoff)
- [x] `history` - Show version history (snapshots)
- [x] `restore` - Restore to previous version
- [x] `share/unshare` - Share dashboards and notebooks
- [x] `alias` - Manage command aliases (set, list, delete, import, export)
- [x] `ctx` - Quick context management (list, switch, describe, set, delete)
- [x] `doctor` - Health check (config, context, token, connectivity, auth)
- [x] `commands` - Machine-readable command catalog (JSON/YAML, `--brief`, resource filter, `howto` subcommand)
- [x] `skills` - AI agent skill file management (install, uninstall, status for Claude, Copilot, Cursor, Kiro, Junie, OpenCode, OpenClaw; cross-client via `--cross-client`)

### Resources

#### Core Resources

| Resource | get | describe | create | delete | edit | apply |
|----------|-----|----------|--------|--------|------|-------|
| workflow | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| execution | ✅ | ✅ | - | - | - | - |
| document | ✅ | ✅ | ✅ | ✅ | ✅ | - |
| dashboard | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| notebook | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| slo | ✅ | ✅ | ✅ | ✅ | - | ✅ |
| slo-template | ✅ | ✅ | - | - | - | - |
| notification | ✅ | ✅ | - | ✅ | - | - |
| bucket | ✅ | ✅ | ✅ | ✅ | - | ✅ |
| settings | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| app | ✅ | ✅ | - | ✅ | - | - |
| function | ✅ | ✅ | - | - | - | - |
| edgeconnect | ✅ | ✅ | ✅ | ✅ | - | - |
| user | ✅ | ✅ | - | - | - | - |
| group | ✅ | ✅ | - | - | - | - |
| analyzer | ✅ | ✅ | - | - | - | - |
| copilot | ✅ | - | - | - | - | - |
| lookup | ✅ | ✅ | ✅ | ✅ | - | ✅ |
| extension | ✅ | ✅ | - | - | - | - |
| extension-config | ✅ | ✅ | - | - | - | ✅ |
| intent | ✅ | ✅ | - | - | - | - |
| segment | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| anomaly-detector | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

#### Cloud Connections

| Resource | get | describe | create | delete | apply |
|----------|-----|----------|--------|--------|-------|
| azure connection | ✅ | ✅ | ✅ | ✅ | ✅ |
| azure monitoring | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ activate |
| aws connection | - | - | - | - | - |
| aws monitoring | - | - | - | - | - |
| gcp connection (Preview) | ✅ | ✅ | ✅ | ✅ | ✅ |
| gcp monitoring (Preview) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ activate |

#### Advanced Operations

| Resource | diff | exec | logs | share | history | restore | --mine | --watch |
|----------|------|------|------|-------|---------|---------|--------|---------|
| workflow | ✅ | ✅ | - | - | ✅ | ✅ | ✅ | ✅ |
| execution | - | - | ✅ | - | - | - | - | ✅ |
| document | - | - | - | - | ✅ | ✅ | ✅ | ✅ |
| dashboard | ✅ | - | - | ✅ | ✅ | ✅ | ✅ | ✅ |
| notebook | ✅ | - | - | ✅ | ✅ | ✅ | ✅ | ✅ |
| slo | - | ✅ | - | - | - | - | - | ✅ |
| slo-template | - | - | - | - | - | - | - | - |
| notification | - | - | - | - | - | - | - | ✅ |
| bucket | - | - | - | - | - | - | - | ✅ |
| settings | - | - | - | - | - | - | - | - |
| app | - | - | - | - | - | - | - | - |
| function | - | ✅ | - | - | - | - | - | - |
| analyzer | - | ✅ | - | - | - | - | - | - |
| copilot | - | ✅ | - | - | - | - | - | - |
| segment | - | - | - | - | - | - | - | ✅ |
| anomaly-detector | - | - | - | - | - | - | - | - |

### Watch Mode Features
- [x] Watch all `get` commands: `dtctl get workflows --watch`
- [x] Live mode for DQL queries: `dtctl query "fetch logs" --live`
- [x] Configurable polling interval: `--interval` (default: 2s for watch, 60s for live)
- [x] Skip initial state: `--watch-only` (for `get` commands only)
- [x] Incremental change display with kubectl-style prefixes:
  - `+` (green) for additions
  - `~` (yellow) for modifications
  - `-` (red) for deletions
- [x] Graceful shutdown on Ctrl+C
- [x] Automatic retry on transient errors (timeouts, rate limits, network issues)
- [x] Memory-efficient (only stores last state)
- [x] Works with existing filters and flags (e.g., `--mine`, `--name`)

### DQL Query Features
- [x] Inline queries: `dtctl query "fetch logs | limit 10"`
- [x] File-based queries: `dtctl query -f query.dql`
- [x] Template variables: `--set key=value`
- [x] All output formats supported (table, JSON, YAML, CSV, TOON)
- [x] Decoded Live Debugger snapshots: `dtctl query "fetch application.snapshots | limit 5" --decode-snapshots`
- [x] Chart output for timeseries: `dtctl query "timeseries ..." -o chart`
- [x] Live mode with periodic updates: `--live`, `--interval`
- [x] Watch mode with incremental updates: `--watch`, `--interval`
- [x] Customizable chart dimensions: `--width`, `--height`, `--fullscreen`
- [x] Custom record/byte/scan limits
- [x] Query metadata output: `--metadata` / `-M` with field selection

### SLO Features
- [x] List SLOs: `dtctl get slos`
- [x] Get SLO details: `dtctl describe slo <id>`
- [x] List SLO templates: `dtctl get slo-templates`
- [x] Create/update SLOs: `dtctl apply -f slo.yaml`
- [x] Evaluate SLOs: `dtctl exec slo <id>`
- [x] Evaluation with custom timeout: `--timeout`
- [x] Automatic polling with exponential backoff
- [x] Table, JSON, YAML, and TOON output formats

### Diff Features
- [x] Compare local file with remote resource: `dtctl diff -f workflow.yaml`
- [x] Compare two local files: `dtctl diff -f file1.yaml -f file2.yaml`
- [x] Compare two remote resources: `dtctl diff workflow prod-wf staging-wf`
- [x] Multiple output formats:
  - Unified diff (default)
  - Side-by-side comparison (`--side-by-side`)
  - JSON Patch (RFC 6902) (`-o json-patch`)
  - Semantic diff with impact analysis (`--semantic`)
- [x] Metadata filtering: `--ignore-metadata`
- [x] Order-independent comparison: `--ignore-order`
- [x] Quiet mode for CI/CD: `--quiet` (exit code only)
- [x] Proper exit codes: 0 (no changes), 1 (changes), 2 (error)
- [x] Supported resources: workflow, dashboard, notebook
- [x] Auto-detection of resource type and ID from files
- [x] Deep nested structure comparison
- [x] Colorized output support

### Davis AI Features
- [x] List analyzers: `dtctl get analyzers`
- [x] Execute analyzer: `dtctl exec analyzer <name> -f input.json`
- [x] Chat with CoPilot: `dtctl exec copilot "question"` (streaming)
- [x] NL to DQL: `dtctl exec copilot nl2dql "show error logs"`
- [x] Document search: `dtctl exec copilot document-search "query"`

### Custom Anomaly Detector Features
- [x] List detectors: `dtctl get anomaly-detectors` (alias: `ad`)
- [x] Filter by enabled state: `--enabled` / `--enabled=false`
- [x] Get detector details: `dtctl describe anomaly-detector <id-or-title>`
- [x] Create from YAML/JSON: `dtctl create anomaly-detector -f detector.yaml`
- [x] Edit in $EDITOR: `dtctl edit anomaly-detector <id-or-title>`
- [x] Delete: `dtctl delete anomaly-detector <id-or-title>`
- [x] Apply (create/update): `dtctl apply -f detector.yaml`
- [x] Flattened YAML format (human-friendly) and raw Settings API format
- [x] Source defaults to `"dtctl"` when omitted
- [x] Recent problems cross-reference via DQL in describe output
- [x] Template variables: `--set threshold=95`

### App Functions Features
- [x] List all functions: `dtctl get functions`
- [x] Filter by app: `dtctl get functions --app <app-id>`
- [x] Get function details: `dtctl get function <app-id>/<function-name>`
- [x] Describe function: `dtctl describe function <app-id>/<function-name>`
- [x] Execute functions: `dtctl exec function <app-id>/<function-name>`
- [x] Function metadata: title, description, resumable, stateful flags
- [x] Wide output with all metadata

### Azure Connection Features
- [x] List connections: `dtctl get azure connections`
- [ ] Filter connection list with dedicated flags
- [x] Get by name or object ID: `dtctl get azure connections <name-or-id>`
- [x] Describe connection: `dtctl describe azure connection <id>`
- [x] Create connection: `dtctl create azure connection --name <name> --type <federatedIdentityCredential|clientSecret>`
- [x] Update connection: `dtctl update azure connection --name <name> --directoryId <tenant-id> --applicationId <client-id>`
- [x] Delete by name or ID: `dtctl delete azure connection <name-or-id>`
- [x] Apply from manifest (idempotent): `dtctl apply -f azure_connection.yaml`

### Azure Monitoring Configuration Features
- [x] List configs: `dtctl get azure monitoring`
- [ ] Filter config list with dedicated flags
- [x] Get by description or ID: `dtctl get azure monitoring <description-or-id>`
- [x] Describe config: `dtctl describe azure monitoring <id-or-name>`
- [x] Runtime status in describe (Smartscape, metrics, recent events)
- [x] Create config (created as disabled): `dtctl create azure monitoring --name <name> --credentials <connection-name-or-id>`
- [x] Activate config (update connection + enable): `dtctl activate azure monitoring --name <name> [--directoryId <tenant-id>] [--applicationId <client-id>]`
- [x] Update config: `dtctl update azure monitoring --name <name> [--locationFiltering ...] [--featureSets ...]`
- [x] Delete by name or ID: `dtctl delete azure monitoring <name-or-id>`
- [x] Apply from manifest (idempotent): `dtctl apply -f azure_monitoring_config.yaml`
- [x] Schema helpers: `dtctl get azure monitoring-locations`, `dtctl get azure monitoring-feature-sets`

### AWS Connection Features
- [ ] List connections
- [ ] Filter connections
- [ ] Get by name or ID
- [ ] Describe connection
- [ ] Create/update/delete/apply connection

### AWS Monitoring Configuration Features
- [ ] List monitoring configs
- [ ] Filter monitoring configs
- [ ] Get by name or ID
- [ ] Describe monitoring config
- [ ] Create/update/delete/apply monitoring config

### GCP Connection Features (Preview)
- [x] List connections: `dtctl get gcp connections`
- [ ] Filter connections
- [x] Get by name or ID: `dtctl get gcp connections <name-or-id>`
- [x] Describe connection: `dtctl describe gcp connection <id>`
- [x] Create connection: `dtctl create gcp connection --name <name> --serviceAccountId <service-account-email>`
- [x] Update connection: `dtctl update gcp connection --name <name> --serviceAccountId <service-account-email>`
- [x] Delete by name or ID: `dtctl delete gcp connection <name-or-id>`
- [x] Apply from manifest (idempotent): `dtctl apply -f gcp_connection.yaml`
- [x] Dynatrace GCP principal is auto-created by backend on first HAS connection

### GCP Monitoring Configuration Features (Preview)
- [x] List monitoring configs: `dtctl get gcp monitoring`
- [ ] Filter monitoring configs
- [x] Get by description or ID: `dtctl get gcp monitoring <description-or-id>`
- [x] Describe config: `dtctl describe gcp monitoring <id-or-name>`
- [x] Runtime status in describe (Smartscape, metrics, recent events)
- [x] Create config (created as disabled): `dtctl create gcp monitoring --name <name> --credentials <connection-name-or-id>`
- [x] Activate config (update connection + enable): `dtctl activate gcp monitoring --name <name> [--serviceAccountId <email>]`
- [x] Update config: `dtctl update gcp monitoring --name <name> [--locationFiltering ...] [--featureSets ...]`
- [x] Delete by name or ID: `dtctl delete gcp monitoring <name-or-id>`
- [x] Apply from manifest (idempotent): `dtctl apply -f gcp_monitoring_config.yaml`
- [x] Schema helpers: `dtctl get gcp monitoring-locations`, `dtctl get gcp monitoring-feature-sets`

### App Intents Features
- [x] List all intents: `dtctl get intents`
- [x] Filter by app: `dtctl get intents --app <app-id>`
- [x] Get intent details: `dtctl get intent <app-id>/<intent-id>`
- [x] Describe intent: `dtctl describe intent <app-id>/<intent-id>`
- [x] Find matching intents: `dtctl find intents --data <key>=<value>`
- [x] Generate intent URL: `dtctl open intent <app-id>/<intent-id> --data <key>=<value>`
- [x] Open URL in browser: `--browser` flag
- [x] JSON file support: `--data-file` flag
- [x] Intent metadata: properties, required fields, descriptions

### Live Debugger Features (Experimental)
- [x] Configure workspace filters: `dtctl update breakpoint --filters key:value[,key:value...]` (also supports `key=value`)
- [x] Create breakpoint: `dtctl create breakpoint File.java:line`
- [x] List breakpoints: `dtctl get breakpoints`
- [x] Describe breakpoint status by ID or location: `dtctl describe <id|filename:line>`
- [x] Update breakpoint condition/enabled state: `dtctl update breakpoint <id|filename:line> --condition ... --enabled ...`
- [x] Delete breakpoint by ID/location and bulk delete with confirmation: `dtctl delete breakpoint <id|filename:line|--all> [-y] [--dry-run]`
- [x] Verbose GraphQL troubleshooting output with `-v/--debug`
- [x] Safety checks applied to create/update/delete and workspace filter updates
- [x] User guide: `docs/LIVE_DEBUGGER.md`

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
- [ ] Patch command
- [ ] Bulk operations (apply from directory)
- [ ] JSONPath output

### Resource Gaps
- [x] Document trash (list/restore deleted) - See [DOCUMENT_TRASH_DESIGN.md](DOCUMENT_TRASH_DESIGN.md)

---

## Future Planned Features 🔮

See [FUTURE_FEATURES.md](FUTURE_FEATURES.md) for the complete implementation plan including:
- Platform Management (environment info, license)
- State Management for Apps
- ~~Grail Filter Segments~~ → Implemented (see `segment` resource)
- Grail Fieldsets
- Grail Resource Store

---

## Quality & Infrastructure

### Distribution
- [x] Multi-platform binaries (Linux, macOS, Windows - AMD64/ARM64)
- [x] GitHub Releases
- [x] Homebrew tap
- [ ] Container image

### Testing
- [x] Unit tests for core packages
- [x] Integration tests
- [x] E2E tests
- [x] Golden (snapshot) tests for all output formatters (`pkg/output/golden_test.go`, 139 golden files)
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
