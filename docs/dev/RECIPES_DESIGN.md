# Recipes Design

**Status:** Ships as a `development` feature (opt in with
`DTCTL_DEVELOPMENT=recipes` or `dtctl config set development.recipes on`).
**Created:** 2026-10-04
**User guide:** [docs/RECIPES.md](../RECIPES.md) · **Authoring guide:** [recipes/README.md](../../recipes/README.md)

## Problem

An agent asked a common question ("which pods restart?", "what problems are
open?") spends most of its calls, and most of its wrong answers, on exploration
before the query: guessing data objects, fields, entity types and the window.
Grail answers a wrong question with `[]` and exit 0, so a bad guess looks like
a finding. A recipe is a named, parameterized, verified DQL query that encodes
that knowledge once, says what its rows and an empty result mean, and hands
back its DQL so the agent can adapt it into `dtctl query`. Recipes do not
replace DQL; `dtctl query` stays the escape hatch.

## Recipe format

One recipe per file, `recipes/<domain>/<name>.yaml`:
`apiVersion: dtctl.dev/v1alpha1`, `kind: Recipe`.

| Field | Rule |
|---|---|
| `metadata.name` | `<domain>-<name>`, kebab-case; domain registered in `recipes/_domains.yaml`; becomes the `run` subcommand |
| `metadata.version`, `tags` | content version (bump when output shape changes); free-form search terms |
| `spec.summary` | required, one line; phrased as the question it answers (it is what `--search` matches) |
| `spec.params.<snake_name>` | `type`: string, int, bool, enum (`values`, optional `render: identifier`), list; `required`, `default`, `positional` (≤1), `min`/`max`, `pattern` (regex for strings); flag spells `_` as `-` |
| `spec.scope` | dimensions from `recipes/_scopes.yaml`: cluster, namespace, host_group, aws_account, azure_subscription, gcp_project, tag (`key=value` → `primary_tags.<key>`) |
| `spec.timeframe` | duration (default window), `none` (state query: no window, no flags), or `{default, min, max, fixed, inline, align}` |
| `spec.dql` | Go `text/template` over params, `.scope.stage`/`.scope.expr`, `.window.*` (inline windows only); fragments from `recipes/_fragments/*.tmpl` via `{{template}}` |
| `spec.means`, `spec.emptyMeans` | **required**: what a row is (units, decision rule, what is not shown); what an empty result means |
| `spec.next[]` | `recipe`, `with` (bind from params), `bind` (from result fields), `when: empty\|nonempty`, `follow: true` (the edge `--follow` runs) |
| `spec.checks[]` | `match`/`unless` regexes over a user's own DQL, `warn` text, `example` the check must fire on |
| `spec.requires` | capability names from `dtctl inventory`; all must hold |
| `spec.segments`, `spec.empty` | `off` for data segments must not narrow; `zero-row` treats one all-zero aggregate row as empty |
| `spec.deprecated` | `message`, `replacedBy`; renames/removals survive ≥2 minor releases |

`kind: RecipeBundle` files (recipes plus domains, capabilities, fragments)
are gated by `metadata.minDtctlVersion`. Governors: new keywords default to
no; variants are two recipes; DQL ≤~20 lines; templates only test presence and
compare enums; shared prefixes are fragments of whole pipeline stages.

## Commands

| Command | Does |
|---|---|
| `dtctl run <recipe> [positional] [--<param> …] [--from/--to] [--dry-run] [--follow]` | render and execute; `--dry-run` prints DQL to stdout, window to stderr |
| `dtctl get recipes [--domain d] [--search words] [--tag t] [--all] [--no-inventory] [--inventory-budget s]` | index; agent mode returns a domain index unless filtered |
| `dtctl describe recipe <name>` | params, scope, timeframe, `means`, `emptyMeans`, `next`, rendered DQL, source, shadows |
| `dtctl verify recipe [<name> \| -f file \| --all] [--offline]` | schema + lints, then the DQL parser check unless `--offline` |

All four are `development` tier (`addDevelopmentCommand`, feature
`recipes`); `dtctl query --from/--to` are `experimental` flags on a stable
command. Each recipe is a generated cobra leaf under `run` (params are typed
flags, `--help` is its documentation), excluded from `docs/STABILITY.md`. The
`dtctl commands` catalog lists `run` only, so it stays environment-independent.

## Loading

- **Built-in:** `recipes/` embedded with `go:embed`.
- **User:** `~/.config/dtctl/recipes/**/*.yaml` (XDG). Shadows a built-in of
  the same name, whole recipe, no field merge; `describe recipe` and
  `context.recipe.source` show which answered. A user layer may add domains,
  scopes and fragments but not redefine them.
- **Engine/sessions:** built-in only (the user directory is host state);
  `verify recipe -f` reads through `readFileFlag` (vfs).
- The loader runs only for recipe surfaces; invalid user files are skipped
  with a warning, so they never break unrelated commands.

## Execution and agent mode

- `run` renders, then calls the **same execution path as `dtctl query`**:
  limits, spill, empty-result diagnosis, window advice, printers. Human output
  equals `query` output for the rendered DQL.
- `--from/--to` accept a duration ago or RFC3339, resolve client-side to
  absolute timestamps, and are sent as `defaultTimeframeStart/End`. Recipes
  leave `from:`/`to:` out of DQL (lint) except `inline` windows.
- Envelope context: `recipe` (name, version, source), `query` (rendered DQL),
  `means`, `window` (effective, from response metadata), `scope` (dimensions
  and segments that narrowed the result), `inventory` (cache age, hidden
  count), `follow_up` (≤20 rows of the `--follow` edge). An empty result sets
  `empty_reason.code = recipe_empty_means`; dtctl's own concrete diagnosis
  takes precedence. `next` edges become ready-to-run `suggestions`.
- **Push, not just pull.** `dtctl query` in agent mode names a matching recipe
  as a runnable command (binding literals the query already contains): only a
  strong match after success, a weaker one after an error or empty result, ≤2.
  Fired `checks` add a warning naming the recipe that does it right. An
  unknown top-level noun (e.g. `problems`) points at recipes. Pull-only
  discovery went unused: agents write DQL as soon as they have a question.
- **Inventory filter:** `get recipes` hides a recipe only when a cached
  `dtctl inventory` verdict says a required capability is `absent`; a cold
  cache gets a structural check bounded by `--inventory-budget`. A hidden
  recipe still runs, with a warning.

## Analyzer recipes (designed, not built)

A Davis analyzer is fed by a DQL query, so a recipe can carry it: the recipe
still owns the hard part (metric, filter, window), and the analyzer replaces
`query` as the execution step. Live check on 2026-10-06, one environment:

| Analyzer | Input | Raw result | Time |
|---|---|---|---|
| `dt.statistics.GenericForecastAnalyzer` | `timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}`, 24 steps | 12 KB, 1 series | 0.8 s |
| same | disk used %, 5 disks, 14d at 1h, 168 steps | 105 KB; 3 of 5 series `FAILED` (sparse recent history) | 0.9 s |
| `dt.loganalysis.LogPatternExtractor` | 5,000 `ERROR` lines, 24h | 19 KB, 41 DPL patterns with counts and a sample each | 2.3 s |

Latency is fine. The problem is the result: `dtctl exec analyzer` prints raw
JSON (arrays of points, the echoed input, per-series system logs) with no table
and no agent context even under `-A`. Two layers fix that.

**Layer 1, generic, in `exec analyzer` itself (agent mode):** analyzer results
embed ordinary DQL result objects (`{metadata, records, types}`) — the echoed
`analyzedTimeSeriesQuery`, a forecast's `timeSeriesDataWithPredictions`, a
threshold suggestion's `resultTimeseries`. So `query`'s existing agent-mode
transforms apply unchanged to each embedded object: `--series` summary
(`n/min/max/avg/last`), `--precision` rounding, `--compact` (nulls, constants).
Plus: drop the echoed `input` and the `types` schema blocks, and say so when
`output` is empty (an anomaly detector that found nothing returns
`SUCCESSFUL` with no output, which an agent reads as "no data"). Measured on
12 analyzers, compact JSON bytes:

| Result | raw | −input, types, nulls | + series summary |
|---|---|---|---|
| forecast, 1 series | 6 KB | 4 KB | 1 KB |
| forecast, 5 disks × 168 steps | 47 KB | 39 KB | 5 KB |
| threshold suggestion | 6 KB | 5 KB | <1 KB |
| static-threshold anomalies | 4 KB | 3 KB | 1 KB |
| log patterns, 41 patterns | 16 KB | 16 KB | 16 KB (text; lever is `numberOfExamples` and sample length) |
| change point, characteristics | <1 KB | — | already rows |

Layer 1 benefits every analyzer, recipe or not, and is an agent-mode output
change (no deprecation). **Layer 2, per recipe:** a jq fragment where a
summary is the wrong shape — a forecast wants the value *at the horizon*, not
min/max; patterns want share-of-total and one truncated sample.

```yaml
spec:
  dql: timeseries avg(dt.host.disk.used.percent), by:{dt.entity.host, dt.entity.disk}, interval:1h
  analyzer:
    name: dt.statistics.GenericForecastAnalyzer
    input: timeSeriesData              # the input field that takes the rendered DQL
    params: {forecastHorizon: "{{.horizon_hours}}", coverageProbability: 0.9}
    jq: "{{template \"jq/forecast\"}}" # output shaper, a shared fragment
  means: One row per disk: last observed, forecast at the horizon with its band, and quality. FAILED means too little recent data, not a healthy disk.
```

- `run` renders `dql` and the templated `params`, posts `{<input>: dql, …params}`
  through the `exec analyzer` path (`ExecuteAndWait`, `OperationRead`), and
  keeps `--dry-run`, `--from/--to` (as `generalParameters.timeframe`), scopes
  and the envelope (`context.recipe`, `context.query`, `context.means`).
- `jq` (layer 2) shapes the analyzer's `output` into rows where the generic
  summary is not the right view. jq, not Go: gojq already backs `--jq`, agents know it, and
  a shaper is then content a recipe author can change. The programs live in
  `recipes/_fragments/jq/` and `go test ./recipes/` runs each against a
  recorded result. A user's own `--jq` composes after; `-o json` returns the
  full result.
  - `jq/forecast`: per output series — the `by:` dimensions, last observed,
    `point`/`lower`/`upper` at the horizon end, `forecastQualityAssessment`,
    `analysisStatus` with the first `system.logs` message when not `OK`, so
    a FAILED series is a visible row, not a buried log line.
  - `jq/patterns`: per pattern — `numberOfMatches`, share of all matches,
    `patternExpression`, one `sampleMatches` entry; sorted by matches.
  - No `jq`: the raw `output` as `exec analyzer` prints it today.
- DQL recipes get no transform: the query is the transform (`fields`,
  `summarize`), and agent mode already bounds the envelope (`has_more`).
- Horizon is a recipe param in the recipe's own interval (`horizon_hours` with
  `interval:1h`), since the analyzer counts steps, not time.
- Candidates: `hosts-disk-forecast` (replaces the pure-DQL attempt that was
  dropped) and `logs-error-patterns` gaining a DPL variant. Two, until evals
  show agents reach for them.

## Decisions

| Decision | Rationale |
|---|---|
| YAML content, not Go | DQL evolves faster than dtctl; a recipe fix is a content diff, not a handler change |
| Go templates, not a DSL | params render as escaped DQL literals via typed values; `missingkey=error`; no raw-DQL param type (that question belongs in `dtctl query`) |
| Fixed scope dimension set | only primary fields/tags mean the same on every signal; anything else is a param. Always `in()`, since some are arrays on problems and `==` never matches |
| No `scanLimitGBytes` in recipe DQL | a capped scan returns plausible, silently low numbers; `run` reports a stopped scan as a partial-result warning (`recipe_partial` when empty) |
| `run` reuses the query path | a recipe is a pre-filled `dtctl query`, not a second engine; inherits every limit and diagnosis |
| `next` edges, not chained execution | no workflow engine; pre-bound multi-hop drilldowns showed no gain; `--follow` covers the one-turn case |
| Window reported, not assumed | a silently ignored relative window mislabelled answers ("7 days" that was 2h) |
| `emptyMeans` mandatory | an empty result that explains itself is the most context-saving thing a recipe returns |
| Only `absent` hides | inventory evidence can be wrong; hiding on `unknown` would lose working recipes |
| `development` tier | format and envelope still moving; no promise of any kind until evidence settles them |
| Read-only DQL, no `CheckSafety` | same as `dtctl query`; `run` maps to read access |

Rejected: a recipe option on `query` (params cannot be typed flags); top-level
verbs (collide with plugin dispatch); `get` resources (content in the resource
model); a Go catalog (couples content to code); per-environment generated books
(prototype complexity, little value); tags as navigation (domains are curated).

## DQL traps (lints in `pkg/recipes/dqllint.go`, run by `go test ./recipes/`)

- `multi-key-timeseries`: several metric keys without `union: true` keep only series every key reports.
- `sampling-source`: `samplingRatio:` works only on `fetch logs` and `fetch spans` (events ignore it, problems reject it).
- `sampling-unscaled`: sampled counts must be scaled by `dt.system.sampling_ratio`.
- `limit-before-aggregate`: `| limit` before an aggregate is not a sample.
- `coalesce-filter`: `coalesce(a, b) == x` defeats the field index; write `a == x or b == x`.
- `case-folded-filter`: `lower(field) == x` reads every record; use `caseSensitive: false`.
- `unaliased-aggregate`: an unnamed aggregate's column is the expression, which adapted queries must backquote.
- `interval-equals-window`: the aligned grid straddles two buckets and reads up to 2x.
- `filter-beyond-window`: `filter timestamp > now() - 24h` cannot reach past a 2h fetch window.
- Not linted: `smartscapeNodes` takes no `from:`; `countDistinct` is an estimate; `takeFirst`/`takeLast` are unordered; casts fail silently; units differ (span ns, service response time µs, OTel s, k8s CPU millicores).

## Testing

`go test ./recipes/` lints every built-in recipe offline (schema, template
references, rendering, `next` targets, `requires` names, checks firing on their
example, trap lints) and pins rendered DQL in `recipes/testdata/golden/rendered/`.
`dtctl verify recipe --all` adds the live DQL parser check.

## Evidence

A/B evals on Haiku, 27–30 cases × 3 repetitions, judged against ground truth.
Recipes plus agent-output repairs: +1.16 on a 0–3 scale vs main at −13% cost,
roughly half from recipes. Later iterations (v7) added no further measurable
gain; SLO cases went from 1/2/1 in 21 calls to 3/3/3 in 5 after an SLO note.

## Deferred (not implemented)

- Remote sourcing: git, archive and app-bundle sources, `recipes sync`, locks, content store, `DTCTL_RECIPE_PATH` org layer.
- `followIf` (conditional follow-ups).
- Recipe skills (skills generated from or pointing into the book).
- Field and metric lookup recipes.
- Analyzer recipes (designed above; needs forecast and log-pattern eval cases before it is built).
