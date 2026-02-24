# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/dynatrace-oss/dtctl/compare/v0.12.0...HEAD
[0.12.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.9.0...v0.10.0
