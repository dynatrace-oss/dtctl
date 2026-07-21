---
layout: docs
title: AI Agent Skills
---

AI agent skills are portable knowledge packages that give coding agents the context they need to work with Dynatrace. dtctl ships with its own skill and integrates with the broader [Dynatrace for AI](https://github.com/Dynatrace/dynatrace-for-ai) skill collection.

## What are skills?

Skills follow the [Agent Skills](https://agentskills.io) open format. They are small, structured files that teach AI coding agents about a specific domain or tool. Agents load only what they need: a short description for discovery, full instructions when relevant, and detailed reference files on demand.

Skills work with Claude Code, GitHub Copilot, Cursor, Kiro, Junie, OpenCode, OpenClaw, Gemini CLI, and [many other compatible tools](https://agentskills.io).

## dtctl skill

dtctl includes a built-in skill at `skills/dtctl/` that teaches AI agents how to operate dtctl: which commands to run, what flags to use, how to read output, and how to chain operations together.

### Install with skills.sh

```bash
npx skills add dynatrace-oss/dtctl
```

### Install with dtctl

```bash
dtctl skills install              # Auto-detects your AI agent
dtctl skills install --for claude # Or specify explicitly
dtctl skills install --global     # User-wide installation
dtctl skills install --cross-client  # Cross-client (.agents/skills/)
dtctl skills status               # Check installation status
```

### Manual installation

Copy the skill directory into your agent's skills path:

```bash
cp -r skills/dtctl ~/.agents/skills/                       # Cross-client (any agent)
cp -r skills/dtctl ~/.github/skills/                       # GitHub Copilot
cp -r skills/dtctl ~/.claude/skills/                       # Claude Code
cp -r skills/dtctl ~/.openclaw/workspace/skills/dtctl/     # OpenClaw
```

## Dynatrace domain skills

The dtctl skill teaches agents how to use the CLI tool. For deeper Dynatrace domain knowledge, install the skills from [Dynatrace/dynatrace-for-ai](https://github.com/Dynatrace/dynatrace-for-ai):

```bash
npx skills add dynatrace/dynatrace-for-ai
```

These skills cover:

| Category | Skills |
|----------|--------|
| **DQL & Query Language** | DQL syntax rules, common pitfalls, query patterns |
| **Observability** | Services, frontends, distributed tracing, hosts, Kubernetes, AWS, logs, problems |
| **Platform** | Dashboards, notebooks |
| **Migration** | Classic entity-based DQL to Smartscape equivalents |

The domain skills provide context (how to write DQL queries, which metrics to use for service health, how to navigate distributed traces) while the dtctl skill provides the operational tool to act on it. Together they give AI agents everything they need to work with Dynatrace effectively.

You can install all skills without penalty. Agents use progressive disclosure and only load what they need for the current task.

## Runtime bootstrapping

In addition to skills, AI agents can discover dtctl's capabilities at runtime:

```bash
# Minimal overview: verbs, resources, subcommands (TOON default, ideal for system prompts)
dtctl commands

# Brief catalog: adds mutating status, access levels, flag types, and scopes
dtctl commands --brief -o json

# Full catalog with descriptions, flag defaults, and global flags
dtctl commands --full -o json

# Human-readable how-to guide
dtctl commands howto

# What data exists in THIS environment: fetchable objects, buckets, entity census, capabilities
dtctl inventory
```

`dtctl commands` answers *"what can I run?"*; [`dtctl inventory`]({{ '/docs/inventory/' | relative_url }}) answers *"what is there to query?"* — with every absent capability carrying the evidence checked, so agents can cite negatives instead of re-probing.

See [AI Agent Mode]({{ '/docs/ai-agent-mode/' | relative_url }}) for details on structured JSON output, auto-detection, and the `--agent` flag.
