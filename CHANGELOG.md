# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **Command structure refactoring**: Changed `dtctl query verify` to `dtctl verify query` for better consistency with kubectl-style verb-noun pattern. This establishes the `verify` command as a top-level verb for validation operations across different resource types (query, analyzer, settings, etc.). This change was made before the initial release, so no backward compatibility is needed.

### Added
- New `dtctl verify` parent command for verification operations
- `dtctl verify query` subcommand with all existing DQL query verification functionality

### Removed
- `dtctl query verify` subcommand (moved to `dtctl verify query`)
