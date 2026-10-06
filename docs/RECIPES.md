# Recipes

> **Development.** `run`, `get recipes`, `describe recipe` and `verify recipe`
> are unfinished and carry no guarantees (see [STABILITY.md](STABILITY.md)).
> They are not registered until you opt in:
>
> ```bash
> export DTCTL_DEVELOPMENT=recipes          # this shell
> dtctl config set development.recipes on   # durably
> ```
>
> Design and rationale: [dev/RECIPES_DESIGN.md](dev/RECIPES_DESIGN.md).

A recipe is a named, parameterized DQL query that answers one common question (which problems
are open, which pods restarted) without you writing DQL. It carries typed params, a default
window, scope filters, and how to read the result, including an empty one. Adapt its DQL with `dtctl query`.

## Find a recipe

```bash
dtctl get recipes                         # every recipe this environment can answer
dtctl get recipes --domain k8s            # one domain
dtctl get recipes --search "slow endpoints"
dtctl get recipes --tag rca -o wide       # by tag; wide adds tags and requirements
dtctl describe recipe problems-active     # params, scope, window, how to read it, the DQL
```

Recipes whose data is absent per the cached `dtctl inventory` are hidden (`--all` shows them); in agent mode, an unfiltered `get recipes` returns a **domain index**.

## Run a recipe

```bash
dtctl run problems-active
dtctl run problems-get P-12345                       # a positional argument
dtctl run k8s-pod-restarts --namespace=checkout --from=6h
dtctl run services-red --cluster=prod-eu --cluster=prod-us
dtctl run logs-error-patterns --tag=team=payments    # primary Grail tags
dtctl run problems-active -S my-segment              # filter segments
dtctl run problems-active --dry-run                  # print the DQL, run nothing
dtctl run problems-active --follow                   # also run the first follow-up
```

- **Params** are the recipe's own flags (`dtctl run <recipe> --help`); `--from`/`--to` take `2h`, `7d` or RFC3339.
- **Scope**: repeat a flag to OR its values. Output flags work as in `dtctl query` (`-o`, `--jq`, `--spill`).

### Agent mode

| Field | Meaning |
|---|---|
| `context.recipe` / `context.query` | the recipe (name, version, layer) and its rendered DQL |
| `context.means` / `context.has_more` | how to read a non-empty result; set when it is the top N, not a total |
| `context.window` / `context.scope` | resolved window and scope |
| `context.empty_reason` | `recipe_empty_means` (what empty means) or `recipe_partial` (a limit cut the result) |
| `context.suggestions` | runnable follow-ups, e.g. `dtctl run problems-get P-12345` |
| `context.follow_up` | with `--follow`: the follow-up's command, query, records and `means` |

`dtctl query` in agent mode suggests a matching recipe and warns about the traps recipes know.

## Where recipes come from

**user** (`~/.config/dtctl/recipes/**.yaml`, read live) wins over **builtin** ([`recipes/`](../recipes/), ships with dtctl).

## Write your own

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata:
  name: payments-slow-checkouts      # <domain>-<name>; new domains go in _domains.yaml
  version: 1
  tags: [payments, latency]
spec:
  summary: Checkout requests slower than a threshold
  timeframe: 1h
  params: {threshold_ms: {type: int, default: 2000, min: 1}}
  dql: |
    fetch spans
    | filter endpoint.name == "/checkout" and duration > {{.threshold_ms}} * 1ms
    | summarize {count = count(), p95 = percentile(duration, 95)}, by: {service.name}
  means: Checkout spans over the threshold, per service.
  emptyMeans: No checkout span exceeded the threshold in the window.
```

<!-- prose-check:ignore 4 -->
```bash
dtctl verify recipe -f ~/.config/dtctl/recipes/payments-slow-checkouts.yaml
dtctl run payments-slow-checkouts --threshold-ms=5000 --dry-run
```

`verify recipe` lints the file and runs the environment's DQL parser (`--offline` skips it).
Full format: [`recipes/README.md`](../recipes/README.md).

### Cost and correctness cheat-sheet

- Default to the shortest window that answers; cap scan-heavy recipes with `timeframe.max`, never `scanLimitGBytes`.
- Filter early with `==`/`in()` on plain fields (`lower()`, `coalesce() == x` read every record); cross-table, `x in [subquery]`, not `lookup`.
- Sample only logs and spans, scale counts with `sum(coalesce(dt.system.sampling_ratio, 1))`, and never `| limit` before an aggregate.
- Multi-key `timeseries` needs `union: true`; `interval:` must be shorter than the window; `countDistinct` is an estimate.
- Units: span `duration` ns, `dt.service.request.response_time` µs, K8s CPU millicores.
- `means` says what a row is, a decision rule, and `Not shown:`. More: [DQL traps](../recipes/README.md#dql-traps-worth-knowing).
