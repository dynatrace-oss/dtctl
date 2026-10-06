# Built-in recipes

Named, parameterized DQL queries compiled into dtctl; `dtctl run <recipe>` runs
them through the same path as `dtctl query`. Users add their own in
`~/.config/dtctl/recipes/` with the same format. Design: [docs/dev/RECIPES_DESIGN.md](../docs/dev/RECIPES_DESIGN.md) · User guide: [docs/RECIPES.md](../docs/RECIPES.md).

## Layout

```text
recipes/
  _domains.yaml          # the domain registry: every name is <domain>-<name>
  _scopes.yaml           # scope dimensions (--cluster, --namespace, --tag k=v, ...)
  _fragments/*.tmpl      # shared {{define}} blocks
  <domain>/<name>.yaml   # one recipe per file, file name = recipe name
```

## A recipe

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata:
  name: k8s-pod-restarts          # <domain>-<name>; the domain must be registered
  version: 1                      # bump when the DQL or the output shape changes
  tags: [k8s, pods, oom, triage]  # lowercase search terms
spec:
  summary: Pods whose containers restarted or were OOM-killed   # one line, <= 100 chars
  requires: [k8s-metrics]
  timeframe: 1h
  scope: [cluster, namespace, tag]
  params:
    min_restarts: {type: int, default: 5, min: 1}   # flag: --min-restarts
  dql: |
    timeseries restarts = sum(dt.kubernetes.container.restarts),
      by: {k8s.cluster.name, k8s.namespace.name, k8s.pod.name}
      {{- with .scope.expr}}, filter: {{.}}{{end}}
    | ...
  means: >-
    ...
  emptyMeans: >-
    ...
  next:
    - recipe: k8s-warning-events
      bind: { namespace: k8s.namespace.name }
```

## Field reference

| Field | Meaning |
|---|---|
| `params.<x>.type` | `string`, `int`, `bool`, `enum` (`values: [...]`), `list` (comma-separated) |
| `params.<x>.required` / `default` | an optional param without a default is nil: guard with `{{if .x}}` |
| `params.<x>.positional` | may be the single argument (`dtctl run problems-get P-12345`); at most one |
| `params.<x>.min` / `max` / `pattern` | int bounds; regexp a string must match |
| `params.<x>.render: identifier` | splice an `enum` as a bare identifier, not a string |
| `scope` | dimensions from `_scopes.yaml`; place `{{.scope.stage}}` (a whole filter stage) or `{{.scope.expr}}` (bare, for `timeseries … filter:`) right after `fetch`. A declared scope must be used |
| `segments: off` | filter segments must not narrow the data; `-S` is rejected |
| `timeframe` | `2h` (overridable) · `none` (state query) · `{default: 7d, min: 1d}` · `{default: 1h, max: 1d}` (scan cap) · `{default: 30m, fixed: true}` (snapshot) · `{default: 7d, align: utc-day}` · `{default: 7d, inline: true}` |
| `dql` | Go template; params render as **escaped DQL literals** (never quote them). No `from:`/`to:` unless `inline`, which gets `{{.window.from}}`, `{{.window.to}}`, `{{.window.minutes}}` |
| `means` | what a row is and its units, a decision rule, and a `Not shown:` sentence |
| `emptyMeans` | required: separate "nothing happened" from "asked wrong"; say "not an all-clear" when empty is weak evidence |
| `empty: zero-row` | a single all-zero row counts as empty |
| `next[]` | follow-ups: `recipe`, `with:` (templates over this recipe's params), `bind:` (fields of a result row), `when: empty`/`nonempty`, `window:` (`{from: event.start, to: event.end, pad: 15m}` or `{from: 30d}`), `follow: true` (at most one, cheap) |
| `checks[]` | traps an ad-hoc `dtctl query` is tested against: `match` (all), `unless` (none), `warn`, `example` |
| `requires` | inventory capabilities; empty for data every environment has |
| `description` | optional: when to use it, what it does not cover |
| `deprecated` | `{message, replacedBy}`: adds a warning naming the replacement; removal waits at least two minor releases |

Capabilities for `requires`: `hosts`, `k8s`, `aws`, `azure`, `gcp`, `spans`,
`logs`, `bizevents`, `rum`, `davis`, `davis-events`, `security`, `synthetic`,
`host-metrics`, `process-metrics`, `k8s-metrics`, `service-metrics`, `aws-cloudwatch`.

Fragments (`_fragments/*.tmpl`): `{{define "name"}}…{{end}}`, used as
`{{template "name" .param}}`. Call one inside `{{if .param}}`, not `{{with}}`.
An unused fragment fails the lint.

## Checking a recipe

```bash
dtctl verify recipe -f recipes/k8s/k8s-pod-restarts.yaml --offline  # schema, template, references
dtctl verify recipe -f recipes/k8s/k8s-pod-restarts.yaml            # + the DQL parser
dtctl verify recipe --all                                           # every loaded recipe
dtctl verify recipe -f ./my-recipe.yaml                             # try one without rebuilding
```

`go test ./recipes/` lints every built-in recipe offline and pins its rendered
DQL in `testdata/golden/`; refresh after an intended change with
`go test ./recipes/ -update` and review the diff.

A built-in recipe lands only after it ran against real environments: non-empty
where the data exists, a correct `emptyMeans` where it does not, the units
`means` claims, bounded results, and no needless scan (check `scannedBytes` with `-M`).

## DQL traps worth knowing

Each returns a wrong or costly answer **without an error**. Lint names in brackets.

- Scope renders as `in(field, {…})`: problem k8s/host-group fields are arrays, where `==` never matches.
- `arrayFirst(x[][k])` returns nulls; use `arrayFirst(iCollectArray(x[][k]))`.
- Units: `dt.service.request.response_time` µs, span `duration` ns.
- Service metrics fold endpoints into `NON_KEY_REQUESTS`; use spans with `span-multiplicity`.
- Filter a big table by a small one with `x in [subquery]`, not `lookup` (reads every record).
- Casts fail silently: `toString(trace.id)` vs log `trace_id`; `toSmartscapeId("…")`; classic IDs differ for most entity types (`meta-entity-id`).
- Logs carry k8s names, not IDs; many pod logs lack `k8s.workload.name` (`k8s-workload-logs`).
- Problem display IDs are reused: dedup by `event.id`, latest first.
- A capped `collectArray` plus `total = count()` yields one row over no input: `| filter total > 0`.
- long / long is integer division: `toDouble(n) / {{.window.minutes}}`.
- `countDistinct` is an estimate; use `countDistinctExact` and count events by `event.id`.
- `takeFirst`/`takeLast` have no order; use `takeMax(record(timestamp, x))`.
- `replacePattern(x, "ISO8601", …)` misses ISO timestamps; spell it out (`text-mask`).
- Most span exceptions sit on successful requests; say so in `means`.
- Multi-key `timeseries` keeps only series every key reports [`multi-key-timeseries`]: `union: true`.
- `samplingRatio` on anything but logs/spans [`sampling-source`].
- Sampled `count()` under-reports [`sampling-unscaled`]: `sum(coalesce(dt.system.sampling_ratio, 1))`.
- `limit` before `summarize` [`limit-before-aggregate`].
- `coalesce(a, b) == x` [`coalesce-filter`]: `a == x or b == x`.
- `lower(field)` in a filter [`case-folded-filter`]: `contains(f, "x", caseSensitive: false)`.
- Unnamed aggregate [`unaliased-aggregate`]: `n = count()`.
- `interval:` equal to the window reads up to 2x [`interval-equals-window`].
