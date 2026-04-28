# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`iam:service-users:use` OAuth scope** — added to the `readwrite-mine`, `readwrite-all`, and `dangerously-unrestricted` safety levels so `dtctl create workflow` can use a Dynatrace [service user as the workflow actor](https://docs.dynatrace.com/docs/analyze-explore-automate/workflows/security#service-users); existing sessions need to re-run `dtctl auth login` to pick up the new scope; note that this slightly broadens the privilege footprint of `readwrite-mine` since holders can now act as a service user when creating workflows

### Fixed
- **`create lookup` now handles CSV files with a UTF-8 BOM** — Excel on macOS/Windows and many editors prepend a byte order mark (`EF BB BF`) when saving as CSV; the BOM was previously embedded in the first column name during parse-pattern auto-detection, producing a DPL pattern the upload API rejected with `Syntax error: extraneous input ''`; the BOM is now stripped before the header is parsed; fixes [#187](https://github.com/dynatrace-oss/dtctl/issues/187)

## [0.25.1] - 2026-04-21

### Fixed
- **`apply` now accepts array input for bulk resource updates** — `dtctl apply -f` can now process files containing arrays of resources (e.g., the output of `dtctl get settings --schema ... -o yaml`); each element is applied individually with per-item error reporting so a single failure does not abort the batch; works for all resource types, not just settings; fixes [#180](https://github.com/dynatrace-oss/dtctl/issues/180)

## [0.25.0] - 2026-04-20

### Added
- **`apply --share-environment` flag** — creates an environment-wide share for applied notebooks and dashboards in one step, so newly created documents come up as `isPrivate: false` without a manual UI click; accepts `read` (default when flag is bare) or `read-write`; idempotent: no-ops when a matching share exists, and replaces the share if access level differs; other resource types in the same apply invocation are skipped silently; requires `document:environment-shares:read` + `:write` scopes (already in the `readwrite-all` safety level)
- **`apply --write-id` and `apply --id` flags** — two complementary flags for idempotent applies; `--write-id` stamps the generated resource ID back into the source file after a successful create, so every subsequent apply updates in place without creating duplicates; `--id` injects or overrides the resource ID at the CLI level without modifying the file, ideal for CI pipelines using reusable template files; works for dashboards, notebooks, and workflows; a recovery hint is printed to stderr when a resource is created without `--write-id`
- **Extension installation** — install extensions with `dtctl create extension`; `--hub-extension <id>` installs a Hub catalog extension (optionally pin a release with `--version`); `-f <file.zip>` uploads a custom extension package; `--dry-run` previews without applying; requires the `extensions:definitions:write` token scope
- **Extended `describe extension` command** — `--monitoring-configuration-schema` outputs the JSON Schema for monitoring configurations of a specific extension version; `--active-gate-groups` lists available ActiveGate groups for a version; `--no-fluff` strips `documentation`, `displayName`, and `customMessage` fields from schema output (use with `--monitoring-configuration-schema`)
- **`enable gcp|azure monitoring` command** — new `dtctl enable` verb that completes cloud monitoring onboarding in one step: optionally updates the linked connection credentials (service account for GCP; directory/application ID for Azure) and enables the monitoring config; `--serviceAccountId`, `--directoryId`, `--applicationId` are all optional — if omitted, only the enabled state is toggled; supports `--dry-run`
- **Cloud monitoring configs created as disabled** — `dtctl create gcp monitoring` and `dtctl create azure monitoring` now create configs in a disabled state (`enabled: false`); use `dtctl enable gcp|azure monitoring` to enable
- **`auth status` command** — new `dtctl auth status` subcommand reports OAuth session health for the current context: access token validity and time-to-expiry, refresh token presence and expiry; supports `-o json/yaml` for scripting
- **Doctor "OAuth session" check** — `dtctl doctor` now includes an OAuth session row reporting access token expiry and whether a refresh token is present; row is omitted for platform-token contexts
- **`offline_access` OAuth scope** — all four safety levels now request the OIDC `offline_access` scope, causing the token endpoint to return a refresh token; this enables automatic access-token refresh on every subsequent command without re-running `dtctl auth login`
- **Improved keyring compact-storage fallback** — when a keyring backend rejects the full token payload for being too large, dtctl now tries a medium-compact form first (drops access/ID token JWTs but keeps scope and expiry metadata) before falling back to the minimal form (refresh token + name only); `auth status` remains informative in both compact cases
- **App function custom error detection** — `dtctl exec function` now detects the Dynatrace app-function error envelope (`{"error": "message", "data": ...}`) on HTTP 200 responses and surfaces the error message with a non-zero exit code instead of silently returning success
- **OAuth scopes for Hub catalog and extension definitions** — added `hub:catalog:read` scope to all safety levels (readonly and above) and `extensions:definitions:write` scope to readwrite-all and dangerously-unrestricted levels; fixes #166
- **`token-scopes` help topic** — `dtctl help token-scopes` now works as advertised in error messages, providing a quick reference for required scopes at each safety level

### Fixed
- **`delete notebook|dashboard` now works at `readwrite-mine` and `readwrite-all` safety levels** — the OAuth scopes requested at login were missing `document:documents:delete` for both `readwrite-mine` and `readwrite-all`, so `dtctl delete notebook <id>` returned `403 access denied to document`; document deletion is a soft-delete (moves to trash, recoverable) and does not require `dangerously-unrestricted`; permanent trash purging remains gated to `dangerously-unrestricted`; fixes [#160](https://github.com/dynatrace-oss/dtctl/issues/160)
- **Multi-series chart rendering panic** — fixed a panic in the chart renderer when DQL queries returned multiple series; fixes [#169](https://github.com/dynatrace-oss/dtctl/issues/169)
- **`auth status` no longer claims 'valid' for uncached tokens** — when the access token is not cached locally (compact keyring storage), `auth status` now correctly reports the token state instead of claiming it is valid
- **Environment share fixes** — exact access-level matching, 409 race-condition recovery, correct POST body shape, delete-loop fix, and pagination support for environment shares
- **`create extension --version` rejected with `--file`** — `dtctl create extension -f <file.zip> --version 1.2.3` now returns a clear error explaining that `--version` only applies to Hub installs; 409 conflict errors now include a clarifying message

## [0.24.0] - 2026-04-14

### Added
- **OpenTelemetry distributed tracing** — every dtctl invocation now creates an OpenTelemetry span covering the entire CLI process; export spans via OTLP by setting `OTEL_EXPORTER_OTLP_ENDPOINT`; inherits caller trace context from `TRACEPARENT`/`TRACESTATE` environment variables (W3C Trace Context), so dtctl appears as a child span in CI/CD pipelines or other distributed traces; outgoing HTTP requests to Dynatrace APIs carry `traceparent`/`tracestate` headers for end-to-end correlation; non-intrusive — tracing is silently disabled when no exporter is configured; see `docs/OBSERVABILITY.md` for setup guides and examples
- **Hub catalog extensions** — browse the Dynatrace Hub extension catalog with `dtctl get hub-extensions`, `dtctl describe hub-extensions`, and `dtctl get hub-extension-releases`; client-side `--filter` flag for case-insensitive substring matching against name, ID, or description; all commands are read-only
- **File-based OAuth token storage** — new `DTCTL_TOKEN_STORAGE=file` environment variable enables file-based OAuth token persistence as a fallback when the OS keyring is unavailable (headless Linux, WSL, CI/CD, containers); tokens are stored under `$XDG_DATA_HOME/dtctl/oauth-tokens/` with `0600` permissions; `dtctl doctor` reports the active storage backend; all OAuth flows (login, logout, token refresh, DQL queries) work transparently with either backend

### Fixed
- **`auth login --context` uses correct environment URL** — `dtctl auth login --context <name>` previously resolved the environment URL and token name from the *current* context instead of the named one, silently overwriting the target context's URL; now correctly reads from the specified context's configuration
- **Helpful redirect for `update settings`** — users attempting `dtctl update settings` now receive a clear message directing them to use `dtctl apply -f <file>` instead of a confusing unknown-flag error

### Documentation
- **Observability guide** — new `docs/OBSERVABILITY.md` documenting distributed tracing setup, environment variables, CI/CD integration with GitHub Actions examples, and a behavior matrix for all configuration combinations

## [0.23.0] - 2026-04-10

### Added
- **Pre-apply hooks** — run external validation commands before `dtctl apply` sends resources to the API; configure globally via `preferences.hooks.pre-apply` or per-context via `contexts[].context.hooks.pre-apply`; the hook receives the resource type and source file as positional parameters ($1, $2) and the processed JSON on stdin; non-zero exit rejects the apply with the hook's stderr shown to the user; skip with `--no-hooks`; set `pre-apply: none` on a context to disable a global hook for that context
- **Transparent DQL-to-AST filter conversion for segments** — segment filters can now be written as human-readable DQL expressions (e.g., `status == "ERROR"`) instead of raw JSON AST; dtctl transparently converts between the two formats on read and write, so `get`, `describe`, `apply`, and `edit` all work with the DQL form; existing JSON AST filters are passed through unchanged
- **Automatic keyring collection creation** — on Linux/WSL, `dtctl auth login` now detects when a persistent Secret Service keyring collection is missing and offers to create one automatically, prompting for a password if needed; `dtctl doctor` reports keyring status and suggests running `auth login` to recover

### Fixed
- **Segment updates use PATCH instead of PUT** — segment updates now use `PATCH` to avoid overwriting fields not included in the request body; field ordering in responses is preserved for stable `apply` round-trips
- **Improved auth login error when keyring is unavailable** — `auth login` now prints a clear message with recovery steps when the OS keyring cannot be accessed, instead of a raw library error

### Security
- **Go upgraded to 1.26.2** — fixes four stdlib vulnerabilities in `crypto/x509` and `crypto/tls` (applies to all CI workflows and release builds)

## [0.22.0] - 2026-04-01

### Added
- **Custom anomaly detector support** — full CRUD for custom anomaly detectors (`builtin:davis.anomaly-detectors`): `get`, `describe`, `create`, `edit`, `delete`, and `apply`; accepts both flattened YAML format (human-friendly, recommended) and raw Settings API format; source defaults to `"dtctl"` when omitted; `describe` includes recent problems cross-reference via DQL; filter by enabled state with `--enabled` / `--enabled=false`; alias `ad` for brevity (e.g., `dtctl get ad`)
- **DQL auto-refresh OAuth token on 401** — long-running `dtctl query` sessions now automatically refresh the OAuth token when a 401 is received during poll loops, preventing interrupted queries on token expiry

### Fixed
- **Shell completion: bash v2 with zsh alias support** — switched bash completion from v1 (`GenBashCompletion`) to v2 (`GenBashCompletionV2`) which includes a self-contained `__dtctl_init_completion` fallback, eliminating the `_init_completion: command not found` error when the `bash-completion` package is not installed; added `compdef dt=dtctl` instructions for zsh users with aliases; added a note about clearing stale completion files when upgrading
- **Missing safety check on `restore trash`** — `restoreTrashCmd` allowed trash restoration even in `readonly` contexts; now enforces `SetupWithSafety(safety.OperationUpdate)` consistent with all other restore subcommands
- **OAuth messages polluting stdout in agent mode** — interactive browser authentication messages ("Opening browser...", auth URL, fallback instructions) were printed to stdout, corrupting the structured JSON envelope in agent mode (`-A`); these are now redirected to stderr
- **Safety checks enforced for `apply` on settings objects** — `apply` with settings resources now correctly enforces safety checks before making API calls
- **SLO evaluation table output** — fixed formatting issues in SLO evaluation results table output
- **Build version injection** — `make build` and CI build workflow now correctly inject version, commit, and date into the binary via `-ldflags`; previously targeted non-existent `cmd.version` vars instead of `pkg/version.Version`

### Changed
- **Architecture refactor** — reduced boilerplate across command handlers with centralized `SetupClient`/`SetupWithSafety` helpers; split the monolithic `pkg/apply/applier.go` into per-resource files; extracted reusable pagination helper into `pkg/client/pagination.go`; fixed remaining stdout usage in library code

## [0.21.0] - 2026-03-30

### Added
- **Grail filter segments** — full CRUD support for segment management (`get`, `describe`, `create`, `edit`, `delete`, `apply`) plus query-time filtering via `--segment`/`-S`, `--segments-file`, and `--segment-var`/`-V` flags on `dtctl query`; supports inline variable binding with URL-query syntax (`-S "seg?var=val"`); segments are AND-combined per Grail semantics with client-side validation (max 10 per query); supports name resolution so you can pass segment names instead of UIDs

## [0.20.2] - 2026-03-30

### Added
- **Cross-client skill installation** — `dtctl skills install --cross-client` installs skills to the shared `.agents/skills/` directory defined by the [agentskills.io](https://agentskills.io) convention, so any compatible agent automatically discovers them without needing per-agent installation; use `--cross-client --global` to install to `~/.agents/skills/dtctl/` for user-wide availability; `--for cross-client` is also supported on `status` for targeted checks
- **AI Agent Skills documentation** — new "AI Agent Skills" section in the Quick Start guide covering install, cross-client, status, uninstall, and listing agents; new "Skills Management" subsection in the API Design docs

### Fixed
- **`skills status` blank env var in output** — when displaying status for the cross-client pseudo-agent, `printStatus` would produce `"(detected via  env)"` with a blank environment variable name; now correctly omits the detection suffix for agents without an env var
- **Shell completion for `--for cross-client`** — the `--for` flag tab completion on `skills status` now includes `cross-client` as a valid option alongside all per-agent names

### Documentation
- **Improved installation instructions and contribution guidelines** — updated README and CONTRIBUTING.md with clearer setup steps and contributor guidance

## [0.20.1] - 2026-03-25

### Added
- **TOON output for `query` and `verify query`** — `-o toon` is now accepted by `dtctl query` and `dtctl verify query`; previously the command-level format allowlists omitted `toon` even though the printer already supported it
- **`verify query` format validation** — `dtctl verify query` now rejects unsupported output formats with a clear error instead of silently falling through to the human-readable default

## [0.20.0] - 2026-03-24

### Added
- **TOON output format** — new `-o toon` output format using [TOON (Token-Oriented Object Notation)](https://github.com/toon-format/toon), a compact encoding optimised for LLM token efficiency (~40-60% fewer tokens vs JSON for tabular data); use `-A -o toon` in agent mode for maximum token savings
- **Windows installation guide** — comprehensive installation documentation for Windows users, including a PowerShell install script (`install.ps1`) and platform-specific troubleshooting

### Changed
- **`describe` commands respect `-o` flag** — all `describe` subcommands now support `--output json|yaml|toon|csv` and agent mode (`-A`); previously most describe commands hardcoded `fmt.Printf` output and ignored the format flag; fixed partial implementations in `describe lookup` (inverted routing), `describe extension` and `describe extension-config` (dead `outputFormat == ""` check)
- **Live Debugger marked experimental** — Live Debugger features are now documented as experimental; underlying APIs and query behavior may change in future releases

### Fixed
- **Settings API pagination** — fixed HTTP 400 errors on page 2+ when listing settings with filters; the Settings API rejects `schemaIds` and `scopes` query parameters when `nextPageKey` is present (all params are embedded in the page token); these params are now only sent on the first request

## [0.19.1] - 2026-03-20

### Fixed
- **Pagination: filter dropped on page 2+** — all paginated list endpoints placed filter/search query parameters inside the first-page-only branch of the pagination loop; page tokens do not always preserve filter context server-side (confirmed on the Document API), causing subsequent pages to return unfiltered results; e.g., `dtctl get dashboards` on environments with many documents fetched all document types instead of just dashboards
- **Pagination: page-size dropped on page 2+ (Document API)** — the Document API accepts `page-size` alongside `page-key` and does not embed the page size in the token (defaulting to 20/page if omitted); combined with the filter bug, this caused `dtctl get dashboards` on a 1,307-dashboard environment to make ~229 HTTP requests over ~2 minutes instead of 3 requests in ~5 seconds
- **`--chunk-size` default restored to 500** — reverts the v0.19.0 change that set the default to 0 (first page only), which silently truncated results for all resources; the underlying pagination bugs are now fixed properly

### Changed
- **Cleaner CLI output** — centralized message formatting with new `PrintHumanError`, `PrintHint`, `DescribeKV`, `DescribeSection` helpers; bold labels in `describe` output; bold `--help` section headers; softer status colors in tables; fixed table header misalignment caused by a `tablewriter` ANSI-width bug
- **Removed `-o describe` output format** — the redundant `--output describe` format on `get` commands has been removed; use `dtctl describe <resource>` instead

## [0.19.0] - 2026-03-20

### Added
- **Workflow task result retrieval** — new `dtctl get wfe-task-result <execution-id> --task <name>` command retrieves the structured return value of a specific workflow task (e.g., the object returned by a JavaScript task's `default` export function); previously this data was only accessible through the raw REST API
- **`exec workflow --show-results`** — new `--show-results` flag for `dtctl exec workflow --wait` prints each task's structured return value after the execution completes, removing the need for separate `get wfe-task-result` calls per task; in agent mode, task results are included in the JSON envelope
- **Environment URL confusion detection** — dtctl now detects common URL misconfiguration (e.g., `live.dynatrace.com` instead of `apps.dynatrace.com`, bare `dynatrace.com`, or missing `.apps.` on internal domains) and prints corrective suggestions; surfaces in `dtctl doctor` as a dedicated check, as warnings during `auth login` and `ctx set`, and as hints on 401/403/connection errors
- **Junie agent support** — `dtctl skills install --for junie` installs skill files for the Junie IDE agent; includes auto-detection via `JUNIE` env var and both project-local (`.junie/skills/dtctl/`) and global (`~/.junie/skills/dtctl/`) install paths

### Changed
- **Skills: migrate to agentskills.io standard** — `dtctl skills install` now copies the full skill directory (`SKILL.md` + `references/`) using the [agentskills.io](https://agentskills.io) open standard path (`<client>/skills/dtctl/`) instead of agent-specific file formats; YAML frontmatter and relative links are preserved verbatim; existing installations should run `dtctl skills uninstall && dtctl skills install` to migrate
- **Default `--chunk-size` changed from 500 to 0** — list commands now return only the first page of results by default (matching kubectl behavior); this fixes a performance regression where environments with many documents made 200+ sequential API requests taking 4+ minutes; users who need all results should pass `--chunk-size 500` explicitly
- **Global skill installs for more agents** — `dtctl skills install --global` now supports Copilot (`~/.copilot/skills/dtctl/`), OpenCode (`~/.config/opencode/skills/dtctl/`), and Junie (`~/.junie/skills/dtctl/`) in addition to previously supported agents

### Fixed
- **Slow pagination on large environments** — the Document API ignores the `page-size` parameter and always returns ~20 items per page; after the pagination fix in v0.18.0, this caused list commands to issue hundreds of sequential requests; resolved by defaulting `--chunk-size` to 0
- **Embedded skill files with CRLF on Windows** — added `.gitattributes` rules to force LF line endings for embedded skill files, fixing frontmatter detection failures (`"---\n"` prefix check) when building on Windows with `autocrlf=true`

## [0.18.0] - 2026-03-18

### Added
- **OpenClaw agent support** — `dtctl skills install --for openclaw` installs SKILL.md with YAML frontmatter and reference files to the OpenClaw workspace skills directory; includes auto-detection via `OPENCLAW` env var, global install support, and proper cleanup on uninstall
- **Visual output improvements** — bold table headers, status-aware coloring (green/red/yellow for known states), dimmed UUIDs, colored error prefix, dimmed empty-state message; all styling respects `NO_COLOR`, `FORCE_COLOR`, `--plain`, and TTY detection

### Changed
- **Consistent stderr messaging** — all success, warning, and info messages now use dedicated `PrintSuccess`/`PrintInfo`/`PrintWarning` helpers that write to stderr, ensuring stdout stays clean for piping and scripting; covers auth, ctx, config, alias, lookups, azure, and all create/edit/delete flows

### Fixed
- **Describe label formatting** — underscores in struct tags now render as spaces (e.g., `Display Name` instead of `Display_name`), and known acronyms (ID, UUID, SLO, URL, API, HTTP, etc.) are preserved in their uppercase form
- **Pagination page-size errors** — fixed HTTP 400 errors on paginated requests for extensions, SLOs, IAM, and document resources by not sending `page-size` together with `page-key`/`next-page-key`

## [0.15.0] - 2026-03-11

### Added
### Added
- **Live Debugger CLI workflow** (experimental -- underlying APIs and query behavior may change)
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
- **TOON output format** — new `-o toon` output format using [TOON (Token-Oriented Object Notation)](https://github.com/toon-format/toon), a compact encoding optimised for LLM token efficiency; achieves ~40-60% fewer tokens vs JSON for tabular data while preserving lossless round-trip fidelity; use `-A -o toon` to enable in agent mode


### Documentation
- Added/updated Live Debugger documentation in:
  - `docs/LIVE_DEBUGGER.md`
  - `docs/QUICK_START.md`
  - `docs/dev/API_DESIGN.md`
  - `docs/dev/IMPLEMENTATION_STATUS.md`
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

[0.25.1]: https://github.com/dynatrace-oss/dtctl/compare/v0.25.0...v0.25.1
[0.25.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.20.2...v0.21.0
[0.20.2]: https://github.com/dynatrace-oss/dtctl/compare/v0.20.1...v0.20.2
[0.20.1]: https://github.com/dynatrace-oss/dtctl/compare/v0.20.0...v0.20.1
[0.20.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.19.1...v0.20.0
[0.19.1]: https://github.com/dynatrace-oss/dtctl/compare/v0.19.0...v0.19.1
[0.19.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.3...v0.14.0
[0.13.3]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.2...v0.13.3
[0.13.2]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/dynatrace-oss/dtctl/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/dynatrace-oss/dtctl/compare/v0.9.0...v0.10.0
