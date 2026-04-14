# dtctl

[![Release](https://img.shields.io/github/v/release/dynatrace-oss/dtctl?style=flat-square)](https://github.com/dynatrace-oss/dtctl/releases/latest)
[![Build Status](https://img.shields.io/github/actions/workflow/status/dynatrace-oss/dtctl/build.yml?branch=main&style=flat-square)](https://github.com/dynatrace-oss/dtctl/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/dynatrace-oss/dtctl?style=flat-square)](https://goreportcard.com/report/github.com/dynatrace-oss/dtctl)
[![License](https://img.shields.io/github/license/dynatrace-oss/dtctl?style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dynatrace-oss/dtctl?style=flat-square)](go.mod)

**Your Dynatrace platform, one command away.**

`dtctl` is a CLI for the Dynatrace platform. Manage workflows, dashboards, queries, and more from your terminal or let AI agents do it for you. Its predictable verb-noun syntax (inspired by `kubectl`) makes it easy for both humans and AI agents to operate.

```bash
dtctl get workflows                           # List all workflows
dtctl query "fetch logs | limit 10"           # Run DQL queries
dtctl apply -f workflow.yaml --set env=prod   # Declarative configuration
dtctl get dashboards -o json                  # Structured output for automation
dtctl exec copilot nl2dql "error logs from last hour"
```

![dtctl dashboard workflow demo](docs/assets/dtctl-1.gif)

> **Early Development**: This project is in active development. If you encounter any bugs or issues, please [file a GitHub issue](https://github.com/dynatrace-oss/dtctl/issues/new). Contributions and feedback are welcome!

**[Documentation](https://dynatrace-oss.github.io/dtctl/)** · **[Installation](https://dynatrace-oss.github.io/dtctl/docs/installation/)** · **[Quick Start](https://dynatrace-oss.github.io/dtctl/docs/quick-start/)** · **[Command Reference](https://dynatrace-oss.github.io/dtctl/docs/command-reference/)**

---

## Install

```bash
# Homebrew (macOS/Linux)
brew install dynatrace-oss/tap/dtctl
```

```bash
# Shell script (macOS/Linux)
curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.sh | sh
```

```powershell
# PowerShell (Windows)
irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
```

Binary downloads, building from source, shell completion setup, and more in the **[Installation Guide](https://dynatrace-oss.github.io/dtctl/docs/installation/)**.

## Authenticate

```bash
# OAuth login (recommended, no token management needed)
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"

# Verify everything works
dtctl doctor
```

Token-based authentication and multi-environment configuration are covered in the **[Quick Start](https://dynatrace-oss.github.io/dtctl/docs/quick-start/)**.

## Why dtctl?

- **Familiar CLI conventions**: `get`, `describe`, `edit`, `apply`, `delete`. If you (or your AI) know `kubectl`, you already know dtctl.
- **Built for AI agents**: Structured output (`--agent`), machine-readable command catalog (`dtctl commands`), and a bundled [Agent Skill](https://agentskills.io) that teaches AI assistants how to operate Dynatrace
- **Multi-environment**: Switch between dev/staging/prod with a single command; safety levels prevent accidental changes
- **Watch mode**: Real-time monitoring with `--watch` for all resources
- **DQL passthrough**: Execute queries directly, with template variables and file-based input
- **[NO_COLOR](https://no-color.org/) support**: Respects `NO_COLOR`, `FORCE_COLOR=1`, and auto-detects TTY

## Supported Resources

| Resource | Operations |
|----------|------------|
| Workflows | get, describe, create, edit, delete, apply, execute, logs, history, restore, diff, watch |
| Dashboards & Notebooks | get, describe, create, edit, delete, apply, share, history, restore, diff, watch |
| Documents & Trash | get, describe, create, edit, delete, share, history, restore |
| DQL Queries | execute, verify, template variables, live mode, filter segments, wait conditions |
| SLOs | get, describe, create, edit, delete, apply, evaluate, watch |
| Settings | get schemas, get/create/update/delete objects |
| Buckets | get, describe, create, delete, apply, watch |
| Segments | get, describe, create, edit, delete, apply, watch |
| Lookup Tables | get, describe, create, delete, apply (CSV auto-detection) |
| Anomaly Detectors | get, describe, create, edit, delete, apply |
| Extensions 2.0 | get, describe, apply monitoring configs |
| Hub Extensions | get, describe, list releases, filter by keyword |
| App Functions & Intents | get, describe, execute, find, open (deep linking) |
| Davis AI | analyzers, CoPilot chat, NL-to-DQL, document search |
| Cloud Integrations | Azure & GCP connections and monitoring (get, describe, create, delete, apply, update) |
| EdgeConnect | get, describe, create, delete |
| Notifications | get, describe, delete, watch |
| Users & Groups | get, describe |
| Live Debugger | breakpoints, workspace filters, snapshot decoding |

See the **[Command Reference](https://dynatrace-oss.github.io/dtctl/docs/command-reference/)** for the full list of verbs, flags, resource types, and aliases.

## AI Agent Skills

dtctl ships with an [Agent Skill](https://agentskills.io) that teaches AI coding assistants how to use dtctl. Agents can also bootstrap at runtime with `dtctl commands --brief -o json`.

```bash
# Install via skills.sh
npx skills add dynatrace-oss/dtctl

# Or install with dtctl itself
dtctl skills install              # Auto-detects your AI agent
dtctl skills install --for claude # Or specify explicitly
dtctl skills install --global     # User-wide installation

# Or copy manually
cp -r skills/dtctl ~/.agents/skills/   # Cross-client (any agent)
```

Compatible with GitHub Copilot, Claude Code, Cursor, Kiro, Junie, OpenCode, OpenClaw, and other [Agent Skills](https://agentskills.io)-compatible tools. See the **[AI Agent Mode docs](https://dynatrace-oss.github.io/dtctl/docs/ai-agent-mode/)** for details on the structured JSON envelope and agent auto-detection.

### Dynatrace domain skills

For deeper Dynatrace domain knowledge (DQL syntax, observability patterns, dashboards, logs, Kubernetes, and more) install the skills from **[Dynatrace/dynatrace-for-ai](https://github.com/Dynatrace/dynatrace-for-ai)**:

```bash
npx skills add dynatrace/dynatrace-for-ai
```

These skills provide the domain context (e.g., how to write DQL queries, which metrics to use for service health, how to navigate distributed traces) while dtctl provides the operational tool to act on it. Together they give AI agents everything they need to work with Dynatrace effectively.

## Observability

dtctl supports W3C Trace Context propagation and OTLP span export via the OpenTelemetry SDK. See [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) for full details on distributed tracing, environment variables, and CI/CD pipeline integration.

## Documentation

Full documentation is available at **[dynatrace-oss.github.io/dtctl](https://dynatrace-oss.github.io/dtctl/)**:

- [Installation](https://dynatrace-oss.github.io/dtctl/docs/installation/): Homebrew, shell script, binary download, build from source, shell completion
- [Quick Start](https://dynatrace-oss.github.io/dtctl/docs/quick-start/): Authentication, first commands, common patterns
- [Configuration](https://dynatrace-oss.github.io/dtctl/docs/configuration/): Contexts, credentials, safety levels, aliases
- [Command Reference](https://dynatrace-oss.github.io/dtctl/docs/command-reference/): All verbs, flags, resource types, and examples
- [Output Formats](https://dynatrace-oss.github.io/dtctl/docs/output-formats/): Table, JSON, YAML, CSV, charts
- [AI Agent Mode](https://dynatrace-oss.github.io/dtctl/docs/ai-agent-mode/): Structured envelope, auto-detection, agent skill
- [Token Scopes](https://dynatrace-oss.github.io/dtctl/docs/token-scopes/): Required API token scopes per safety level

Resource-specific guides: [DQL Queries](https://dynatrace-oss.github.io/dtctl/docs/dql-queries/) · [Workflows](https://dynatrace-oss.github.io/dtctl/docs/workflows/) · [Dashboards](https://dynatrace-oss.github.io/dtctl/docs/dashboards/) · [SLOs](https://dynatrace-oss.github.io/dtctl/docs/slos/) · [Settings](https://dynatrace-oss.github.io/dtctl/docs/settings/) · [Extensions](https://dynatrace-oss.github.io/dtctl/docs/extensions/) · [Davis AI](https://dynatrace-oss.github.io/dtctl/docs/davis-ai/) · [and more...](https://dynatrace-oss.github.io/dtctl/docs/quick-start/)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE).
