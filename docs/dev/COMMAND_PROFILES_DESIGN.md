# Command Profiles Design

**Status:** Design Proposal
**Created:** 2026-07-10
**Author:** dtctl team

## Overview

Command profiles let an operator restrict *which commands dtctl exposes* to a
named subset. A profile shapes every discovery surface at once -- `--help`, the
`dtctl commands` / `commands howto` catalog, and shell completion -- and hard-blocks
invocation of any command outside the set.

The motivating use case is agentic products. dtctl is frequently embedded in AI
agents where only a slice of the CLI is relevant: an investigation agent may need
only `query` and `analyzers`, and commands like `auth login`, `config`, or the
cloud-provisioning verbs are noise at best and a footgun at worst. Agents degrade
when `dtctl commands` or `dtctl --help` return a large, mostly-irrelevant menu.

Today the only way to achieve this is to fork dtctl and strip commands. Profiles
make the subset a **configuration concern**, not a build concern -- one binary,
many surfaces.

## Goals

1. **Reduce agent confusion** -- Present only the commands relevant to the task.
2. **No fork** -- A single binary serves every surface; the subset is config-driven.
3. **One switch, all surfaces** -- Help, `commands`, `howto`, and completion all
   reflect the active profile with no per-surface code.
4. **Hard enforcement** -- Out-of-profile commands are not merely hidden; invoking
   one fails with a clear, signposted error.
5. **Zero-flag activation** -- An embedding product can pin a profile via context
   or environment so agents inherit it without passing anything.
6. **Compose with safety levels** -- Profiles govern the *surface* axis; safety
   levels govern the *permission* axis. They stack cleanly and never reimplement
   each other.

## Non-Goals

- **A security boundary.** Like [safety levels](context-safety-levels.md), profiles
  are a client-side convenience, not a security control. A determined caller can
  set `--profile full` or edit config. For real restriction, scope the API token.
- **Per-flag filtering.** Profiles operate at command granularity, not flag
  granularity. Hiding individual flags of a visible command is out of scope.
- **Dynamic/remote profiles.** Profiles are defined locally in config. No
  server-delivered profile definitions.
- **Reimplementing read/write filtering.** A profile that wants "no writes"
  delegates to a safety level (see below), it does not enumerate mutating commands.

---

## Concepts: Profiles vs. Safety Levels

dtctl already has [context safety levels](context-safety-levels.md). Profiles are a
**different, orthogonal axis**. Keeping them distinct is the central design
constraint of this document.

| | **Safety level** | **Profile** |
|---|---|---|
| Question | "What may this command *do*?" | "Which commands *exist* here?" |
| Axis | Permission / blast radius | Topic / surface / relevance |
| Ordered? | Yes: `readonly` < `readwrite-mine` < `readwrite-all` < `dangerously-unrestricted` | No -- arbitrary sets |
| Granularity | Per-*operation*, ownership-aware | Per-*command* identity |
| When enforced | Runtime, per operation | Startup, shapes the command tree |
| Effect on help/catalog | **None** -- a `readonly` context still lists `delete workflows`; it blocks at run time | Removes the command from help / `commands` / completion entirely |

The two are **filters in series**: a profile decides what is on the menu, then the
safety level polices what the chosen command is allowed to do. Composed, they
express things like "this agent only sees `query`/`analyzers` **and** can never
mutate."

### The overlap trap and how we avoid it

The only place these axes can collide is the read/write dimension. If a profile
called `readonly` worked by *hiding every mutating command*, we would have two
"readonly" knobs with different semantics -- the profile version being a weaker,
non-ownership-aware reimplementation of the safety level. Users would then hit
confusing contradictions:

- "I set `safety=readonly`, why is `delete` still in `--help`?" (expecting profile
  behavior from safety)
- "I set `profile=query`, why does safety still complain about ownership?"
  (expecting safety behavior from a profile)

**Rules that keep the axes clean:**

1. **Profiles never reimplement read/write.** When a profile wants "no writes," it
   *pins a safety level* (`safety-level: readonly`), keeping one source of truth
   for the permission axis.
2. **Profile preset names are topical, never permission words.** Ship `query`,
   `investigate`, `full` -- never a profile named `readonly`. The word `readonly`
   belongs to safety levels.
3. **Error messages are distinct and self-explaining.** A profile block says the
   command *does not exist here*; a safety block says the command is *not allowed
   to act*.

One sentence carries the whole mental model:

> A profile decides which commands are on the menu; the safety level decides what
> those commands are allowed to do.

---

## User Experience

### Defining profiles

Profiles live in the config file under a top-level `profiles` map. Each profile is
an include/exclude selector over the command tree, optionally pinning a safety
level.

```yaml
profiles:
  query-only:
    description: DQL + analysis for read-only investigation agents
    include: [query, analyzers, "describe *"]
    exclude: [auth, config, apply]
    safety-level: readonly       # delegates the permission axis; does not duplicate it

  investigate:
    extends: query-only          # inherit include/exclude/safety, then adjust
    include-also: [get problems, get slo]
```

Selection semantics:

- **`include`** -- verbs (`query`), resources (`workflows`), full paths
  (`get workflows`), or globs (`describe *`). Omitting `include` means "everything".
- **`exclude`** -- same syntax; **exclude wins over include**.
- **`extends`** -- inherit another profile's fields as the base.
- **`safety-level`** -- optional; pins the safety level while the profile is active,
  unless the context/flag specifies one explicitly (see precedence below).

### Always-available commands

A small set of commands is implicitly included regardless of profile, because
removing them would make dtctl unusable or unrecoverable:

- `commands`, `commands howto` -- agents must always be able to read the catalog.
- `config` and context selection -- a product must be able to set/inspect context.
- `version`, `completion`, `help`.

A profile may still *explicitly* exclude these if it really wants to (e.g. an agent
product that manages auth/config itself and wants `config` gone). The implicit
inclusion only applies when a command is neither included nor excluded.

### Selecting the active profile

Precedence (highest wins):

```
DTCTL_PROFILE env  >  context-bound profile  >  config current-profile  >  none (= full)
```

- **`DTCTL_PROFILE`** -- for products that wrap the binary in a controlled
  environment.
- **Context binding** -- a `profile:` field on a context; shipped products pin it so
  agents inherit the right surface with zero flags. This is the recommended path
  for embedding.
- **`current-profile`** -- a global default in config, mirroring `current-context`.
- **None** -- the full command tree (today's behavior; fully backward compatible).

> A hidden `--profile` flag is intentionally **not** part of the documented surface.
> It may exist as an internal escape hatch for debugging a locked-down context, but
> it is deliberately not advertised so agents are not tempted to widen their own
> surface.

### Managing profiles

```bash
# List defined profiles (built-in + config), marking the active one
dtctl profile list

# Show the resolved command set for a profile
dtctl profile show query-only

# Set the global default
dtctl profile use query-only

# Bind a profile to a context
dtctl config set-context prod-agent --profile=query-only
```

### What a blocked invocation looks like

```
$ DTCTL_PROFILE=query-only dtctl auth login
Error: command "auth login" is not available in profile "query-only"

  This profile exposes a reduced command set. Run 'dtctl commands' to see
  what is available, or unset DTCTL_PROFILE to use the full CLI.
```

Compare with a safety block (unchanged), which is about *permission*, not surface:

```
Error: context 'prod' (readonly) does not allow delete operations
```

### Built-in presets

Shipped so products need not hand-author YAML. All names are **topical**; any
"no writes" behavior is expressed by pinning a safety level, not by hiding verbs.

| Preset | Surface | Pinned safety |
|--------|---------|---------------|
| `full` | Everything (default; today's behavior) | none |
| `query` | `query`, `analyzers`, `describe`, plus always-available | `readonly` |
| `investigate` | `query` set + `get problems`/`get slo`/`get logs` | `readonly` |

User-defined profiles may `extends:` a preset.

---

## Design

### Where the filter runs

dtctl already post-processes the fully-registered command tree inside `execute()`
(`cmd/root.go`) -- `setupErrorHandlers(rootCmd)` and `installScopePreflight(rootCmd)`
both run after every `AddCommand`. The profile filter slots into the same phase:

```go
func execute() int {
	setupErrorHandlers(rootCmd)
	installScopePreflight(rootCmd)
	applyProfile(rootCmd, resolveProfile(cfg))   // <- new pass
	// ... alias resolution, tracing, rootCmd.Execute()
}
```

`resolveProfile(cfg)` walks the precedence chain (env -> context -> current-profile
-> none) and returns the resolved, `extends`-flattened profile (or nil for `full`).

### The filter mechanism: `Hidden` + a `RunE` guard

The key enabling fact: **every discovery surface we care about already honors
`cmd.Hidden`.**

- Cobra's help and shell completion skip hidden commands natively.
- `pkg/commands/listing.go` (which backs `dtctl commands` and `commands howto`)
  already skips hidden commands -- e.g. `listing.go:196`
  (`if hiddenCommands[name] || cmd.Hidden { continue }`) and the analogous checks
  for nested subcommands and flags.

So `applyProfile` needs to do only two things per out-of-profile command:

1. **Set `cmd.Hidden = true`** -- removes it from help, `commands`, `howto`, and
   completion for free, across all three surfaces, with no per-surface code.
2. **Wrap `RunE` with a guard** -- to satisfy the hard-enforcement goal, since a
   hidden command is still *runnable* by name in cobra.

```go
func applyProfile(root *cobra.Command, p *Profile) {
	if p == nil { // "full"
		return
	}
	walk(root, func(cmd *cobra.Command) {
		if p.Allows(commandPath(cmd)) {
			return
		}
		cmd.Hidden = true
		if cmd.RunE != nil || cmd.Run != nil {
			cmd.RunE = blockedRunE(cmd, p.Name)   // returns the "not available" error
			cmd.Run = nil
		}
	})
	if p.SafetyLevel != "" {
		applyPinnedSafetyLevel(p.SafetyLevel)      // only if not overridden by ctx/flag
	}
}
```

Because the guard wraps `RunE` rather than removing the command, cobra still parses
the command and its args -- which lets us return a *specific* "not available in
profile X" message instead of a generic "unknown command". This is why we mask
rather than `RemoveCommand`.

### Parent/child masking rules

- Masking a **parent** (e.g. `create`) masks the whole subtree; the guard on the
  parent covers `create workflow`, `create azure ...`, etc.
- A profile can **include a parent but exclude specific children** (`include: [get]`,
  `exclude: [get buckets]`) -- the walk evaluates each node against the selector, so
  a hidden child under a visible parent is fine (cobra already renders parents with
  a partially hidden child set).
- The **always-available** set is applied last so it cannot be masked by a broad
  parent exclusion unless the profile names the command explicitly.

### Config schema changes

`pkg/config/config.go`:

```go
type Config struct {
	// ... existing fields ...
	CurrentProfile string             `yaml:"current-profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

type Profile struct {
	Description  string      `yaml:"description,omitempty"`
	Extends      string      `yaml:"extends,omitempty"`
	Include      []string    `yaml:"include,omitempty"`
	IncludeAlso  []string    `yaml:"include-also,omitempty"` // additive over extends
	Exclude      []string    `yaml:"exclude,omitempty"`
	SafetyLevel  SafetyLevel `yaml:"safety-level,omitempty"`
}

// NamedContext gains a profile binding:
type Context struct {
	// ... existing fields ...
	Profile string `yaml:"profile,omitempty"`
}
```

### Selector matching

A profile selector matches a command by its space-joined path (`get workflows`).
Matching precedence for a given command path:

1. If any `exclude` pattern matches -> **masked**.
2. Else if `include` is empty -> **allowed** (everything).
3. Else if any `include` pattern matches (verb prefix, resource, full path, or
   glob) -> **allowed**.
4. Else if the command is in the always-available set -> **allowed**.
5. Else -> **masked**.

Reusing the existing catalog vocabulary matters: `pkg/commands` already models
verbs and resources (`FilterByResource`, `RequiredScopesForResource`), so selector
matching can share that command-path model rather than inventing a new one.

### Interaction with `dtctl commands` metadata

The catalog and the `--agent` envelope should advertise both active constraints so
an agent (and a human debugging) can see the two independent axes at once:

```json
{
  "profile": "query-only",
  "safety_level": "readonly",
  "verbs": { "...": "..." }
}
```

This is emitted from the same `commands.Build(rootCmd)` path; the profile name comes
from the resolved profile, the safety level from the effective context.

---

## Security & Safety Notes

- **Client-side only.** Profiles are convenience/UX, not a security boundary --
  identical caveat to safety levels. The authoritative restriction is the API token
  scope. This must be stated wherever profiles are documented.
- **Not a substitute for `--check-scopes`.** Profiles limit surface, not
  entitlements. A `query` profile with a broadly-scoped token can still run every
  query the token permits.
- **`extends` cycles** must be detected at resolution time and rejected with a clear
  config error.
- **Unknown profile name** (env/context/current-profile references a profile that
  does not exist) should fail fast with a clear error rather than silently falling
  back to `full`, which would be a surprising surface expansion.

## Backward Compatibility

- No `profiles` / `current-profile` / context `profile` -> resolves to `full` ->
  identical to today. Fully backward compatible.
- The change is purely additive to the config schema.

## Open Questions

1. **Glob syntax.** Shell-style (`describe *`) vs. explicit prefix matching. Leaning
   shell-style for familiarity; needs a decision on whether `*` crosses path
   segments.
2. **Should `profile show` render the tree or a flat path list?** Flat list is more
   agent-friendly; tree is more human-friendly. Possibly both via `-o`.
3. **Precedence of a profile-pinned `safety-level` vs. a context `safety-level`.**
   Proposal: an explicit context/flag safety level always wins over a
   profile-pinned one (the profile pin is a *default*, not an override). Needs
   confirmation.
4. **Telemetry.** Should the active profile be attached to the root OTel span (like
   command name is today) for usage analysis? Likely yes, low cost.

## Implementation Sketch (phased)

1. **Config schema + resolution** -- `Profile` type, `resolveProfile`, `extends`
   flattening, validation (unknown profile, cycles). Unit-tested in isolation.
2. **`applyProfile` pass** -- the tree walk, `Hidden` + `RunE` guard, always-available
   set. Verified against help, `commands`, and completion output.
3. **Built-in presets** -- `full`/`query`/`investigate`, merged under user config.
4. **`dtctl profile` command group** -- `list`/`show`/`use`, plus
   `config set-context --profile`.
5. **Catalog metadata + `--agent` envelope** -- surface `profile` + `safety_level`.
6. **Docs** -- user-facing page under `docs/site/_docs/`, cross-linked with
   `context-safety-levels.md` and `ai-agent-mode.md`.
