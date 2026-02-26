# dtctl

[![Release](https://img.shields.io/github/v/release/dynatrace-oss/dtctl?style=flat-square)](https://github.com/dynatrace-oss/dtctl/releases/latest)
[![Build Status](https://img.shields.io/github/actions/workflow/status/dynatrace-oss/dtctl/build.yml?branch=main&style=flat-square)](https://github.com/dynatrace-oss/dtctl/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/dynatrace-oss/dtctl?style=flat-square)](https://goreportcard.com/report/github.com/dynatrace-oss/dtctl)
[![License](https://img.shields.io/github/license/dynatrace-oss/dtctl?style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dynatrace-oss/dtctl?style=flat-square)](go.mod)

**Your Dynatrace platform, one command away.**

`dtctl` is a CLI for the Dynatrace platform — manage workflows, dashboards, queries, and more from your terminal or let AI agents do it for you. Its predictable verb-noun syntax (inspired by `kubectl`) makes it easy for both humans and AI agents to operate.

```bash
dtctl get workflows                           # List all workflows
dtctl query "fetch logs | limit 10"           # Run DQL queries
dtctl apply -f workflow.yaml --set env=prod   # Declarative configuration
dtctl get dashboards -o json                  # Structured output for automation
dtctl exec copilot nl2dql "error logs from last hour"
```

![dtctl dashboard workflow demo](docs/assets/dtctl-1.gif)

> **Early Development**: This project is in active development. If you encounter any bugs or issues, please [file a GitHub issue](https://github.com/dynatrace-oss/dtctl/issues/new). Contributions and feedback are welcome!

## Why dtctl?

- **Built for AI agents** — Predictable verb-noun commands, structured output (`--plain`, `-o json`), and YAML-based editing make dtctl a natural tool for LLM-driven automation
- **Agent Skill included** — Ships with an [Agent Skill](https://agentskills.io) that teaches AI assistants how to operate your Dynatrace environment
- **Familiar CLI conventions** — `get`, `describe`, `edit`, `apply`, `delete` — if you (or your AI) know `kubectl`, you already know dtctl
- **Watch mode** — Real-time monitoring with `--watch` flag for all resources
- **Multi-environment** — Switch between dev/staging/prod with a single command
- **Template support** — DQL queries with Go template variables
- **Shell completion** — Tab completion for bash, zsh, fish, and PowerShell

## AI Agent Skill

dtctl ships with an [Agent Skill](https://agentskills.io) at `skills/dtctl/` — a compact command reference that teaches AI coding assistants how to use dtctl effectively.

**To use:** Copy the skill folder to your AI assistant's skill directory:

```bash
cp -r skills/dtctl ~/.github/skills/   # For GitHub Copilot
cp -r skills/dtctl ~/.claude/skills/   # For Claude Code
```

Compatible with GitHub Copilot, Claude Code, and other Agent Skills-compatible tools.

## Quick Start

```bash
# Install via Homebrew (macOS/Linux)
brew install dynatrace-oss/tap/dtctl

# Or download a binary: https://github.com/dynatrace-oss/dtctl/releases/latest
# Or build from source: git clone https://github.com/dynatrace-oss/dtctl.git && cd dtctl && make install

# Configure your environment
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token

dtctl config set-credentials my-token --token "dt0s16.YOUR_TOKEN"

# Go!
dtctl get workflows
dtctl describe workflow "My Workflow" -o yaml  # Structured output
dtctl query "fetch logs | limit 10"
dtctl apply -f workflow.yaml --set env=prod    # Template variables
dtctl create lookup -f error_codes.csv --path /lookups/production/errors --lookup-field code
```

## What Can It Do?

| Resource | Operations |
|----------|------------|
| Workflows | get, describe, create, edit, delete, execute, history, diff |
| Dashboards & Notebooks | get, describe, create, edit, delete, share, diff |
| DQL Queries | execute with template variables, verify syntax without execution |
| SLOs | get, create, delete, apply, evaluate |
| Settings | get schemas, get/create/update/delete objects |
| Buckets | get, describe, create, delete |
| Lookup Tables | get, describe, create, delete (CSV auto-detection) |
| App Functions | get, describe, execute (discover & run serverless functions) |
| App Intents | get, describe, find, open (deep linking across apps) |
| And more... | Apps, EdgeConnect, Davis AI |

## Documentation

| Guide | Description |
|-------|-------------|
| [Installation](docs/INSTALLATION.md) | Homebrew, binary download, build from source, shell completion |
| [Quick Start](docs/QUICK_START.md) | Configuration, examples for all resource types |
| [Token Scopes](docs/TOKEN_SCOPES.md) | Required API token scopes for each safety level |
| [API Design](docs/dev/API_DESIGN.md) | Complete command reference |
| [Architecture](docs/dev/ARCHITECTURE.md) | Technical implementation details |
| [Implementation Status](docs/dev/IMPLEMENTATION_STATUS.md) | Roadmap and feature status |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE)

---

<sub>Built with ❤️ and AI</sub>
