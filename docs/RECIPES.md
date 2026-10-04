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
| `context.empty_reason` | on an empty result, code `recipe_empty_means` and what the emptiness means |
| `context.suggestions` | follow-up recipes, with arguments bound from the result (for example `dtctl run problems-get P-12345`) |

## Where recipes come from

Recipes load from four layers. On a name clash, a stronger layer replaces a
weaker one. `describe recipe` shows which layer won and what it shadows.

| Layer | Location | Notes |
|---|---|---|
| user | `~/.config/dtctl/recipes/**.yaml` | yours |
| org | directories in `DTCTL_RECIPE_PATH` (`:`-separated) | a team's shared checkout |
| environment | bundles that installed Dynatrace apps ship | refreshed hourly; `get recipes --refresh` forces it |
| builtin | compiled into dtctl ([`recipes/`](../recipes/)) | verified against live environments |

**App bundles.** An app ships its recipes as a `RecipeBundle` YAML document
whose name matches `recipes/<name>.yaml`. It uses the Document store's
`ai-agent-resource` type, the same mechanism apps already use to ship agent
skills. dtctl only trusts a document that an installed app created, meaning one
with an `originAppId`. A hand-made document with the same name is ignored. A
bundle may also define inventory capabilities that its recipes require. dtctl
caches bundles per context. A failed refresh falls back to the cache and never
breaks `run`.

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

## Embedding

In a session-backed invocation (`pkg/engine`, `dtctl serve`), recipes load from
the built-in and environment layers only. The user and org directories are
host state, and bundles are cached in memory per environment and principal.
