# Recipes vs. skills eval

Do recipes (`dtctl run <recipe>`) make a measurable difference for an AI agent
answering Dynatrace questions, compared with an agent that already has the
dtctl skill and the dynatrace-for-ai `dt-*` skills? Results and analysis:
[docs/dev/RECIPES_EVAL.md](../../../docs/dev/RECIPES_EVAL.md).

Nothing in this directory names a tenant, URL, service or customer. Every
tenant-specific value is in `env.sh`, which git ignores. Runs, transcripts and
ground truth go to `$EVAL_RUNS_DIR`, outside the repo.

## Layout

| File | What it does |
|---|---|
| `tasks/tNN.md` | One task each: front matter (tenant, domain, recipe coverage, trap), the prompt (with `${EVAL_*}` placeholders), and the 0–3 rubric |
| `env.example.sh` | Template for `env.sh`: dtctl context names, the entity names that prompts use, paths |
| `build.sh` | Builds `bin/dtctl-main` (origin/main) and `bin/dtctl-recipes` (this tree), and copies each tree's `skills/dtctl` |
| `ground_truth.py` | Independent DQL per task, run with the main binary against a read-only config, before and after a batch |
| `run.py` | Runs arms × tasks × reps, each as a fresh headless `claude -p` in its own workspace |
| `judge.py` | Blind judge: question, rubric, ground truth and final answer only, scored 0–3 |
| `analyze.py` | Per-arm and per-task tables, paired bootstrap CIs, recipe/skill usage, and a policy audit |
| `lib.py` | Shared helpers: env loading, read-only config, query runner, task parser |

## Arms

| Arm | dtctl binary | dtctl skill | dt-* skills | System-prompt nudge |
|---|---|---|---|---|
| A | origin/main | main | yes | none |
| B | this tree | this tree (with the recipes section) | yes | none |
| C | this tree | this tree | no | none |
| A2 | origin/main | main | yes | "load the dtctl skill first" |
| B2 | this tree | this tree | yes | "load the dtctl skill first" |
| B3 | this tree | this tree | yes | B2 plus "look for a recipe before writing DQL" |

A, B and C answer the question as asked. A2, B2 and B3 are diagnostic: they
measure what recipes are worth once an agent reads the skill or uses the
recipes. The user prompt is byte-identical in every arm; the nudge goes
through `--append-system-prompt`.

## Isolation and read-only enforcement

- **Read-only.** The `dtctl` on the agent's PATH is a wrapper. It sets
  `DTCTL_CONFIG` to a single-context copy of the context with
  `safety-level: readonly` (chmod 444). Credentials stay in the keyring and are
  found by `token-ref`. The wrapper refuses mutating and config verbs before
  dtctl sees them and logs every call (argv, stdout, stderr, rc, ms) under
  `calls/`. It also enforces the call budget (20) that the prompt states.
- **No developer state.** Each run gets its own `HOME`, `CLAUDE_CONFIG_DIR`
  (credentials plus copied skills) and `XDG_*` dirs. A private
  `XDG_CONFIG_HOME` matters: dtctl loads user recipe books from there. MCP is
  off (`--strict-mcp-config`).
- **Runs dir outside `$HOME`.** Claude Code loads `.claude/skills`, settings
  and `CLAUDE.md` from every ancestor of its working directory. The first
  pilot ran under `~/.cache` and every arm, the no-skills arm included,
  silently got the developer's `~/.claude/skills`. `run.py` now refuses a runs
  dir that has such an ancestor.
- **Tools.** Bash, Read and Skill only. Bash is allowed for `dtctl` and a few
  text tools. Read is allowed only under the run's skills and work dirs.
  WebFetch, WebSearch and subagents are disallowed. `analyze.py` audits every
  command for paths outside the workspace and for reads of ground truth or
  the repo.
- **Nothing on the allow-list can start a program.** Anything that can would
  bypass the wrapper and its verb guard. So `awk` (`system()`, `| getline`),
  `sed` (GNU `e`, `s///e`, `w`), `xargs`, `env` and `find -exec` are not on
  the list, and `sort` reaches `/usr/bin/sort` only through a per-run shim
  that refuses `--compress-program` (and its `--co…` abbreviations). Claude's
  permission check refuses output redirection to a file in `dontAsk` mode,
  and `bin/` is locked (0555) in case that ever changes. The real binaries
  are not on PATH. Called by absolute path, they are refused by the
  permission rules, and they would find no config under the run's HOME. A
  probe run that tries each of these vectors leaves no trace of having
  executed any of them.
- **Credentials.** The copied `.credentials.json` is deleted when a run ends,
  including on a timeout or an exception. The read-only dtctl config lives in
  `<batch>/_cfg/`, outside the agent's cwd and HOME, and carries only a
  keyring `token-ref`.

## Running it

```bash
cp env.example.sh env.sh && $EDITOR env.sh       # contexts, entity names, paths
./build.sh                                       # bin/dtctl-main, bin/dtctl-recipes, skills
python3 run.py pilot --arms A,B,C --tasks t08,t19,t22 --reps 1
python3 judge.py pilot && python3 analyze.py pilot
python3 run.py full --arms A,B,C --reps 2        # ground truth runs before and after
python3 judge.py full && python3 analyze.py full [more batches...]
```

`run.py` measures ground truth into `<batch>/_gt/` at the start and end of
each batch, because open problems and the last hour's logs move while it
runs. To add reps later, start a new batch (with `--rep-offset`) and pool the
batches in `analyze.py`. Each batch is judged against its own ground truth.

Cost guide (Sonnet investigator, Opus judge): about $0.05–0.15 per run, plus
about $0.03 to judge it.

## Writing a task

Ground truth must not come from the recipe under evaluation. Write the DQL in
`ground_truth.py` independently, cross-check it against a second formulation
or source (metric against events, a dedup against a distinct count), and set
the rubric's tolerances from how much the value moves between the start and
end measurements. Keep prompts unambiguous: if a strong agent with no recipes
gets it "wrong" because the question admits two readings, the task measures
the wording, not the tool.
