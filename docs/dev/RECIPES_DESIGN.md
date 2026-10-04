# Recipes Design

**Status:** Proposed
**Created:** 2026-10-04
**Author:** dtctl team

## Overview

A **recipe** is a named, parameterized DQL query defined in a YAML file and
exposed as a dtctl command:

```bash
dtctl get recipes                                  # what is in the book
dtctl run service-failures --service checkout --from 6h
dtctl run service-failures --service checkout --dry-run   # show the DQL, don't run it
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

One recipe per file, `recipes/<name>.yaml`.

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata:
  name: service-failures          # kebab-case, unique; the `run` subcommand name
  version: 1                      # content version; bump when the output shape changes
  tags: [services, errors, triage]
spec:
  summary: Failed requests of a service by endpoint and status
  description: |                  # optional; shown by describe/--help
    Groups failed root requests of one service by endpoint and HTTP status,
    most frequent first.
  requires: [spans]               # optional; capability names from `dtctl inventory`
  params:
    service:
      type: string
      required: true
      positional: true            # at most one; `dtctl run service-failures checkout`
      description: Service name as shown in Smartscape
    top:
      type: int
      default: 10
      min: 1
      max: 100
  timeframe:
    default: 2h                   # or `none` for state queries (smartscapeNodes)
  dql: |                          # illustrative; shipped bodies are verified (§9)
    fetch spans
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
    with `dtctl run services --name <part>`.
  next:
    - recipe: service-logs
      with: { service: "{{.service}}" }
  deprecated:                     # optional
    message: ...
    replacedBy: ...
```

Schema rules, enforced by the loader and by `dtctl verify recipe`:

| Field | Rule |
|---|---|
| `metadata.name` | `^[a-z][a-z0-9-]{1,48}$`; unique within a layer |
| `metadata.version` | positive integer; reported in the envelope |
| `spec.summary` | required, single line, ≤ 100 chars (it is the catalog/`--help` line) |
| `spec.dql` | required; a Go template over `params` (see §2) |
| `spec.means`, `spec.emptyMeans` | **required** |
| `spec.timeframe` | required: a default window, or `none` |
| `spec.params.*` | name `^[a-z][a-z0-9-]*$`, not a reserved framework flag (§4) |
| `spec.requires` | names defined by `dtctl inventory` (built-in or user definitions) |
| `spec.next[].recipe` | must resolve in the merged book (warning for user layers) |

**Complexity governors**, carried over because the prototype needed them:

- Every new keyword is "default no".
- A recipe that needs `variants:` is two recipes.
- DQL bodies stay short enough to read in `describe` (≈20 lines).
- Templates may test whether a param is present. They may not contain logic.

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
  marker and description, so `dtctl run service-failures --help` is the recipe's
  documentation.
- The `Short` line is the recipe's `summary`.
- `run` is a verb in the catalog. Its recipes appear as its resources in the
  minimal `dtctl commands` output (names only), so the agent bootstrap path lists
  them for free.

**Framework flags**, present on every recipe and reserved as param names:

- the timeframe: `--from`, `--to`
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
    "resource": "service-failures",
    "recipe": { "name": "service-failures", "version": 1, "source": "builtin" },
    "query": "fetch spans\n| filter dt.service.name == \"checkout\"\n...",
    "window": { "from": "2026-10-04T08:00:00Z", "to": "2026-10-04T10:00:00Z" },
    "suggestions": [
      "dtctl run service-logs --service checkout --from 2h",
      "adapt: dtctl query '<context.query>' --from 2h"
    ]
  }
}
```

- `context.query` is the rendered DQL. This is the escape hatch, and it is what
  the evals showed agents actually use.
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

### 6. Discovery for agents

The bootstrap path agents already take is the place to advertise recipes:

1. `dtctl commands` (minimal) lists `run` and its recipe names.
2. `dtctl get recipes` gives one line per recipe: name, summary and required
   params. This is the "briefing" the prototype found agents always read.
   - Agent mode returns a compact list.
   - `--tag` filters.
   - Deprecated recipes are hidden unless `--all` is given.
3. The `dtctl` skill (`skills/dtctl/SKILL.md`) gets one rule: *before writing
   DQL, check `dtctl get recipes`. `dtctl run <name> --dry-run` shows a recipe's
   DQL as a starting point.* There is no recipe content in the skill. The prototype
   found the CLI listing beats skill files, and duplicating content would create
   another copy to keep in sync.

`spec.requires` is used in phase 2: `get recipes --check` runs the inventory's
structural probes (cheap, no data scans) and marks recipes that cannot work
here. Phase 1 only validates that the names exist, and shows them in `describe`.
Nothing is hidden on an unverified guess. A recipe marked unavailable stays
runnable, because inventory evidence can be wrong.

### 7. Where recipes live

| Layer | Location | Phase |
|---|---|---|
| Built-in | `recipes/*.yaml` at the repo root, embedded with `go:embed` (as `skills/dtctl/` is) | 1 |
| User | `$XDG_CONFIG_HOME/dtctl/recipes/*.yaml` (`~/.config/dtctl/recipes/`) | 1 |
| Team/org | directories in `DTCTL_RECIPE_PATH` (colon-separated) | 2 |
| Project | `.dtctl/recipes/`, found by searching upward like `.dtctl.yaml` | 3, DQL-only, trusted only after opt-in (open question) |
| Pulled bundle | a versioned recipe bundle released independently of dtctl | later, if content cadence demands it |

**Precedence:** user overrides org, which overrides built-in, and a whole recipe
replaces another recipe of the same name. There is no field-level merge, which
is where the prototype's complexity came from.

- An override is visible: `describe recipe` names the file and what it shadows,
  and `context.recipe.source` says `user`.
- Overriding a built-in is a feature. A team can pin a recipe to its own
  environment's field names.

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
| `recipes/` | content and `embed.go` only |
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
  - `run` in a profile's allowlist grants every recipe. `run service-failures`
    grants one; existing segment-prefix matching covers both.
  - Profiles are default-deny, so no existing profile gains recipes silently.
  - Whether the `investigate` preset should include `run` is an open question.
- **Engine / service mode.**
  - Built-in recipes are available in the engine.
  - The user directory is host state: a `Session` does not read it, the same as
    aliases.
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

## Initial recipe set

The prototype's eval suite is a good source for the first ~12 recipes: most of
them encode a trap that cost control-arm agents calls or a wrong answer. Every
one is re-verified on at least two environments before it lands.

| Recipe | Question | Why it earns a recipe |
|---|---|---|
| `error-log-sources` | top sources of ERROR logs | most common triage entry point |
| `service-latency` | services ranked by p95 | sampling hid the true slow tail and flipped a ranking in one eval |
| `service-failures` | failure signatures of one service | common triage step (a correlation-graph evidence recipe) |
| `active-problems` | open Davis problems | problem update records vs distinct current problems |
| `host-census` | hosts by OS/cloud | the `dt.entity.*` lookback view under-counted on one tenant and over-counted on another; Smartscape is live state |
| `cloud-function-census` | serverless functions per cloud | same lookback trap at fleet scale (~1.2k of ~9.3k Lambdas found) |
| `k8s-workload-status` | ready vs desired replicas | era-specific manifest keys; superseded pods linger in topology |
| `oom-killed-pods` | pods with OOM kills | truncated series undercounted (21–50 of 252) at scale |
| `rum-volume` | user events per frontend | control agents guessed 8–10 wrong data object names |
| `genai-token-usage` | tokens by model/provider | guided-by-skill query scanned ≈6 GB where the recipe scanned ≈0 |
| `security-detections` | attack detections | proving absence cheaply (≈10× less scan) |
| `compliance-findings` | posture findings | a 2h default-window count reported as "7 days" |

## Phasing

**Phase 1 (this design):**
- single-query DQL recipes
- built-in plus user layers
- `run`, `get/describe/verify recipe`
- `--from/--to` on recipes and `query`, `context.window`
- `emptyMeans`, plus `next` bound from params
- ~12 built-in recipes
- the skill pointer

**Phase 2, gated on the evaluation below:**
- `entity` param type: name-or-ID resolution with ambiguity errors, plus
  per-signal scoping, the "service logs are on pods" hop that dynatui, correlate
  and the prototype each solved separately
- `next` bound from result rows
- `get recipes --check` against inventory
- `DTCTL_RECIPE_PATH`

**Phase 3:**
- `steps:` (named DQL steps referencing earlier steps' results; parallel when
  independent; only "skip if empty" as control flow)
- read-only API steps
- multiple output sections
- project-local recipes
- an independently released recipe bundle, if the content cadence outgrows
  dtctl releases

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
| Recipes as top-level verbs (`dtctl service-failures`) | Pollutes the verb namespace; collides with plugin dispatch; blurs built-in vs content |
| Recipes as `get <name>` resources | Mixes content into the resource model and the stability manifest |
| Go-defined catalog (dynatui `catalog.Spec`) | Couples content changes to code changes, the opposite of the goal |
| Raw template substitution (today's `--set`) | DQL injection by accident; silent empty renders on typos |
| Recipe content only in skills | Skills are skipped in about half of agent sessions; still an open comparison, which the evaluation settles |
| Separate recipe repository from day one | Decided against for now: release coupling is acceptable while the set is small, and user overrides cover urgent fixes |

## Open questions

1. Should the `investigate` profile preset include `run` (and `get recipes`,
   `describe recipe`)?
2. Should project-local recipes ever be trusted without an explicit opt-in? DQL
   is read-only, but recipe text is prompt input for agents.
3. Recipe namespacing: is flat naming with whole-recipe override enough, or do
   team layers need prefixes (`team/name`)?
4. Should `get recipes` in agent mode include each recipe's params inline, saving
   the `describe` call at the price of a longer index?
5. Where should the eval harness live: in this repository (`test/evals/`, data
   outside the repo) or alongside the prototype?
