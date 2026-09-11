# Onboarding Verification Design Proposal

**Status:** Implemented (see Implementation notes)
**Created:** 2026-09-11
**Author:** dtctl team

## Overview

After you onboard a new data source — instrument a service, point an OTel collector
at the tenant, `dtctl ingest` a file — the next question is always the same:

> Did it land? Which signals, how many records, how recently?

Today that question is answered either by an LLM investigation (minutes, non-deterministic)
or by hand-authoring one DQL query per signal and knowing each stream's schema. This
proposal makes it a single deterministic command by extending `dtctl inventory` with a
**time window** and a **scope filter**, and by closing the confirmation gap that
`dtctl ingest` is about to open.

## Goals

1. **Deterministic** — same inputs, same answer, in seconds; no model in the loop.
2. **Per-signal** — logs / spans / metrics / bizevents / … answered separately, each with
   a count and a `last_seen`.
3. **Windowed** — "since T", so a source that emitted once last week does not read as healthy.
4. **Honest** — keep `inventory`'s three-state verdict. A probe that could not run is
   `unknown`, never `absent`.
5. **Cheap** — an onboarding check that scans 200 GB is not a check anyone will run in a loop.
6. **Agent-consumable** — one call, small structured result, usable as a gate.

## Non-Goals

- A new server-side endpoint or any backend-mediated read. dtctl holds the tenant's own
  credentials and already has DQL; this check belongs in the client.
- Entity/service joins, VCS-repository matching, or a `--service` flag — that is product-side
  domain modelling, and `dt.entity.*` is not available on Grail-only tenants.
- A second query language. The scope filter is a DQL fragment, nothing else.
- Judging "health". Report presence, counts, and timestamps; the caller judges.

---

## What exists today

| Need | Command | Gap |
|---|---|---|
| Poll until data arrives | `dtctl wait query '<dql>' --for=any` | You must author the DQL and know each stream's schema; one signal per invocation |
| What data exists here | `dtctl inventory` | Environment-wide, **no time window**, **no scope filter** |
| Validate before executing | `dtctl verify query` / `verify openpipeline-*` | Static; never touches live data |
| Push data in | `dtctl ingest` (design, `feat/ingest`) | Per **D12**, reports *HTTP status only* — a `202` says nothing about landing |

So roughly 80% of the machinery is already in the repo. The gap is narrower than it first
appears, and it is concentrated in `inventory`.

## The core design tension

`inventory`'s structural capability shapes (`DataObject`, `EntityTypes`, `MetricKey`) are
**retention-scoped, not window-scoped**. `DataObject` presence is decided from
`facts.streamRows` — bucket record counts summed across the whole retention period
(`sdk/inventory/discover.go:150`). "logs: present" therefore means *logs exist somewhere in
retention*, which is precisely the false-healthy reading this proposal sets out to fix.

A window cannot be bolted onto the structural shapes; it needs a different evidence shape —
an actual counting query per stream. That is the real work in this proposal, and it turns out
to be dominated by query cost, not by plumbing.

---

## Measured: probe cost is the design constraint

All figures measured against a live busy tenant (`gmg`) on 2026-09-11, 15-minute window
unless stated. This is not incidental detail — it determines the command's shape.

| Probe shape | Scanned | Time |
|---|---|---|
| `fetch logs \| summarize count()` — unfiltered | **203.7 GB** | 1293 ms |
| `fetch logs \| summarize total=count(), matched=countIf(<expr>), last_seen=takeMax(timestamp)` | **204.9 GB** | 1752 ms |
| `fetch logs \| filter <expr> \| summarize matched=count(), last_seen=takeMax(timestamp)` | **10.8 GB** | 209 ms |
| …same, 5-minute window | 4.1 GB | 150 ms |
| `fetch spans \| summarize …countIf(<expr>)…` | 56.5 GB | 1278 ms |
| `fetch spans \| filter <expr> \| summarize …` | **1.2 GB** | 45 ms |
| `fetch bizevents \| filter <expr> \| summarize …` | 12.1 GB | 635 ms |
| `metrics \| filter matchesValue(metric.key,"dt.kubernetes.*") \| summarize count()` | **0 B** | 1132 ms |
| `timeseries count(<key>), from:now()-15m, filter:<expr>` — *scoped* metric arrival | **0 B** | 24 ms |
| `fetch dt.system.buckets \| summarize sum(records), by:{dt.system.table}` — retention metadata | **0 B** | 45 ms |

Three conclusions, each of which changes the design:

1. **Filter-first, never `countIf`.** Pushing the predicate into `| filter` before
   `summarize` is **19× cheaper on logs** and **48× on spans** than computing
   `total` and `matched` together with `countIf`. The combined-probe shape — the obvious
   way to write it, and the one that would let us report "stream is live but your filter
   matched nothing" in a single query — is unaffordable.
2. **The unfiltered count is the most expensive query available.** `fetch logs | summarize
   count()` scans 203.7 GB — *more* than the filtered probe, despite referencing no columns.
   An unscoped windowed count must never run.
3. **Metric arrival, and the retention metadata, are free.** `timeseries … filter:<expr>`
   scopes metrics by the same `--where` predicate at 0 scanned bytes and 24 ms, and the
   bucket-metadata query that backs the `empty` vs `no-data` distinction is likewise free.
   Metrics get a different probe than the `fetch` streams, and cost is a non-issue on either.

Scan scales roughly linearly with window: 5m = 4.1 GB, 15m = 10.8 GB on logs. At
`--scan-limit-gbytes 25` (today's default) a 1-hour unfiltered-stream window would be
refused and correctly degrade to `unknown` — useless, but not wrong. Hence a small default
window and a mandatory scope.

---

## Design

### `dtctl inventory --since <t> --where <dql-filter>`

```
--since <duration>             Windowed arrival mode, e.g. 15m or 2h. Requires a scope.
--where <dql-filter>           DQL filter fragment applied to each probed stream.
--signals <list>               Restrict probing to named capabilities (default: signal streams).
--require <list>               Exit non-zero unless every named signal is live.
--stale-after <duration>       Age past which a signal is stale, not live (default max(2m, window/3)).
```

`--where` takes a **DQL filter expression**, inserted as `| filter <expr>` immediately after
the fetch. It is not a new mini-language, not a key=value DSL, and not translated. This
keeps faith with the "no second query language" constraint and means anything the user can
express in DQL works on day one.

Windowed mode probes only the signal-stream capabilities. Topology (`EntityTypes`) and
metric-family (`MetricKey`) shapes have no natural "since" meaning for a live census, so
they are reported as evaluated-unwindowed rather than silently reinterpreted.

### Why `--since` requires a scope

Onboarding verification is inherently *"did **this new thing** arrive"*. Without a scope you
are asking "is the tenant alive", which plain `inventory` already answers for free from
bucket metadata. And per measurement 2, the unscoped windowed count is the single most
expensive query in the battery. Requiring a scope — `--where`, `-S/--segment`, or both (see
open question 5) — removes a 200 GB footgun and costs nothing in expressiveness.

### Distinguishing "no data" from "wrong filter"

The valuable disambiguation — *your filter matched nothing, but the stream itself is live* —
would naively come from the combined `countIf` probe, which measurement 1 rules out. It
comes for free instead from the bucket metadata `inventory` **already collects**
(`facts.streamRows`, one query for all streams). That answers "does this stream hold data at
all, in retention" without a window. Combining the two gives three distinguishable absences:

| `streamRows` | windowed `matched` | Verdict and evidence |
|---|---|---|
| stream not in catalog | — | `absent` — "no `bizevents` in the data-object catalog" |
| 0 rows in retention | 0 | `absent` — "stream exists but holds no data in retention" |
| > 0 rows in retention | 0 | `absent` — "stream holds data in retention, but 0 records matched `<filter>` in the window" ← *the onboarding-failure case* |
| any | > 0 | `present` |
| any | probe truncated / scan-capped / errored | **`unknown`** — never absent |

That third row is the one that actually matters after an instrumentation change, and it is
reachable with zero extra queries.

### Per-stream time field

`fetch spans` carries **no `timestamp` field** — verified: `takeMax(timestamp)` on spans
returns type `undefined` and the field is dropped from the record entirely, while
`takeMax(start_time)` works. So `last_seen` cannot use one field name across streams.

`CapabilityDef` gains an optional `timeField` (default `timestamp`; `spans` → `start_time`),
used only in windowed mode. It is a fourth optional attribute, not a fifth discovery shape,
so the exactly-one-shape contract in `validateDef` is untouched.

### Per-signal ingest state

The three-state capability verdict (`present` / `absent` / `unknown`) is too coarse once a
window exists. Windowed mode assigns each signal type one of six states, derived from the
windowed probe plus the free bucket metadata:

| State | Condition | Means |
|---|---|---|
| `live` | `matched > 0`, `last_seen` within `--stale-after` | Arriving now |
| `stale` | `matched > 0`, `last_seen` older than `--stale-after` | **Started, then stopped inside the window** |
| `empty` | `matched == 0`, stream holds records in retention | Stream works; this source is not producing into it |
| `no-data` | `matched == 0`, stream empty in retention | Stream exists but is unused on this tenant |
| `absent` | Stream not in the data-object catalog | Not available in this environment |
| `unknown` | Probe truncated, scan-capped, or errored | No verdict — never read as absence |

`stale` is the state that only becomes expressible once there is a window, and it is the most
diagnostic one for onboarding: a source that emitted during rollout and then stopped looks
identical to a healthy one under any retention-scoped check. Default `--stale-after` is
`max(2m, window/3)`.

For `--require` purposes `live` passes; `stale`, `empty`, `no-data`, and `absent` fail;
`unknown` exits 2.

### Exit codes

Plain `dtctl inventory` keeps exit code 0 always — no behaviour change. `--require` opts in
to gate semantics, matching the `verify` family's conventions:

```
0 - every required signal present
1 - at least one required signal absent
2 - at least one required signal unknown (no verdict — retry or widen the budget)
```

`unknown` deliberately does not collapse into `absent`: a CI gate that fails closed on a
scan-capped probe is reporting a dtctl budget problem as a customer telemetry problem.

---

## Examples

### 1. Did the new namespace start reporting?

Every number below is real: measured on the `gmg` tenant, 2026-09-11, with the probe shapes
this document specifies.

```console
$ dtctl inventory --since 15m --where 'k8s.namespace.name == "dps-ingest"'

Context:    gmg
Generated:  2026-09-11T06:25:00Z
Window:     now()-15m → now()  (15m, stale after 5m)
Scope:      k8s.namespace.name == "dps-ingest"

SIGNAL             STATE     RECORDS    LAST SEEN   AGE
spans              live    3,630,872    06:22:53    19s
logs               live      237,194    06:22:32    39s
events             live        7,434    06:22:46    28s
security.events    live        4,920    06:23:55    47s
bizevents          live        4,499    06:22:52    21s
k8s-metrics        live    400 points   06:24:00     —
user.events        empty           0           —     —
davis-events       absent          —           —     —

6 live · 1 empty · 1 absent · 0 unknown

Notes
  user.events  — 0 records matched in the window, but the stream holds 2.9B records
                 tenant-wide: the stream works, this source is not producing into it
  davis-events — not in this environment's data-object catalog

Discovery: 8 queries, 1.5s query time, 29.0 GB scanned
```

The `user.events` line is what earns the feature. A retention-scoped check reports that
stream as simply "present" (2.9B records exist); an unscoped windowed check cannot attribute
it to this source at all. Only the combination says *the stream works, your source isn't
producing into it* — and it costs no extra query, because the retention figure comes from the
bucket metadata `inventory` already collects for free.

### 2. The false-healthy case this exists to prevent

```console
$ dtctl inventory --where 'service.name == "pricing-service"'
Capabilities: logs, spans, bizevents        # ← retention-scoped: "yes, sometime in 35 days"
```

```console
$ dtctl inventory --since 30m --where 'service.name == "pricing-service"'

SIGNAL      STATE    RECORDS   LAST SEEN   AGE
logs        stale     12,904    05:58:11   27m
spans       stale      3,551    05:58:09   27m
bizevents   empty          0           —    —

0 live · 2 stale · 1 empty · 0 unknown

Notes
  logs, spans — data arrived in this window but stopped 27m ago (stale after 10m):
                the source emitted during rollout and is no longer reporting
```

Same service, opposite answers. The unwindowed check calls it healthy. The windowed one shows
the actual failure mode — instrumentation that worked at deploy time and then stopped — which
is invisible to any retention-scoped check. A source that emitted once last week, or once
during a deploy, reads as perfectly healthy without a window.

### 3. As a CI / agent gate

```console
$ dtctl inventory --since 10m \
    --where 'k8s.namespace.name == "payments"' \
    --require logs,spans
$ echo $?
0
```

```console
$ dtctl inventory --since 10m --where 'service.name == "checkout"' --require logs,spans,bizevents
...
$ echo $?
1        # bizevents absent
```

### 4. Agent mode

```console
$ dtctl inventory --since 15m --where 'k8s.namespace.name == "payments"' --agent
```

```json
{
  "ok": true,
  "result": {
    "context": "prod-eu",
    "generatedAt": "2026-09-11T09:14:22Z",
    "window": { "since": "now()-15m", "filter": "k8s.namespace.name == \"payments\"" },
    "summary": { "live": 6, "stale": 0, "empty": 1, "noData": 0, "absent": 1, "unknown": 0 },
    "signals": [
      { "signal": "spans",           "state": "live",   "records": 3630872, "lastSeen": "2026-09-11T06:22:53Z", "ageSeconds": 19 },
      { "signal": "logs",            "state": "live",   "records": 237194,  "lastSeen": "2026-09-11T06:22:32Z", "ageSeconds": 39 },
      { "signal": "events",          "state": "live",   "records": 7434,    "lastSeen": "2026-09-11T06:22:46Z", "ageSeconds": 28 },
      { "signal": "security.events", "state": "live",   "records": 4920,    "lastSeen": "2026-09-11T06:23:55Z", "ageSeconds": 47 },
      { "signal": "bizevents",       "state": "live",   "records": 4499,    "lastSeen": "2026-09-11T06:22:52Z", "ageSeconds": 21 },
      { "signal": "k8s-metrics",     "state": "live",   "datapoints": 400,  "lastSeen": "2026-09-11T06:24:00Z", "ageSeconds": 60 },
      { "signal": "user.events",     "state": "empty",  "records": 0,
        "evidence": "0 matched in window; stream holds 2902744233 records in retention" },
      { "signal": "davis-events",    "state": "absent",
        "evidence": "no dt.davis.events in the data-object catalog" }
    ],
    "discovery": { "queries": 8, "seconds": 1.5, "scannedGbytes": 29.0 }
  },
  "context": {
    "verb": "inventory",
    "duration": "1.6s",
    "suggestions": [
      "user.events is live tenant-wide but empty for this scope — check the source's emission, not the tenant",
      "Unknown signals got no verdict; re-run with --budget-seconds raised rather than treating them as absent"
    ]
  }
}
```

The envelope is the existing `output.Response` shape (`pkg/output/agent.go:37`) — agents that
already consume `inventory` need no changes beyond the new `arrivals` block.

### 5. Closing the ingest loop

`dtctl ingest` reports HTTP status only (**D12**: the 3rd-gen endpoints will not return
accepted-line counts at all), so `202` is not confirmation. The documented recipe:

```console
$ dtctl ingest logs -f payload.json
202 Accepted

$ dtctl inventory --since 5m --where 'log.source == "batch-import"' --require logs
$ echo $?
0
```

Or, when you want to block rather than check once:

```console
$ dtctl wait query 'fetch logs | filter log.source == "batch-import"' \
    --for=count-gte=1 --timeout 60s
```

### 6. An unknown is not an absence

```console
$ dtctl inventory --since 1h --where 'k8s.namespace.name == "payments"'

Unknown (no verdict — not evidence of absence)
  logs — probe exceeded the 25 GB scan cap (1h window ≈ 43 GB on this stream);
         narrow --since or raise --scan-limit-gbytes

Discovery: 6 queries, 12.1s query time
  note: 1 probe refused by the scan cap — the inventory is partial
```

Exit code with `--require logs` would be **2**, not 1: dtctl ran out of budget, the customer's
telemetry is not implicated.

---

## Decision log

- **D1 — Extend `inventory`, do not add a verb.** The evidence machinery, the budget, the
  three-state verdict, and the agent envelope all already exist. `verify` is the wrong home:
  in dtctl it means "validate without executing", and this executes.
- **D2 — `--where` is a DQL filter fragment.** No key=value DSL, no `--service`. Upholds the
  no-second-query-language constraint and works with any field on day one.
- **D3 — Filter-first probe shape; never `countIf`.** Measured 19× cheaper on logs, 48× on
  spans. Forfeits the single-query "live but unmatched" signal, which D4 recovers for free.
- **D4 — `--since` requires a scope** (`--where` or `-S/--segment`). The unscoped windowed count is the most expensive
  query available (203.7 GB measured), and bucket metadata already answers the unscoped
  question at no cost.
- **D5 — Per-stream `timeField` on `CapabilityDef`** (default `timestamp`, `spans` →
  `start_time`). Verified necessary: spans carry no `timestamp`. An optional attribute, not a
  new discovery shape — `validateDef`'s exactly-one-shape contract is untouched.
- **D6 — Metrics are probed with `timeseries … , filter:<where>`, not `fetch`, over a bounded
  sample of the family's keys.** Measured **0 scanned bytes / ~25 ms** per probe while
  honouring the same `--where` scope. Two constraints found during implementation, both
  verified on a live tenant:
  - **The keys cannot be combined into one query.** A multi-aggregation
    `timeseries a = count(k1), b = count(k2), …` returns *no rows at all* when any single key
    has no data — one quiet key erases the evidence of the live ones.
  - **One key is not a family.** Resolving the glob to a single representative produced a
    confident `empty` for a namespace whose k8s metrics were demonstrably flowing: the
    arbitrary pick (`dt.kubernetes.container.cpu_throttled`, first alphabetically) is a key
    nobody emits, while `…cpu_usage` was reporting 25 datapoints a minute.

  So up to 8 keys are probed, one free query each. A hit settles the family. A miss across the
  sample becomes `empty` **only if the sample was the whole family and the catalog was
  complete** — otherwise `unknown`, naming how many of how many keys were checked. Metric
  signals report datapoints rather than records.
- **D7 — Three-state verdict preserved.** Truncated, scan-capped, or errored probe →
  `unknown`. Absence evidence distinguishes catalog-absent / retention-empty / window-empty.
- **D8 — Exit codes are opt-in via `--require`.** Plain `inventory` stays exit-0.
- **D9 — No entity or repository joins.** Domain modelling belongs on the product side;
  Grail-only tenants have no `dt.entity.*`.
- **D10 — No `--wait-for` on `ingest` in v1.** `ingest` is pass-through by design and cannot
  know what identifies a payload; inferring a discriminator would mean introspecting the body.
  Ship the documented `ingest` → `inventory --since` / `wait query` recipe instead. A future
  `ingest --wait-for '<dql-filter>'` — where the *user* supplies the discriminator — stays
  compatible with pass-through and can be added without rework.
- **D11 — New `arrivals` block on `Inventory`.** Additive; existing consumers unaffected.

## Open questions — with recommendations

All five carry a recommendation; three were settled empirically on `gmg`, 2026-09-11.

### 1. Time fields for the remaining streams — RESOLVED

`timestamp` works for `logs`, `bizevents`, `events`, `security.events`, and `user.events`;
`spans` requires `start_time`. The default-plus-one-override model in D5 holds, with `spans`
as the only known override. `dt.synthetic.events` and `dt.davis.events` are absent from that
tenant and remain unverified — treat an unresolvable time field as `unknown`, not `absent`.

### 2. The `metrics` "result cap" — RESOLVED, and it exposed a bug in `main`

**It is not a result cap.** `metrics | summarize count()` returns exactly 100,000 alongside
this notification:

```
FETCH_EXEC_TIME_LIMIT (WARNING) — "Your result is incomplete because the data couldn't be
read within the internal time limit of 10008 ms. Please try narrowing your timeframe."
```

So the 100,000 is wherever a **10-second internal read limit** cut the read, not a fixed
ceiling. Nothing should hardcode it.

**And sometimes there is no notification at all.** The same catalog query scoped to a 15m
window returned exactly 100,000 keys in 3.9 s with an empty `notifications` array — a silent
cap. Notification-based truncation detection is necessary but not sufficient: the read site
must also treat "returned exactly as many rows as the limit" as truncation, which is what
`metricCatalogLimit` in `sdk/inventory/discover.go` now does.

**Recommendation — fix `classifyNotification` first, as a standalone bug fix.**
`pkg/exec/dql.go:481` handles `FETCH_TIMEOUT` but **not** `FETCH_EXEC_TIME_LIMIT`, and neither
message fallback matches this wording. It therefore returns `""`, so `ResultIsPartial`
(`pkg/exec/dql.go:509`) reports **false** and a truncated result is treated as complete.

That is a live fabricated-absence bug in `inventory` today, independent of this proposal: a
metric catalog cut short at 100k keys is read as complete, and every metric family below the
cut is reported `absent` with confident evidence. It is the same class of bug
`inventoryMaxResultRecords` (`cmd/inventory.go:158`) was introduced to prevent, arriving by a
different route.

Add `case "FETCH_EXEC_TIME_LIMIT": return notifTimeout`, plus a message fallback on
`"internal time limit"`. Once that lands, windowed mode needs no metric-specific truncation
handling at all — the existing `Truncated` → `unknown` path does the right thing.

### 3. Default window — recommend 15m, leave the cap alone, add a pre-flight warning

Scan cost is essentially linear in window width, measured on the same scope:

| Window | Scanned | Records |
|---|---|---|
| 5m (`kube-system`) | 4.1 GB | 29.1M |
| 15m (`kube-system`) | 10.8 GB | 85.1M |
| 15m (`dps-ingest`) | 6.0 GB | 84.7M |
| 30m (`dps-ingest`) | 11.7 GB | 155.3M |

Two-fold window, 1.94× the bytes. Extrapolating, the 25 GB default cap bites somewhere
between 30m and 60m depending on how selective the scope is.

**Recommendation:** default `--since 15m`. Do **not** make the scan cap window-aware — the
cap is a global safety net and silently raising it for this command would defeat its purpose.
Instead warn at parse time when `--since` exceeds 1h that probes are likely to truncate to
`unknown`, and let the existing scan-limit hint (`pkg/exec/dql.go:524`) carry the remedy. The
honest-degradation path already exists; it just needs to be reachable and explained.

### 4. Filter validity — recommend *not* validating fields, and adding a tripwire instead

I tried to resolve this with free schema metadata and the answer is that it cannot be done:

- `fetch dt.system.data_objects` carries `name`, `type`, `scope`, `owner`, `query_string`,
  `description`, `has_access`, `usable_with` — **no field list at all**.
- `describe logs` is free (0 bytes, 2 ms) but returns only the 8 **core** fields
  (`timestamp`, `content`, `ordinal`, the `dt.system.*` set). It does **not** list
  `k8s.namespace.name` — the very field that matched 237,194 records in the example above.

So neither surface distinguishes "this field does not exist" from "this field is a dynamic
attribute". Validation built on either would reject exactly the attributes people actually
scope by. **Do not build it.**

**Recommendation — two cheap measures instead:**

1. **Syntax-check** the assembled probe through the existing `verify query` path before
   running the battery. It catches typos and malformed DQL; it will not catch unknown fields,
   and the docs should say so.
2. **All-zero tripwire.** If *every* probed signal returns 0 while the streams themselves hold
   records in retention, that is far more likely a wrong `--where` than a source that failed
   on every signal at once. Emit a warning saying so rather than a confident all-absent
   verdict. Costs no extra query — both facts are already in hand.

This does not fully close the gap, and the doc should be honest that it does not.

### 5. Segment interaction — recommend composing, and let `-S` satisfy the scope requirement

**Not implemented in the first PR** — `--where` only. Segment name-to-UID resolution lives in
`cmd/query.go`, and wiring it into `inventory` is a separate change.

**Recommendation for the follow-up:** `-S/--segment` and `--where` compose with AND. They are
both filters, and a segment is simply the reusable, governed form of the same idea.

Consequently, restate D4 as *`--since` requires **a scope***, satisfied by `--where`, by
`-S`, or by both. Demanding a redundant `--where` next to an existing segment would be
busywork, and the cost argument that motivates D4 is satisfied either way.

## Implementation sketch

| Area | Change |
|---|---|
| `pkg/exec/dql.go` | **Prerequisite bug fix** (open question 2): `classifyNotification` must map `FETCH_EXEC_TIME_LIMIT` → `notifTimeout`, else truncated probes are read as complete |
| `sdk/inventory/inventory.go` | `SignalState` enum + `Signal` type; `Signals []Signal`, `Summary`, `Window` on `Inventory`; `TimeField` on `CapabilityDef` |
| `sdk/inventory/discover.go` | `DiscoverOptions.Since` / `.Where` / `.StaleAfter`; windowed branch in `evaluateCapabilities` emitting filter-first probes; state derivation from windowed count + `facts.streamRows` |
| `sdk/inventory/definitions.go` | `timeField` parse + `spans: start_time` in `BuiltinDefinitions` |
| `cmd/inventory.go` | `--since`, `--where`, `--signals`, `--require`, `--stale-after`; mutual-requirement validation; signal-state table in `printInventoryHuman`; exit-code mapping |
| `docs/` | Onboarding-verification recipe; `ingest` → verify pairing |

**Tests**
- SDK: fixture-driven `Runner` covering each of the six signal states, including the
  `empty` vs `no-data` split and the `live`/`stale` boundary at `--stale-after`; truncated
  probe → `unknown`; `--since` without `--where` rejected; `spans` uses `start_time`.
- cmd: flag validation, exit-code mapping (0/1/2), human and `--agent` golden output.
- Integration (not run in CI, Grail-only tenant per repo convention): probe shapes execute and
  return the expected columns on `logs`, `spans`, `bizevents`, `metrics`.

---

## Implementation notes

| Area | What landed |
|---|---|
| `pkg/exec/dql.go` | Prerequisite fix: `FETCH_EXEC_TIME_LIMIT` → `notifTimeout`, plus an `"internal time limit"` message fallback |
| `sdk/inventory/inventory.go` | `SignalState`, `Signal`, `ArrivalWindow`, `StateSummary`; `TimeField` on `CapabilityDef` |
| `sdk/inventory/arrivals.go` | Windowed probing, state derivation, metric-family sampling, the all-zero tripwire |
| `sdk/inventory/discover.go` | `Since`/`Where`/`StaleAfter`/`Signals` options; windowed branch; metric-catalog truncation by row count |
| `sdk/inventory/definitions.go` | `spans` → `start_time`; `timeField` validation |
| `cmd/inventory.go`, `cmd/inventory_signals.go` | Flags, the `--since`/`--where` mutual requirement, the signal table, `--require` exit codes, result-specific agent suggestions |

**Deferred:** `-S/--segment` composition (open question 5).

**Verified against a live tenant.** A 15m window scoped to one Kubernetes namespace: 8 signals
live, 3 empty, 2 unknown, in 31 queries / 13.8 s. Restricted with `--signals logs,spans` it
runs in 4 queries / 1.8 s. `--require` exited 0 with both signals live and 1 with an empty
signal required.

Two defects the live run caught that no unit test would have: the metric-family false `empty`
described in D6, and timeseries bucket timestamps landing in the future (a bucket is stamped
with its end, so the current one is ahead of `now`; it is clamped).
