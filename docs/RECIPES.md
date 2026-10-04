# Recipes

> **Experimental.** `run`, `get recipes`, `describe recipe` and `verify recipe`
> may change or be removed in any release (see [STABILITY.md](STABILITY.md)).
> Design and rationale: [dev/RECIPES_DESIGN.md](dev/RECIPES_DESIGN.md).

A recipe is a named, parameterized DQL query that answers one common question,
such as which problems are open, which pods restarted, or how a service's RED
metrics look. You run it as `dtctl run <recipe>` without writing DQL. Each
recipe carries the query, typed parameters, a default time window, the scope
filters it supports, and a note on how to read the result, including what an
empty result means.

DQL is still the escape hatch. Every recipe shows the exact query it runs, so
when a recipe is close but not quite right, you can copy that query and adapt
it with `dtctl query`.

## Find a recipe

```bash
dtctl get recipes                         # every recipe this environment can answer
dtctl get recipes --domain k8s            # one domain
dtctl get recipes --search "slow endpoints"
dtctl get recipes --tag rca -o wide       # by tag; wide adds tags and requirements
dtctl describe recipe problems-active     # params, scope, window, how to read it, the DQL
```

`get recipes` hides recipes whose data the environment lacks: Kubernetes
recipes on an environment without Kubernetes, GenAI recipes without LLM spans,
and so on. The decision uses the capability verdicts of the last
`dtctl inventory` run, which are cached for a day. Without cached verdicts, a
quick structural check runs first, bounded by `--inventory-budget` (seconds).
Only a confident *absent* verdict hides a recipe. A recipe whose requirement is
unknown stays listed. `--all` shows everything, and `--no-inventory` skips the
filter.

In agent mode, an unfiltered `get recipes` returns a **domain index** with one
line per domain and its recipe count, not every recipe. `--domain`, `--search`
or `--tag` then list the recipes. This keeps discovery cheap even when apps
ship hundreds of recipes.

## Run a recipe

```bash
dtctl run problems-active
dtctl run problems-get P-12345                       # a positional argument
dtctl run k8s-pod-restarts --namespace=checkout --from=6h
dtctl run services-red --cluster=prod-eu --cluster=prod-us
dtctl run problems-active --tag=team=payments        # primary Grail tags
dtctl run problems-active -S my-segment              # filter segments
dtctl run problems-active --dry-run                  # print the DQL, run nothing
```

- **Params** are the recipe's own flags, listed in `dtctl run <recipe> --help`.
  Values are type-checked (enum, int ranges, patterns) and rendered as escaped
  DQL literals, never spliced in as raw text.
- **Window**: `--from` and `--to` accept a duration ago (`2h`, `7d`) or an
  RFC3339 timestamp. The default comes from the recipe. A *state* recipe (for
  example, a list of clusters) takes no window. A recipe with a fixed or
  minimum window refuses one that would make its answer wrong.
- **Scope**: `--cluster`, `--namespace`, `--host-group`, `--tag` and the other
  dimensions a recipe declares narrow it to part of the environment. Repeat a
  flag to OR-combine its values. Filter segments (`-S`) work on every recipe,
  just as they do for `dtctl query`.
- **Output**: a recipe runs through the same path as `dtctl query`, so `-o`,
  `--jq`, `--spill`, `--max-result-records` and every output format behave as
  they do there.

### Agent mode

The envelope adds what an agent needs to trust and continue the result:

| Field | Meaning |
|---|---|
| `context.recipe` | name, version and source layer of the recipe that ran |
| `context.query` | the rendered DQL, to adapt with `dtctl query` |
| `context.window` | the resolved `from`/`to` |
| `context.scope` | the scope filters applied |
| `context.empty_reason` | on an empty result, code `recipe_empty_means` and what the emptiness means; `recipe_partial` when a scan or record limit cut the result, so an absent row may exist |
| `context.warnings` | a partial non-empty result: counts are lower bounds, with the flag that lifts the limit |
| `context.has_more` | the result filled the recipe's final `limit`: it is the top N, not a total |
| `context.suggestions` | follow-up recipes, with arguments bound from the result (for example `dtctl run problems-get P-12345`) |

`dtctl query` points the other way. In agent mode, when a hand-written query
reads the same data as a recipe (the same source or metric keys, and fields
specific to that recipe), its `context.suggestions` names the recipe. On a
result that worked it does so only for a strong match. On an empty, partial or
failed result, it appears after any diagnosis of the query itself.

## Where recipes come from

Recipes load from four layers. On a name clash, a stronger layer replaces a
weaker one, and `describe recipe` shows which source won and what it
shadows.

| Layer | Location | Updates when |
|---|---|---|
| user | `~/.config/dtctl/recipes/**.yaml` | you edit it (read live) |
| org | declared `git`, `archive` and `dir` sources, then `DTCTL_RECIPE_PATH` directories | you run `dtctl recipes sync` (`dir` and `DTCTL_RECIPE_PATH` are read live) |
| environment | declared `app` sources: bundles an installed app ships | you run `dtctl recipes sync` |
| builtin | compiled into dtctl ([`recipes/`](../recipes/)) | you upgrade dtctl |

**Nothing changes under you.** Remote recipes are fetched only by
`dtctl recipes sync`, which records what it fetched in a lock. `dtctl run`
reads the lock and a local store and never the network, so the recipes an
agent sees change only when someone syncs. The agent envelope names the
version that answered (`context.recipe.source: "team@3f2a1c9e0b7d"`).

### Declare, sync, pin

```bash
# A GitHub repository, pinned to the commit the ref names now
dtctl recipes add team --git github.com/<owner>/<repo> --ref main --path recipes

# The recipes an installed app ships to the current environment
dtctl recipes add genai --app <app-id>

# A tarball anywhere, pinned by its sha256
dtctl recipes add shared --archive https://example.invalid/recipes.tar.gz

# A local directory, read live while you write recipes
dtctl recipes add drafts --dir ./my-recipes

dtctl get recipe-sources              # every source, its pin, last sync, status
dtctl recipes outdated                # which pins moved upstream (changes nothing)
dtctl recipes sync --update team      # move one pin; prints which recipes changed
dtctl recipes sync                    # install exactly what the lock names
dtctl recipes remove drafts
```

`add` writes `~/.config/dtctl/recipe-sources.yaml` and syncs. The lock is
`recipe-sources.lock` next to it, and the fetched content lives in
`~/.local/share/dtctl/recipes/store/`, keyed by content digest.

- **git** sources are GitHub repositories, fetched over HTTPS (no git binary).
  Set `GITHUB_TOKEN` for a private repository. Other hosts: use an `archive`
  source.
- **app** sources load only documents of type `ai-agent-resource` named
  `recipes/<name>.yaml` that the platform deployed with the app (they carry an
  `originAppId`); a hand-uploaded document never loads. Pins are per
  environment, so syncing against one context never moves what another runs.
  `get recipes` mentions apps on the environment that ship recipes you have
  not enabled.
- `builtin: false` in a sources file turns the built-in recipes off (their
  domains and fragments stay available to other sources).

### Projects

A project can declare its own sources in `.dtctl/recipes.yaml`
(`dtctl recipes add <name> ... --project`). Commit it with
`.dtctl/recipes.lock`, and everyone who runs `dtctl recipes sync` in the
checkout gets byte-identical recipes. dtctl finds the file by walking up from
the working directory.

A project's sources apply only after you ran `dtctl recipes sync` in it, and
again after its sources file or lock changes (a `git pull`, for example).
Until then `get recipe-sources` shows them as `untrusted`. Recipe text is
prompt input for agents, and a cloned repository must not be able to plant it
just by being your working directory.

## Write your own

Put a file in `~/.config/dtctl/recipes/`:

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata:
  name: payments-slow-checkouts      # <domain>-<name>; the domain must exist
  version: 1
  tags: [payments, latency]
spec:
  summary: Checkout requests slower than a threshold
  timeframe: 1h
  params:
    threshold_ms: {type: int, default: 2000, min: 1}
  dql: |
    fetch spans
    | filter endpoint.name == "/checkout" and duration > {{.threshold_ms}} * 1ms
    | summarize {count = count(), p95 = percentile(duration, 95)}, by: {service.name}
  means: Checkout spans over the threshold, per service.
  emptyMeans: No checkout span exceeded the threshold in the window.
```

A new domain goes in a `_domains.yaml` next to it. Then check the recipe and
run it:

```bash
dtctl verify recipe -f ~/.config/dtctl/recipes/payments-slow-checkouts.yaml
dtctl run payments-slow-checkouts --threshold-ms=5000 --dry-run
```

`verify recipe` lints the file (schema, params, template references, scope and
window rules, follow-up targets). It then sends the DQL, rendered with defaults
and placeholders, to the environment's DQL parser. `--offline` skips the
parser step. `verify recipe --all` checks every loaded recipe.

The full format (params, scope dimensions, inline windows, fragments,
`next` edges, deprecation, bundles) is documented in
[`recipes/README.md`](../recipes/README.md), the authoring guide for
built-in recipes.

### Cost and correctness cheat-sheet

A recipe runs unattended and its result is read as fact, so a query that is
merely slow or slightly off in an editor is a wrong answer here.

**Scan cost, the levers in order**

1. **The window.** Default to the shortest window that answers the question
   (problems: 24h, not 7d) and cap scan-heavy recipes with `timeframe.max`.
2. **Buckets.** `fetch logs, bucket:{"<name>"}` reads one bucket instead of
   the table (`dtctl run meta-buckets --table logs` lists them). Recipes
   cannot hard-code bucket names, which differ per environment.
3. **Filter early on plain fields.** `==` and `in()` on a field use its
   index; `lower(field)`, `coalesce(a, b) == x` and full-text `contains` on
   `content` read every record. Write `a == x or b == x`, and
   `contains(f, "x", caseSensitive: false)` for case-insensitive matching.
4. **Subqueries for cross-table filters.** `filter x in [fetch …]` reads only
   the column it filters; a `lookup` or `join` reads every record in full.
5. **Sampling** (`samplingRatio:`) works on `fetch logs` and `fetch spans`
   only. Scale every count back with `sum(coalesce(dt.system.sampling_ratio,
   1))`. Fine for presence, ratios and rates; wrong for maxima, tail
   percentiles and rankings by them.
6. **Never `| limit` before an aggregate**: it is not a sample.
7. **No `scanLimitGBytes` in recipe DQL.** A capped scan returns plausible,
   silently low numbers; dtctl reports a stopped scan as `context.partial`.

**Correctness traps**

The ones a lint can see fail `go test ./recipes/` and `dtctl verify recipe`
(the lint table is in `recipes/README.md`); the rest are on you.

- `interval:` equal to the window reads up to twice the data (the grid is
  aligned); use `1m`/`5m` and sum, or `summarize`.
- A `timeseries` over several metric keys keeps only the series every key
  reports; add `union: true`.
- `smartscapeNodes` takes no `from:`: a window degrades fields and adds
  dead nodes.
- `countDistinct` is an estimate; `countDistinctExact` is exact up to 1M
  values. Count events by `event.id`.
- `takeFirst`/`takeLast` have no defined order; use
  `takeMax(record(timestamp, x))` for the latest value.
- Casts fail silently: `toString(trace.id)` against a log's `trace_id`,
  `toSmartscapeId("…")` for Smartscape ID fields, and classic IDs differ from
  Smartscape IDs for most entity types.
- Units: span `duration` is nanoseconds, `dt.service.request.response_time`
  microseconds, OTel `http.server.request.duration` seconds, Kubernetes CPU
  millicores.

**What `means` must say**: what a row is and its units, a decision rule that
tells the finding from the background, and a `Not shown:` sentence naming what
the recipe leaves out. The full list of measured traps is in the
[authoring guide](../recipes/README.md#dql-traps-worth-knowing).

## Embedding

In a session-backed invocation (`pkg/engine`, `dtctl serve`), recipes load from
the built-in set plus the app bundles the request names in
`Request.RecipeApps` (`<app-id>`, or `<app-id>@<version>` to pin a bundle
document's version). Sources files, locks, the store and the user directory
are host state that a request never reads, and `dtctl recipes` is blocked in
a service. Bundles are cached in memory per environment and principal.
