# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.18.0] - 2026-03-18

### Added
- **OpenClaw agent support** — `dtctl skills install --for openclaw` installs SKILL.md with YAML frontmatter and reference files to the OpenClaw workspace skills directory; includes auto-detection via `OPENCLAW` env var, global install support, and proper cleanup on uninstall
- **Visual output improvements** — bold table headers, status-aware coloring (green/red/yellow for known states), dimmed UUIDs, colored error prefix, dimmed empty-state message, and a new `-o describe` vertical key-value format (similar to `kubectl describe`); all styling respects `NO_COLOR`, `FORCE_COLOR`, `--plain`, and TTY detection

### Changed
- **Consistent stderr messaging** — all success, warning, and info messages now use dedicated `PrintSuccess`/`PrintInfo`/`PrintWarning` helpers that write to stderr, ensuring stdout stays clean for piping and scripting; covers auth, ctx, config, alias, lookups, azure, and all create/edit/delete flows

### Fixed
- **Describe label formatting** — underscores in struct tags now render as spaces (e.g., `Display Name` instead of `Display_name`), and known acronyms (ID, UUID, SLO, URL, API, HTTP, etc.) are preserved in their uppercase form
- **Pagination page-size errors** — fixed HTTP 400 errors on paginated requests for extensions, SLOs, IAM, and document resources by not sending `page-size` together with `page-key`/`next-page-key`

## [0.15.0] - 2026-03-11

### Added
- **Live Debugger CLI workflow**
  - `dtctl update breakpoint --filters ...` for workspace filter configuration
  - `dtctl create breakpoint <file:line>` for breakpoint creation
  - `dtctl get breakpoints` with breakpoint ID in default table output
  - `dtctl describe <id|filename:line>` for breakpoint rollout/status breakdown
  - `dtctl update breakpoint <id|filename:line> --condition/--enabled`
  - `dtctl delete breakpoint <id|filename:line|--all>` with confirmation / `-y` / `--dry-run`
- **Snapshot query decoding**
  - `dtctl query ... --decode-snapshots` decodes Live Debugger snapshot payloads with simplified plain values
  - `dtctl query ... --decode-snapshots=full` preserves full decoded tree with type annotations
  - Composable with any output format (`-o json`, `-o yaml`, `-o table`, etc.)


### Documentation
- Added/updated Live Debugger documentation in:
  - `docs/LIVE_DEBUGGER.md`
  - `docs/QUICK_START.md`
  - `docs/dev/API_DESIGN.md`
  - `docs/dev/IMPLEMENTATION_STATUS.md`
  
### Added
- **Generic document resource** — full lifecycle management for Dynatrace documents via `dtctl get/describe/create/edit/delete/history/restore document`; supports all document types stored in the Document API

### Changed
- **DQL query `--metadata` flag** — include response metadata (e.g. query cost, execution time) in query output; supports format-specific rendering and an optional field allow-list to restrict which metadata fields are shown

### Fixed
- **Document version field unmarshalling** — the `version` field is now correctly handled whether the API returns it as a string or an integer, preventing unmarshalling errors on certain document types

## [0.14.4] - 2026-03-10

### Changed
- **`dtctl skills install` minimal output** — installed skill files now contain only `SKILL.md` (~283 lines / ~10 KB) instead of inlining all reference documents (~1,100 lines / ~35 KB); reference docs remain embedded in the binary but are no longer concatenated into the installed file

## [0.14.3] - 2026-03-10

### Fixed
- **`dtctl doctor` false token failure** — the token check now uses the same OAuth-aware token resolution path as all other commands; previously it called `cfg.GetToken()` directly which cannot handle OAuth tokens stored in compact keyring format, causing `[FAIL] Token: cannot retrieve token "...-oauth": token not found` even when the context was fully functional

## [0.14.2] - 2026-03-10

### Added
- **Kiro Powers support** — `dtctl skills install --for kiro` installs skill files in [Kiro IDE](https://kiro.dev/)'s Powers format
  - Generates `POWER.md` with YAML frontmatter (`name`, `displayName`, `description`, `keywords`, `author`) in `.kiro/powers/dtctl/`
  - Powers activate dynamically in Kiro based on keyword matching in conversations
  - Automatic detection of Kiro via `KIRO` environment variable
  - Works with all existing skills subcommands: `install`, `uninstall`, `status`

## [0.14.0] - 2026-03-07

### Added
- **`dtctl skills` command** — Install, uninstall, and check status of AI agent skill files
  - `dtctl skills install --for <agent>` installs skill files for Claude, Copilot, Cursor, Kiro, or OpenCode
  - `dtctl skills uninstall --for <agent>` removes skill files from both project-local and global locations
  - `dtctl skills status` shows installation status across all supported agents
  - Auto-detects the current AI agent environment when `--for` is omitted
  - `--global` flag for user-wide installation (supported agents only)
  - `--force` flag to overwrite existing skill files
  - `--list` flag to show all supported agents without installing
  - Agent-mode structured output for all subcommands
- **Golden (snapshot) tests** — Comprehensive output format regression testing
  - 49 golden files covering all output formats (table, JSON, YAML, CSV, wide, chart, sparkline, barchart, braille, agent envelope, watch, errors)
  - Uses real production structs from `pkg/resources/*` to catch field changes automatically
  - `make test-update-golden` to update after intentional changes
  - Windows line-ending normalization for cross-platform CI
- **Zero-warnings linter policy** — CI now fails on any golangci-lint warning

### Changed
- **Go 1.26.1** — Upgraded from Go 1.24.13 to 1.26.1
- **golangci-lint v2.11.1** — Upgraded for Go 1.26 compatibility

## [0.13.3] - 2026-03-05

### Fixed
- Lookup table export silently truncates data at 1000 records (#58)
- Expanded dtctl agent skill with reference docs

## [0.13.2] - 2026-03-04

### Fixed
- `auth login`/`logout` writes to local `.dtctl.yaml` when present instead of always using global config

## [0.13.1] - 2026-03-02

### Added
- Structured output for `dtctl apply` command

### Fixed
- Document URLs updated to use new app-based format (#51)
- Config tests no longer overwrite real user config
- Implementation status features table formatting

## [0.13.0] - 2026-03-02

### Added
- **OAuth login** — `dtctl auth login` with PKCE flow, keyring-backed token storage, and automatic refresh
  - `dtctl auth logout` to clear tokens
  - `dtctl auth whoami` to show current identity
  - Safety level-based scope selection (readonly, readwrite-mine, readwrite-all)
  - Keyring integration for secure token persistence
- **NO_COLOR support** — Implement the [no-color.org](https://no-color.org/) standard for color control
  - Color is automatically disabled when stdout is not a TTY (piped output)
  - `NO_COLOR` environment variable suppresses all ANSI color output
  - `FORCE_COLOR=1` overrides TTY detection to force color output
  - `--plain` flag also disables color (existing behavior, now centralized)
  - Centralized color logic in `pkg/output/styles.go` (`ColorEnabled()`, `Colorize()`, `ColorCode()`)
  - All color usage across output package updated: styles, charts, sparklines, bar charts, braille graphs, watch mode, live mode
- **Help text improvements** — Consistent, detailed help across all parent verb commands
  - All 9 parent verbs (get, delete, create, edit, exec, find, update, open, describe) now have detailed `Long` descriptions and Cobra `Example` fields
  - Added missing `RunE: requireSubcommand` to `create` and `exec` commands
  - Migrated `doctor` examples from `Long` to Cobra `Example` field
  - Added tests enforcing help text coverage (`TestAllCommandsHaveHelpText`, `TestParentVerbsHaveExamples`)
- **Agent output envelope (`--agent` / `-A`)** — Wrap all CLI output in a structured JSON envelope (`ok`, `result`, `error`, `context`) for AI agents and automation consumers
  - Auto-detects AI agent environments and enables agent mode automatically (opt out with `--no-agent`)
  - Enriched context (suggestions, pagination, warnings) for `get workflows`, `get workflow-executions`, `delete workflow`, and `apply` commands
  - Structured error output with machine-readable error codes and suggestions
- **`dtctl ctx` command** — Top-level context management shortcut (like kubectx)
  - `dtctl ctx` lists all contexts, `dtctl ctx <name>` switches context
  - Subcommands: `current`, `describe`, `set`, `delete`/`rm`
  - Shared helper functions extracted from `config.go` to eliminate duplication
- **`dtctl doctor` command** — Health check for configuration and connectivity
  - 6 sequential checks: version, config, context, token, connectivity, authentication
  - Token expiration warning (< 24h remaining)
  - Lightweight HEAD request for connectivity probe
- **`dtctl commands` command** — Machine-readable command catalog for AI agents
  - Walks the Cobra command tree and outputs structured JSON/YAML describing all verbs, flags, resource types, mutating status, and safety levels
  - `--brief` flag strips descriptions and global flags for compact output
  - Positional resource filter with alias resolution and singular/plural fuzzy matching
  - `dtctl commands howto` subcommand generates Markdown how-to guides
  - Implementation: `pkg/commands/` (schema types, tree walker, howto generator)

### Changed
- **Release signing & SBOM** — Added cosign signing and syft SBOM generation to GoReleaser and release workflow
- **Linter hardening** — Re-enabled `errcheck` and `staticcheck` in golangci-lint v2 config with targeted exclusions (0 issues)
- **CI coverage threshold** — Increased from 49% to 50% as a regression guard
- Refactored `cmd/config.go` to use shared context management helpers (~150 lines of duplication removed)

## [0.12.0] - 2026-02-24

### Added
- **Homebrew Distribution** (#41)
  - `brew install dynatrace-oss/tap/dtctl` now available
  - GoReleaser `homebrew_casks` integration auto-publishes Cask on tagged releases
  - Shell completions (bash, zsh, fish) bundled in release archives and Cask
  - Post-install quarantine removal for unsigned macOS binaries

### Fixed
- Fixed OAuth scope names and removed dead IAM code (#40)
- Fixed `make install` with empty `$GOPATH` (#39)

### Changed
- GoReleaser config modernized: fixed all deprecation warnings (`formats`, `version_template`)
- Pinned `goreleaser/goreleaser-action` to commit SHA for supply-chain safety

## [0.11.0] - 2026-02-18

### Added
- **Azure Cloud Integration Support**
  - `dtctl create azure connection` - Create Azure cloud connections with client secret or federated identity credentials
  - `dtctl get azure connections` - List Azure cloud connections
  - `dtctl describe azure connection` - Show detailed Azure connection information
  - `dtctl update azure connection` - Update Azure connection configurations
  - `dtctl delete azure connection` - Remove Azure cloud connections
  - `dtctl create azure monitoring` - Create Azure monitoring configurations
  - `dtctl get azure monitoring` - List Azure monitoring configurations
  - `dtctl describe azure monitoring` - Show detailed monitoring configuration
  - `dtctl update azure monitoring` - Update monitoring configurations
  - `dtctl delete azure monitoring` - Remove monitoring configurations
  - Support for both service principal and managed identity authentication
  - Comprehensive unit tests with 86%+ coverage for Azure components
- **Command Alias System** (#30)
  - Define custom command shortcuts in config file
  - Support for positional parameters ($1, $2, etc.)
  - Shell command aliases for complex workflows
  - `dtctl alias set`, `dtctl alias list`, `dtctl alias delete` commands
  - Import/export alias configurations
- **Config Init Command** (#32)
  - `dtctl config init` to bootstrap configuration files
  - Environment variable expansion in config values
  - Custom context name support
  - Force overwrite option for existing configs
- **AI Agent Detection** (#31)
  - Automatic detection of AI coding assistants (OpenCode, Cursor, GitHub Copilot, etc.)
  - Enhanced error messages tailored for AI agents
  - User-Agent tracking for telemetry
  - Environment variable controls (DTCTL_AI_AGENT, OPENCODE_SESSION_ID)
- **HTTP Compression Support** (#33)
  - Global gzip response compression enabled
  - Automatic decompression handling
  - Improved performance for large API responses
- **Email Token Scope** (#35)
  - Added `email:emails:send` scope to documentation

### Changed
- **Quality Improvements** (Phase 0 - #29)
  - Test coverage increased from 38.4% to 49.6%
  - Improved diagnostics package with 98.3% coverage
  - Enhanced diff package with 88.5% coverage
  - Better prompt handling with 91.7% coverage
- Updated Go version to 1.24.13 for security fixes
- Enhanced TOKEN_SCOPES.md documentation (#28)
- Updated project status documentation

### Fixed
- Integration test compilation errors in trash management tests
- Corrected document.CreateRequest usage in test fixtures
- Documentation references cleanup

### Documentation
- Added QUICK_START.md with Azure integration examples
- Enhanced API_DESIGN.md with cloud provider patterns
- Updated IMPLEMENTATION_STATUS.md with Azure support status
- Improved AGENTS.md for AI-assisted development

## [0.10.0] - 2026-02-06

### Added
- New `dtctl verify` parent command for verification operations
- `dtctl verify query` subcommand for DQL query validation without execution
  - Multiple input methods: inline, file, stdin, piped
  - Template variable support with `--set` flag
  - Human-readable output with colored indicators and error carets
  - Structured output formats (JSON, YAML)
  - Canonical query representation with `--canonical` flag
  - Timezone and locale support
  - CI/CD-friendly `--fail-on-warn` flag
  - Semantic exit codes (0=valid, 1=invalid, 2=auth, 3=network)
  - Comprehensive test coverage (11 unit tests + 6 command tests + 13 E2E tests)

### Changed
- Updated Go version to 1.24.13 in security workflow

[0.14.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.3...v0.14.0
[0.13.3]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.2...v0.13.3
[0.13.2]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.9.0...v0.10.0
