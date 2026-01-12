# dtctl

**Your Dynatrace platform, one command away.**

`dtctl` brings the power of `kubectl` to Dynatrace — manage workflows, dashboards, queries, and more from your terminal. Built for developers who prefer the command line and AI-assisted workflows.

```bash
dtctl get workflows                           # List all workflows
dtctl query "fetch logs | limit 10"           # Run DQL queries
dtctl edit dashboard "Production Overview"    # Edit resources in your $EDITOR
dtctl apply -f workflow.yaml                  # Declarative configuration
```

> ⚠️ **Alpha** — Not officially supported by Dynatrace

## Why dtctl?

- **kubectl-style UX** — Familiar commands: `get`, `describe`, `edit`, `apply`, `delete`
- **AI-friendly** — Plain output modes and YAML editing for seamless AI tool integration
- **Multi-environment** — Switch between dev/staging/prod with a single command
- **Template support** — DQL queries with Go template variables
- **Shell completion** — Tab completion for bash, zsh, fish, and PowerShell

## Quick Start

```bash
# Build from source
git clone https://github.com/dynatrace-oss/dtctl.git && cd dtctl
make build && make install

# Configure your environment
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token

dtctl config set-credentials my-token --token "dt0s16.YOUR_TOKEN"

# Go!
dtctl get workflows
dtctl query "fetch logs | limit 10"
```

## What Can It Do?

| Resource | Operations |
|----------|------------|
| Workflows | get, describe, create, edit, delete, execute, history |
| Dashboards & Notebooks | get, describe, create, edit, delete, share |
| DQL Queries | execute with template variables |
| SLOs | get, create, delete, apply, evaluate |
| Settings | get, create, delete, apply |
| Buckets | get, describe |
| And more... | OpenPipeline, EdgeConnect, Davis AI |

## Documentation

| Guide | Description |
|-------|-------------|
| [Installation](docs/INSTALLATION.md) | Build from source, shell completion setup |
| [Quick Start](docs/QUICK_START.md) | Configuration, examples for all resource types |
| [API Design](docs/dev/API_DESIGN.md) | Complete command reference |
| [Architecture](docs/dev/ARCHITECTURE.md) | Technical implementation details |
| [Implementation Status](docs/dev/IMPLEMENTATION_STATUS.md) | Roadmap and feature status |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE)
