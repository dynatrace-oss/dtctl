# dtctl

[![Release](https://img.shields.io/github/v/release/dynatrace-oss/dtctl?style=flat-square)](https://github.com/dynatrace-oss/dtctl/releases/latest)
[![Build Status](https://img.shields.io/github/actions/workflow/status/dynatrace-oss/dtctl/build.yml?branch=main&style=flat-square)](https://github.com/dynatrace-oss/dtctl/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/dynatrace-oss/dtctl?style=flat-square)](https://goreportcard.com/report/github.com/dynatrace-oss/dtctl)
[![License](https://img.shields.io/github/license/dynatrace-oss/dtctl?style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dynatrace-oss/dtctl?style=flat-square)](go.mod)

**Your Dynatrace platform, one command away.**

`dtctl` brings the power of `kubectl` to Dynatrace — manage workflows, dashboards, queries, and more from your terminal. Built for developers who prefer the command line and AI-assisted workflows.

```bash
dtctl get workflows                           # List all workflows
dtctl query "fetch logs | limit 10"           # Run DQL queries
dtctl edit dashboard "Production Overview"    # Edit resources in your $EDITOR
dtctl apply -f workflow.yaml                  # Declarative configuration
```

![dtctl dashboard workflow demo](docs/assets/dtctl-1.gif)

> This product is not officially supported by Dynatrace

## Why dtctl?

- **kubectl-style UX** — Familiar commands: `get`, `describe`, `edit`, `apply`, `delete`
- **AI-friendly** — Plain output modes and YAML editing for seamless AI tool integration
- **Multi-environment** — Switch between dev/staging/prod with a single command
- **Template support** — DQL queries with Go template variables
- **Shell completion** — Tab completion for bash, zsh, fish, and PowerShell

## AI Agent Skill

dtctl includes an [Agent Skill](https://agentskills.io) at `skills/dtctl/` that teaches AI assistants how to use dtctl.

**To use:** Copy the skill folder to `.github/skills/` (GitHub Copilot) or `.claude/skills/` (Claude Code):

```bash
cp -r skills/dtctl ~/.github/skills/   # For GitHub Copilot
cp -r skills/dtctl ~/.claude/skills/   # For Claude Code
```

Compatible with GitHub Copilot, Claude Code, and other Agent Skills tools.

## Quick Start

```bash
# Install dtctl - download the latest release for your platform:
# https://github.com/dynatrace-oss/dtctl/releases/latest
#
# Or build from source:
# git clone https://github.com/dynatrace-oss/dtctl.git && cd dtctl
# make build && make install

# Configure your environment
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token

dtctl config set-credentials my-token --token "dt0s16.YOUR_TOKEN"

# Go!
dtctl get workflows
dtctl query "fetch logs | limit 10"
dtctl create lookup -f error_codes.csv --path /lookups/production/errors --lookup-field code
```

## What Can It Do?

| Resource | Operations |
|----------|------------|
| Workflows | get, describe, create, edit, delete, execute, history |
| Dashboards & Notebooks | get, describe, create, edit, delete, share |
| DQL Queries | execute with template variables |
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
| [Installation](docs/INSTALLATION.md) | Build from source, shell completion setup |
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
