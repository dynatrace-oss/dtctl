# dtctl documentation

Reference documentation for [`dtctl`](https://github.com/dynatrace-oss/dtctl), the Dynatrace platform CLI. Start at the repository [README](../README.md) for install and first steps; this folder is the detailed reference.

## How this is organized

### Per-resource reference

[`resources/`](resources/) holds one page per resource type (workflows, DQL queries, dashboards and notebooks, SLOs, settings, cloud integrations, and more). Each page pairs three **generated** tables — Supported operations, Flags, and Required token scopes — with hand-authored Overview, Output, Examples, and Notes prose.

### Cross-cutting guides

- **[QUICK_START.md](QUICK_START.md)**: Install, authenticate, and run your first commands
- **[INSTALLATION.md](INSTALLATION.md)**: All install methods, build from source, shell completion
- **[CONFIGURATION.md](CONFIGURATION.md)**: Contexts, credentials, safety levels, apply hooks, aliases
- **[COMMANDS.md](COMMANDS.md)**: Generated reference of every verb and the resources it operates on
- **[TOKEN_SCOPES.md](TOKEN_SCOPES.md)**: API token scopes each resource requires, by safety level
- **[OUTPUT_FORMATS.md](OUTPUT_FORMATS.md)**: Output formats and the agent envelope
- **[AGENT_MODE.md](AGENT_MODE.md)** and **[AGENT_SKILLS.md](AGENT_SKILLS.md)**: Use dtctl with AI agents
- **[COOKBOOK.md](COOKBOOK.md)**: Recipes, tips, and troubleshooting
- **[OBSERVABILITY.md](OBSERVABILITY.md)**: Distributed tracing, OTLP span export, CI/CD integration
- **[EXTENSIONS.md](EXTENSIONS.md)**, **[LIVE_DEBUGGER.md](LIVE_DEBUGGER.md)**, **[SERVE.md](SERVE.md)**: Feature guides (SERVE is experimental)

## Generated vs. authored content

Some content is generated from dtctl's own command catalog (`dtctl commands --full -o json`) so it cannot drift from the CLI:

- The three tables on each `resources/*.md` page, all of `COMMANDS.md`, and the scope tables in `TOKEN_SCOPES.md`.
- Generated regions are wrapped in `<!-- GENERATED:<tag>:start -->` / `<!-- GENERATED:<tag>:end -->` markers. **Do not edit inside those markers by hand** — run `make docs-generate` instead.
- Everything outside the markers (Overview, Output, Examples, Notes, and all other prose) is hand-authored and is preserved on regeneration.

Regenerate after changing the CLI surface:

```bash
make docs-generate
```

A CI check ([`.github/workflows/docs-generate.yml`](../.github/workflows/docs-generate.yml)) fails a pull request if the generated content is stale, so regenerate and commit the result. The generator lives in [`scripts/gen-docs/`](../scripts/gen-docs/).

## Contributing

See the repository [CONTRIBUTING.md](../CONTRIBUTING.md).
