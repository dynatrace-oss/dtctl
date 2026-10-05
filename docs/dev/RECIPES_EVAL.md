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

---

# v2 results

Everything above this line is the preregistration and is unchanged.

## v2 verdict

**Recipes help Haiku a lot, but the primary arm misses the preregistered
rule on its no-harm check. The variant with domain-skill pointers passes.**

- **B − A (primary): +0.60 points [+0.22, +0.99]** on the 0–3 scale. That
  clears criterion 1 by a wide margin. But criterion 3 fails: on the five
  uncovered tasks, B − A is +0.28 with a CI of [−0.40, +1.00], and the rule
  needs the lower bound above −0.15. **By the rule as written, B is "no
  benefit".** The point estimate on uncovered tasks is positive. The
  check fails because five tasks give a wide CI, and one uncovered task
  (t11) went down by 0.6. Nothing in that task's runs touches a recipe (see
  "Where recipes made answers worse").
- **B+ − A: +0.46 [+0.10, +0.82]**, uncovered +0.56 [−0.00, +1.24]. **B+
  meets every criterion: benefit.**
- **H2 (agents reach recipes) holds.** Unprompted, agents ran a recipe in
  49 of 90 covered-task runs in B and 47 of 90 in B+. In v1 the figure was
  0 of 168.
- **H3 (pointers beat B) is not supported.** B+ − B is −0.14 [−0.47, +0.20].
- Cost is a wash: $0.106 per run in B vs $0.108 in A. Calls are 9% lower
  (16.5 vs 18.1, CI excludes 0), short of criterion 2's 25%.
- **H5 (Sonnet points the same way): yes, but not significantly.** Sonnet
  B − A is +0.18 [−0.02, +0.44] (A 2.66, B 2.84), with no task worse. Sonnet
  sits near the ceiling, ran a recipe in only 6 of 36 covered-task runs, and
  cost $0.010 more per run [+0.002, +0.020]. On Sonnet the rule is not met.
- **H6 (headroom): BR's nudge adds a little more.** BR − A is +0.84
  [+0.30, +1.36]. BR − B is +0.24 [−0.18, +0.66], not significant, but it
  takes 3.6 fewer calls [−5.8, −1.5]. Discovery now captures most of what
  the nudge gets.

What the user asked: does this whole thing bring a benefit for AI agents?
On the primary model, yes in effect size: a 54% higher mean score, half as
many zero scores (30 vs 61 of 125), and the same cost. The strict
preregistered rule is not met for B, because the no-harm check is
underpowered on five tasks. It is met for B+.

## Deviations from the preregistration

1. **Pilot build.** The plan said to pilot on the discovery build
   (94d420cb). The pilot ran on the final build, 7095ff0a, because the
   content landed before the pilot could run (a spend limit delayed it). It
   is still reported separately and not pooled.
2. **Harness: explicit deny rules.** In the pilot, claude ran `sed` and
   `xargs wc -l` without an allow rule. It treats them as read-only on its
   own. The preregistration promised no `sed`/`xargs` for investigators. So
   before the scored runs, every program that can run another one was added
   to `--disallowedTools` (`sed`, `awk`, `xargs`, `env`, `find`, `python`,
   `perl`, `node`, shells, `tee`, `cp`, `mv`, `rm`, `curl`, …). A probe run
   confirmed that each is refused and the allow-listed filters still work.
   This applies to every arm alike.
3. **Analysis fixes, no rule change.** The policy audit now separates
   attempts that claude's permission check denied from calls that ran. It
   skips heredoc bodies and joins continued lines before reading program
   names. Reads of a run's own files (a spilled query result, claude's
   persisted tool output) count as inside the workspace. For scanned GB, a
   failed query counts as zero, and so does query metadata without
   `scannedBytes` (timeseries and entity queries scan no bytes). Only a
   successful call whose output dropped the metadata (`--jq`, csv) counts as
   unknown. A per-arm summary table with a score CI was added.
4. **`services-failed-calls` did not ship.** t27 stays "covered": it is
   still covered by `services-failure-signatures` and `traces-errors`, both
   of which shipped. All other "(planned)" recipes shipped, so the
   coverage-as-shipped sensitivity analysis equals the primary one.
5. **Spend cap.** The coordinator capped total spend at $100, not the
   prereg's $120 threshold for R. The pilot projected about $85, so R = 5 as
   planned.
6. **Account spend limit.** The account's spend limit hit during the run.
   It refused the Sonnet batch (every run got a 429 before its first tool
   call) and 27 of the 50 BR judgements. Under the harness-failure rule,
   these were re-run once after the limit reset. The 27 missing BR
   judgements were judged, and all 50 BR runs are now judged. The Sonnet
   batch was re-run in full with fresh ground truth: 100 runs, all rc 0, no
   429, all judged. No investigator run that had made a tool call was
   discarded.
7. Ground truth, rubrics and task prompts are unchanged since the
   preregistration.

## Builds and runs

- Control A: `origin/main` 8815e206 with main's dtctl skill.
- B, B+ and BR: docs/recipes-design 7095ff0a with that build's dtctl skill
  (55 built-in recipes). B+ pointer lines went into 14 `dt-*` skills.
- Haiku: A, B and B+ × 25 tasks × 5 reps (375 runs, batch `v2-haiku`), plus
  BR × 25 × 2 (`v2-haiku-br`). Sonnet: A and B × 25 × 2 (`v2-sonnet`).
- Every run finished with rc 0. No run was re-run, apart from those under
  deviation 6.
- Spend: $79.73 in total, investigators plus judge, pilots included.
  Haiku scored: $54.38. BR: $6.54. Sonnet: $12.92. Pilots: $5.88. That is
  under the $100 cap.

## Haiku (primary)

### Summary per arm

| arm | runs | score [95% CI] | dtctl calls | cost $/run | scanned GB/run (mean, median) | runs with a scan-limited result | recipe runs/run | runs with ≥1 recipe |
|---|---|---|---|---|---|---|---|---|
| A | 125 | 1.12 [0.78, 1.47] | 18.1 | 0.108 | 75.4, 1.4 | 47 | 0.00 | 0/125 |
| B | 125 | 1.72 [1.39, 2.06] | 16.5 | 0.106 | 90.7, 0.9 | 44 | 1.16 | 52/125 |
| B+ | 125 | 1.58 [1.23, 1.93] | 16.7 | 0.104 | 82.9, 0.6 | 41 | 1.08 | 48/125 |
| BR (nudged) | 50 | 1.96 [1.54, 2.38] | 12.9 | 0.091 | 123.2, 0.8 | 9 | 1.72 | 31/50 |

- Fully correct (3) / zero: A 28 / 61, B 48 / 30, B+ 43 / 37 of 125.
- Runs that hit the 20-call budget: A 67, B 52, B+ 52.
- **Scanned GB is dominated by one task.** t33 (count all log records over 7
  days) accounts for 1,429 GB per run in A, 1,781 in B and 1,835 in B+.
  Without t33 the means are 19.0 (A), 20.3 (B) and 9.9 (B+) GB per run. The
  medians (1.4, 0.9, 0.6) show that a typical run scans less with recipes.
  Calls whose output carried no scan metadata (`--jq`, csv): A 58, B 49,
  B+ 39 of about 2,100 per arm.

### Decision rule (two-level bootstrap, 10,000 resamples)

| pair | score diff | calls diff | cost diff $ | uncovered tasks score diff | verdict |
|---|---|---|---|---|---|
| **B − A** | **+0.60 [+0.22, +0.99]** | −1.60 [−3.09, −0.14] | −0.002 [−0.013, +0.008] | +0.28 [−0.40, +1.00] | criterion 1 met; **criterion 3 fails** (−0.40 ≤ −0.15) → no benefit by the rule |
| **B+ − A** | **+0.46 [+0.10, +0.82]** | −1.43 [−2.93, +0.07] | −0.005 [−0.014, +0.005] | +0.56 [−0.00, +1.24] | criteria 1 and 3 met → **benefit** |
| B+ − B | −0.14 [−0.47, +0.20] | +0.17 [−1.12, +1.50] | −0.002 [−0.011, +0.006] | +0.28 [−0.28, +0.80] | H3 not supported |
| BR − A | +0.84 [+0.30, +1.36] | −5.21 [−7.99, −2.47] | −0.017 [−0.033, −0.000] | +0.20 [−0.64, +1.16] | headroom only, not part of the rule |
| BR − B | +0.24 [−0.18, +0.66] | −3.61 [−5.83, −1.46] | −0.014 [−0.029, +0.001] | −0.08 [−0.72, +0.64] | headroom only |

BR has 2 reps per task and the other arms have 5. The two-level bootstrap
handles the unequal counts.

Sensitivity check (task-level bootstrap, v1's method): B − A +0.60
[+0.30, +0.93], with B better on 19 tasks, worse on 4 and tied on 2.
B+ − A +0.46 [+0.20, +0.72], better on 17, worse on 6, tied on 2.

### By coverage

| coverage | tasks | A | B | B+ |
|---|---|---|---|---|
| covered | 18 | 0.99 | 1.73 | 1.48 |
| partial | 1 | 3.00 | 2.60 | 2.40 |
| framework (t33) | 1 | 1.20 | 1.80 | 1.80 |
| none | 5 | 1.20 | 1.48 | 1.76 |

### Per task (score per rep)

| task | coverage | A | B | B+ | B − A |
|---|---|---|---|---|---|
| t03 | covered | 3 2 0 1 0 | 3 0 2 0 1 | 2 2 2 0 0 | 0.0 |
| t04 | covered | 0 0 3 0 0 | 2 3 3 3 2 | 3 2 0 2 0 | +2.0 |
| t06 | covered | 0 0 0 0 0 | 0 0 3 0 3 | 0 0 3 0 3 | +1.2 |
| t07 | covered | 0 0 0 0 0 | 3 3 3 3 3 | 0 2 3 0 3 | +3.0 |
| t08 | covered | 0 3 0 3 0 | 3 0 3 3 3 | 3 3 3 0 0 | +1.2 |
| t10 | none | 0 1 3 0 0 | 1 1 1 1 1 | 1 3 1 3 0 | +0.2 |
| t11 | none | 2 3 2 2 2 | 2 2 2 2 0 | 2 2 2 2 2 | −0.6 |
| t19 | covered | 0 0 2 2 0 | 3 2 1 2 2 | 2 1 1 1 2 | +1.2 |
| t22 | none | 0 3 3 0 3 | 3 3 3 3 3 | 3 3 3 3 3 | +1.2 |
| t24 | partial | 3 3 3 3 3 | 3 1 3 3 3 | 3 3 3 0 3 | −0.4 |
| t25 | covered | 3 3 0 2 2 | 3 3 3 3 3 | 3 3 3 3 3 | +1.0 |
| t26 | covered | 0 0 2 1 3 | 3 3 0 0 3 | 2 2 1 1 3 | +0.6 |
| t27 | covered | 1 1 0 1 2 | 2 0 1 2 3 | 3 2 1 1 2 | +0.6 |
| t28 | covered | 0 0 0 0 0 | 1 1 1 0 1 | 0 1 0 0 1 | +0.8 |
| t29 | covered | 0 3 3 0 3 | 0 2 2 0 2 | 2 2 0 1 0 | −0.6 |
| t30 | covered | 2 1 0 0 1 | 3 0 1 1 0 | 2 1 0 0 0 | +0.2 |
| t31 | covered | 3 0 1 0 0 | 2 0 0 0 3 | 3 0 0 3 3 | +0.2 |
| t32 | covered | 0 0 0 0 2 | 1 0 0 2 3 | 1 0 0 0 2 | +0.8 |
| t33 | framework | 3 0 0 0 3 | 0 3 3 0 3 | 3 3 3 0 0 | +0.6 |
| t34 | covered | 2 2 0 3 0 | 2 2 3 0 2 | 0 3 1 0 0 | +0.4 |
| t35 | covered | 2 2 0 2 0 | 3 1 0 2 2 | 3 3 3 3 3 | +0.2 |
| t36 | covered | 3 0 3 3 3 | 3 3 3 3 3 | 0 3 3 3 3 | +0.6 |
| t37 | covered | 1 1 1 1 1 | 0 1 1 1 1 | 0 1 1 1 1 | −0.2 |
| t38 | none | 2 2 1 0 1 | 2 1 2 1 0 | 2 1 3 2 2 | 0.0 |
| t39 | none | 0 0 0 0 0 | 3 0 0 0 0 | 0 1 0 0 0 | +0.6 |

### Do agents reach recipes? (H2)

- Unprompted recipe use on covered tasks: A 0/90, **B 49/90, B+ 47/90**
  (v1: 0/168). Across all tasks, 1.16 recipe runs per run in B and 1.08 in
  B+.
- `get recipes` browsing: B 29 runs, B+ 31.
- Most-run recipes in B: `services-failures` 16, `services-red` 15,
  `genai-token-usage` 12, `problems-evidence` 12, `k8s-warning-events` 11,
  `k8s-pod-restarts` 8. B+'s pointers shifted use toward the domain they
  name: `logs-search` 10, `changes-recent` 10, `logs-error-patterns` 9.
- On covered tasks, B runs that used a recipe scored 2.02 (n = 51) and
  those that did not scored 1.53 (n = 49). For B+ the figures were 1.83 and
  1.27. This is not causal: agents pick when to use a recipe.
- Skills were almost never loaded (A 8, B 6, B+ 4 of 125 runs). The
  recipes reach agents through `dtctl` itself (the envelope hints, the
  skill's question table and `dtctl commands`), not through skill loading.

### Where the lift comes from

- **t07 (+3.0).** `genai-token-usage` deduplicates the double-instrumented
  model. Control reported that the model had no data in all 5 runs. B was
  fully correct in all 5.
- **t04 (+2.0), t25 (+1.0), t19, t06, t08 (+1.2 each).** In control, Haiku
  spent its 20 calls exploring and answered "could not retrieve within the
  limit" or "no data". `services-red`, `problems-evidence`,
  `frontends-web-vitals` and `security-vulns-open` give it one call to a
  correct table.
- **t22 (+1.2, uncovered).** This gain is not from recipes. Control
  sometimes looked in events or logs instead of workflow executions. Both
  recipe arms got it right every time, so the build's other changes (skill
  and envelope hints) helped here too. B − A measures the whole package,
  as preregistered.

### Where recipes made answers worse

Per-task B − A was negative on t11 (−0.6), t29 (−0.6), t24 (−0.4) and t37
(−0.2). B+ − A was negative on t24 (−0.6), t34 (−0.6), t29 (−0.8), t11,
t30 and t37 (−0.2 each). Each run was checked:

- **t34, open vulnerabilities, recipe-caused.** `security-vulns-open` and
  `security-vulns-open-latest` default to `--min-level HIGH`. Their `means`
  calls `total` "the answer to how many", but `total` counts only the
  vulnerabilities at or above the level. Three runs reported about 210
  (HIGH and CRITICAL only) as the number of open vulnerabilities. The true
  number is 728, so those runs scored 0–2: B r2, B+ r1 and B+ r4. The two runs
  that got 728 exactly (B r3, B+ r2) ran the same recipe, then re-ran its
  DQL from `describe recipe` without the risk filter. **Fix:
  default to all levels for a count, or make `total` say "at or above
  --min-level" and suggest `--min-level LOW` when asked "how many".**
- **t24, records across environments, recipe-caused once.**
  `logs-for-service` took the workload-only route in one run (B r2),
  giving 147,812 instead of about 210k. The recipe's partial k8s field
  carriage is the same trap v1 found. B+ r4 used the query's
  `scannedRecords` metadata as the count, which is not related to recipes.
- **t11 (uncovered), t29, t37: not recipe-caused.** t11's loss is one run
  (B r5) that concluded no monitor failed after struggling with spilled
  1,000-row results; no recipe was involved. t29 swings with run-to-run
  noise in both arms: the same pod-name attribution trap, and double counts
  over two hours. t37 is 1 in nearly every run of every arm. Agents stop
  at the problem's description. `problems-evidence` (used in 3 B runs)
  does not lead them on to `problems-logs`, which holds the actual error.
  So recipes did not help t37, but they did not hurt either.

### Policy audit

- **Calls that ran outside the allow-list: 2 runs** (3 with BR). One used
  `nl` to number lines. One set two shell variables with `$(date …)` and
  echoed them. In BR, one used `paste`. None of them can run another
  program. No run read or wrote outside its
  workspace, and none touched ground truth or the repository.
- **Attempts that claude's permission check denied: 41 runs (82
  findings).** These were mostly `python3`, `sed`, `bc`, output redirection,
  and Write, which does not exist for investigators.
- No mutating verb reached a tenant. The wrapper refused 5 calls for a
  disallowed verb. Every other refusal (267) was the call budget.

### Headroom: BR (nudged to look for a recipe first)

- With the nudge, agents ran a recipe in 31 of 50 runs (29 of 36 on
  covered tasks) and browsed the catalog in 37. That is not unprompted use.
- BR is the best Haiku arm, at 1.96, with the fewest calls (12.9) and the
  lowest cost ($0.091). By coverage: covered 2.08, framework 3.00, none 1.40,
  partial 1.50.
- BR − B is +0.24 and not significant. Without any instruction, discovery
  already gets B most of the way to BR. The remaining gap is mostly in calls
  (−3.6 per run), not in correctness.
- Where BR lost to B: t36 r1 ran `k8s-pod-restarts` three times and
  answered with unrelated container OOM kills instead of the rare heap-OOM
  log line. A recipe steered it to the wrong entity. t24 r1 used a 2-hour
  window for a 1-hour question, which is not recipe-related.

## Sonnet (secondary)

| arm | runs | score [95% CI] | dtctl calls | cost $/run | scanned GB/run (mean, median) | runs with a scan-limited result | recipe runs/run | runs with ≥1 recipe |
|---|---|---|---|---|---|---|---|---|
| A | 50 | 2.66 [2.36, 2.90] | 4.3 | 0.084 | 142.3, 1.5 | 2 | 0.00 | 0/50 |
| B | 50 | 2.84 [2.60, 3.00] | 4.6 | 0.094 | 142.3, 1.9 | 3 | 0.22 | 6/50 |

- **B − A: +0.18 [−0.02, +0.44]** (two-level). The task-level sensitivity
  check gives +0.18 [+0.04, +0.38]: B better on 5 tasks, worse on none,
  tied on 20. Calls +0.30 [−0.58, +1.18]. Cost +$0.010 [+0.002, +0.020].
  Uncovered tasks +0.20 [+0.00, +0.60].
- By the rule: lift CI touches 0, and B is not cheaper, so there is **no
  benefit on Sonnet**. H5 holds only as a direction.
- Sonnet ran a recipe in 6 of 36 covered-task runs and browsed the catalog
  in 13 of 50. Runs used `frontends-web-vitals`, `traces-get`,
  `logs-error-templates-new`, `k8s-warning-events`, `problems-evidence` and
  `changes-recent`.
- Gains: t06 (+2, `frontends-web-vitals` gave the exact p75 INP) and t11 (+1,
  uncovered, exact count). **No task scored lower in B than in A.**
- Sonnet answered most tasks correctly in 4–5 calls, so there was little
  for recipes to add. t32 scored 1 in every run of both arms. Each run
  wrote one query, summed the doubly recorded spans, and stopped.
  `genai-token-usage` would have deduplicated them, but no Sonnet run used
  it.
- Policy: no off-policy call ran. 8 runs made attempts that claude's
  permission check denied.

## Pilot (7095ff0a, not scored, not pooled)

Haiku, 8 tasks × A/B/B+ × 1 rep: A 1.38, B 1.38, B+ 1.75. Recipe use on
covered tasks: B 4/6, B+ 3/6. Sonnet, 8 tasks × A/B × 1 rep: A 2.62, B 2.88.
The pilot found the `sed`/`xargs` gap (deviation 2) and the audit and
scan-metadata problems (deviation 3). It found no ground-truth or rubric
problem. Every low score traced to the agent's answer, and Sonnet reached
a full score on each of those tasks in at least one arm.

## v2 recommendations

1. **Ship recipes.** On Haiku, recipes turned "ran out of calls" and "found
   no data" into correct answers on the hardest tasks, at the same cost.
   Agents now find them unprompted. On Sonnet they did no harm and cost
   about 12% more per run.
   - **The pointer lines are optional.** B+ passed the rule and B did not,
     but B+ scored no better than B (−0.14 [−0.47, +0.20]). The difference
     in verdicts comes from the noise in the five-task harm check, not from
     a measured effect of the pointers.
2. **Recipe defects found here** were fixed on docs/recipes-design after
   the run. The fixes were not measured; this eval used 7095ff0a.
   - Vulnerability totals counted only `--min-level` and above (t34):
     3e0d61f0.
   - `logs-for-service` totals took the workload-only route (t24):
     29ee5d53.
   - `problems-evidence` did not lead on to the logs (t37): 29ee5d53.
3. **`k8s-pod-restarts` pulled a nudged agent away from a log question**
   (BR t36). Its `means` could say that it covers container restarts and
   OOM kills, not application heap OOM in logs.
4. **A future eval needs more than five uncovered tasks** if no-harm stays a
   CI-based criterion. With this much variance, five tasks cannot put the
   lower bound within 0.15 of the point estimate.

---

# v3: fewer turns for agents that write their own DQL

## v3 question

v2 found that recipes lift Haiku a lot, but Sonnet, which writes good DQL
itself, paid for them: +0.4 turns and +$0.010 per run for a +0.18 score that
was not significant. The transcripts show why. Sonnet writes its own query
first and meets a recipe through the query hint. It then calls `describe
recipe` to learn the params, and often rewrites its own query from the DQL.
Turns, not dtctl calls, set an agent's latency and cost: a Sonnet turn is
several seconds and most of the run's cost, and a dtctl call is under one
second.

f21b4f94 moves the recipe's value into responses the agent reads anyway:

- **Trap checks on ad-hoc queries.** `dtctl query` warns, in
  `context.warnings`, when a statement falls into a trap a recipe knows
  (GenAI tokens summed over duplicated spans, logs filtered by
  `dt.service.name`, vulnerability events counted without their latest
  state, INP percentiles that include zeros), plus the DQL lints.
- **Runnable hints.** The query hint is a complete `dtctl run` command, with
  the problem ID, service, scope and window the query already names. The run
  envelope carries `context.means`, so no `describe` is needed.
- **`run --follow`** runs the first follow-up in the same call.
- **Compact output.** A 16KB default budget on a recipe's envelope, a short
  recipe `--help`, and a leaner `traces-get`.

Does that make recipes pay off for Sonnet in turns and cost, without losing
Haiku's lift?

## v3 hypotheses

- **H7 (primary, Sonnet): fewer turns.** B − A turns per run < 0.
- **H8 (Sonnet): no cost penalty.** B − A cost per run ≤ 0.
- **H9 (Sonnet): quality holds.** B − A score is non-inferior.
- **H10 (Haiku): the lift holds**, at no extra cost.

## v3 decision rule

Two-level bootstrap (tasks, then runs within task and arm), 10,000
resamples, 95% CIs, on the v2 taskset (25 tasks):

- **Sonnet benefits** if the score diff's lower bound is above −0.15 *and*
  either the turns diff's upper bound is below 0 or the cost diff's upper
  bound is below 0.
- **Sonnet breaks even** if the score is non-inferior and both point
  estimates (turns, cost) are ≤ 0, with neither CI excluding 0.
- **Haiku keeps its lift** if the score diff's lower bound is above 0 and
  the cost diff's point estimate is at most +$0.005.

Secondary, descriptive only:

- wall time;
- recipe use on covered tasks;
- how often `describe recipe` ran;
- how often a trap warning fired and whether the next query acted on it;
- `--follow` use;
- per-task changes against v2's B (different data and day, so not a test).

## v3 arms, models, reps

- **A**: `origin/main` at the time of the run, with main's dtctl skill.
- **B**: docs/recipes-design at the commit that adds this preregistration
  (f21b4f94 plus docs and harness only), with that build's dtctl skill.
- Both arms load the dynatrace-for-ai skills (the v2 snapshot), as in v2.
- **Sonnet**: 25 tasks × 2 arms × 3 reps = 150 runs.
- **Haiku**: 25 tasks × 2 arms × 4 reps = 200 runs.
- Same 20-call budget, deny rules, judge, rubrics and ground-truth code as
  v2, with ground truth measured per batch.
- **Spend cap: $60** in total, judging included.
- A run that hits a harness failure (429 before any tool call) is re-run
  once.

The environment variables (`env.sh`) were reconstructed from the v2 runs'
rendered prompts and ground truth, since the file is git-ignored and did not
survive the worktree. The values are the ones v2 used.

---

# v3 results

Everything above this line is unchanged.

## v3 verdict

**Sonnet now benefits on all three criteria: higher score, fewer turns, lower
cost. Haiku keeps its lift.**

| Sonnet, B − A (two-level 95% CI) | v2 (7095ff0a) | v3 (c124892b) |
|---|---|---|
| score | +0.18 [−0.02, +0.44] | **+0.27 [+0.05, +0.53]** |
| turns | +0.4 | **−0.91 [−1.76, −0.05]** |
| dtctl calls | +0.30 [−0.58, +1.18] | **−1.29 [−2.44, −0.23]** |
| cost $/run | +0.010 [+0.002, +0.020] | **−0.011 [−0.021, −0.001]** |
| wall s/run | 0 | **−4 [−7, −1]** |

- **H7 (fewer turns): yes.** **H8 (no cost penalty): yes, and B is
  cheaper.** **H9 (quality): yes, B is better**, not just non-inferior. By
  the rule, **Sonnet benefits.** B is 12% cheaper and 21% faster per run
  than A, and it scores higher.
- **H10 (Haiku keeps its lift): yes, by the letter of the rule.** The lift
  is +0.57 [+0.09, +1.05]. The cost diff is +$0.0045 [−0.007, +0.016], just
  under the +$0.005 bound. Haiku's turns, calls and wall time do not
  change. But Haiku on the five uncovered tasks fell by −0.70 [−1.45,
  +0.05] (see "Haiku on uncovered tasks").
- The mechanism worked as designed. Sonnet in B never called `describe
  recipe` (0 of 75 runs; v2: 13 of 50). It ran a recipe in 21 of 54
  covered-task runs (v2: 6 of 36), and 21 of its runs took a hinted
  command.

## v3 runs

- A: `origin/main` 8815e206 (unchanged since v2). B: c124892b. Both arms use
  the v2 dynatrace-for-ai skills snapshot.
- Sonnet: 25 × 2 × 3 = 150 runs (`v3-sonnet`). Haiku: 25 × 2 × 4 = 200
  runs (`v3-haiku`). Every run finished rc 0, with no 429 and none re-run.
  All 350 were judged.
- Spend: $47.78 in total, under the $60 cap. Sonnet $19.00, Haiku $28.29,
  and a 4-run unscored smoke test $0.48. The smoke test was not in the
  preregistration; it only checked the reconstructed `env.sh`.
- Policy: no off-policy dtctl call ran. Four runs (two per arm) used
  `paste` or `date` in a pipeline, or tripped the audit's path heuristic on
  a service name. All were benign.

## Sonnet (primary for v3)

| arm | runs | score [95% CI] | turns | dtctl calls | cost $/run | wall s/run | runs with ≥1 recipe |
|---|---|---|---|---|---|---|---|
| A | 75 | 2.56 [2.21, 2.84] | 5.5 | 5.4 | 0.092 | 19 | 0/75 |
| B | 75 | 2.83 [2.59, 2.99] | 4.5 | 4.1 | 0.081 | 15 | 21/75 |

- Per task, B was better on 7 tasks, worse on none and tied on 18. The
  uncovered tasks scored 3 in every run of both arms.
- **t32 (GenAI tokens): 1 1 1 → 3 3 3.** In v2 every Sonnet run summed the
  doubly recorded spans. Now the first query's envelope carries the
  `genai-token-usage` check, and the next query deduplicates. It takes 2.3
  calls. This is the trap warning doing exactly what it was built for.
- **Fewer calls on the investigations.**

  | task | A calls | B calls | A scores | B scores | how |
  |---|---|---|---|---|---|
  | t03 | 13.7 | 6.0 | 2 3 0 | 3 3 3 | |
  | t25 | 10.7 | 3.7 | | | the hint bound the problem ID into `run problems-evidence P-…` |
  | t35 | 7.0 | 2.0 | | | |
  | t28 | 13.3 | 9.3 | 3 1 2 | 3 3 2 | |
- Trap warnings fired in 20 B runs, and the next query acted on 11 of
  them.
- One lint fired (`interval-equals-window`, twice).

## Haiku

| arm | runs | score [95% CI] | turns | dtctl calls | cost $/run | wall s/run | runs with ≥1 recipe |
|---|---|---|---|---|---|---|---|
| A | 100 | 1.10 [0.71, 1.52] | 19.0 | 17.6 | 0.101 | 57 | 0/100 |
| B | 100 | 1.67 [1.28, 2.04] | 18.3 | 16.6 | 0.105 | 57 | 52/100 |

- Fully correct runs (3): A 25, B 38. Zero-score runs: A 48, B 28.
- Unprompted recipe use on covered tasks: 49 of 72. `describe recipe`
  dropped to 2 calls (v2: 31).
- Gains of 2 points or more: t04 (+2.75), t29, t32 and t34 (+2.25 each).
- Haiku uses the query-side changes far less than Sonnet. Trap
  warnings fired in 6 runs, and the next query acted on 5 of them. The
  `multi-key-timeseries` lint fired 13 times. Haiku mostly runs recipes
  directly, as in v2.

### Haiku on uncovered tasks

| task | A scores | B scores | v2 A | v2 B |
|---|---|---|---|---|
| t10 | 3 1 1 3 | 0 0 1 1 | 0.8 | 1.0 |
| t38 | 3 0 1 3 | 1 1 1 1 | 1.2 | 1.2 |
| t39 | 2 3 0 1 | 0 0 3 0 | 0.0 | 0.6 |
| t11, t22 | | | | |

- On t11 and t22 the arms are about even.
- On t10, t38 and t39, B was worse in v3 but even or better in v2, with the
  same control binary. A's runs on these tasks did unusually well this
  time, so most of the gap is run-to-run variance on five tasks.
- The forensics found one real cost:
  - **No recipe involved.** In t10 and t38, none of the losing B runs ran a
    recipe or got a trap warning. Two t10 losses filtered
    `timestamp > now() - 24h` under the default 2h window and counted 2h
    of data. t38's B runs listed the SLOs and never evaluated them.
  - **A hint pulled the agent off course (t39 B-r4).** A `logs-search`
    hint on a `fetch logs` query led the agent to run `logs-search` three
    times. The answer was in bizevents, and the run scored 0.
  - **The label overstates generic matches.** On generic `fetch events`
    queries (`summarize count(), by:{event.type}`), the hint names
    `k8s-warning-events` "for this question". The words of the source name
    count as words of the question, and that pushes the match over the
    strong threshold.
- **Tried and reverted:** counting source words for score but not for
  strength. On a replay of the 5,024 queries from v2, this cut the strong
  hints that named an expected recipe from 375 to 245. Their precision did
  not improve (24% → 23%), and Sonnet's first-query hits fell from 17 to
  15. Sonnet's v3 gain runs on those hints, so this needs a better
  relevance signal, not a lower threshold.

## v3 recommendations

1. **Keep the v3 design.** Trap checks, runnable hints, `context.means`,
   `--follow` and the output budget turned recipes from a cost into a
   saving for Sonnet. They kept the Haiku lift.
2. **Add a check for the window trap.** It cost Haiku points on an
   uncovered task: `filter timestamp > now() - X` with X longer than the
   default window (and no `from:`) silently searches 2h. That trap is the
   query's own, not a recipe's, so it belongs in the DQL lints that
   `dtctl query` already runs.
3. **Hint relevance on generic queries.** Fixing it needs a signal that
   tells a generic events query from a k8s-events one: the filter fields,
   not the source's name. Measure it on the replay before shipping. Then
   re-run the uncovered tasks with more reps; v2's advice of more than five
   uncovered tasks stands.
4. `--follow` was not used by either model in v3. It costs nothing, but it
   only pays off if the skill or the envelope prompts it, e.g. by
   suggesting `--follow` on the run whose `next` edge is the obvious
   continuation.

---

## v4 question

47f0ef7f acts on the v3 recommendations:

- **Window-trap warning.** `dtctl query` warns when `filter timestamp >
  now() - X` reaches past the window the query reads (the default 2h when it
  names none). On the replay of all earlier queries it fires on 157 of
  5,024. The sampled hits were all genuine traps. Most were on t10 and t11.
- **The usual next step under `--follow`.** A `next` edge can be marked
  `follow: true`. `run --follow` takes it, and every suggestion for a recipe
  with such an edge already ends in `--follow`. Marked:
  - problems-get → problems-evidence;
  - problems-evidence → problems-logs;
  - k8s-workload-status → k8s-warning-events.
- **Hint relevance: not shipped.** Dropping the fetch source's name from the
  query's words cut the expected strong hints from 375 to 270. Precision fell
  too (24% → 22%), so the change was reverted.

Does B' (47f0ef7f) keep v3's gains, and do the two changes add to them?

## v4 design (lean)

- **Only a new B' arm is run.** A is reused from v3: the same control build
  (8815e206), taskset, judge, environment and day.
- **Sonnet**: 25 tasks × 3 reps = 75 runs. **Haiku**: 25 tasks × 4 reps =
  100 runs.
- **Spend cap: $35.**
- Analysis: the same two-level bootstrap, B' − A, with v3's decision rule.
  The comparison B' − B (v3) is descriptive, since it is the same day but a
  different run.
- **Secondary:**
  - `--follow` use;
  - window-trap warnings fired and acted on;
  - score on t10 and t11.

---

# v4 results

Everything above this line is unchanged.

## v4 verdict

**Recipes still pay off for both models. The two v4 changes are sound but
too rarely exercised to move the totals: B' is indistinguishable from v3's
B.**

| B − A (two-level 95% CI) | Sonnet v3 | Sonnet v4 (B') | Haiku v3 | Haiku v4 (B') |
|---|---|---|---|---|
| score | +0.27 [+0.05, +0.53] | +0.21 [−0.01, +0.48] | +0.57 [+0.09, +1.05] | **+0.59 [+0.03, +1.14]** |
| turns | −0.91 [−1.76, −0.05] | −0.49 [−1.32, +0.35] | −0.77 [−2.31, +0.60] | **−1.89 [−3.92, +0.05]** |
| dtctl calls | −1.29 [−2.44, −0.23] | −0.48 [−1.59, +0.65] | −1.00 [−2.55, +0.40] | **−1.91 [−3.83, −0.08]** |
| cost $/run | −0.011 [−0.021, −0.001] | −0.005 [−0.015, +0.005] | +0.0045 [−0.007, +0.016] | −0.001 [−0.013, +0.010] |
| wall s/run | −4 [−7, −1] | −3 [−6, +0] | 0 [−6, +6] | −1 [−9, +8] |
| uncovered tasks | 0 | 0 | −0.70 [−1.45, +0.05] | −0.50 [−1.15, +0.15] |

- **By v3's rule, Sonnet breaks even in v4.** The score is non-inferior
  (lower bound −0.01 > −0.15). Turns and cost both point down, but neither
  interval excludes 0. v3 met "benefits".
- **Haiku keeps its lift**, and it now costs no more than control. B' makes
  1.9 fewer dtctl calls per run (interval excludes 0) and 1.9 fewer turns.
- **B' − B (v4 against v3, descriptive):**
  - Sonnet: score −0.05 [−0.23, +0.09], turns +0.41 [−0.24, +1.13], cost
    +$0.006 [−0.002, +0.014].
  - Haiku: score +0.02 [−0.38, +0.44], turns −1.12 [−3.02, +0.68], cost
    −$0.006 [−0.018, +0.005].
  - Every interval includes 0. The two rounds measure the same thing twice.

## v4 runs

- B' 4a954d52, A reused from v3 (8815e206, same taskset and environment).
- Sonnet: 75 runs (`v4-sonnet`). Haiku: 100 runs (`v4-haiku`). Every run
  finished rc 0, with no 429, and all were judged.
- **Spend: $23.28**, under the $35 cap. Sonnet $9.50, Haiku $13.78.
- Analysis batches `v4c-<model>` link v3's A runs next to v4's B' runs.

## What the v4 changes did

- **Window-trap warning.**
  - Sonnet never hit it: it always sets `from:` itself.
  - In Haiku it fired in 11 runs, and every one widened the window in a later
    query. Some repeated the trap once or twice first.
  - It did not reach t10, which stayed at 1 1 1 1. The t10 runs in v4 wrote
    no timestamp filter. They counted bizevents over the default 2h and
    reported that as 24h, and nothing in the query says it is wrong.
- **`--follow`**: 2 runs per model (Sonnet 5 calls, Haiku 3). It was used
  more than in v3 (0), but still rarely.
- **Sonnet's per-task changes against v3's B do not trace to v4's changes:**
  - t27 went 3 3 3 → 2 2 2 on judged counts. No recipe and no warning was
    involved.
  - t08 and t39 took more turns. The t39 runs explored without a recipe.
    The t08 runs called the recipe's `--help` and `--dry-run` before
    running it.

## v4 recommendations

1. **Keep v3's design and v4's two additions.** The window warning was
   followed every time it fired. The marked edges cost nothing. Neither
   moves the totals.
2. **Say the window in the query envelope.** t10's remaining failure is a
   query with no window at all, so a lint has nothing to match. A
   `context.window` such as "last 2h (default; --from widens it)" would let
   the agent compare the window against the question's. This is a dtctl
   query change, not a recipe change.
3. **Run a fresh control next time.** Haiku's uncovered-task gap repeated
   (−0.50 after −0.70), but both rounds compare against the same A runs,
   and those A runs did unusually well on t10, t38 and t39. Only a new A
   separates a real cost from one lucky control sample.
4. **Hint relevance** still needs a signal other than the source name.
   Two replay variants lost recall without gaining precision.

---

# v5: a fresh control, ten uncovered tasks

## v5 question

v4 left three things open:

1. Both v3 and v4 compared against the same control runs. Those runs did
   unusually well on three uncovered tasks.
2. The no-harm check rests on five uncovered tasks.
3. t10 fails on a query that names no window at all.

v5 runs both arms fresh, adds five uncovered tasks, and adds one change.
478a4474 makes `dtctl query` report `context.window` with its `span`, and a
`note` when the query named no window and read the default 2h. The skill
gets a matching line.

**New tasks, all `coverage: none`:**
- **t40:** event kinds in 24h. A generic `fetch events` query, where the
  k8s-warning-events hint is known to misfire.
- **t41:** the top bizevents provider's share over 7d, a window trap.
- **t42:** the slowest HTTP synthetic monitor. Durations are in ns.
- **t43:** the most frequent workflow over 7d, and the workflows that always
  failed.
- **t44:** the log source with the most records in 1h. Logs hints fire, but
  no recipe answers this.

Taskset `v5` = `v2` + t40–t44: 30 tasks, 10 of them uncovered.

## v5 hypotheses and decision rule

v3's rule, unchanged, on a fresh A and B:

- **Sonnet benefits** if the score diff's lower bound is above −0.15 *and*
  either the turns diff's or the cost diff's upper bound is below 0.
- **Sonnet breaks even** if the score is non-inferior and both point
  estimates (turns, cost) are ≤ 0, with neither CI excluding 0.
- **Haiku keeps its lift** if the score diff's lower bound is above 0 and
  the cost diff's point estimate is at most +$0.005.

**H11 (no harm outside the recipes):** on the 10 uncovered tasks, B − A has
a lower bound above −0.15, per model. If the bound misses but the point
estimate is ≥ 0, report it as underpowered, not as harm.

**Descriptive only:**
- t10 and t41 scores;
- how often `context.window.note` appeared and whether the next query set a
  window;
- hints shown on uncovered tasks, and whether they were taken.

## v5 arms, models, reps

- **A**: `origin/main` (8815e206). **B**: docs/recipes-design at the commit
  that adds this preregistration.
- **Sonnet**: 30 × 2 × 3 = 180 runs. **Haiku**: 30 × 2 × 4 = 240 runs.
- **Spend cap: $60**, judging included. A run that hits a harness failure is
  re-run once.

---

# v5 results

Everything above this line is unchanged.

## v5 verdict

**Haiku keeps its lift against a fresh control. Sonnet breaks even, as in
v4. Recipes do no harm on uncovered tasks for Sonnet. For Haiku the
uncovered-task gap is gone in the point estimate but not ruled out.**

| B − A (two-level 95% CI) | Sonnet v5 | Haiku v5 |
|---|---|---|
| score | +0.18 [−0.04, +0.41] | **+0.60 [+0.17, +1.03]** |
| turns | −0.21 [−0.92, +0.44] | −0.62 [−2.34, +1.19] |
| dtctl calls | +0.04 [−0.87, +1.06] | −0.68 [−2.37, +1.07] |
| cost $/run | −0.002 [−0.010, +0.006] | +0.004 [−0.008, +0.016] |
| wall s/run | −1 [−4, +1] | +2 [−5, +8] |
| uncovered tasks (10) | +0.13 [+0.00, +0.37] | −0.05 [−0.70, +0.62] |

- **Sonnet breaks even.** The score is non-inferior (lower bound −0.04 >
  −0.15), turns and cost both point down, and neither interval excludes 0.
- **Haiku keeps its lift**, just inside the cost limit: +$0.004 against a
  limit of +$0.005. Covered tasks go from 0.75 to 1.69.
- **H11 holds for Sonnet** (lower bound 0.00). **For Haiku it is not met.**
  The lower bound is −0.70, and the point estimate (−0.05) is just below 0,
  so the preregistered "underpowered" reading does not apply. Against the
  old control the gap was −0.70 and −0.50. One task carries most of what is
  left (t22, below).
- **The fresh control did better than v3's A for Sonnet** (4.7 turns and
  $0.083 per run, against 5.5 turns and $0.092). So part of v3's Sonnet
  efficiency gain was control-sample luck. That is what v4's
  recommendation 3 suspected.

## v5 runs

- A 8815e206, B 168bbc85. Sonnet: 180 runs. Haiku: 240 runs. Every run was
  judged, with no 429.
- **Deviation:** the Haiku batch's login expired after 91 runs. The next
  148 runs failed to authenticate before doing anything, and so did their
  judging. After a fresh login the 148 runs were re-run once, as the
  preregistration allows for harness failures. All 148 finished rc 0. The
  ground-truth end snapshot was re-measured after the re-runs, so it
  brackets every run.
- **Spend: $54.03**, under the $60 cap. Sonnet $21.46, Haiku $32.58
  (failed runs cost nothing).
- The hint fix 28063d2a (each recipe word counts once) was committed after
  the v5 build. It is validated by replay only, not by this round.

## What v5 showed

- **The window note works.**
  - Haiku B: the note appeared in 103 of 120 runs, and 79 of those then
    set a window.
  - t41 (a 7d share) went from 1 1 1 1 to 3 3 3 3. Every B run read the
    note and re-ran with `from: now()-7d`.
  - Sonnet saw the note in 6 runs and set a window after 5 of them.
  - t10 for Haiku stayed mixed: A 1 1 0 3, B 1 3 3 0.
- **Hints did not mislead on uncovered tasks.** Hints were shown in 14 of
  30 Sonnet runs and 27 of 40 Haiku runs, and no run ran a recipe because
  of one. t40's generic events query drew the k8s-warning-events hint that
  28063d2a now suppresses. It was ignored.
- **t22 (Haiku, −1.5): B never found `dtctl get workflow-executions`.**
  - All four A runs found the command. No B run did. The B runs issued
    11–16 DQL queries looking for a workflow-execution table.
  - No run in either arm loaded the dtctl skill, so the skill text is not
    the cause. Three of the four A runs called `dtctl --help` early and
    found the command there. Two B runs saw hints on every exploratory
    query (16 mentions each) and kept writing DQL. With four runs per arm
    this may be chance. If not, the hints keep an agent in query mode
    after the data it wants turned out not to be in Grail.
- **The skills are available but rarely read.** A dtctl or dt-* skill was
  loaded in 3 of 90 Sonnet A runs, none of 90 Sonnet B runs, and 4 of 120
  runs in each Haiku arm. What reaches the agent is the dtctl binary's own
  output: help, hints, warnings, `context.window`. That is why every
  change that worked lives in that output.
- **t44 (Haiku, −1.0):** three B runs grouped by the wrong field
  (`dt.openpipeline.source` rather than `log.source`). No recipe was run.
- **t08 (Sonnet, B 7.7 calls vs A 2.0):** after the hint, the runs called
  the recipe's `--help`, `--dry-run` and `--show-query`, and then several
  `-o json` runs piped through jq. Two things made the output hard to
  read:
  - with `--jq` in agent mode, the records arrive as one YAML-encoded
    string;
  - the totals sit in `result.constant`, which the runs found late.

## v5 recommendations

1. **Keep the design.** Across v3, v4 and v5, Haiku's lift is stable
   (+0.57, +0.59, +0.60) and Sonnet breaks even or better.
2. **Point back at resource commands from inside dtctl, not from the
   skill.** The skill is rarely read, so a skill line would not reach
   t22's runs. When a query fails with `unknown_data_object`, the error
   envelope should suggest `dtctl commands`, because platform objects such
   as workflow executions come from `get`, not Grail.
3. **Make recipe output easier to read in agent mode.** Two parts:
   - `--jq` should return JSON records, not a YAML string;
   - a recipe's hint or envelope should say where its totals are.
   That targets t08's extra calls.
4. **Ship 28063d2a, and check it in the next round,** along with 2 and 3.

---

# v6: repairs in dtctl's own output, with and without skills

## v6 question

v5 found that Haiku mostly fails by spending its 20-call budget, about five
calls per run going to errors (A 5.7, B 5.0). Only 11 of 420 runs loaded a
skill, so anything the skills say rarely reaches the agent. v6 moves the
repairs into dtctl's output (a3bc232a, ed5a135e):

- **DQL habits:** SQL and KQL habits get a rewritten query that runs. These
  are stats/as/by, single quotes, function and command synonyms, bare
  references to unnamed aggregations, guessed entity fields, count(filter:),
  and a window on a later command. The repair comes before the recipe hints.
- **Nouns as commands:** `dtctl <noun>` and `dtctl get <noun>` name the
  commands that read the noun, not an edit-distance guess.
- **Platform objects:** a fetch of workflows, SLOs or dashboards points at
  `dtctl get`.
- **`--jq`:** with no `-o`, it returns JSON.
- **Compaction:** the compaction note names the columns in
  `result.constant`.

**Offline replay, before any run:** v5's errored calls were re-issued with
the new build, read-only.
- Of 1,028 errored queries, 431 now get a rewrite as the first suggestion,
  and 342 of those rewrites run. In v5, 160 got a rewrite first.
- Of 254 unknown-command errors, 155 now name a command that reads the
  noun. In v5, 1 did.

The second question is the one the user asked for: **what do skills add?**
v6 runs every arm with and without skills.

## v6 hypotheses and decision rule

The v3/v5 rule applies to B − A, with skills:

- **Sonnet benefits** if the score diff's lower bound is above −0.15 *and*
  the upper bound of the turns diff or the cost diff is below 0.
- **Sonnet breaks even** if the score is non-inferior and both point
  estimates (turns, cost) are ≤ 0.
- **Haiku keeps its lift** if the score diff's lower bound is above 0 and the
  cost diff's point estimate is at most +$0.005.

**H12 (fewer wasted calls, Haiku):** errored dtctl calls per run, B − A, has
an upper bound below 0. In v5 the point estimate was −0.7.

**H13 (no skills needed):** B0 − A0 meets the same Haiku rule as B − A.
The recipes and repairs reach the agent through dtctl's output alone.

**H14 (skills add little):** A − A0 and B − B0 each have a CI that includes 0,
per model. If one excludes 0, report its direction. Prediction: they include
0, since skills were loaded in about 3% of v5 runs.

**H11 (no harm outside the recipes)** is unchanged, for B − A and B0 − A0.

**Descriptive only:**
- the runs that ended without an answer;
- how often a first-suggestion rewrite appeared and whether the next call
  used it;
- noun advice shown and taken;
- skill loads per arm.

## v6 arms, models, reps

- **Builds:**
  - A and A0 use 8815e206, v5's control. The one later main commit is a
    skill-text change (#671). Keeping v5's base isolates the branch's work.
  - B and B0 use docs/recipes-design at the commit that adds this
    preregistration.
- **Arms:**
  - **A**: main, its dtctl skill, and the dt-* skills.
  - **B**: recipes, its dtctl skill, and the dt-* skills.
  - **A0**: main, no skills at all.
  - **B0**: recipes, no skills at all.
- **Runs:**
  - Haiku: 30 × 4 × 3 = 360 runs.
  - Sonnet: 30 × 4 × 2 = 240 runs.
  - Taskset `v5`.
- **Spend cap: $85**, judging included. At v5's per-run cost, about $78. A
  run that hits a harness failure is re-run once.

---

# v6 results

Everything above this line is unchanged.

## v6 verdict

| hypothesis | Haiku | Sonnet |
|---|---|---|
| B − A, decision rule | **lift kept**: +1.16 [+0.71, +1.60], cost −$0.013 [−0.025, −0.001] | **non-inferior** +0.17 [−0.02, +0.40]; turns +0.15, cost +$0.001, so neither "benefits" nor "breaks even" by the letter of the rule |
| H12, errored calls B − A | **met**: −1.49 [−2.36, −0.66] | n/a (0.5 per run) |
| H13, B0 − A0 (no skills) | **met**: +0.83 [+0.46, +1.22], cost −$0.020 | breaks even: +0.10 [−0.05, +0.27], turns −0.23, cost −$0.001 |
| H14, A − A0 | −0.07 [−0.40, +0.27]: includes 0 | −0.08 [−0.37, +0.15]: includes 0; cost **+$0.021** [+0.013, +0.029] |
| H14, B − B0 | +0.26 [−0.09, +0.61]: includes 0 | −0.02 [−0.22, +0.17]: includes 0; cost **+$0.024** [+0.017, +0.031] |
| H11, uncovered B − A | +1.40 [+0.70, +2.10] | +0.00 [+0.00, +0.00] (all at 3) |
| H11, uncovered B0 − A0 | +0.77 [+0.23, +1.30] | +0.00 [−0.20, +0.20]: underpowered, not harm |

All scores are per-task mean differences on the 0–3 rubric, with a
two-level bootstrap 95% CI. Haiku ran 90 runs per arm, Sonnet 60.

## v6 runs

- **Builds:**
  - A and A0: 8815e206.
  - B and B0: f2a8ee47.
  - Taskset `v5`, 30 tasks.
- **Runs:**
  - Sonnet: 240 runs.
  - Haiku: 360 runs.
  - $74.53 in all, judging included. The cap was $85.
- **Harness failures:**
  - 28 Sonnet runs hit an account spend limit (HTTP 429) and were re-run
    after it reset.
  - The eval's tmpfs filled during the Haiku batch. 73 runs never started
    or never finished, and 3 finished runs had dtctl calls fail with
    ENOSPC. All 76 were re-run once, after the run data moved to disk.
  - End ground truth for Haiku was measured after the re-runs.
  - The preregistration allows one re-run for a harness failure.

| arm | Haiku score | calls | cost $/run | 20-call budget hit | no answer | Sonnet score | cost $/run |
|---|---|---|---|---|---|---|---|
| A (main + skills) | 0.99 | 17.4 | 0.104 | 50/90 | 29/90 | 2.67 | 0.080 |
| A0 (main, no skills) | 1.06 | 17.9 | 0.108 | 52/90 | 25/90 | 2.75 | 0.059 |
| B (recipes + skills) | 2.14 | 13.5 | 0.091 | 21/90 | 7/90 | 2.83 | 0.081 |
| B0 (recipes, no skills) | 1.89 | 13.3 | 0.088 | 30/90 | 12/90 | 2.85 | 0.058 |

## What v6 showed

1. **Haiku's lift almost doubled, and it is cheaper.** In v5, recipes alone
   gave +0.60. v6 adds the dtctl-side repairs and gives +1.16, with 3.8 fewer
   calls and 1.3 cents less per run. Fully correct runs: 50 of 90 against 15;
   runs scoring zero: 11 against 42.
   The control scored the same as v5's (0.99 against 0.98), so the rounds
   compare.
2. **The gain reaches tasks no recipe covers.** On the ten uncovered tasks,
   Haiku went from −0.05 in v5 to +1.40. The repairs are not tied to
   recipes:
   - t22 (failed workflow runs) went from scores 0, 0, 0, 0 in v5 to 2, 2, 2.
     The noun advice turned `dtctl workflow executions list` into
     `dtctl get workflow-executions`.
   - t42 (slowest synthetic monitor) went to 3/3/3.
   - t10 and t40 went to 3/3/3.
3. **Errors no longer eat the budget.**
   - Errored calls per run fell by 1.5.
   - Runs that hit the 20-call budget fell from 50 to 21 of 90.
   - Runs that ended with no answer fell from 29 to 7.
   - A runnable rewrite came first after 103 errors in B. The next call ran
     it 37 times, and 29 of those succeeded.
   - Noun advice was shown 60 times and followed 39 times.
4. **Skills add nothing measurable, and they cost Sonnet money.**
   - Skills were loaded in 13 of 600 runs.
   - With or without skills, scores are the same within noise for both
     models: every A − A0 and B − B0 interval includes 0.
   - For Sonnet, carrying the skill catalog costs $0.02 more per run, about
     +35%.
   - B0 is the cheapest arm for both models.
   - B − B0 for Haiku (+0.26) leans positive but is not significant.
5. **Sonnet stays at its ceiling.** All arms score 2.67–2.85. Recipes are
   non-inferior, with no turn or cost change.

**Caveat:** B bundles recipes and the repairs, so v6 cannot split the gain
between them. v5 (recipes alone) and v6 ran on the same tasks with
comparable controls. Read across them, roughly half of Haiku's lift comes
from each.

## v6 recommendations

1. **Propose the branch as it is.** The case for it is Haiku's numbers:
   - +1.16 points;
   - fewer calls;
   - lower cost;
   - no harm outside the recipes.
2. **Put agent guidance in dtctl's output, not in skills.** Skills are rarely
   loaded, they don't change scores, and for Sonnet they cost about a third
   more per run.
3. **Next targets are the tasks still near zero for Haiku:**
   - t28 (traces investigation);
   - t39 (bizevents);
   - t43 (always-failing workflows);
   - t37, where every arm scores 1, so it is probably the rubric or the task.
