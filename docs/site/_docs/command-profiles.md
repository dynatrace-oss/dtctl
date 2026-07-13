---
layout: docs
title: Command Profiles
---

Command profiles restrict **which commands dtctl exposes** to a named subset. A
profile shapes every discovery surface at once — `--help`, the
[`dtctl commands`](ai-agent-mode) catalog, and shell completion — and hard-blocks
invocation of anything outside the set.

The motivating use case is embedding dtctl in AI agents. When only a slice of the
CLI is relevant — say an investigation agent that needs `query` and Davis
analyzers but never `auth login` or the cloud-provisioning verbs — a large,
mostly-irrelevant command menu confuses the agent. A profile trims the surface to
what matters, from configuration alone. No fork, one binary.

> **Profiles are a convenience, not a security boundary.** Like
> [safety levels](#profiles-vs-safety-levels), they are client-side. A determined
> caller can unset `DTCTL_PROFILE` or edit the config. For real restriction,
> scope the API token.

## Quick start

Bind a built-in profile to a context so an embedded agent inherits the reduced
surface with zero flags:

```bash
dtctl config set-context prod-agent \
  --environment https://abc12345.apps.dynatrace.com \
  --token-ref prod-token \
  --profile query \
  --safety-level readonly

# Now the agent only sees the query surface:
dtctl commands            # catalog reflects the profile
dtctl --help              # help reflects the profile
dtctl auth login          # blocked with a clear error
```

Or select a profile for a single controlled environment via the environment
variable, which takes precedence over any context binding:

```bash
DTCTL_PROFILE=query dtctl commands
```

## Built-in profiles

| Profile | Surface (plus always-available commands) |
|---------|------------------------------------------|
| `full` | Everything (the default; today's behavior) |
| `query` | `query`, `get analyzers`, `describe analyzer`, `exec analyzer`, `verify analyzer` |
| `investigate` | `query`, `logs`, `get`, `find`, `describe` |

Profile names are deliberately **topical**, never permission words like
`readonly` — that axis belongs to safety levels (see below). If a profile should
also forbid writes, pair it with `--safety-level readonly` on the context.

## Defining your own

Profiles live in the config file under a top-level `profiles` map. A profile is a
`description` plus a flat `commands` **allowlist**:

```yaml
profiles:
  triage:
    description: Read-only incident triage for on-call agents
    commands:
      - query
      - logs
      - get slos      # only SLOs from the get verb, not all of `get`
      - describe

contexts:
  - name: oncall-agent
    context:
      environment: https://abc12345.apps.dynatrace.com
      token-ref: oncall-token
      profile: triage
      safety-level: readonly
```

Matching rules:

- Each `commands` entry is a **command-path prefix**: a verb (`query`), a
  resource (`get workflows`), or a full path. An entry matches a command when it
  equals or is a segment-prefix of that command's path, so listing a parent verb
  (`describe`) includes its whole subtree.
- **Default-deny**: anything not matched (and not always-available) is masked.
  There is no denylist — you always list what is *allowed*. Adding a new command
  to dtctl never silently widens an existing profile.
- To allow a parent but only some children, list the specific child paths
  (`commands: [get analyzers, get slos]` rather than `commands: [get]`).

User-defined profiles take precedence over a built-in preset of the same name.

## Always-available commands

Only two commands are allowed regardless of profile — the irreducible core that
lets an agent discover its surface and a user get help:

- `commands` (and `commands howto`) — the machine-readable catalog agents
  bootstrap from
- `help`

Everything else is subject to the allowlist, **including `config`, `ctx`,
`completion`, and `version`**. This is deliberate: `config`/`ctx` can rotate
credentials and switch environments, so a locked-down agent profile must be able
to withhold them. If a profile needs any of these, list it explicitly (e.g.
`commands: [query, config]`).

## Selecting the active profile

Precedence, highest first:

```
DTCTL_PROFILE env  >  context-bound profile  >  none (= full)
```

- **`DTCTL_PROFILE`** — for products that wrap the binary in a controlled
  environment.
- **Context binding** (`--profile` on `set-context`) — the recommended path for
  embedding; agents inherit the surface with zero flags.
- **None** — the full command tree (default, fully backward compatible).

There is no global default-profile setting: a config-wide profile is a footgun
(set once, forgotten, every invocation silently restricted). Activation is always
explicit per-environment or scoped to a context.

## What a blocked invocation looks like

```
$ DTCTL_PROFILE=query dtctl auth login
Error: command "auth login" is not available in profile "query"

  This profile exposes a reduced command set. Run 'dtctl commands' to see
  what is available, or unset DTCTL_PROFILE to use the full CLI.
```

In [agent mode](ai-agent-mode) the same block is reported as a structured error
with `code: "profile_blocked"`. The `dtctl commands` catalog also advertises the
active `profile` and `safety_level` so an agent can see both constraints at once.

## Profiles vs. safety levels

Profiles and [safety levels](configuration#safety-levels) are **orthogonal axes**
that compose on a context:

| | Safety level | Profile |
|---|---|---|
| Question | "What may this command *do*?" | "Which commands *exist* here?" |
| Axis | Permission / blast radius | Topic / surface |
| Effect on help & catalog | None — blocks at run time | Removes the command entirely |

> A profile decides which commands are on the menu; the safety level decides what
> those commands are allowed to do.

Set **both** on a context to express, e.g., "this agent only sees `query`/Davis
analyzers **and** can never mutate."
