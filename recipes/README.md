# Built-in recipes

A recipe is a named, parameterized DQL query that answers one common question.
`dtctl run <recipe>` renders it and runs it through the same path as
`dtctl query`. The files here are compiled into dtctl; organizations and users
add their own with the same format (`DTCTL_RECIPE_PATH`,
`~/.config/dtctl/recipes/`), and Dynatrace apps ship them as bundles.

Design: [docs/dev/RECIPES_DESIGN.md](../docs/dev/RECIPES_DESIGN.md).
User guide: [docs/RECIPES.md](../docs/RECIPES.md).

## Layout

```text
recipes/
  _domains.yaml          # the domain registry: every name is <domain>-<what>
  _scopes.yaml           # scope dimensions (--cluster, --namespace, --tag k=v, ...)
  _fragments/*.tmpl      # shared {{define}} blocks
  <domain>/<name>.yaml   # one recipe per file, file name = recipe name
```

## A recipe

```yaml
apiVersion: dtctl.dev/v1alpha1
kind: Recipe
metadata:
  name: k8s-pod-restarts          # <domain>-<what>; the domain must be registered
  version: 1                      # bump when the DQL or the output shape changes
  tags: [k8s, pods, oom, triage]  # search terms; lowercase
spec:
  summary: Pods whose containers restarted or were OOM-killed   # one line, <= 100 chars
  description: >-                 # optional: when to use it, what it does not cover
    ...
  requires: [k8s-metrics]         # inventory capabilities (see below)
  timeframe: 1h                   # default window; see "Windows"
  scope: [cluster, namespace, tag]
  params:
    min_restarts:                 # snake_case; the flag is --min-restarts
      type: int                   # string | int | bool | enum | list
      default: 5
      min: 1
  dql: |
    timeseries restarts = sum(dt.kubernetes.container.restarts),
      by: {k8s.cluster.name, k8s.namespace.name, k8s.pod.name}
      {{- with .scope.expr}}, filter: {{.}}{{end}}
    | ...
  means: >-                       # how to read a row; units; traps
    ...
  emptyMeans: >-                  # required: what an empty result means, and what does not
    ...
  next:
    - recipe: k8s-warning-events
      bind: { namespace: k8s.namespace.name }   # params taken from the first row
```

### Params

| field | meaning |
|---|---|
| `type` | `string`, `int`, `bool`, `enum` (`values: [...]`), `list` (comma-separated) |
| `required` | the run fails without it |
| `positional` | may be given as the single argument: `dtctl run problems-get P-123` (at most one) |
| `default` | used when unset; an optional param without a default is unset (`{{if .x}}`) |
| `min` / `max` | bounds for an `int` |
| `pattern` | regexp a `string` must match (`^P-[0-9]+$`) |
| `render: identifier` | an `enum` value is spliced as a bare identifier (a field or function name) instead of a string literal |

Values reach the template as **escaped DQL literals**: `{{.service}}` renders
`"checkout"` with quotes, a list renders `{"a", "b"}` for `in(field, {{.list}})`.
Never quote a param yourself. An unset optional param is nil: guard it with
`{{if .name}}…{{end}}` or `{{with .name}}…{{end}}`; rendering fails loudly
otherwise.

### Scope

`scope: [cluster, namespace, tag]` declares which dimensions from
`_scopes.yaml` the recipe accepts. They are primary Grail fields and tags,
enriched on every signal, so one predicate fits every recipe. The template
places them:

- `{{.scope.stage}}` — a whole pipeline stage, `| filter <expr>`, or nothing.
- `{{.scope.expr}}` — the bare expression, for a `timeseries … filter:`
  argument: `{{- with .scope.expr}}, filter: {{.}}{{end}}`.

Place the scope **where it narrows the scan**: right after `fetch`, or in the
`timeseries` filter argument, not after an aggregation that dropped the field.
A recipe that declares a scope must use it. Service names, host names and
endpoints are *params*, not scope: their field differs per data object.

`segments: off` declares that filter segments must not narrow the data (billing
totals, account-wide security posture); `-S` is then rejected.

### Windows

| `timeframe:` | meaning |
|---|---|
| `2h` | default window, overridable with `--from/--to` (`d` and `w` work) |
| `none` | a state query with no window (entity inventory, smartscape) |
| `{default: 7d, min: 1d}` | a trend needs at least `min` |
| `{default: 1h, max: 1d}` | a scan-heavy recipe caps one run's window; longer goes through `dtctl query` |
| `{default: 30m, fixed: true}` | a snapshot: `--from/--to` are rejected |
| `{default: 7d, align: utc-day}` | whole UTC days (billing) |
| `{default: 7d, inline: true}` | the DQL writes `{{.window.from}}`/`{{.window.to}}` itself |

dtctl sends the window as the query's default timeframe, so the DQL has **no
`from:`/`to:`** of its own (validation rejects one in the first command).
An inline recipe also gets `{{.window.minutes}}`, the window's length in whole
minutes (at least 1), for rates per minute.

### `means` and `emptyMeans`

These are what an agent reads instead of guessing, and it reads them
literally. Every `means` carries:

- **what a row is**, the units (microseconds!) and what is extrapolated or
  estimated;
- **a decision rule**: how to tell the finding from the background in this
  result ("p99_vs_base near 1 is how the endpoint always behaves, well above
  1 is a regression", "on_failed near 0 is a handled exception");
- **`Not shown:`**, one sentence naming what the recipe leaves out and, where
  one exists, the recipe that has it. An agent that does not know a recipe is
  root-only, current-only or capped reads its silence as an answer.

`emptyMeans` is mandatory and has to separate "nothing happened" from "you
asked wrong": a misspelled name, a missing capability, a window too short.
Where an empty result is weak evidence (logs that do not carry the field,
events that many environments never send), say "not an all-clear" and name
the recipe to try instead; a `when: empty` edge makes it runnable. dtctl puts
it in the agent envelope as `empty_reason` when the result is empty.

A recipe that aggregates without `by:` returns one row even when nothing
happened (`count = 0`). `empty: zero-row` makes that row count as empty: a
single row whose numbers are all zero or null. A partial result (a scan or
record limit hit) is never reported as `emptyMeans`, because an absent row may
exist; the envelope says `recipe_partial` instead.

### `next`

Follow-up recipes, emitted as runnable `dtctl run …` suggestions. `with:` maps
target params to templates over this recipe's params (`{service: "{{.service}}"}`);
`bind:` maps them to fields of a result row: the first of the leading rows
whose bound fields all hold a value the target param accepts. An array field binds only to a
`list` param, joined with commas. `when: empty` / `when: nonempty` restricts an
edge. The window, scope and segments carry over, unless `window:` derives one
from the row:

```yaml
next:
  - recipe: logs-for-service
    bind: {service: dt.service.name}
    window: {from: event.start, to: event.end, pad: 15m}   # clamped to the target's timeframe.max
```

`window: {from: 30d}` (a duration instead of a field) needs no row: with
`when: empty`, a recipe can suggest itself further back. The edge is dropped
when the run already looked that far.

### Fragments

A DQL piece that several recipes must spell identically goes in
`_fragments/*.tmpl` as `{{define "name"}}…{{end}}` and is used with
`{{template "name"}}`, or `{{template "name" .param}}` when it takes an
argument (the fragment sees it as `.`). A fragment nobody uses fails the lint.

| Fragment | Argument | What it spells |
|---|---|---|
| `span-multiplicity` | | how many requests one stored span stands for (sampling extrapolation) |
| `log-source` | | the source a log record is attributed to |
| `log-template` | | stages that reduce a log line to a masked message template (`template`) |
| `text-mask` | a string expression | timestamps, IPs, UUIDs, hex and numbers masked, in that order |
| `k8s-workload-logs` | quoted workload name | a workload's records, recovering the name from the pod name when `k8s.workload.name` is missing |
| `entity-signal` | quoted entity ID | a record about an entity under its Smartscape ID, classic ID or source entity |
| `problem-affects` | quoted entity ID or name | a `dt.davis.problems` record that affects or relates to the entity |
| `vuln-latest-state` | | the latest state of each vulnerability |

Call a fragment inside `{{if .param}}…{{end}}` with `.param` as the argument,
not inside `{{with .param}}`: the unused-fragment lint does not look inside
`with` bodies yet.

### Focus and control

"Is this new?" is answered against a control period, in one scan. Declare an
inline window, take the control length as an int param, and reach back with
`timeAdd`:

```yaml
timeframe: { default: 30m, max: 2h, inline: true }
params:
  control: { type: int, default: 60, min: 15, max: 360, description: Minutes before the window to compare against }
dql: |
  fetch spans, from: {{timeAdd .window.from (printf "-%vm" .control)}}, to: {{.window.to}}
  | fieldsAdd infocus = start_time >= {{.window.from}}
  | summarize { now = countIf(infocus), before = countIf(not(infocus)) }, by: { … }
  | fieldsAdd new = before == 0,
      per_min = now / toDouble(({{.window.to}} - {{.window.from}}) / 1m), before_per_min = before / {{.control}}
```

A control on another day (the same hour yesterday, a week ago) is a second
fetch of the shifted window, `| append [fetch …, from: {{timeAdd .window.from
"-24h"}}, to: {{timeAdd .window.to "-24h"}} …]`, so the scan doubles: offer
`none` to skip it. Aggregate with `if(…)` on the period flag, and render a
separate branch when there is no control, since `if()` over a constant draws
a warning.

### DQL traps worth knowing

Each of these returns a wrong or needlessly expensive answer **without an
error**; most were measured on real environments.

- Scope renders as `in(field, {…})`, even for one value: on
  `dt.davis.problems` the k8s and host-group fields are arrays, where `==`
  never matches. Write your own array filters the same way, or with
  `iAny(arr[] == x)`.
- `arrayFirst(x[][k])` evaluates per element and returns nulls; use
  `arrayFirst(iCollectArray(x[][k]))`.
- `dt.service.request.response_time` is in microseconds, span `duration` in
  nanoseconds.
- Service metrics fold most endpoints into `NON_KEY_REQUESTS`; per-endpoint
  numbers come from spans, extrapolated with `span-multiplicity`.
- An empty string param is rejected, so `""` can't silently mean "all".
- **Subquery, not lookup, to filter a big table by a small one.**
  `fetch logs | filter trace_id in [fetch spans | … | fields tid]` reads only
  the `trace_id` column; the same match as a `lookup` read every log record
  in full (8 GB against 347 GB on a large environment). The form is
  `x in [subquery]`; `in(x, [subquery])` is rejected. The subquery sees the
  outer window.
- **Casts fail silently.** Span `trace.id` is a UID and log `trace_id` a
  string: compare `toString(trace.id)`. Smartscape ID fields need
  `toSmartscapeId("…")`. Classic IDs equal Smartscape IDs only for services,
  hosts, disks and a few more; `PROCESS_GROUP_INSTANCE-`, `KUBERNETES_*` and
  cloud IDs differ (`meta-entity-id` maps them). A wrong spelling matches
  nothing.
- **Logs carry names, not IDs**, for most Kubernetes data: match a workload by
  name (`k8s-workload-logs`), and expect a fifth to a third of pod logs to lack
  `k8s.workload.name`.
- **Problem display IDs are reused**: one environment can hold two problems
  `P-…` with the same number. Keep the latest record per `event.id`
  (`sort timestamp desc | dedup event.id`), not one record per display ID.
- **A total next to a capped list**: `summarize { v = collectArray(record(…),
  maxLength: 100000), total = count() } | expand v | fieldsFlatten v, prefix:
  "" | fieldsRemove v` keeps the rows and adds the total to each. Over no
  input it still yields one row with `total = 0`: add `| filter total > 0`.
- **`countDistinct` is an estimate** (`countDistinctApprox`). Use
  `countDistinctExact` for small counts that are the answer (problems,
  events), and count events by `event.id`: open events re-emit records.
- **`takeFirst`/`takeLast` have no defined order.** For the latest value use
  `takeMax(record(timestamp, x))`.
- `replacePattern(x, "ISO8601", …)` did not match ISO timestamps; spell the
  pattern out (see `text-mask`).
- Most exceptions on spans sit on requests that succeeded, and most
  error-status spans are below the root: a recipe on failed root spans says
  so in `means`.

The traps that give a wrong answer without an error are lints: `go test
./recipes/` fails on them, and `dtctl verify recipe` reports them for your own
recipes.

| Lint | What goes wrong | Fix |
|---|---|---|
| `multi-key-timeseries` | a timeseries over several metric keys keeps only the series every key reports; a host without one metric, or a window without an OOM kill, drops out | `union: true` |
| `sampling-source` | `samplingRatio` on anything but logs and spans: events ignore it with a warning, problems reject it | sample logs or spans only |
| `sampling-unscaled` | a sampled `count()` under-reports by the ratio | `sum(coalesce(dt.system.sampling_ratio, 1))` |
| `limit-before-aggregate` | `limit` then `summarize` aggregates an arbitrary subset | limit after aggregating |
| `coalesce-filter` | `coalesce(a, b) == x` in a filter defeats the field index | `a == x or b == x` |
| `case-folded-filter` | `lower(field)`/`upper(field)` in a filter defeats the n-gram index and reads every record | `contains(f, "x", caseSensitive: false)`, `matchesPhrase`, `matchesValue` |
| `unaliased-aggregate` | the column is named by the expression, which an adapted query or `--jq` must backquote | `n = count()` |
| `interval-equals-window` | an interval equal to the window straddles two aligned buckets and reads up to 2x | a smaller interval, or `summarize` |

## Capabilities (`requires`)

`requires` names capabilities from `dtctl inventory`. `dtctl get recipes` hides
a recipe whose requirement the cached inventory found **absent** (an unknown
verdict hides nothing). Use the built-in names: `hosts`, `k8s`, `aws`, `azure`,
`gcp`, `spans`, `logs`, `bizevents`, `rum`, `davis`, `davis-events`,
`security`, `synthetic`, `host-metrics`, `process-metrics`, `k8s-metrics`,
`service-metrics`, `aws-cloudwatch`. Leave `requires` empty for data every
environment has (billing events, the data-object catalog).

## Checking a recipe

```bash
dtctl verify recipe -f recipes/k8s/k8s-pod-restarts.yaml --offline  # schema, template, references
dtctl verify recipe -f recipes/k8s/k8s-pod-restarts.yaml            # + the DQL parser
dtctl verify recipe --all                                           # every loaded recipe
DTCTL_RECIPE_PATH=./my-recipes dtctl run my-recipe --dry-run        # try one without rebuilding
```

`go test ./recipes/` checks every built-in recipe offline and pins its
rendered DQL.

**A built-in recipe lands only after it ran against real environments**:
non-empty where the data exists, a correct `emptyMeans` where it does not,
values in the units `means` claims, and no query that scans more than it must
(check `scannedBytes` with `-M`). Keep result sets bounded (`limit`, top-N).
