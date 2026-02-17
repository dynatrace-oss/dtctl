# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/dynatrace-oss/dtctl/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.9.0...v0.10.0
