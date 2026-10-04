# Recipes Design

**Status:** Proposed. A prototype is implemented on this branch (experimental
commands); see [Prototype status](#prototype-status).
**Created:** 2026-10-04
**Author:** dtctl team

## Overview

A **recipe** is a named, parameterized DQL query defined in a YAML file and
exposed as a dtctl command:

```bash
dtctl get recipes                                  # what is in the book
dtctl run services-failures --service checkout --from 6h
dtctl run services-failures --service checkout --dry-run   # show the DQL, don't run it
```

The goal is to let AI agents (and humans) answer common questions **without
writing DQL**. A good recipe saves more than the tokens of a query. It also
saves the exploration that comes before the query: guessing data object names,
fields, entity types and the right time window. That exploration is where agents
spend most of their calls and most of their wrong answers.

Recipes do not replace DQL. `dtctl query` remains the escape hatch for any
question the recipe book does not cover. Every recipe hands back the DQL it ran,
so an agent can adapt it into a `dtctl query` in one step.

**Content is separate from code.** Recipes live in YAML files, not in Go. DQL
evolves at a different pace than dtctl: new data objects, semantic-dictionary
changes, better idioms. A recipe change should be a content edit reviewed as
content. It should not be a code change to a command handler.

## Goals

1. **Fewer tokens, fewer calls, fewer wrong answers for agents** on common
   questions. One call with typed flags replaces discovery plus authoring plus
   retries.
2. **Externalized content.** Recipes are YAML files with a schema. dtctl is the
   engine that renders, runs and explains them. Adding or fixing a recipe needs
   no Go change.
3. **DQL stays first-class.** Every recipe exposes its rendered DQL. Nothing a
   recipe does is impossible with `dtctl query`.
4. **Native CLI ergonomics.** Recipe parameters are real flags: `--help`, shell
   completion and the `dtctl commands` catalog work without special cases.
5. **Self-explaining results.** A recipe states what its rows mean and, above
   all, what an **empty** result means. Grail answers a wrong question with `[]`
   and exit 0. An empty result that explains itself is the single most
   context-saving thing a recipe can return.
6. **User extensibility.** Users and teams can add their own recipes without
   forking dtctl.

## Non-goals

- **A workflow engine.** Multi-step recipes (later phase) are a small step graph
  with no loops and no branching. Orchestration belongs in Dynatrace Workflows.
- **Per-environment generation.** No discovery pass writes an environment-specific
  recipe book (see [Prior work](#prior-work)). Environment capabilities come from
  `dtctl inventory`, which already exists.
- **Query filters as flags on built-in commands.** Design principle 2 ("no custom
  query flags, use DQL passthrough") still holds for resource commands. A recipe's
  flags are parameters of a *named question*, declared in content. They are not
  generic filters bolted onto `get`.
- **Mutations.** Phase 1 recipes are DQL only, so they are read-only. API and
  mutating steps are a later phase with their own safety design.
- **A security boundary.** Like profiles and safety levels, recipes are a
  convenience. The token's scopes are the real control.

## Prior work

This design starts fresh, but it is shaped by an unreleased prototype and its
evals (branches `recipes` and `feat/recipes` on the maintainer's fork:
`docs/dev/RECIPES_CONCEPT.md`, `docs/dev/RECIPES_EVAL.md`). It also draws on two
neighbours: the dynatui view catalog (branch `tui`) and the `dtctl-correlate`
plugin (correlation-graph). What carries over:

| Finding | Consequence here |
|---|---|
| Across eight eval rounds (three models, a held-out task suite, two tenants), agents given a recipe book were the most correct arm. On the main test environment they were 100% correct across pooled batches, against 88% for bare dtctl. They used ~40% of the calls and ~40% of the wall time, had zero empty results, and were the cheapest arm. Haiku with the book beat Opus without it. | Recipes are worth shipping. |
| Agents mostly **read** recipes (briefing → `describe` → adapted query) rather than executing them directly. | The rendered DQL is part of every result (`--dry-run`, `context.query`), and adapting it into `dtctl query` must be trivial. |
| Most of the *correctness* gain later moved into binary-level ergonomics that ship to everyone. That includes empty-result diagnosis, lookback and default-window advice in `pkg/exec`, and `dtctl inventory` (#368). | Recipes don't re-implement any of it. What's left for recipes is efficiency, determinism and curated knowledge. |
| The prototype's per-environment machinery (pack/book lockfile, discovery generator, run stamps, capability DSL, widen-on-empty) was the bulk of its complexity. | Dropped. One file kind, no generated state, no writes on execution. |
| Agents read the dtctl briefing in every cell. Without the book, about half of the skills-arm cells never loaded a skill body; with the book present, skills were loaded in only 2–9 cells per batch. | Discovery goes through the CLI (`get recipes`, the catalog). The skill carries only a one-line pointer. |
| A loud, default 2h query window silently mislabelled answers ("7 days" that was really 2h) in control arms of several rounds. | One timeframe mechanism for recipes **and** `dtctl query` (`--from`/`--to`), with the effective window reported back. |
| correlation-graph: every catalog entry must say what an empty result means, or the loader refuses it. Pre-bound multi-hop drilldowns showed no measurable gain over plain dtctl plus DQL. | `emptyMeans` is mandatory. Phase 1 is single-query recipes; multi-step waits for evidence. |
| The prototype never ran its key comparison: the same content as markdown versus as executable commands. | It is the gate for phase 2 (see [Evaluation](#evaluation)). |

## Decisions

| # | Question | Decision |
|---|---|---|
| 1 | Command name | `dtctl run <recipe>` |
| 2 | Scope of the first version | Start simple: single-query DQL recipes, built-in plus user directory |
| 3 | Where built-in recipes live | In the dtctl repository (`recipes/`, embedded). A separately versioned bundle is a later option. |
| 4 | Timeframe flags | `--from`/`--to` are shared by recipes **and** `dtctl query` |

## Design

### 1. Recipe file format

One recipe per file, `recipes/<domain>/<name>.yaml`. Fourteen fuller worked
examples live in [examples/recipes/](examples/recipes/) (see §12).

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata:
  name: services-failures         # <domain>-<name>, kebab-case; the `run` subcommand
  version: 1                      # content version; bump when the output shape changes
  tags: [services, errors, triage]
spec:
  summary: Failed requests of a service by endpoint and status
  description: |                  # optional; shown by describe/--help
    Groups failed root requests of one service by endpoint and HTTP status,
    most frequent first.
  requires: [spans]               # optional; capability names from `dtctl inventory`
  scope: [cluster, namespace, tag]  # cross-cutting scope flags (§10)
  params:
    service:
      type: string
      required: true
      positional: true            # at most one; `dtctl run services-failures checkout`
      description: Service name as shown in Smartscape
    top:
      type: int
      default: 10
      min: 1
      max: 100
  timeframe: 2h                   # or `none`, or {default, min, fixed, ...} (§3)
  dql: |                          # illustrative; shipped bodies are verified (§9)
    fetch spans
    {{- with .scope.stage}}
    {{.}}{{end}}
    | filter service.name == {{.service}}
    | filter request.is_root_span and request.is_failed
    | summarize n = count(), by: {endpoint.name, http.response.status_code}
    | sort n desc
    | limit {{.top}}
  means: >-
    One row per failure signature; n is how often it occurred in the window.
  emptyMeans: >-
    The service had no failed root requests in the window. Failures may sit on
    a downstream service, or the service name may not match exactly. Check it
    with `dtctl run services-list`.
  next:
    - recipe: logs-for-service
      with: { service: "{{.service}}" }   # bound from this invocation's params
    - recipe: traces-get
      bind: { trace_id: trace.id }        # phase 2: bound from a result row
    - recipe: services-list
      when: empty                         # only suggested on an empty result
  deprecated:                     # optional
    message: ...
    replacedBy: ...
```

Schema rules, enforced by the loader and by `dtctl verify recipe`:

| Field | Rule |
|---|---|
| `metadata.name` | `^[a-z][a-z0-9-]{1,48}$`, starting with a registered domain (`k8s-`, `services-`, …; §11); unique within a layer |
| `metadata.version` | positive integer; reported in the envelope |
| `spec.summary` | required, single line, ≤ 100 chars (it is the catalog/`--help` line) |
| `spec.dql` | required; a Go template over `params`, `scope` and `window` (§2) |
| `spec.means`, `spec.emptyMeans` | **required** |
| `spec.timeframe` | required: a duration, `none`, or a map (§3) |
| `spec.params.*` | name `^[a-z][a-z0-9_]*$` (a valid template field; the flag spells `_` as `-`); not a reserved framework flag (§4) |
| `spec.params.*.pattern` | optional regular expression a `string` value must match (`^P-\d+$`) |
| `spec.scope` | optional list of scope dimensions from `_scopes.yaml`; the DQL must place `.scope.stage` or `.scope.expr`, and must not when no scope is declared (§10) |
| `spec.segments` | `on` (default) or `off` for recipes over data that filter segments must not narrow (§10) |
| `spec.requires` | names defined by `dtctl inventory` (built-in or user definitions) |
| `spec.next[]` | `recipe` must resolve in the merged book (warning for user layers); `with` binds from params, `bind` from result fields (phase 2); `when: always\|empty\|nonempty` |

**Complexity governors**, carried over because the prototype needed them:

- Every new keyword is "default no".
- A recipe that needs `variants:` is two recipes.
- DQL bodies stay short enough to read in `describe` (≈20 lines).
- Templates may test whether a param is present and compare an enum. They may
  not contain other logic.
- Shared DQL prefixes go into fragments (§2), not copy-paste. Fragments hold
  complete pipeline stages, never half an expression.

### 2. Parameters and rendering

Phase 1 types: `string`, `int`, `bool`, `enum` (with `values:`), and
`list` (of strings).

- **Values render as DQL literals, never as raw text.**
  - `{{.service}}` becomes `"checkout"`, escaped as a DQL string literal.
  - `{{.top}}` becomes `10`.
  - A `list` becomes an array literal `{"a", "b"}` for `in()`.
  - An `enum` renders as a string literal unless the param sets
    `render: identifier`. In that case the value must match `values:` exactly,
    which allows field names and sort directions.
  - There is no raw-DQL param type. A recipe that needs one is a sign the
    question belongs in `dtctl query`.
- **Strict templates.** Rendering uses `missingkey=error`. Every declared param
  is present in the template data, and an unset optional param is `nil`, so
  `{{if .namespace}}` works. A template that references an undeclared param
  fails lint. Template references are extracted from the parsed template tree,
  not by regex.
- **Validation before rendering.**
  - Required params, `min`/`max` and enum membership are checked by the
    generated cobra command.
  - Errors use the standard `invalid_argument` envelope and list the valid
    values.
- This is deliberately stricter than today's `query --set` (`missingkey=zero`,
  raw substitution). `--set` keeps its behaviour, because it is a stable flag.
- **Names.** A param is a template field, so it is snake_case (`min_restarts`).
  Its flag uses dashes (`--min-restarts`). `scope` and `window` are reserved:
  they hold the framework's template data.
- **`pattern`** validates a `string` before rendering. For example,
  `problems-get` takes only `^P-\d+$`, so a wrong ID fails with a usage error
  instead of an empty result.
- **Implementation note.** Each param value is a named Go type whose `String()`
  returns the escaped DQL literal. `{{.provider}}` then prints `"aws"`, while
  `eq .provider "aws"` still compares the raw value, because `text/template`
  compares by kind.

**Template data**

| Field | Contents |
|---|---|
| `.<param>` | each declared param, rendered as a DQL literal; `nil` when unset |
| `.scope.stage` | `\| filter <expr>` built from the scope flags, or empty (§10) |
| `.scope.expr` | the same predicate as a bare expression, for `timeseries … filter:`, or empty |
| `.window.from`, `.window.to` | the resolved window as DQL timestamp expressions; only for `timeframe: {inline: true}` (§3) |

**Fragments.** Several skill families share a long prefix. In the security
skill, four-step pipelines repeat a 20–45 line "latest state per vulnerability"
base in about 15 variants. Fragments are Go's own `{{define}}`/`{{template}}`:
`recipes/_fragments/*.tmpl` files are parsed into every recipe's template set,
and a recipe writes `{{template "vuln-latest-state" .}}`. The only additions
are lint rules: fragment names are unique, every fragment is used, and the
golden rendered DQL shows the expanded result, so a fragment change shows its
effect in every recipe that uses it.

### 3. Timeframe: `--from` / `--to` for recipes and `query`

The DQL query API already has the right primitive: `defaultTimeframeStart` and
`defaultTimeframeEnd` apply to any query that does not set its own `from:`/`to:`.
dtctl exposes it today only as `--default-timeframe-start/-end`, which take
RFC3339 timestamps. An eval found that a relative value (`-1h`) was silently
ignored, so the agent reported a 2h answer as "last hour".

**New flags, on `dtctl query` and every recipe:**

| Flag | Accepts | Default |
|---|---|---|
| `--from` | a duration (`2h`, `30m`, `7d`; read as "ago"), or an RFC3339 timestamp | recipe's `timeframe.default`; for `query`, unset (server default) |
| `--to` | a duration (ago) or an RFC3339 timestamp | now |

How it works:

- dtctl resolves both values to absolute RFC3339 timestamps on the client and
  sends them as `defaultTimeframeStart/End`. Resolving to absolute timestamps
  makes multi-request recipes (later phases) consistent, and it makes the
  reported window exact.
- `--from/--to` and `--default-timeframe-start/-end` are mutually exclusive.
- The existing flags additionally reject a value that is not RFC3339. That
  turns input dtctl used to ignore into an error, so it is not a breaking change
  under the stability rules.
- **Recipes leave `from:`/`to:` out of their DQL.** The window comes from the
  flags, and lint rejects a top-level `from:` in a recipe that declares a
  timeframe. A `timeframe: none` recipe (Smartscape state, catalog lookups)
  sends no default window and offers no `--from/--to` flags, because a window
  changes what state queries return.
- **The effective window is reported, not assumed.** In agent mode the envelope
  gains `context.window: {from, to}`, taken from the response's
  `analysisTimeframe` metadata. If a query's own `from:` overrode `--from` (the
  API's precedence), the reported window shows it, and dtctl adds a warning that
  `--from` was overridden by the query. This needs no DQL parsing on the client.
  It also closes the "non-empty answer, wrong window" failure that the
  empty-result advice cannot catch.
- **Timeframe forms.** The worked examples needed more than a default:

  | Form | Example | Meaning |
  |---|---|---|
  | duration | `timeframe: 2h` | default window; `--from/--to` override it |
  | `none` | `timeframe: none` | state query; no window is sent and no flags are offered |
  | `min` | `{default: 30d, min: 14d}` | a trend needs history; a shorter `--from` is a usage error |
  | `fixed` | `{default: 30m, fixed: true}` | a snapshot of current state (security RVA); `--from/--to` are rejected, not silently ignored |
  | `inline` | `{default: 7d, inline: true}` | the DQL writes `from:`/`to:` itself from `.window` (billing fetches past the window end to catch late usage) |
  | `align` | `{default: 7d, align: utc-day, inline: true}` | rounds the resolved window to UTC midnights |

  A search horizon is not a separate form. `problems-get` uses `timeframe: 30d`,
  and its `describe` text says the window is how far back to look.
- New flags on the stable `query` command declare their tier:
  `stability.MarkFlag(queryCmd, "from", stability.Experimental, "0.42.0")`,
  and the same for `to`.

### 4. CLI surface

| Command | Purpose |
|---|---|
| `dtctl run <recipe> [positional] [--param ...] [framework flags]` | Render and execute |
| `dtctl get recipes [--tag t]` | Index: name, summary, required params, default window, source |
| `dtctl describe recipe <name>` | Everything: params, timeframe, DQL, `means`, `emptyMeans`, `next`, source file, what it shadows |
| `dtctl verify recipe <name> \| -f <file>` | Schema check, then render with defaults and validate the DQL through the verify API (no execution) |

**Each recipe is a cobra subcommand of `run`**, generated from its file.

- Params become flags (`--service`, `--top`), with type, default, required
  marker and description, so `dtctl run services-failures --help` is the recipe's
  documentation.
- The `Short` line is the recipe's `summary`.
- `run` is a verb in the catalog. Listing every recipe there does not scale
  (§11): about 300 names cost about 2k tokens on every agent bootstrap. The
  catalog therefore lists `run` alone, and its description points to
  `get recipes` (§6, level 0). *Prototype:* no catalog level lists recipes,
  `--full` included. The catalog is environment-independent, which keeps the
  stability manifest free of content; `get recipes` is the listing.

**Framework flags**, present on every recipe and reserved as param names:

- the timeframe: `--from`, `--to`
- the scope dimensions the recipe declares (`--cluster`, `--namespace`,
  `--tag key=value`, …; §10) and the segment flags shared with `query`
  (`-S/--segment`, `--segments-file`, `-V/--segment-var`)
- `--dry-run`: print the rendered DQL and resolved window, execute nothing
- the output and limit flags shared with `dtctl query`: `-o`,
  `--max-result-records`, `--max-result-bytes`,
  `--default-scan-limit-gbytes`, `--no-query-limits`, `--spill*`, `-M/--metadata`,
  the output bounds
- the global flags

A recipe never sees or overrides these. Lint rejects a param named after one.

`dtctl run` with no recipe prints the index, the same as `get recipes`.

`--dry-run` uses the existing opt-in mechanism (`dryRunCommands`). A recipe run
writes nothing, so the preview is trivially write-free, and the dry-run agent
envelope (#663) applies.

### 5. Execution and output

`run` renders the DQL and hands it to the **same execution path as
`dtctl query`**. That path is refactored out of `query`'s `RunE` into a shared
function, so recipes inherit all of the following unchanged:

- query limits and the scope precheck (`dqlScopePrecheck`)
- spill and the output bounds
- empty-result diagnosis
- lookback and window advice
- every output format and the golden-tested printers

A recipe is a pre-filled `dtctl query`, not a second query engine.

**Agent envelope additions** (additive context keys):

```json
{
  "ok": true,
  "result": { "kind": "records", "records": [ ... ] },
  "context": {
    "verb": "run",
    "resource": "services-failures",
    "recipe": { "name": "services-failures", "version": 1, "source": "builtin" },
    "query": "fetch spans\n| filter dt.service.name == \"checkout\"\n...",
    "window": { "from": "2026-10-04T08:00:00Z", "to": "2026-10-04T10:00:00Z" },
    "scope": { "namespace": ["payments"], "segments": ["team-checkout"] },
    "suggestions": [
      "dtctl run logs-for-service --service checkout --from 2h",
      "adapt: dtctl query '<context.query>' --from 2h"
    ]
  }
}
```

- `context.query` is the rendered DQL. This is the escape hatch, and it is what
  the evals showed agents actually use.
- `context.scope` names every filter that narrowed the result and that does not
  appear in the recipe's own params: scope dimensions, plus segments, including
  ones a context applied by default (§10). An agent reading an empty or small
  result must be able to see that something outside the query text narrowed
  it. `context.query` shows the scope predicate, but not the segments, which
  Grail applies on the server.
- **Empty result:** `context.empty_reason` carries a new code,
  `recipe_empty_means`, whose `evidence` is the recipe's `emptyMeans`. dtctl's
  own empty-result diagnosis still runs. If it finds something concrete (a field
  not in the sample, a metric not in the window), that finding takes precedence,
  and `emptyMeans` is added as a warning.
- `means` is **not** repeated in every response; it costs context on every call.
  It appears in `describe recipe` and `--help`.
- `next` edges are rendered as ready-to-run `suggestions`, with params bound
  from the current invocation's params. Phase 1 does not bind values from
  result rows.
- `deprecated` adds a warning naming `replacedBy`.
- Human output is exactly `dtctl query`'s output for the rendered DQL. There is
  no header or banner, so piping stays clean. `--dry-run` prints the DQL to
  stdout and the window to stderr, so
  `dtctl query "$(dtctl run x --dry-run)"` works.

### 6. Discovery for agents: progressive disclosure

At 300 built-in recipes, a few dozen per app, and more from org layers, a
flat list is too big to hand an agent. One summary line costs about 25 tokens,
so 300 recipes cost about 7.5k tokens and 3,000 cost about 75k. Discovery
therefore has four levels. Each one is a separate call and stays small no
matter how large the book grows:

| Level | Call | Returns | Size |
|---|---|---|---|
| 0 | `dtctl commands` | `run` with a recipe count and a pointer to `get recipes` | constant, ~50 tokens |
| 1 | `dtctl get recipes` | the **domain index**: domain, one line, `available/total`, sources | one line per domain; ~15–30 domains |
| 2 | `get recipes --search "<words>"`, `--domain k8s`, `--tag triage` | one line per recipe: name, summary, required params | capped at 25 lines, with `has_more` |
| 3 | `dtctl describe recipe <name>` | params, scope, `means`, `emptyMeans`, `next`, rendered DQL | one recipe |

- **`--search` is the main path.** The agent already has a question, so the
  question is the query: `get recipes --search "pods oom killed"`. Matching is a
  local keyword ranking over name, summary, tags and domain description. There
  are no embeddings and no network calls. Summaries are written as the question
  the recipe answers, because that is what they are matched against.
- **Domains are for navigation; tags are for search.** Every recipe has exactly
  one domain, taken from its name prefix. Domains come from registries, so they
  stay a short, curated list even when many owners contribute (§13). Tags are
  free-form and decentralized, and different owners will spell them differently
  (`k8s`, `kubernetes`). That makes them good search terms and bad categories:
  they feed the `--search` ranking and the `--tag` filter, but the index never
  groups by them.
- **The index shrinks with the environment.** Recipes that need data the
  environment does not have are hidden (see below). Domains with nothing
  available collapse into one line: `unavailable here: k8s, aws, gcp`. On an
  environment without Kubernetes and one cloud, that removes most of the
  catalog before the agent reads it.
- **Every truncation says so** (`context.has_more`, `context.hidden`), with the
  flag that shows the rest (`--all`). This follows the agent-mode output rule in
  AGENTS.md.
- Human mode prints the full table, grouped by domain.

A typical agent bootstrap is one search, or the index plus one domain. Either
costs a few hundred tokens and replaces dozens of `dtctl commands` and
failed-DQL calls.

The `dtctl` skill (`skills/dtctl/SKILL.md`) gets one rule: *before writing DQL,
check `dtctl get recipes --search …`. `dtctl run <name> --dry-run` shows a
recipe's DQL as a starting point.* There is no recipe content in the skill. The
prototype found the CLI listing beats skill files, and duplicating content would
create another copy to keep in sync.

**Inventory-aware listing.** `spec.requires` names capabilities from `dtctl
inventory`. The listing uses them to hide recipes that cannot work here.
Discovery has real latency (a full `dtctl inventory` runs a budgeted battery of
queries), so listing never waits for a full run. Three rules keep it fast:

1. **The verdicts are cached.** `dtctl inventory` writes its capability verdicts
   to a per-context cache (`$XDG_CACHE_HOME/dtctl/inventory/<context>.json`),
   with a 24h TTL. Capabilities are retention-scoped and change over days, not
   minutes, so a day-old verdict is still useful. (Today `inventory` persists
   nothing; this is a deliberate change.) In service mode the cache is in
   memory, keyed by environment and principal, and never on the host disk.
2. **A cold cache gets the structural pass only.** With no cached verdict,
   `get recipes` runs discovery limited to the capabilities its slice requires,
   and only the structural shapes: data-object catalog, entity census and
   metric catalog. That is a fixed handful of catalog queries, however many
   capabilities there are, and none of them scans data. It has a short budget
   (`--inventory-budget`, default 10s). If the budget runs out, the listing is
   unfiltered and says so. Probe-shaped capabilities cost one query each, so
   they are evaluated only by an explicit `dtctl inventory`, and are *unknown*
   until then.
3. **Only an `absent` verdict hides a recipe.** `unknown`, or no cache, shows
   the recipe. Inventory evidence can be wrong, so a hidden recipe still
   runs: `run` on a recipe whose requirement is absent in the cache executes
   it and adds a warning with the cited evidence.

The envelope reports what filtering did: `context.inventory: {age: "3h",
hidden: 112}`, and a suggestion to refresh it (`dtctl inventory`) or see
everything (`--all`). `--no-inventory` disables the filter for one call.

`requires` is a list, and all of its entries must be present. A family recipe
whose enum spans providers (`cloud-inventory --provider aws|azure|gcp`) lists
no provider capability. Per-value requirements are an open question.

### 7. Where recipes live

| Layer | Location | Phase |
|---|---|---|
| Built-in | `recipes/<domain>/*.yaml` at the repo root, embedded with `go:embed` (as `skills/dtctl/` is) | 1 |
| Environment | `app` sources: the recipe bundles an installed app ships to the environment, synced and pinned per environment (§13, §14) | 2 |
| Team/org | declared `git`, `archive` and `dir` sources (§14), then directories in `DTCTL_RECIPE_PATH` (colon-separated, read live) | 2 |
| User | `$XDG_CONFIG_HOME/dtctl/recipes/**/*.yaml` (`~/.config/dtctl/recipes/`) | 1 |

Sources are declared in a user file or a project's `.dtctl/recipes.yaml`,
synced into a local store, and pinned in a lock (§14). A run reads the lock
and the store and never the network.

**Precedence:** user overrides org, which overrides environment, which
overrides built-in. Within the org layer a later source wins, and a project
source replaces a user source of the same name. A whole recipe replaces another recipe of the same name.
There is no field-level merge, which is where the prototype's complexity came
from.

- An override is visible: `describe recipe` names the file or document and
  what it shadows, and `context.recipe.source` says `builtin`, `user`, `org`
  (`DTCTL_RECIPE_PATH`), `<source>@<pin>` for a declared source
  (`team@3f2a1c9e0b7d`), or `app:<app-id>@<version>`. An agent's transcript
  therefore records exactly which version of a recipe answered.
- Overriding a built-in is a feature. A team can pin a recipe to its own
  environment's field names.
- A user layer may add fragments and scope dimensions, but may not redefine
  built-in ones. A changed fragment would silently change built-in recipes
  that the user never overrode.

**Failure isolation:**

- A broken user recipe file never breaks unrelated commands.
- The loader runs only for commands that need the recipe tree: `run`,
  `get recipes`, `describe recipe`, `verify recipe`, `commands`, `completion`
  and `help`.
- Invalid files are skipped with a warning on those surfaces.
- Files whose `kind` is not `Recipe` are skipped silently. The prototype wrote
  generated `RecipeBook` files into the same directory, so early adopters may
  have them.
- **Startup budget:** loading the embedded set plus a typical user directory
  stays under 5 ms. A benchmark test guards it.

**Code layout** (following the SDK delegation rule):

| Path | Contents |
|---|---|
| `recipes/` | content and `embed.go` only: `<domain>/*.yaml`, `_domains.yaml`, `_scopes.yaml`, `_fragments/*.tmpl`, and the generated index (§11) |
| `pkg/recipes/` | types, loader (layers, precedence), validation, rendering. No cobra and no output code; testable alone. |
| `cmd/run.go` → `cmd/run_recipe.go` | `cmd/run.go` is already the embedding entry point (`cmd.Run`), so the command lives in `run_recipe.go`: building the `run` subtree from the loaded book, plus `get recipes`, `describe recipe` and `verify recipe` in their verb files |
| `pkg/exec` | the shared query execution path (extracted from `cmd/query.go`) |

### 8. Fit with existing invariants

- **Safety.** Phase 1 recipes are DQL, so reads. They need no `CheckSafety`,
  exactly like `query`. `run` is not added to `MutatingVerbs`, and
  `auth.AccessForVerb` maps `run` to read access. A later API-step phase
  classifies each step with `resapi.Classify` and gates the recipe on the
  strictest step through `CheckSafety`. Catalog listings then need per-recipe
  mutating status instead of the per-verb map.
- **Stability.**
  - `run`, `get recipes`, `describe recipe` and `verify recipe` are
    `experimental` (since 0.42.0). `query --from/--to` are experimental flags on
    a stable command.
  - Recipe leaf commands are annotated as recipes and **excluded from
    `docs/STABILITY.md`**. Otherwise every recipe content change would show up as
    manifest drift, coupling content to code again. Their experimental parent
    already exempts them from `TestEveryCommandDeclaresItsTier`.
  - If `run` is ever promoted, the stable promise covers the **framework**:
    framework flags, the param → flag mapping, the envelope keys and
    `--dry-run` output. It does not cover the recipe set.
  - Built-in recipes change by content rules: renaming or removing one goes
    through `deprecated:` and survives at least two minor releases, mirroring the
    stable deprecation window. Changing what a recipe returns bumps
    `metadata.version`.
- **Profiles.**
  - `run` in a profile's allowlist grants every recipe. `run services-failures`
    grants one; existing segment-prefix matching covers both.
  - Profiles are default-deny, so no existing profile gains recipes silently.
  - Whether the `investigate` preset should include `run` is an open question.
- **Engine / service mode.**
  - Built-in recipes are available in the engine. App bundles are too, when
    the request names their apps in `Request.RecipeApps` (`<app-id>` or
    `<app-id>@<version>`, §14); they are cached in memory only.
  - Sources files, locks, the store and the user directory are host state: a
    `Session` reads none of them, the same as aliases, and `recipes` (sync,
    add, remove) is blocked in a service.
  - `verify recipe -f <file>` reads through `readFileFlag` (vfs).
- **Plugins and aliases.**
  - The built-in `run` shadows any `dtctl-run` plugin. Built-ins always win;
    plugin discovery already warns about such collisions.
  - An alias may expand to a `run` invocation like to any command.

### 9. Authoring and CI for built-in recipes

Content is reviewed as content, but it is still tested:

- **`go test ./recipes/...`** (offline, every PR):
  - schema validation
  - name uniqueness
  - template references ⊆ params
  - every recipe renders with defaults and with boundary values
  - `next` targets exist
  - `requires` names exist in the built-in inventory definitions
  - no top-level `from:` when a timeframe is declared
  - golden rendered DQL per recipe. A content change shows up as a reviewable
    diff, the same way golden output tests work.
- **`dtctl verify recipe --all`** against a live environment, run in the
  existing live/E2E job. It validates every rendered query through the DQL verify
  API with no data scans.
- **Execution smoke test** (nightly, optional): run each recipe with defaults
  against at least two environments with different shapes. Fail on errors,
  and report (but do not fail on) empty results. Results stay out of the
  repository, per the privacy rule.
- `CODEOWNERS` can assign `recipes/` to query-knowledge owners separately from
  Go code owners.
- Recipe changes are conventional commits (`feat(recipes): …`,
  `fix(recipes): …`), so they reach users with the normal release flow.

### 10. Scoping and filter context

"Only my team's stuff" reaches a query through five mechanisms. A recipe uses
each of them in a different way:

| Mechanism | Who sets it | Where it acts | In recipes |
|---|---|---|---|
| **Recipe params** | the caller, per recipe | the DQL text | the question's own subject: service, host, problem ID, threshold |
| **Scope dimensions** | the caller, the same flag on every recipe that declares it | the DQL text (`.scope.stage` / `.scope.expr`) | cross-cutting "where": cluster, namespace, host group, cloud account, primary tag |
| **Filter segments** | the caller or the context (phase 2) | server-side, the query API's `filterSegments` | passed through unchanged; DQL text is unaware of them |
| **Permissions** | IAM policies, record-level permissions | server-side, always | cannot be bypassed or detected; `emptyMeans` must allow for them |
| **Buckets** | the data's ingest configuration | `fetch …, bucket:` | only when a recipe's data lives in a known bucket (`network-top-talkers`) |

The mechanisms combine with AND. None of them can widen what another one
narrowed.

**Scope dimensions: primary fields and primary tags only.** A scope dimension is
worth having only if one predicate means the same thing in every recipe. Primary
Grail fields (`k8s.cluster.name`, `k8s.namespace.name`, `dt.host_group.id`,
`aws.account.id`, `azure.subscription`, `gcp.project.id`) and primary Grail tags
(`primary_tags.<key>`) meet that bar: Dynatrace enriches them on logs, metrics,
spans, events and problems, and on the relevant Smartscape nodes. A recipe over
spans and a recipe over logs can therefore both write
`in(k8s.namespace.name, {"payments"})`, with no per-data-object mapping table.
The predicate is always `in()`, even for one value: on some data objects these
fields are arrays (`k8s.cluster.name`, `k8s.namespace.name` and
`dt.host_group.id` on `dt.davis.problems`), where `==` never matches. A
prototype that rendered `==` returned no problems for `--cluster` on a tenant
with 181 matching ones.

Anything else changes spelling per data object, and stays a recipe param:
`service.name` on spans, `dt.service.name` on metrics, `dt.smartscape.service`
as an ID. Service logs reach the service only through the pod. The phase 2
`entity` type handles that hop.

- The dimension list lives in content, `recipes/_scopes.yaml`
  ([example](examples/recipes/_scopes.yaml)). Adding a dimension needs no code
  change.
- A recipe opts in with `scope: [cluster, namespace, tag]` and gets those flags
  only. A recipe over billing events offers no `--namespace`, because the field
  is not there.
- Multiple flags combine with AND. A repeated flag combines with OR, through
  `in()`. `--tag team=payments --tag team=checkout` becomes
  `in(primary_tags.team, {"payments", "checkout"})`.
- The values render as escaped literals, the same as params.
- `.scope.stage` goes after a `fetch` or `smartscapeNodes`. `.scope.expr` goes
  into `timeseries … filter:`, so the filter runs before aggregation instead of
  on a result that has already been aggregated.
- Deliberately excluded: `dt.security_context` (a permission field that IAM
  already enforces; filtering on it duplicates the server's job), and
  `dt.cost.costcenter`/`dt.cost.product`, which are strings on some billing
  events and `record[]` on others. A cost recipe takes those as explicit params.
- Management zones do not exist in DQL. A team that used zones moves to
  segments or primary tags. Recipes have nothing to offer here.
- Primary tag keys are customer-defined. `dtctl run meta-primary-tags` lists the
  keys present in the environment (through the `primary-field` tag in
  `dt.semantic_dictionary.fields`), so an agent can find out that `--tag team=…`
  is meaningful before using it.

**Filter segments: pass-through, not modelled.** A segment is a saved, named
filter that the query API applies on the server. Its include rules are per data
object, which is what the UI uses to offer "my team" across every app. It is
the closest thing to a scope dimension that already exists, so recipes do not
re-implement it:

- Every recipe inherits `query`'s segment flags (`-S/--segment`,
  `--segments-file`, `-V/--segment-var`). They are sent unchanged as
  `filterSegments`.
- **Phase 2: context default segments.** `dtctl config set-context prod
  --segments team-checkout` applies the segment to every `query` and `run` in
  that context, unless `--no-segments` is given. This is the "agent works for
  one team" setup with nothing to remember per call.
- `spec.segments: off` marks a recipe whose data segments must not narrow:
  billing usage, security posture, environment discovery. Passing `-S` to such a
  recipe is a usage error. A context default is skipped and the envelope says
  so, so the agent does not read a full billing total as "my team's cost".
- `context.scope` (§5) lists the applied dimensions and segments, including a
  context default. An empty result with a segment applied is a different
  finding from an empty result without one.

**Permissions.** Record-level permissions and sensitive-data fieldsets remove
records or fields with no error, so a recipe cannot tell "nothing happened"
from "you may not see it". `emptyMeans` of recipes over data that is commonly
restricted (security events, audit logs, billing) says so. Reviewers check
this; it is not something lint can decide.

**Precedence for a dimension set in more than one place.** Explicit wins:
a flag on the command, then the context's default (phase 2), then the recipe's
own default. The context never overrides what the caller typed.

### 11. Size and organization of the recipe book

**How large will the book get?** The best evidence is the `dynatrace-for-ai`
skills: that is where the same knowledge is being written today, as Markdown.

Across 23 query-heavy skills there are 2,093 DQL blocks. About 1,320 ask a real
question. The rest teach syntax, show fragments or duplicate other blocks.
Collapsed with the rules below, the 1,320 become **about 270–315 distinct
recipes**:

| Skill | DQL blocks | Question queries | Distinct recipes |
|---|---|---|---|
| dt-obs-azure | 306 | 192 | 15–20 |
| dt-sec-insights | 210 | 77 | 15–20 |
| dt-obs-aws | 197 | 118 | 15–18 |
| dt-dql-essentials | 173 | 94 | 3–5 (discovery utilities) |
| dt-obs-frontends | 150 | 133 | 30–35 |
| dt-obs-hosts | 140 | 115 | ~25 |
| dt-obs-kubernetes | 136 | 112 | 25–30 |
| dt-obs-tracing | 130 | 106 | 25–30 |
| dt-obs-services | 101 | 94 | 12–14 |
| dt-obs-problems | 90 | 52 | 15–18 |
| dt-obs-gcp | 83 | 38 | 8–10 |
| dt-platform-costs | 68 | 18 | 10–12 |
| dt-obs-genai | 52 | 39 | ~15 |
| dt-obs-network-flows | 49 | 35 | ~15 |
| 9 smaller skills | 208 | 97 | ~45 |
| **Total** | **2,093** | **1,320** | **~270–315** |

(Counts come from scripted classification plus a manual pass over every
heading, so treat them as ±10%. Snapshot: `dynatrace-for-ai` at 4f9aa71.)

The other prior work overlaps heavily with this: dynatui's 35 views, the
prototype's 52 recipes and correlate's 28 evidence recipes. The union is
about **300–350 recipes**.

**Collapsing rules.** These are what keep the number in the hundreds. They are
the review rules for new content.

1. **The same pipeline over a different type or metric is one recipe with an
   enum param.** About 25 Azure and 30 AWS blocks are "one metric for one
   resource type". About 70 runtime blocks (Java, Go, PHP, Node.js, Python,
   .NET) become one `services-runtime-health --runtime --signal`.
2. **The same question with a different group-by or filter is one recipe with an
   optional param or an enum `--by`.** 30 Kubernetes label queries become one
   `k8s-label-coverage --label`. Browser, OS and geo breakdowns are one recipe.
3. **Teaching blocks are never recipes.** ❌/✅ pairs, pitfall demonstrations,
   placeholder syntax and step-by-step walkthroughs (35 of the problems skill's
   90 blocks) stay in the skill.
4. **Copy-paste is a duplicate.** One security reference repeats about 27
   blocks from two others. GCP has 33 blocks with the same shape.
5. **A composed pipeline is a fragment plus variants.** The security skill's
   "base + dedup → optional filter → summarize → variant" accounts for its 77
   fragments.

**When would it be thousands?** If a recipe were written per entity type, per
extension or per technology, with no families. Smartscape has hundreds of node
types, and the Extensions Hub has hundreds of extensions, each with its own
metrics. A recipe per pair would reach thousands quickly, and nearly all of
them would be one template with a different literal. The book does not grow
that way:

- Core (built-in) aims at **300–500** recipes once mature: the union above
  plus areas the skills do not cover yet.
- The per-technology long tail goes to **apps** (§13) and **org layers**
  (`DTCTL_RECIPE_PATH`), owned by whoever owns the technology. An environment
  only carries the bundles of the apps it has installed. Even a large total
  stays manageable in each environment, because inventory filtering (§6) hides
  what does not apply there.
- A family parameter whose values come from the environment (all node types, all
  metric keys) is not an enum but a `meta-` discovery recipe plus a string
  param.

**What scale changes in the design:**

- **Domains.** Names are `<domain>-<name>`, against a registry in
  `recipes/_domains.yaml` ([example](examples/recipes/_domains.yaml)) of about
  15 domains, plus any that app bundles add (§13). Files live in `recipes/<domain>/`. Domains drive the catalog
  index, `get recipes --domain` and `CODEOWNERS`, so the Kubernetes team owns
  `recipes/k8s/`.
- **Discovery** returns the domain index first, then one domain (§6). An agent
  never reads 300 summaries.
- **Loading.** Parsing 300 small YAML files on every `run` would break the 5 ms
  startup budget (§7). The build generates a compact index (name, domain,
  summary, params, scope) into the embedded set. The loader reads the index, and
  parses a full recipe only when it runs or describes it. User layers are small
  and are parsed directly.
- **CI cost.** Offline tests are cheap at any size. `verify recipe --all` makes
  one verify call per recipe, so about 300 calls per live run. That is fine for
  a nightly job. On a PR it runs only for changed recipes and their fragments.
- **One source for skills and recipes.** At this size, the skills and the book
  would be two copies of the same knowledge. The long-term answer is that a
  skill reference *points to* the recipe (`dtctl run k8s-pod-restarts`), and
  the generated Markdown arm of the evaluation is what agents without dtctl get.
  This is an open question, and the evaluation decides it.

### 12. Worked examples

[examples/recipes/](examples/recipes/) holds fourteen recipes adapted from the
`dynatrace-for-ai` skills, plus the domain registry, the scope dimensions, and a
fragment. Together they exercise every schema feature in this design:

| Recipe | Shows |
|---|---|
| `k8s-pod-restarts` | `.scope.expr` inside `timeseries filter:`; an int threshold |
| `services-red` | a `list` param; a unit trap (µs) encoded once in `means` |
| `traces-slow-endpoints` | an optional positional param; `next` bound from a result row |
| `logs-error-patterns` | optional filters plus `.scope.stage` |
| `problems-active` | an `enum` param; the `ACTIVE` vs `OPEN` trap |
| `problems-get` | `pattern` on a positional ID; a lookback window |
| `hosts-disk-saturation` | host-group scope on metrics |
| `cloud-inventory` | `timeframe: none`; an enum that selects the node type |
| `frontends-web-vitals` | unit and threshold conventions |
| `genai-token-usage` | `next: when: empty`, the "is anything instrumented?" protocol |
| `security-vulns-critical-exploitable` | a `fixed` snapshot window; a fragment; `segments: off` |
| `costs-dps-by-capability` | an inline window aligned to UTC days; `segments: off` |
| `capacity-cpu-saturation` | a minimum window for a trend |
| `network-top-talkers` | a bucket-scoped fetch |

They also show where the skills' knowledge goes:

- the query becomes `dql`;
- the prose around it ("response time is in microseconds", "`OPEN` is not a
  status") becomes `means` and `emptyMeans`;
- "next, look at …" becomes `next`.

None of them has been executed against an environment. They are design
material, not the initial set.

### 13. Recipes shipped by apps

Dynatrace apps already ship more than code: dashboards, and now skills, which
the platform stores as documents in the Document store. An app releases on its
own schedule, and its owner owns what it ships. Recipes fit the same model. The
team behind an AI-observability app knows the GenAI queries better than dtctl
does, and should be able to ship and fix them without a dtctl release. The
environment then carries the recipes for the apps it actually has installed.

This adds one layer, **environment** (§7). It reuses everything else: the
recipe format, validation, rendering, discovery and inventory.

**What an app ships: one document per bundle.**

- Document `type: ai-agent-resource`, the type apps already use to ship
  skills (named `/skills/<skill>/SKILL.md`). A bundle is named
  `recipes/<name>.yaml` (leading slash optional), and the name is what tells
  it apart from a skill. Its content is YAML of `kind: RecipeBundle`
  ([example](examples/recipes/bundle-genai.yaml)). A bundle holds `recipes`
  (the same schema as a recipe file), optional new `domains`, optional
  `capabilities` (the `dtctl inventory --definitions` shape), and fragments that
  only its own recipes can use.
- One document per bundle, not one per recipe. That is one list call plus one
  download per app, and a bundle updates atomically, so an app never shows a
  half-updated set.
- `metadata.minDtctlVersion`: an older dtctl skips the bundle and says why,
  instead of failing on a field it does not know.

**Trust: app-deployed documents, from apps you enabled.** Recipe text is
prompt input for agents, so whoever can write it can steer them. Two rules
apply. First, dtctl loads only bundles whose document has an `originAppId`,
meaning the platform deployed it as part of an app; a bundle a user uploads by
hand is never loaded. Second, an app's bundles load only after someone
declared that app as a source and synced it (§14). Installing an app on the
environment is not enough. Declaring the source is the allowlist of app IDs
that open question 11 asked about. Teams that want shared content without an
app use a git, archive or dir source, whose content they control. *To verify
with the platform:* users cannot create or modify a document carrying an
`originAppId`.

**Content rules**, enforced when a bundle is loaded:

- DQL recipes only, the same as phase 1, so app recipes are read-only and need
  no safety gate. When API steps arrive (phase 3), bundles do not get them
  automatically. That is a separate decision.
- A recipe name starts with a built-in domain or one the bundle declares. A
  bundle may add recipes to a built-in domain (`genai`) but cannot re-describe
  it.
- Bundles add capabilities but cannot redefine a built-in or another bundle's
  capability. Merge order for inventory definitions: built-in, then apps, then
  the user's `--definitions` files, which can override anything.
- Bundles cannot add scope dimensions, so `--namespace` means the same thing in
  every recipe whatever its source.
- If two bundles define the same recipe, domain or capability name, **neither
  is loaded**. `get recipes` reports the conflict and both sources. Silently
  picking one would make a recipe's meaning depend on install order.
- An app recipe may shadow a built-in one. Precedence is user > org >
  environment > built-in. The app owner is usually the better authority, and
  `describe recipe` shows the shadowing.

**Fetching.** Bundles are fetched only by `dtctl recipes sync` (§14), never
by a run:

- The bundle list is one call:
  `GET /platform/document/v1/documents?filter=type=='ai-agent-resource' and name contains 'recipes/'`.
  It returns ID, name, version and `originAppId` (returned by default;
  `add-fields=originAppId` is rejected with a 400) for every candidate. Names
  that are not bundle names are dropped before the trust check.
- A bundle is downloaded with `GET .../documents/{id}/content`, the raw
  content without the multipart envelope of a full document read, and only
  when the lock does not already hold that document version's content.
- Pins are per environment: the same app source pins document versions on
  each environment separately, since IDs and versions differ between them.
- **Discovery without loading.** `get recipes` lists the environment's bundle
  documents at most once an hour (cached per context) only to print a hint:
  "1 app on this environment ships recipes that are not enabled: `dtctl
  recipes add genai --app <id>`". An unknown name in `run` reads the same
  cache. Nothing loads from this listing.
- **Service mode.** A request names the apps it wants (`Request.RecipeApps`),
  optionally with the document version it pins. The bundles come from the
  tenant the request targets, with the request's credentials, so they are
  request state; listings are cached in memory for a minute and contents per
  document version, never on the host disk (Embedding Invariants §2).

**Inventory, shipped by the app.** A bundle's `capabilities` let an app say
when its recipes apply. The example bundle defines `genai` as a span probe and
`genai-evaluations` as a bizevents probe, and its recipes require them. On an
environment with no GenAI traffic, `dtctl inventory` finds `genai` absent and
the listing hides those recipes (§6). That an app is installed is not evidence
that its data exists; the capability decides.

**Authoring for app owners.** `dtctl verify recipe -f bundle.yaml` validates a
bundle offline (schema, templates, domain and capability rules) and, with a
context, verifies every rendered query against a live environment. An app's CI
runs it the same way dtctl's CI runs `recipes/`. How the bundle gets into the
app package is the app toolkit's business, not dtctl's.

**Where this leads.** If apps become the main channel, the built-in set can
shrink toward the cross-cutting core (problems, logs, services, discovery),
with technology-specific recipes owned by the apps for those technologies.
The built-in set stays as the fallback for environments without those apps and
for offline use. This is a direction, not a phase-2 commitment. It also
replaces the earlier "independently released recipe bundle" idea: apps already
release independently.

An app that ships a skill and a recipe bundle can have the skill point to the
recipes (`dtctl run genai-agent-errors`), so the query exists once.

### 14. Sources, sync and pinning

The first prototype loaded remote content dynamically: every `run`,
`get recipes` and `describe recipe` re-listed the environment's bundles once
an hour, and an unknown name forced a refresh. That made recipes appear the
moment an app was installed. It also meant:

- **What an agent ran changed without anyone choosing it.** An app update
  rewrote a bundle, and within the hour every agent on the environment ran
  different DQL. A demo, a CI job or an evaluation could not count on getting
  the same recipes twice.
- **Trust was implicit.** Any installed app could add prompt text for every
  dtctl user of the environment, and nobody opted in.
- **The network sat on the run path**, small as the cost was.

The model is now the one package managers use: sources are **declared**, a
**sync** resolves and fetches them, a **lock** records what sync resolved, and
a run reads only the lock and a local content-addressed store.

**Declaring.** A sources file lists what to load:

```yaml
# ~/.config/dtctl/recipe-sources.yaml, or .dtctl/recipes.yaml in a project
apiVersion: dtctl.dev/v1alpha1
kind: RecipeSources
builtin: true                 # false turns the built-in recipes off
sources:
  - name: dt-for-ai
    git: github.com/<owner>/<repo>
    ref: v1.4.0               # branch, tag or commit; sync pins the commit
    path: recipes
  - name: genai
    app: <app-id>             # bundles this app installed on the environment
  - name: team
    archive: https://example.invalid/team-recipes.tar.gz
  - name: drafts
    dir: ./recipes            # live, never pinned: for authoring
```

| Kind | Fetched as | Pinned to |
|---|---|---|
| `git` | GitHub over HTTPS: one ref lookup, one tarball. No git binary, so no subprocess and no capability gate. Other hosts use `archive`. `GITHUB_TOKEN` is sent to GitHub only. | the commit the ref resolved to |
| `archive` | a `.tar.gz` at an https URL, optionally with `sha256` declared up front | the archive's SHA-256 |
| `app` | the environment's bundle documents for that app (§13) | document ID and version, per environment |
| `dir` | a local directory, relative to the sources file | nothing: read live on every run |

`builtin: false` leaves the built-in *recipes* out. The built-in domains,
scope dimensions and fragments still load, because other sources' recipes
are written against that vocabulary.

**Syncing.** `dtctl recipes sync` installs exactly what the lock names, like
`npm ci`: a pinned source whose content is already in the store costs no
request at all, and one whose content is missing (a new machine, a cleaned
store) is fetched at exactly the locked commit, digest or document version.
A source the lock does not cover yet, or whose declaration changed (another
ref, path, URL or app), is resolved and pinned. `sync --update [source...]`
re-resolves and moves pins, and reports which recipes the move added,
changed or removed. A source that fails to sync keeps its previous pin, so
one unreachable source never unpins the others.

**The lock** sits next to its sources file (`recipe-sources.lock`,
`.dtctl/recipes.lock`). Per source it records the fingerprint of the
declaration it was resolved for, the commit or archive digest, the digest of
the stored recipe tree, and for an app source the document versions per
environment. Tree digests depend on content only (sorted paths and file
digests), not on how an archive was compressed.

**The store** is `$XDG_DATA_HOME/dtctl/recipes/store/`: `tree/<digest>/` per
git or archive pin and `blob/<digest>.yaml` per bundle version. Content never
changes under a digest, so switching contexts, projects or pins back and
forth fetches nothing twice. It lives under the data directory rather than
the cache because an app pin the environment has since moved past cannot be
fetched again: the Document store serves only a document's current content.
Sync says so and names `--update` as the way on.

Archive extraction keeps `.yaml`/`.yml` files and `_fragments/*.tmpl` under
the source's path and refuses links, absolute paths and `..` rather than
skipping them, with limits of 1 MB per file, 32 MB and 5,000 files per tree.

**Projects.** A project's `.dtctl/recipes.yaml` is found by walking up from
the working directory. Committed with its lock, it gives a team identical
recipes on every machine. It is honoured only after `dtctl recipes sync` ran
against exactly its current content and lock. Sync prints the sources it is
enabling and records a digest of both files in
`$XDG_STATE_HOME/dtctl/recipes/trusted-projects.json`; a later change (a pull,
a planted edit) suspends the file until the next sync. A cloned repository
cannot plant agent prompts just by being the working directory, the same
reasoning that keeps `.dtctl.yaml` aliases off.

**Commands.**

| Command | Does |
|---|---|
| `dtctl recipes add <name> --git/--archive/--app/--dir … [--project]` | declares a source and syncs it (`--no-sync` to only declare) |
| `dtctl recipes remove <name> [--project]` | removes a source and its pins |
| `dtctl recipes sync [--update [source...]]` | installs the lock; with `--update`, moves pins |
| `dtctl recipes outdated` | resolves every source without fetching content and shows which pins moved upstream; changes nothing |
| `dtctl get recipe-sources` | every source with its pin, last sync and status: `ok`, `live`, `not synced`, `missing`, `untrusted`, `overridden`, `off` |

A source that declares content but contributes none (not synced, content
missing, an untrusted project) is named in a warning on `get recipes` and in
the "unknown recipe" error, with the command that fixes it.

**What pinning does not do.** There is no per-recipe pin (`run x@2`) and no
lockfile of discovered environment state, which was the prototype's
complexity. A pin covers a whole source. To freeze one recipe while its
source moves, copy it into the user directory; the higher layer replaces it
by name and `describe recipe` shows what it shadows.

## Initial recipe set

The prototype's eval suite is a good source for the first ~12 recipes: most of
them encode a trap that cost control-arm agents calls or a wrong answer. Every
one is re-verified on at least two environments before it lands. The worked
examples (§12) are the next candidates, once verified the same way.

| Recipe | Question | Why it earns a recipe |
|---|---|---|
| `logs-error-sources` | top sources of ERROR logs | most common triage entry point |
| `services-latency` | services ranked by p95 | sampling hid the true slow tail and flipped a ranking in one eval |
| `services-failures` | failure signatures of one service | common triage step (a correlation-graph evidence recipe) |
| `problems-active` | open Davis problems | status is `ACTIVE`, not `OPEN`; Davis duplicates must be filtered |
| `hosts-census` | hosts by OS/cloud | the `dt.entity.*` lookback view under-counted on one tenant and over-counted on another; Smartscape is live state |
| `cloud-functions-census` | serverless functions per cloud | same lookback trap at fleet scale (~1.2k of ~9.3k Lambdas found) |
| `k8s-workload-status` | ready vs desired replicas | era-specific manifest keys; superseded pods linger in topology |
| `k8s-oom-killed-pods` | pods with OOM kills | truncated series undercounted (21–50 of 252) at scale |
| `frontends-event-volume` | user events per frontend | control agents guessed 8–10 wrong data object names |
| `genai-token-usage` | tokens by model/provider | guided-by-skill query scanned ≈6 GB where the recipe scanned ≈0 |
| `security-detections` | attack detections | proving absence cheaply (≈10× less scan) |
| `security-compliance-findings` | posture findings | a 2h default-window count reported as "7 days" |

## Phasing

**Phase 1a (MVP):**
- single-query DQL recipes
- built-in plus user layers, domain-prefixed names
- `run`, `get/describe/verify recipe`
- `--from/--to` on recipes and `query`, `context.window`
- typed params, including `pattern`
- `emptyMeans`, plus `next` bound from params
- segment flags passed through from `query`
- ~12 built-in recipes
- the skill pointer

**Phase 1b (the worked examples need these):**
- scope dimensions (`_scopes.yaml`, `.scope.stage`/`.scope.expr`,
  `context.scope`)
- timeframe forms: `none`, `min`, `fixed`, `inline` with `align`
- `next.when`
- fragments
- `segments: off`
- `get recipes --domain/--search` and the domain index in the catalog
- the generated index for lazy loading

**Phase 2, gated on the evaluation below:**
- `entity` param type: name-or-ID resolution with ambiguity errors, plus
  per-signal scoping, the "service logs are on pods" hop that dynatui, correlate
  and the prototype each solved separately
- `next` bound from result rows
- inventory-aware listing: cached verdicts, the structural pass on a cold
  cache, `--no-inventory` (§6)
- the environment layer: app-shipped recipe bundles from the Document store,
  including their capability definitions (§13)
- declared sources, sync, lock and store (§14), and `DTCTL_RECIPE_PATH`
- context default segments, with `--no-segments`

### Prototype status

The prototype on this branch implements phases 1a and 1b, plus the parts of
phase 2 that the progressive-disclosure and distribution questions depend on:
`next` bound from result rows, inventory-aware listing, and declared sources
with sync and pinning (§14), which carry the environment layer (app bundles)
and the org layer. It does not implement the `entity` param
type, context default segments, `--no-segments` or the generated index (the
built-in set is small enough to parse on every `run`). Every command is
`experimental` (since 0.42.0). User guide: [docs/RECIPES.md](../RECIPES.md).

Where the prototype differs from the text above:

- **Bundle documents** use the existing `ai-agent-resource` type with names
  `recipes/<name>.yaml`, not a new `recipe-bundle` type (§13, question 10).
- **Remote content is synced, not discovered** (§14). The first prototype
  re-listed app bundles hourly on every run; that is replaced by declared
  sources, `dtctl recipes sync` and a lock. Project-local sources moved from
  phase 3 into this model, behind the sync-as-opt-in trust rule.
- **The catalog** lists `run` but no recipes, at any level (§4).
- **Follow-up commands** are emitted as `--name=value` words (`--segment=`,
  not `-S`), with the positional slot used only for a value that cannot
  parse as a flag. A value taken from a result row is data, and must not
  become a flag of the suggested command.
- **Built-in content**: 45 recipes across 14 domains, each
  rendered and run against several live environments before it was committed.
  `recipes/testdata/golden/rendered/` pins every recipe's rendered DQL.

**Phase 3:**
- `steps:` (named DQL steps referencing earlier steps' results; parallel when
  independent; only "skip if empty" as control flow)
- read-only API steps
- multiple output sections

## Evaluation

The prototype's harness (headless agents, logging dtctl wrapper, ground truth
measured per batch, holdout tasks, Wilson/bootstrap/McNemar statistics) is
reused. Phase 2 is gated on a three-arm comparison over the same task set,
`-n 3` trials, and at least two environments:

| Arm | Binary | Recipe knowledge |
|---|---|---|
| head | phase-1 binary, recipe layer disabled | none (ergonomics only) |
| markdown | phase-1 binary, recipe layer disabled | the same recipes rendered into a skill reference file |
| cli | phase-1 binary | `get recipes` / `run` |

- If **markdown ≈ cli**, recipes are content, not a CLI feature: invest in the
  content and its distribution into skills, and stop at phase 1.
- If **cli > markdown**, phase 2 proceeds.

Metrics: correctness (silent-wrong separately), calls, errored and empty calls,
tokens in/out, cost, wall time, scanned bytes.

## Rejected alternatives

| Alternative | Why not |
|---|---|
| Per-environment recipe books generated by discovery, with run stamps (the prototype) | Most of the complexity, little of the measured value; capabilities now come from `dtctl inventory` |
| `dtctl query --recipe <name> --set k=v` | Params can't be typed flags; no per-recipe `--help`/completion; recipes invisible in the catalog |
| Recipes as top-level verbs (`dtctl services-failures`) | Pollutes the verb namespace; collides with plugin dispatch; blurs built-in vs content |
| Recipes as `get <name>` resources | Mixes content into the resource model and the stability manifest |
| Go-defined catalog (dynatui `catalog.Spec`) | Couples content changes to code changes, the opposite of the goal |
| Raw template substitution (today's `--set`) | DQL injection by accident; silent empty renders on typos |
| Recipe content only in skills | Skills are skipped in about half of agent sessions; still an open comparison, which the evaluation settles |
| Separate recipe repository from day one | Decided against for now: release coupling is acceptable while the set is small, and user overrides cover urgent fixes. App bundles (§13) give independent release where it matters, without a second dtctl-owned repository |
| One document per recipe in the Document store | Hundreds of downloads on a cold cache, paginated listing, and no atomic update of an app's set |
| Loading user-uploaded bundle documents | Anyone with document write access could inject agent prompts into every dtctl user of the environment; the org layer covers team content |
| Loading every installed app's bundles automatically, refreshed hourly (the first prototype) | Recipes changed under running agents without anyone choosing it, and trust was implicit; declared, synced and pinned sources (§14) keep discovery as a hint |
| A git binary for git sources | A subprocess needs a capability gate and a git install; one ref lookup and one tarball over HTTPS cover pinning |
| Per-recipe version pins (`run x@2`) | Bookkeeping per recipe for a case that copying the recipe into the user directory already covers; pins are per source |
| Running full inventory discovery on `get recipes` | Seconds to minutes of latency on every listing; the cache plus the structural pass bound it |
| Tags as the navigation axis | Free-form tags from many owners fragment (`k8s`, `kubernetes`); domains are curated, tags feed search |

## Open questions

1. Should the `investigate` profile preset include `run` (and `get recipes`,
   `describe recipe`)?
2. ~~Should project-local recipes ever be trusted without an explicit
   opt-in?~~ Answered: no. A project's sources apply only after
   `dtctl recipes sync` ran against its current content (§14).
3. Recipe namespacing: is flat naming with whole-recipe override enough, or do
   team layers need prefixes (`team/name`)?
4. Should `get recipes` in agent mode include each recipe's params inline, saving
   the `describe` call at the price of a longer index?
5. Where should the eval harness live: in this repository (`test/evals/`, data
   outside the repo) or alongside the prototype?
6. Should a context carry default scope dimensions (`--namespace payments` on
   every call), or are default segments (§10) enough? Segments already work
   across all data objects and in the UI. A second mechanism would need a
   strong reason.
7. Should the `dynatrace-for-ai` skills reference recipes instead of carrying
   their own DQL, making the recipe book the single source? This depends on the
   evaluation, and on whether the skills must work without dtctl.
8. Who owns a domain? `CODEOWNERS` per `recipes/<domain>/` assumes owners
   exist for each of the ~15 domains.
9. Should cost attribution fields (`dt.cost.costcenter`, `dt.cost.product`)
   become scope dimensions once their type is consistent across billing events?
10. ~~What document `type` do app-shipped skills use?~~ Answered: skills are
    `ai-agent-resource` documents named `/skills/<skill>/SKILL.md`. Bundles
    use the same type, named `recipes/<name>.yaml`. Still open: whether the
    platform should reserve the `recipes/` prefix for this purpose.
11. ~~Is `originAppId` a sufficient trust anchor, or does dtctl need an
    allowlist of app IDs?~~ Answered: both apply. Declared `app` sources are
    the allowlist, and `originAppId` still decides which documents count (§13,
    §14). Still open: per-context sources files, if one machine's contexts
    need different apps.
12. Should `requires` support per-value requirements for family recipes
    (`--provider aws` needs `aws`), which would hide enum values instead of
    whole recipes?
13. Cache lifetimes: 1h for the availability hint's bundle list and 24h for
    inventory verdicts are guesses. Should they be context settings?
14. Should `dtctl recipes sync` also offer a scheduled mode (sync when the
    lock is older than N days, `outdated` as a notice), or is that the job of
    whoever owns the project lock?
