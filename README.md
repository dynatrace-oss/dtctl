# dtctl

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/dynatrace-oss/dtctl/badge)](https://scorecard.dev/viewer/?uri=github.com/dynatrace-oss/dtctl)

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
```

![dtctl dashboard workflow demo](docs/assets/dtctl-1.gif)

> **Officially supported**: dtctl has been officially supported by Dynatrace since v1.0. Found a bug or have feedback? [Open a GitHub issue](https://github.com/dynatrace-oss/dtctl/issues/new) - that's our support channel.

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

Binary downloads, building from source, shell completion setup, and more in **[docs/INSTALLATION.md](docs/INSTALLATION.md)**.

## Authenticate

```bash
# OAuth login (recommended, no token management needed)
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"

# Verify everything works
dtctl doctor
```

Token-based authentication and multi-environment configuration are covered in **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)**.

## Use with AI agents

dtctl is built for AI agents as much as for humans. The `--agent` flag wraps every response in a structured JSON envelope with stable error codes, `dtctl commands` prints a machine-readable command catalog, and `dtctl inventory` reports what data an environment holds. A bundled Agent Skill teaches AI coding assistants how to operate Dynatrace:

```bash
dtctl skills install              # Auto-detects your AI agent
```

Full agent-mode and Agent Skills reference lives in **[docs/AGENT_MODE.md](docs/AGENT_MODE.md)** and **[docs/AGENT_SKILLS.md](docs/AGENT_SKILLS.md)**.

## Observability

dtctl supports W3C Trace Context propagation and OTLP span export via the OpenTelemetry SDK. See [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) for full details on distributed tracing, environment variables, and CI/CD pipeline integration.

## Documentation

Find your way around this repository:

- **[docs/QUICK_START.md](docs/QUICK_START.md)**: Install, authenticate, and run your first commands
- **[docs/INSTALLATION.md](docs/INSTALLATION.md)**: All install methods, build from source, shell completion
- **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)**: Contexts, credentials, safety levels, apply hooks, aliases
- **[docs/COMMANDS.md](docs/COMMANDS.md)**: Full command reference (auto-generated), every verb, flag, resource type, and alias
- **[docs/resources/](docs/resources/)**: Per-resource guides (workflows, DQL queries, dashboards and notebooks, SLOs, settings, cloud integrations, and more)
- **[docs/AGENT_MODE.md](docs/AGENT_MODE.md)** and **[docs/AGENT_SKILLS.md](docs/AGENT_SKILLS.md)**: Use dtctl with AI agents
- **[docs/OUTPUT_FORMATS.md](docs/OUTPUT_FORMATS.md)**, **[docs/COOKBOOK.md](docs/COOKBOOK.md)**, **[docs/SERVE.md](docs/SERVE.md)**, **[docs/OBSERVABILITY.md](docs/OBSERVABILITY.md)**: Output formats, recipes, local serving, and tracing

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE).
