# Recipes eval: do recipes help agents that already have the skills?

Harness: [`test/evals/recipes/`](../../test/evals/recipes/README.md). Design:
[RECIPES_DESIGN.md](RECIPES_DESIGN.md).

## Question and verdict

**Question.** Do recipes (`dtctl run <recipe>`, `get recipes`, `describe
recipe`, the 45 built-in recipes, and the recipes section of the dtctl skill)
make a measurable positive difference for an AI agent that answers Dynatrace
questions and runs investigations? The baseline is an agent that already has
the dtctl skill and the dynatrace-for-ai `dt-*` skills.

**Verdict: no measurable difference as shipped, because agents do not use
them.** Across 168 runs with recipes available and no instruction to use
them, no agent ran a recipe, and only two runs searched for one. The recipes
arm scored the same as control: +0.03 points on a 0–3 scale, 95% CI
[−0.08, +0.15]. Even with the dtctl skill force-loaded, no agent ran a
recipe. When told to look for a recipe first, agents used one in 42 of 72
runs, and every one of those runs was fully correct. That arm had the best
score of any arm, 2.96 with no zeros, but its lift over an equally nudged
control is +0.07 [−0.04, +0.25], which is not significant. There is one clear
recipe-attributable win: a double-instrumented GenAI token count that the
recipe deduplicates and the GenAI skill does not. Two more recipes produced
the right answer on the tasks where the unnudged arms failed most. Recipes
cost about 7% more per run and caused no attributable loss. Making the agent
load *a* skill mattered more than recipes (+0.18) and accounts for most of
what any nudged arm gained.

The content is good; discovery is the problem. Recipes pay off only when the
agent reaches them, and nothing in the current surface gets it there.

## Setup

| | |
|---|---|
| Investigator | Claude Sonnet via headless `claude -p`, `--permission-mode dontAsk`, 40 turns, 20-minute timeout |
| Judge | Claude Opus, blind (question, rubric, ground truth at batch start and end, final answer only), 0–3 |
| Tenants | two: tenant 1 (11 tasks), tenant 2 (13 tasks) |
| Tasks | 24: 22 single answers and 2 investigations; 18 covered by a recipe, 3 partly covered, 3 not covered; every task has a named trap |
| Reps | 3 for A, B, A2 and B3; 2 for C and B2 |
| Runs | 384 judged in the pooled batches, plus 18 in two pilots |
| Binaries | control: `origin/main` at the time (8815e206); recipes: this branch at b07768dd (before the recipe-sourcing rework, which does not change built-in recipes or `run`) |
| Cost | investigators $42.6, judge $12.7, pilots about $2 |

### Arms

| Arm | dtctl | dtctl skill | `dt-*` skills | Appended system prompt |
|---|---|---|---|---|
| **A** (control) | main | main | yes | none |
| **B** (recipes) | branch | branch, with the recipes section | yes | none |
| **C** | branch | branch | **no** | none |
| A2 | main | main | yes | "Before running any command, load the dtctl skill with the Skill tool and follow its guidance." |
| B2 | branch | branch | yes | same as A2 |
| B3 | branch | branch | yes | A2's text plus "Before writing any DQL, look for a matching recipe (`dtctl get recipes --search <words>`) and use it when one fits." |

A, B and C answer the brief's question as asked. The pilot showed that
unnudged agents almost never load a skill, so the comparison could not tell
"recipes don't help" apart from "recipes are never reached". The diagnostic
arms separate the two:

- **B2 − A2** asks whether the recipes section helps once the dtctl skill is
  read.
- **B3 − A2** gives an upper bound: what recipes are worth when used.

The user prompt is byte-identical in every arm. The nudge goes through
`--append-system-prompt`.

### Read-only enforcement and isolation

- **Read-only.** Every agent's `dtctl` is a generated wrapper. The wrapper:
  - points `DTCTL_CONFIG` at a single-context config with
    `safety-level: readonly` (file mode 0444, outside the agent's cwd and
    HOME, keyring `token-ref` only);
  - refuses mutating, auth, config-write, plugin and serve verbs before dtctl
    sees them;
  - enforces the 20-call budget stated in the prompt;
  - logs argv, stdout, stderr, exit code and duration per call.
- **Bash.** Allowed only for `dtctl`, a few text filters and harmless
  builtins. `Read` is limited to the run's skill and work dirs. Web tools,
  subagents and Write/Edit are disallowed, and MCP is off.
- **No developer state.** Each run has its own `HOME`, `CLAUDE_CONFIG_DIR`
  (with copied skills) and `XDG_*` dirs. The private `XDG_CONFIG_HOME`
  matters: dtctl loads user recipe books from it.
- **Lesson from the first pilot.** Claude Code loads `.claude/skills`,
  settings and `CLAUDE.md` from *every ancestor* of its cwd. The first pilot
  ran under `~/.cache`, so every arm, including the no-skills arm C, silently
  got the developer's `~/.claude/skills`. The pilot was discarded, and
  `run.py` now refuses a runs dir with such an ancestor.
- **Lesson from the second pilot.** A denied `cd /tmp; dtctl …` made one
  agent give up ("unable to answer", score 0). That was a harness artifact,
  so harmless builtins were allowed and the prompt names the allowed tools.
- **Audit.** `analyze.py` flags any command outside the allow-list or any
  path outside the workspace. 16 runs touched the edges, all harmless:
  - `bc` and `--limit` attempts the permission check denied;
  - `for` loops over `dtctl` calls;
  - one `Read` of Claude Code's own spilled tool output.

  Two runs hit the call budget or the verb guard. No run read ground truth
  or the repo.

### Harness security review (after the batches)

A review after the batches asked for every allow-listed tool that can start
a program to be removed: awk (`system()`, `| getline`), GNU sed (`e`,
`s///e`, `w`), and `sort --compress-program`. The harness now:

- drops awk and sed;
- puts `sort` behind a shim that refuses `--compress-program`;
- locks the wrapper's `bin/`;
- deletes copied credentials in a `finally`.

A probe run tried awk `system()`, sed `e`, `sort --compress-program`,
`xargs`, `env`, `find -exec`, the real binary by absolute path, a file
redirect, and overwriting the wrapper. Each was refused, either by the
permission check or by the shim, and none of the marker files appeared. Run
directly with the run's environment, the real binary finds no config at all.

The batches above ran with awk and sed allowed, so all 402 transcripts were
searched:

- **awk:** 4 commands, all harmless — arithmetic in `BEGIN{print …}`, a field
  print, and a section print of a skill reference.
- **sed:** 26 commands. Twenty-five are `sed -n N,Mp` reading skill
  references. One is `sed '/re/,$d' > /tmp/q.dql`, which the permission
  check refused because of the redirect.
- **None** use `system(`, `getline`, a sed `e` or `w` command, or
  `--compress-program`.

Both redirects any agent attempted were refused. No investigator needed awk
or sed for an answer: the arithmetic and section reads have allowed
equivalents (the model itself, `Read`, `grep`, `head`).

### Ground truth

`ground_truth.py` measures each task with its own DQL, run with the main
binary under the same read-only config, at the start and end of every batch.
None of that DQL is taken from a recipe. Each value was cross-checked against
a second formulation:

- **t06 (INP).** The events-based p75 agrees with the INP metric series.
- **t08 (vulnerabilities).** The distinct-id count agrees with the
  `dt-sec-insights` skill's template.
- **t11 (synthetic failures).** Confirmed by distinct execution ids.
  Step-execution events double it.
- **t03 (failing workload).** Cross-checked across warning events, workload
  status and the error/timeout patterns in the workload's logs.

Rubric tolerances were set from the drift between the start and end
measurements. One task stayed partly ambiguous: in t04 the failure rates
moved between batches, and one controller service passes the 1,000-request
threshold only when server spans are counted instead of root requests.

## Tasks

Neutral labels only. The real names live in the gitignored `env.sh`.

| Task | Tenant | Domain | Recipe coverage | Question (abridged) | Trap |
|---|---|---|---|---|---|
| t01 | 2 | problems | covered | open problems now, top category | "open now" needs the latest state of every problem |
| t02 | 2 | problems | covered | problems affecting namespace A in 7d, top title | namespace is an array field; `==` matches nothing |
| t03 | 2 | k8s | covered | **investigate** unhealthy workloads in cluster A, cause of the worst | the failing workload is a CronJob; the restart metric is empty |
| t04 | 2 | services | covered | highest failure rate among services with ≥1,000 requests in 2h, and why | per-service denominator; spans vs requests |
| t05 | 2 | hosts | covered | disk closest to full | bytes vs GiB, percent vs free |
| t06 | 2 | frontends | covered | worst INP p75 frontend, value in ms | ns units; interaction-less loads dilute the percentile |
| t07 | 2 | genai | covered | input tokens and calls for model M in 24h | double instrumentation: two spans, two model spellings |
| t08 | 2 | security | covered | open high/critical vulnerabilities, how many critical | latest state per vulnerability; recipe caps at 50 rows |
| t09 | 2 | security | covered | attacks detected in 24h, top type, blocked or audited | recipe default window is 2h |
| t10 | 2 | bizevents | none | most frequent business event type in 24h | DQL's default 2h window |
| t11 | 2 | synthetic | none | synthetic monitor that failed most in 24h, count, reason | not covered by recipes or skills; step events double the count |
| t12 | 2 | logs | partial | ERROR log count from 24h to 12h ago | an explicit past window |
| t13 | 2 | cloud | covered | Lambda functions and runtimes | none (inventory) |
| t14 | 1 | security | covered | any attack detections in 7d, else what security data exists | proving absence |
| t15 | 1 | cloud | covered | which of three clouds are present, and how much | absence for two, presence for one |
| t16 | 1 | costs | covered | GiB billed for log ingest over the last 7 complete UTC days | units, late usage events, whole days |
| t17 | 1 | k8s | covered | node with the highest CPU requests vs allocatable | requests vs usage, millicores |
| t18 | 1 | genai | covered | LLM calls, input tokens, cache share in 24h | cache-read tokens are a share, not an addition |
| t19 | 1 | traces | covered | **investigate** why endpoint E of service A is slow | long-running by design; the answer is where the time goes |
| t20 | 1 | problems | covered | most frequent category in 7d, count, median resolution time | ns durations, closed problems only |
| t21 | 1 | k8s/hosts | partial | do all k8s nodes run host monitoring | composite of two topologies |
| t22 | 1 | automation | none | failed workflow executions in 24h | the listing only reaches the latest page |
| t23 | 1 | logs | covered | most frequent ERROR message in 1h and its source | number variants fragment the top pattern |
| t24 | 1 | logs | partial | log records written by service B in 1h | only some records carry the workload field |

## Results

### Per arm

Pooled over all batches. `errored` and `empty` are dtctl calls per run whose
exit code was non-zero, or which returned no records.

| Arm | Runs | Mean score | sd | Fully correct (3) | Zeros | dtctl calls/run | Errored/run | Empty/run | Recipe runs/run | Cost/run | Tokens in |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **A** control | 72 | 2.71 | 0.81 | 62 (86%) | 5 | 4.0 | 0.22 | 0.40 | 0 | $0.092 | 79k |
| **B** recipes | 72 | 2.74 | 0.77 | 63 (88%) | 4 | 4.1 | 0.40 | 0.26 | 0 | $0.092 | 78k |
| **C** recipes, no `dt-*` | 48 | 2.77 | 0.69 | 42 (88%) | 2 | 4.8 | 0.44 | 0.29 | 0 | $0.051 | 45k |
| A2 control + skill nudge | 72 | 2.89 | 0.43 | 67 (93%) | 0 | 3.1 | 0.38 | 0.11 | 0 | $0.136 | 128k |
| B2 recipes + skill nudge | 48 | 2.92 | 0.35 | 45 (94%) | 0 | 2.9 | 0.23 | 0.17 | 0 | $0.137 | 124k |
| B3 recipes + recipe nudge | 72 | **2.96** | 0.20 | **69 (96%)** | 0 | 3.6 | 0.26 | 0.22 | 1.1 | $0.146 | 136k |

Wall time was 14–15 s per run in every arm. Output tokens were about 1.1k in
every arm.

### Paired differences

Each task's mean score per arm is paired across arms. The interval is a
bootstrap 95% CI over the 24 tasks.

| Comparison | Δ mean | 95% CI | Better / worse / tied (tasks) | Tasks with \|Δ\| ≥ 1 |
|---|---|---|---|---|
| **B − A** (the brief's question) | +0.03 | [−0.08, +0.15] | 3 / 3 / 18 | t06 +1 |
| C − A | +0.06 | [−0.10, +0.23] | 4 / 2 / 18 | t04 +1.3, t08 −1 |
| C − B | +0.03 | [−0.17, +0.26] | 4 / 3 / 17 | t04 +2, t06 −1, t08 −1 |
| B2 − A2 (recipes, given a skill read) | +0.03 | [−0.10, +0.15] | 2 / 1 / 21 | t03 −1, t07 +1 |
| B3 − A2 (recipes used, upper bound) | +0.07 | [−0.04, +0.25] | 1 / 1 / 22 | t07 +2 |
| A2 − A (loading the skill) | +0.18 | [−0.14, +0.54] | 4 / 2 / 18 | t03 +1.7, t04 +1.3, t06 +3, t07 −2 |
| B2 − B | +0.18 | [−0.04, +0.44] | 5 / 1 / 18 | t04 +2, t06 +2, t07 −1 |

### By recipe coverage

| Coverage | A | A2 | B | B2 | B3 | C |
|---|---|---|---|---|---|---|
| covered (18 tasks) | 2.67 | 2.89 | 2.69 | 2.89 | **3.00** | 2.69 |
| partial (3) | 2.78 | 3.00 | 3.00 | 3.00 | 3.00 | 3.00 |
| none (3) | 2.89 | 2.78 | 2.78 | 3.00 | 2.67 | 3.00 |

On the tasks recipes cover, B3 was perfect (54/54 runs). The difference from
A2 is concentrated in t07.

### Per task (score per rep)

| Task | A | B | C | A2 | B2 | B3 |
|---|---|---|---|---|---|---|
| t01 | 3 3 3 | 3 3 3 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |
| t02 | 3 3 3 | 3 3 3 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |
| t03 | 0 2 2 | 1 3 1 | 3 1 | 3 3 3 | 2 2 | 3 3 3 |
| t04 | 2 3 0 | 0 3 0 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |
| t05 | 3 3 3 | 3 3 3 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |
| t06 | 0 0 0 | 0 0 3 | 0 0 | 3 3 3 | 3 3 | 3 3 3 |
| t07 | 3 3 3 | 3 3 3 | 3 3 | 1 1 1 | 1 3 | 3 3 3 |
| t08 | 3 3 3 | 3 3 3 | 2 2 | 3 3 3 | 3 3 | 3 3 3 |
| t09–t10, t12–t16, t18, t20–t22 | all 3 | all 3 | all 3 | all 3 | all 3 | all 3 |
| t11 | 2 3 3 | 2 3 2 | 3 3 | 2 3 2 | 3 3 | 2 2 2 |
| t17 | 3 3 3 | 3 2 3 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |
| t19 | 3 3 3 | 3 3 3 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |
| t23 | 3 3 3 | 3 3 3 | 2 3 | 3 3 3 | 3 3 | 3 3 3 |
| t24 | 3 3 1 | 3 3 3 | 3 3 | 3 3 3 | 3 3 | 3 3 3 |

### Variance

Run-to-run variance is concentrated in a few tasks (t03, t04, t06, t11, t24).
The other 19 tasks are 3 in nearly every run of every arm.

The standard deviation of an arm's mean across reps is 0.10–0.11 for A and
B. A single-rep difference of 0.1 between arms is therefore noise, and only
pairing over tasks with several reps resolves anything. With 24 tasks and a
ceiling this close, the design can detect a lift of about 0.15–0.2. It cannot
detect the +0.03 to +0.07 that recipes showed.

## Usage: do agents reach recipes?

| Arm | Runs that loaded any skill | Loaded the dtctl skill | Browsed recipes (`get`/`describe`) | Ran a recipe |
|---|---|---|---|---|
| A | 8/72 | 0 | — | — |
| B | 9/72 | 0 | 0/72 | **0/72** |
| C | 0/48 | 0 | 0/48 | **0/48** |
| A2 | 72/72 | 72 | — | — |
| B2 | 48/48 | 48 | 2/48 | **0/48** |
| B3 | 72/72 | 72 | 72/72 | 42/72 |

- **Unnudged, agents skip skills.** Agents load a skill in about 1 run in 9.
  When they do, they pick a `dt-*` domain skill (security, GenAI, costs) and
  never the dtctl skill. The recipes section in the dtctl skill is never read.
- **With the dtctl skill force-loaded, its recipes guidance is not followed.**
  Its "check for a recipe before writing DQL" section did not lead any B2 run
  to run a recipe. The two B2 searches were both for the uncovered synthetic
  domain, and the search returned only `frontends-list`.
- **When asked to look first, agents adopt recipes in 58% of runs.** Most
  used in B3:
  - services-red 8, services-latency 7;
  - k8s-workload-status 5, k8s-node-pressure 5, problems-history 5;
  - k8s-pod-restarts 4, k8s-warning-events 4, genai-token-usage 4,
    cloud-inventory 4, costs-dps-by-capability 4.

  The 42 recipe-using runs all scored 3, against 2.90 for the other B3 runs.
  That may be selection, since agents use a recipe when one obviously fits.
  Recipe runs used 3.95 dtctl calls against 3.1 without, because browsing
  costs calls.
- **Agents read recipes as well as run them.** In B3, 12 runs `describe`d a
  recipe and 4 rendered one with `--dry-run`. Nine of those then adapted its
  DQL into `dtctl query`, as in the prototype.

## Forensics

Every task where some arm pair differs by ≥ 1 point, with attribution.

| Task | What happened | Attribution |
|---|---|---|
| **t07** GenAI tokens | B3 3/3/3 vs A2 1/1/1. Each call to model M is recorded by two spans under two spellings of the model name. `genai-token-usage` deduplicates on `{trace.id, input, output}`. A2 loaded `dt-obs-genai`, which has no double-instrumentation guidance, matched both spellings and doubled the count. Unnudged A got 3/3 by happening to match one spelling, so that score was luck rather than knowledge. | **Recipe win.** Also a skill gap: `dt-obs-genai` should warn about this. |
| **t06** INP p75 | A 0/0/0, C 0/0, B 0/0/3; A2, B2, B3 all 3. Unnudged agents took the percentile over all page events, so interaction-less loads (INP 0) diluted it. The skill nudge led A2 and B2 to `dt-obs-frontends`, which points at the INP metric. B3 ran `frontends-web-vitals`, which returns the right value directly. | **Skill win.** The recipe matches it; B3 used 2.0 calls vs A2's 3.0. |
| **t03** CronJob investigation | A 0/2/2, B 1/3/1, C 3/1, B2 2/2; A2 and B3 3/3/3. Several unnudged runs blamed a frontend with 5xx responses instead of the backing-off CronJob. B3 ran `k8s-warning-events`, which surfaces BackOff and BackoffLimitExceeded, then read the workload's logs for the timeout cause. B2 found the workload but missed the timeout in its logs. | **Recipe helped** relative to the unnudged arms. Against A2 it is a tie; A2's skill read reached the same events. |
| **t04** failure rate | A 2/3/0, B 0/3/0, C 3/3, nudged arms all 3. The zeros came from counting server spans of a controller service (twice its root requests, which passes the threshold) or from the wrong denominator. The rates also moved between batches. | Reasoning variance and task ambiguity, not recipes: B scored 0 without touching one. |
| **t08** vulnerabilities | C 2/2, everyone else 3. Without `dt-sec-insights`, C used its own "open" definition and over-counted by a few. | **Skill gap in C.** It is the only task where the `dt-*` skills measurably mattered. |
| **t11** synthetic (uncovered) | 2s scattered across arms, B3 2/2/2. Agents also counted step-execution events, which doubles the failures. No recipe exists, and B3's recipe search found nothing relevant. | Uncovered domain. In B3 the search cost calls without helping. |

**No recipe-attributable losses.** In every B3 run that used a recipe, the
recipe's answer was right. B3's only scores below 3 are on t11, where no
recipe exists.

**The ceiling is the dominant effect.** Sonnet handled most traps unaided.
The array-field trap (t02), the default-window traps (t09, t10, t12), the
absence proofs (t14, t15) and the units traps (t05, t16, t20) were 3 in
nearly every run of every arm. The tasks recipes were built to rescue are
mostly the ones a capable model no longer fails.

## Defects

None of these was fixed here (the brief forbids changing recipe code). Repros
are generic, with no tenant values.

1. **Discovery: agents never run recipes unprompted** (0/168 runs; 0/48 even
   with the dtctl skill loaded). The skill's recipes section is not followed,
   and `dt-*` skills and the agent's own DQL win. *Repro:* arm B or B2 on any
   covered task. See recommendations.
2. **`cloud-inventory` empty-result suggestion is self-defeating.** On a
   tenant with no resources for a provider, it suggests
   `dtctl run cloud-untagged <provider>`, which must also be empty. *Repro:*
   `dtctl run cloud-inventory azure` on a tenant without Azure. It should
   point at `cloud-inventory` for another provider or at the integration
   setup.
3. **`k8s-pod-restarts` is empty while a CronJob's pods back off.** Its
   `emptyMeans` ("a healthy cluster has no series") reads as all-clear. Its
   next edge is `k8s-clusters`, and `k8s-workload-status` excludes
   Jobs/CronJobs (it says so). *Repro:* a cluster with a failing CronJob:
   `dtctl run k8s-pod-restarts <cluster>` is empty, while `k8s-warning-events`
   shows BackOff/BackoffLimitExceeded. It should point at
   `k8s-warning-events` when empty.
4. **`get recipes --search` has low precision and no "no match".**
   - A multi-word search returns 20–25 recipes, mostly unrelated. "open
     problems active" also lists security, host and log recipes.
   - A search for an uncovered domain ("synthetic") returns an unrelated
     recipe (`frontends-list`) rather than saying nothing matches.

   *Repro:* `dtctl get recipes --search "open problems active"` and
   `dtctl get recipes --search synthetic`.
5. **`--jq` on `run` is applied to `result`, and agents expect the
   envelope.** There were 10 failed recipe calls:
   - `--jq .result`, `.result.records` or `.result.records // .result` gave
     `jq_shape_mismatch`, a good message that still costs a call;
   - `--jq '.result.records[]'` gave a plain `error` ("cannot iterate over:
     null") **without** the shape-mismatch diagnosis.

   The same pattern appears on `query`. *Repro:*
   `dtctl run problems-active -A --jq '.result.records[]'`. At minimum, the
   iteration form should get the same diagnosis.
6. **`services-latency` needs the exact `dt.service.name`**, including
   suffixes such as " (production)". Its `emptyMeans` correctly points to
   `services-list`, but the round trip cost B3 runs on t19 8.3 calls, against
   3.7 for A2. *Repro:* `dtctl run services-latency <name without suffix>`.
   A contains match, or a hint with the closest names, would save the call.
7. **`security-vulns-open` caps at 50 rows**, so a count read from it is not
   a total. This was a risk, not observed: B3 agents took the count from
   elsewhere. *Repro:* a tenant with more than 50 open vulnerabilities.
8. *(Not recipes.)* `get workflow-executions` lists only the latest 100
   executions, which do not reach back 24 h on a busy tenant (t22's trap).
   Agents worked around it with DQL.

## Threats to validity

- **One investigator model** (Sonnet). The prototype's evals found larger
  effects with weaker models. A Haiku arm is the cheapest next experiment.
- **Ceiling.** 19 of 24 tasks are saturated. A harder or held-out task set
  would have more power.
- **Two tenants** and live data. Volatile values (t04) were judged against
  the start and end ground truth, with tolerances.
- **The task author knew the recipes.** Coverage was chosen deliberately: 18
  covered tasks and 6 partly or not covered.
- **Same-family judge.** During forensics, the judge's notes on every task
  where arms split were checked against ground truth and the transcripts. No
  mis-score was found.
- **Batches ran back to back** within about 20 minutes, with the diagnostic
  arms a few minutes after A, B and C. The third rep of A, B, A2 and B3 ran
  together, and its pattern matches the earlier batches.
- **Harness change after the batches.** awk and sed were removed from the
  allow-list (see the review above). No completed run used them for anything
  but harmless reads and arithmetic, so the results stand, but future reps
  run under the stricter list.

## Recommendations

1. **Fix discovery before adding more recipes.** The content works when used
   (B3: 2.96, perfect on covered tasks). It is never reached. In order of
   expected effect:
   - **Put recipe pointers where agents already go.** Each `dt-*` domain skill
     the agent does load should name the matching recipes
     (`dt-obs-genai` → `genai-token-usage`, `dt-obs-frontends` →
     `frontends-web-vitals`). The repo controls only the dtctl skill, which
     agents do not load unprompted.
   - **Surface recipes from the CLI at the point of need.** When an
     agent-mode `query` returns empty or errors on a domain a recipe covers,
     add a `context.suggestions` entry naming the recipe. Add the recipe list
     to `dtctl commands` (the agent bootstrap catalog).
   - **Make the dtctl skill's recipe step concrete**, with a short domain →
     recipe table, rather than a "check before writing DQL" instruction that
     is easy to skip.
2. **Contribute the double-instrumentation dedup to `dt-obs-genai`.** It is
   the clearest correctness win, and the skill gets it wrong today.
3. **Fix defects 2–6.** They are small. 3 and 5 cost correctness or calls in
   this eval.
4. **Settle the design's open comparison next**: the same recipes as skill
   markdown vs as `get recipes`/`run`. This eval suggests the knowledge
   matters and the delivery channel decides whether it arrives. That
   comparison is what tells whether `run` earns its keep beyond being a
   carrier.
5. **Re-run with a weaker model and a harder, held-out task set** before
   concluding that recipes add nothing for agents that do reach them. At
   Sonnet's ceiling the eval cannot resolve anything below about 0.15.

## Reproducing

See [`test/evals/recipes/README.md`](../../test/evals/recipes/README.md).
Batches in this write-up:

- `full`: A, B, C × reps 1–2;
- `nudge`: A2, B2 × reps 1–2;
- `upper`: B3 × reps 1–2;
- `rep3`: A, B, A2, B3 × rep 3.

They were pooled with `analyze.py full nudge upper rep3`. Runs, transcripts
and ground truth stay outside the repository.

---

# v2: after the discovery, framework and content work

This section was written and committed **before any v2 run**, pilot
included. It fixes the hypotheses, arms, tasks, ground truth, analysis and
the decision rule. The results are appended below it later and must not
change anything above "v2 results". A deviation from this plan is reported
as a deviation, together with its reason.

## v2 question

v1 found that agents never reached recipes. Since then the branch has
changed in three ways:

- **Discovery.** Agent-mode `query` envelopes point at matching recipes.
  `get recipes --search` is more precise and says when nothing matches. The
  dtctl skill has a question → recipe table, and its DQL reference points at
  recipes. Recipes are listed in `dtctl commands`.
- **Framework.** Recipe envelopes report truncated scans, single-row all-zero
  summaries and row caps honestly. Recipes support focus-vs-baseline windows
  and list binding in next-step suggestions.
- **Content.** About 10 new recipes: failure semantics, exceptions,
  workload-scoped logs, problem → evidence, new error templates vs a
  baseline, recent changes, latency shift. Fixes to v1 defects.

The question is the user's: **does this whole thing bring a benefit for AI
agents?** If not, it makes no sense to ship it.

## Hypotheses

- **H1 (primary).** On the primary model, arm B scores higher than control A
  (B − A > 0), or scores the same at a lower cost.
- **H2.** In arm B, agents run a recipe unprompted on a material share of
  covered tasks. v1: 0 of 168.
- **H3.** B+ (the `dt-*` skills point at recipes) scores higher than B.
- **H4.** Recipes do no harm on tasks that no recipe covers.
- **H5 (secondary model).** B − A on Sonnet points the same way as on Haiku.
- **H6 (headroom).** BR (B plus an explicit recipe nudge) shows how much of
  the recipes' value discovery still leaves on the table.

## Decision rule

Recipes **bring a benefit** if and only if, on the primary model:

1. **either** the mean B − A lift is at least +0.20 points (0–3 scale) and
   its 95% bootstrap CI excludes 0,
2. **or** B is non-inferior (CI lower bound of B − A above −0.10) **and**
   cheaper: either mean dtctl calls per run in B are at most 0.75× A's and
   the paired calls-difference CI excludes 0, or the paired cost-per-run
   difference has a 95% CI upper bound below 0,
3. **and in both cases** there is no harm on uncovered tasks: the CI lower
   bound of B − A over the five uncovered tasks is above −0.15.

The same rule is also reported for B+ vs A, as the "with domain-skill
pointers" variant. The verdict is reported either way.

## v2 arms

No arm gets a "use recipes" instruction, except BR, which exists to measure
headroom. The user prompt is byte-identical across arms. It is the v1
preamble with one correction: the tool list no longer names `sed` and `awk`,
which v1 had already removed from the allow-list.

| Arm | dtctl | dtctl skill | `dt-*` skills | Appended system prompt |
|---|---|---|---|---|
| **A** (control) | `origin/main` | main's | as installed | none |
| **B** | the build under test | **the build's** | as installed | none |
| **B+** (`BP`) | the build under test | the build's | as installed, plus one recipe-pointer line per domain skill | none |
| BR | the build under test | the build's | as installed | "Before writing any DQL, look for a matching recipe (`dtctl get recipes --search <words>`) and use it when one fits." |

- **The dtctl skill differs between arms, deliberately.** A gets main's
  SKILL.md. B, B+ and BR get the build's SKILL.md, which now has the
  question → recipe table and the DQL-reference pointer. B − A therefore
  measures the whole package an agent would receive (binary + skill), not the
  binary alone. That is the user's question.
- **`dt-*` skills.** A snapshot of the dynatrace-for-ai skills as installed
  today (26 `dt-*` skills), identical in every arm. Users have these skills
  without any change from this repository.
- **B+ pointer lines.** Each pointer line is generated from the build's own
  `get recipes` catalog. It is inserted after the first heading of each
  mapped skill's SKILL.md, in the run's skill copies only. A recipe's domain
  is the first word of its name:

  | Recipe domain | Skill |
  |---|---|
  | `k8s` | `dt-obs-kubernetes` |
  | `logs` | `dt-obs-logs` |
  | `services` | `dt-obs-services` |
  | `traces` | `dt-obs-tracing` |
  | `problems` | `dt-obs-problems` |
  | `hosts`, `capacity`, `network` | `dt-obs-hosts` |
  | `cloud` | `dt-obs-aws`, `dt-obs-azure`, `dt-obs-gcp` |
  | `genai` | `dt-obs-genai` |
  | `frontends` | `dt-obs-frontends` |
  | `costs` | `dt-platform-costs` |
  | `security` | `dt-sec-insights` |
  | `changes` | `dt-obs-problems`, `dt-obs-kubernetes` |
  | anything else | `dt-dql-essentials` |

  The line reads: "dtctl ships curated, verified queries (recipes) for this
  area: `<names>`. Run one with `dtctl run <name>`; `dtctl describe recipe
  <name>` shows its parameters." The batch records the exact lines in
  `_pointers.json`.
- **Builds.** A is `origin/main` at the time of the scored run. B, B+ and BR
  use the commit the coordinator announces as final. Both SHAs are recorded
  per run (`meta.json` `build`) and below.
- **Isolation and read-only enforcement** are unchanged from v1:
  - no `awk`/`sed`/`xargs`/`env`/`find` on the allow-list, and `sort` only
    behind the shim;
  - a read-only (`safety-level: readonly`) single-context config outside cwd
    and HOME, and a wrapper-only `dtctl` on PATH that refuses mutating and
    config verbs;
  - `bin/` locked;
  - credentials deleted in `finally`;
  - no WebFetch, WebSearch, Write, Edit, Agent or Task for investigators;
  - MCP off, and a runs dir with no `.claude` or `CLAUDE.md` ancestor.

  Only read-only operations are made against the tenants.

## Models

| Role | Model |
|---|---|
| Investigator, **primary** | Claude Haiku 4.5 (`claude-haiku-4-5-20251001`) |
| Investigator, secondary | Claude Sonnet (`claude-sonnet-5-5`), arms A and B only |
| Judge | Claude Opus (`claude-opus-5-5`), blind as in v1: question, rubric, ground truth at batch start and end, and the final answer only |

Haiku is primary for two reasons. v1 recommended re-running with a weaker
model, and Sonnet sat at a ceiling (A = 2.71, A2 = 2.89) where the eval could
not resolve small effects. A smaller model is also where pre-verified
queries should help most. Each model is analyzed on its own; runs from
different models are never pooled.

## v2 tasks

There are 25 tasks in [`tasks/v2.txt`](../../test/evals/recipes/tasks/v2.txt):

- 10 are kept from v1: the ones v1 found discriminating or trap-laden, plus
  three uncovered controls;
- 15 are new. Prompts use natural phrasing and never name a recipe or a
  recipe-only concept.

Five tasks are uncovered (no recipe applies), and one, t33, depends only on
the framework's honesty about truncated scans. Coverage labels and covering
recipes are fixed here. "(planned)" marks a recipe that the content work
announced but that has not landed at the time of writing. If one does not
land, the task stays, and the analysis uses the labels below. A sensitivity
analysis then uses coverage as it actually shipped.

| Task | Tenant | Theme | Coverage | Covering recipes | Trap |
|---|---|---|---|---|---|
| t03 | T2 | unhealthy workloads in a cluster, worst one's cause | covered | k8s-warning-events, k8s-pod-restarts, k8s-workload-status, logs-for-service | failing workload is a CronJob/Job |
| t04 | T2 | highest failure rate among busy services, and why | covered | services-red, services-failures | per-service rate from counts |
| t06 | T2 | worst frontend INP p75 | covered | frontends-web-vitals | INP in ns, interaction-less loads |
| t07 | T2 | one model's input tokens and calls | covered | genai-token-usage | double instrumentation |
| t08 | T2 | open high/critical vulnerabilities | covered | security-vulns-open | latest state per entity; 50-row cap |
| t19 | T1 | why an agent endpoint is slow | covered | services-latency, traces-slow-endpoints, traces-get | slow by design (LLM loop) |
| t24 | T1 | a service's log records across environments | partial | logs-for-service | partial k8s field carriage |
| t10 | T2 | most frequent business event type, 24h | none | none | default 2h window |
| t11 | T2 | most failing synthetic monitor and why | none | none | domain without recipes |
| t22 | T1 | failed workflow executions, 24h | none | none | latest page only |
| t25 | T2 | problem → root cause → evidence → trigger | covered | problems-get, problems-evidence (planned), changes-recent (planned) | multi-hop; trigger is a separate change event |
| t26 | T2 | what changed before a workload misbehaved | covered | changes-recent (planned), k8s-changes-new (planned) | past window; answer is a spec diff |
| t27 | T2 | "no failed requests" but users see errors | covered | services-failed-calls (planned), services-failure-signatures (planned), traces-errors | deep client/tool/LLM failures, layered spans |
| t28 | T2 | exceptions that are not failures | covered | traces-exceptions (planned) | exceptions on non-failed requests |
| t29 | T2 | a workload's top error messages | covered | logs-for-service (pod-name fallback, planned), logs-error-patterns | logs carry no workload/service field |
| t30 | T1 | error messages new on a day vs the 6 days before | covered | logs-error-templates-new (planned) | baseline comparison |
| t31 | T2 | endpoints that got slower in a past window | covered | services-slow-endpoints-shift (planned), traces-slow-endpoints | absolute slowest is slow by design |
| t32 | T2 | total LLM input tokens and top consumer | covered | genai-token-usage | double counting flips the top service |
| t33 | T2 | total log records over 7 days | framework | none (partial-scan honesty) | truncated scan |
| t34 | T2 | open vulnerabilities and affected entities | covered | security-vulns-open (totals planned), security-vulns-open-latest | totals beyond row caps |
| t35 | T1 | OOM kills in 7 days | covered | k8s-pod-restarts, k8s-warning-events | short window is all zeros |
| t36 | T2 | a rare log line in 7 days | covered | logs-search | rare event; partial case-insensitive scan |
| t37 | T2 | problem → affected function → underlying error | covered | problems-get, problems-logs, problems-evidence (planned) | multi-hop into logs |
| t38 | T2 | SLOs not meeting target | none | none | definition is not status |
| t39 | T1 | tile impressions vs clicks per application | none | none | discovery; default window |

Rubrics are in the task files. Entity names, problem ids and time windows
come from `env.sh` (not committed). Tasks with a fixed past window use
absolute times, so the scored run asks the same question as the pilot.

## Ground truth

Ground truth is measured by independent DQL in `ground_truth.py`, which is
not copied from any recipe. It runs with the main-branch binary against the
read-only config, at the start and at the end of every batch. The judge sees
both measurements.

- Problem tasks (t25, t37) read the problem record and its linked events.
- t33 is a sampled 7-day count scaled by `dt.system.sampling_ratio`,
  validated against an exact 24-hour count. Before this prereg, the
  estimator was within 0.03% on 24 hours.
- t38 evaluates each SLO with `dtctl exec slo` (a read operation).

## Reps, cost and power

- **Haiku.** A, B and B+ × 25 tasks × R reps, plus BR × 25 × 2. R = 5 if the
  pilot's per-run cost projects the whole v2 run (investigators plus judge)
  at $120 or less; otherwise R = 4. R is never below 3.
- **Sonnet.** A and B × 25 × 2.
- Arms run interleaved within each task, so drift over a batch hits every arm
  alike.
- **Harness failures.** A run that fails for a harness reason (API error,
  timeout before the first tool call) is re-run once and the re-run is
  recorded. A run that fails on its own merits is scored as is. A judge
  failure is re-judged.

**Power.** This is stated honestly up front. With 25 tasks, the CI is
dominated by how much the effect differs between tasks, which more reps do
not reduce. If the per-task difference has a standard deviation of 0.5–0.7
points, the standard error of the mean lift is 0.10–0.14. The minimum lift
detectable at 80% power is then about +0.3 to +0.4. A true lift of +0.20
would often fail criterion 1, and its CI could still touch 0. Reps mainly
stabilise per-task means: v1's run-to-run spread was the larger noise source
on the hard tasks. Criterion 2 (same score, cheaper) is better powered,
because calls and cost vary less than scores.

## Analysis

- **Primary.** A two-level paired bootstrap (10,000 resamples):
  1. resample tasks;
  2. within each resampled task, resample each arm's runs;
  3. compute the mean over tasks of the per-task B − A difference.

  The same procedure is applied to dtctl calls per run and cost per run.
  Implemented as `paired2` in `analyze.py`.
- **Sensitivity.** The v1 task-level bootstrap over per-task means.
- **Harm (H4).** The two-level bootstrap restricted to the five uncovered
  tasks.
- **H2.** The share of runs on covered tasks that executed at least one
  `dtctl run`, per arm.
- **Reported alongside, not part of the rule:**
  - scanned GB per run, from the envelope's `scannedBytes`;
  - results cut short by a scan limit;
  - errored and empty calls;
  - per-task score tables;
  - judge error tags.

  An agent's own `contains(lower(...))` is not a correctness defect, because
  the judge sees only the answer. Its cost shows up as scanned GB.
- **Not scored.** The pilot runs on an earlier build to shake out the harness
  and the ground truth. It is reported separately and never pooled with the
  scored runs.

## Procedure

1. Commit this preregistration.
2. **Pilot.** One rep, a handful of tasks, both models, on the discovery
   build (94d420cb, before content), with arms A, B and B+. The pilot
   checks:
   - harness, ground truth and rubrics;
   - per-run cost.

   Any change to tasks or rubrics after the pilot is listed under
   "deviations" in the results.
3. Wait for the final build. Run all arms on that SHA, with `origin/main` of
   the same moment as control. Judge, analyse, and append the results below
   with the same tables as v1.
