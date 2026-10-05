#!/usr/bin/env python3
"""Run the eval matrix: arms × tasks × reps, one fresh headless `claude -p` each.

    run.py <batch> [--arms A,B] [--tasks t01,t02 | --taskset v2] [--reps 2] [--model M]
           [--parallel N] [--no-gt]

Arms
  A  control: dtctl from origin/main + its dtctl skill + the dt-* skills
  B  recipes: dtctl from this tree   + its dtctl skill + the dt-* skills
  C  recipes, no dt-* skills: dtctl from this tree + its dtctl skill only
  A0/B0  A/B with no skills at all (no dtctl skill, no dt-* skills)
  A2/B2  A/B plus a system-prompt nudge to load the dtctl skill first (diagnostic)
  B3     B2 plus a nudge to look for a recipe before writing DQL (upper bound)
  BP     "B+": B, with a one-line recipe pointer added to each dt-* skill that a
         recipe domain maps to (patched copies in the run's skills dir only)
  BR     B plus only the recipe nudge (v2 upper bound; no "load the skill" nudge)

Every run gets its own workspace under $EVAL_RUNS_DIR/<batch>/<task>/<arm>-r<rep>:
  cfg/      CLAUDE_CONFIG_DIR: copied credentials + the arm's skills (copies)
  home/     HOME
  work/     the agent's cwd (empty: no CLAUDE.md, no repo)
  bin/dtctl the logging wrapper - the only dtctl on PATH (bin/ is locked 0555)
  bin/sort  a shim that refuses --compress-program
  calls/    one record per dtctl invocation (argv, stdout, stderr, rc, seconds)
  transcript.jsonl, prompt.md, meta.json

Read-only is enforced three times over: the wrapper points DTCTL_CONFIG at a
single-context config with safety-level readonly (chmod 444), refuses mutating
and config verbs before dtctl sees them, and the claude permission rules only
allow dtctl and a few text filters in Bash - none of which can start another
program. The real binary is not on PATH; called by its absolute path it is
refused by the permission rules, and would find no config under the run's HOME.
The read-only config lives in <batch>/_cfg/, outside the agent's cwd and HOME.

Ground truth is measured before (gt-start) and after (gt-end) the batch into
<batch>/_gt/, a directory no agent rule allows reading.
"""
import argparse
import concurrent.futures as cf
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import lib

ARMS = {
    "A": dict(binary="dtctl-main", skill="skill-main", dt_skills=True),
    "B": dict(binary="dtctl-recipes", skill="skill-recipes", dt_skills=True),
    "C": dict(binary="dtctl-recipes", skill="skill-recipes", dt_skills=False),
    # v6: no skills at all. Skills were loaded in 11 of 420 v5 runs, so what
    # an agent meets is dtctl's own output; these arms measure exactly that.
    "A0": dict(binary="dtctl-main", skill=None, dt_skills=False),
    "B0": dict(binary="dtctl-recipes", skill=None, dt_skills=False),
    # Diagnostic pair: A and B, plus a system-prompt nudge to load the dtctl
    # skill first. The full matrix showed agents never load it on their own,
    # so B never meets the recipes section; A2/B2 measure recipes *given*
    # that the agent reads its dtctl skill. The user prompt stays identical.
    "A2": dict(binary="dtctl-main", skill="skill-main", dt_skills=True, nudge=True),
    "B2": dict(binary="dtctl-recipes", skill="skill-recipes", dt_skills=True, nudge=True),
    # Upper bound: B2 even when nudged never ran a recipe, so B3 is told to
    # look for one. Compare with A2 (equally nudged to its skill).
    "B3": dict(binary="dtctl-recipes", skill="skill-recipes", dt_skills=True, nudge=True, nudge_recipes=True),
    # v2. B+: the dt-* skills an agent already loads point at the recipes of
    # their domain - one line each, generated from the build's own catalog.
    "BP": dict(binary="dtctl-recipes", skill="skill-recipes", dt_skills=True, dt_pointers=True),
    # v2 upper bound: B plus the recipe nudge alone. Unlike B3 it does not also
    # tell the agent to load the dtctl skill, so it isolates the recipe nudge.
    "BR": dict(binary="dtctl-recipes", skill="skill-recipes", dt_skills=True, nudge_recipes=True),
}

NUDGE = "Before running any command, load the dtctl skill with the Skill tool and follow its guidance."
NUDGE_RECIPES = (" Before writing any DQL, look for a matching recipe (`dtctl get recipes --search <words>`)"
                 " and use it when one fits.")


def system_nudge(arm):
    a = ARMS[arm]
    text = (NUDGE if a.get("nudge") else "") + (NUDGE_RECIPES if a.get("nudge_recipes") else "")
    return text.strip()


# B+: which dt-* skill a recipe domain (the recipe name's first word) points
# from. A domain that maps to no observability skill goes to dt-dql-essentials,
# the skill every DQL-writing agent loads.
DOMAIN_SKILLS = {
    "k8s": ["dt-obs-kubernetes"], "logs": ["dt-obs-logs"], "services": ["dt-obs-services"],
    "traces": ["dt-obs-tracing"], "problems": ["dt-obs-problems"],
    "hosts": ["dt-obs-hosts"], "capacity": ["dt-obs-hosts"], "network": ["dt-obs-hosts"],
    "cloud": ["dt-obs-aws", "dt-obs-azure", "dt-obs-gcp"], "genai": ["dt-obs-genai"],
    "frontends": ["dt-obs-frontends"], "costs": ["dt-platform-costs"], "security": ["dt-sec-insights"],
    "changes": ["dt-obs-problems", "dt-obs-kubernetes"],
}
POINTER_FALLBACK = "dt-dql-essentials"


def recipe_pointers(binary):
    """{skill: one pointer line} from the build's own recipe catalog."""
    with tempfile.TemporaryDirectory() as tmp:
        p = subprocess.run([str(lib.BIN / binary), "--plain", "--no-agent", "get", "recipes", "-o", "json"],
                           capture_output=True, text=True, check=True,
                           env=lib.isolated_env(Path(tmp) / "none.yaml", Path(tmp) / "iso"))
    by_skill = {}
    for r in json.loads(p.stdout):
        if r.get("source") != "builtin":
            continue
        for sk in DOMAIN_SKILLS.get(r["name"].split("-")[0], [POINTER_FALLBACK]):
            by_skill.setdefault(sk, []).append(r["name"])
    return {sk: ("> dtctl ships curated, verified queries (recipes) for this area: "
                 + ", ".join(f"`{n}`" for n in sorted(names))
                 + ". Run one with `dtctl run <name>`; `dtctl describe recipe <name>` shows its parameters.")
            for sk, names in by_skill.items()}


def add_pointer(skill_md, line):
    """Insert the pointer line after the skill's first top-level heading."""
    lines = skill_md.read_text().splitlines(keepends=True)
    body = 0
    if lines and lines[0].strip() == "---":
        body = next(i for i in range(1, len(lines)) if lines[i].strip() == "---") + 1
    at = next((i + 1 for i in range(body, len(lines)) if lines[i].startswith("# ")), body)
    lines[at:at] = ["\n", line + "\n"]
    skill_md.write_text("".join(lines))

CALL_BUDGET = 20
MAX_TURNS = 40
TIMEOUT_S = 20 * 60

PREAMBLE = f"""You are answering a question about a Dynatrace environment. Use the `dtctl`
CLI, which is already configured for the right environment (read-only), to
answer it from data.

Rules:
- Use at most {CALL_BUDGET} dtctl invocations in total; further calls are refused.
- Run every command in the foreground. Never background a command or wait/sleep.
- Bash is limited to dtctl plus basic text tools (jq, grep, head, tail, sort,
  uniq, wc, cut, tr, cat, column, echo, printf, date). If a command is denied,
  adjust it and retry instead of giving up.
- Report only values you actually measured. If the data cannot answer the
  question, say so instead of guessing.
- End with a short explanation, then a final line that starts with `ANSWER:`
  and states the answer in one line.

Question:
"""

# verbs the wrapper refuses before dtctl sees them (readonly would refuse most
# of them too; this keeps config and keyring untouched as well)
BLOCKED = ["create", "edit", "apply", "delete", "update", "restore", "share", "unshare",
           "auth", "skills", "serve", "plugin", "alias", "install", "upgrade", "login", "logout"]
CONFIG_OK = ["view", "get-contexts", "current-context"]

WRAPPER = r"""#!/usr/bin/env bash
# dtctl logging wrapper for one eval run (generated).
calls={calls}
budget={budget}
mkdir -p "$calls"
n=$(ls "$calls" | grep -c '\.argv$')
# unique and chronological even when the agent runs calls in parallel
id=$(date +%s%N)-$$
printf '%s\0' "$@" > "$calls/$id.argv"

verb=""; sub=""; skip=0
for a in "$@"; do
  if [ $skip = 1 ]; then skip=0; continue; fi
  case "$a" in
    -o|--output|--context|-c|--jq|--config) skip=1; continue;;
    -*) continue;;
  esac
  if [ -z "$verb" ]; then verb=$a; else sub=$a; break; fi
done

refuse() {{ echo "$1" >&2; echo 2 > "$calls/$id.rc"; echo 0 > "$calls/$id.ms"; echo "$1" > "$calls/$id.refused"; exit 2; }}
if [ "$n" -ge "$budget" ]; then
  refuse "eval harness: call budget of $budget dtctl invocations exhausted - answer with what you have"
fi
case " {blocked} " in *" $verb "*) refuse "eval harness: '$verb' is not allowed (read-only evaluation)";; esac
if [ "$verb" = config ] || [ "$verb" = ctx ]; then
  case " {config_ok} " in *" $sub "*) ;; *) refuse "eval harness: '$verb $sub' is not allowed (read-only evaluation)";; esac
fi

export DTCTL_CONFIG={cfg}
export XDG_CONFIG_HOME={iso}/config XDG_CACHE_HOME={iso}/cache XDG_STATE_HOME={iso}/state XDG_DATA_HOME={iso}/data
unset DTCTL_CONTEXT
start=$(date +%s%N)
reads_stdin=0
for a in "$@"; do case "$a" in -|--file=-|-f=-) reads_stdin=1;; esac; done
if [ $reads_stdin = 1 ]; then
  tee "$calls/$id.in" | {binary} "$@" > "$calls/$id.out" 2> "$calls/$id.err"
  rc=${{PIPESTATUS[1]}}
else
  {binary} "$@" > "$calls/$id.out" 2> "$calls/$id.err"
  rc=$?
fi
cat "$calls/$id.out"
cat "$calls/$id.err" >&2
echo $rc > "$calls/$id.rc"
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$calls/$id.ms"
exit $rc
"""

# Bash allow-list besides dtctl. Text tools only; the first pilot showed that a
# denied `cd /tmp; dtctl ...` made an agent give up, which is a harness
# artifact, so the harmless shell builtins are allowed too. Reading files is
# possible with these (the skills' references need it); analyze.py audits every
# command for paths outside the run's workspace.
#
# Nothing on this list may be able to run another program, or the wrapper and
# its verb guard are bypassed. That rules out awk (`system()`, `| getline`),
# sed (GNU `e` command, `s///e`, `w`), xargs, env and find (`-exec`). `sort` can
# exec through `--compress-program`, so it stays only behind SORT_SHIM. Output
# redirection to a file is refused by claude's own permission check in
# dontAsk mode (observed: `... > /tmp/x` was denied in every run that tried).
FILTERS = ["jq", "head", "tail", "wc", "sort", "uniq", "grep", "cut", "tr", "cat", "column",
           "cd", "echo", "printf", "date", "ls", "true"]

# Leaving a program off FILTERS is not enough: claude treats some commands as
# read-only on its own and runs them without an allow rule (the v2 pilot ran
# `... | sed -E 's/x/y/'` and `... | xargs wc -l`). Deny the ones that can run
# another program explicitly; a deny rule wins over that built-in judgement.
DENIED_PROGRAMS = ["sed", "awk", "gawk", "mawk", "xargs", "env", "find", "perl", "python", "python3",
                   "node", "bash", "sh", "zsh", "eval", "exec", "command", "nohup", "timeout", "nice",
                   "watch", "parallel", "tee", "dd", "cp", "mv", "rm", "ln", "chmod", "curl", "wget"]
DISALLOWED = ["WebSearch", "WebFetch", "Task", "Agent", "Write", "Edit", "Grep", "Glob"] + \
    [f"Bash({p}:*)" for p in DENIED_PROGRAMS]

# GNU sort runs --compress-program; getopt also accepts any unambiguous prefix
# (`--co`, `--compress`), so refuse every argument that starts with `--co`.
SORT_SHIM = r"""#!/usr/bin/env bash
# sort shim for one eval run (generated): refuses --compress-program.
for a in "$@"; do
  case "$a" in --) break;; --co*) echo "eval harness: sort --compress-program is not allowed" >&2; exit 2;; esac
done
exec /usr/bin/sort "$@"
"""


def q(p):
    return "'" + str(p).replace("'", "'\\''") + "'"


def unlock(d):
    """Make a locked bin/ dir writable again so a rerun can remove the run."""
    if d.exists():
        d.chmod(0o755)


def check_no_ancestor_config(path):
    """Claude Code picks up .claude/skills, .claude/settings*.json and CLAUDE.md
    from every ancestor of its cwd. A runs dir under $HOME would hand every arm
    the developer's own skills (the first pilot did exactly that), so refuse."""
    for d in [path, *path.parents]:
        for name in (".claude", "CLAUDE.md", "CLAUDE.local.md", ".mcp.json"):
            if (d / name).exists():
                sys.exit(f"{d / name} is an ancestor of the runs dir {path}: claude would load it into "
                         "every run. Set EVAL_RUNS_DIR outside it (e.g. under /tmp).")


def setup_run(env, batch_dir, task, arm, rep, cfg_by_tenant, pointers=None):
    ws = batch_dir / task["id"] / f"{arm}-r{rep}"
    if ws.exists():
        unlock(ws / "bin")
        shutil.rmtree(ws)
    for d in ("cfg/skills", "home", "work", "bin", "calls", "iso"):
        (ws / d).mkdir(parents=True, exist_ok=True)
    a = ARMS[arm]

    # claude config dir: credentials + skills (copies, so nothing points back
    # into the repo or the dynatrace-for-ai checkout)
    cred = Path.home() / ".claude" / ".credentials.json"
    if cred.exists():
        shutil.copy(cred, ws / "cfg" / ".credentials.json")
    if a["skill"]:
        shutil.copytree(lib.BIN / a["skill"], ws / "cfg" / "skills" / "dtctl")
    if a["dt_skills"]:
        for sk in sorted(Path(env["EVAL_DFAI_DIR"], "skills").glob("dt-*")):
            shutil.copytree(sk, ws / "cfg" / "skills" / sk.name)
            if a.get("dt_pointers") and sk.name in (pointers or {}):
                add_pointer(ws / "cfg" / "skills" / sk.name / "SKILL.md", pointers[sk.name])
    (ws / "cfg" / "settings.json").write_text(json.dumps({"includeCoAuthoredBy": False}))

    wrapper = WRAPPER.format(
        calls=q(ws / "calls"), budget=CALL_BUDGET, blocked=" ".join(BLOCKED),
        config_ok=" ".join(CONFIG_OK), cfg=q(cfg_by_tenant[task["meta"]["tenant"]]),
        iso=q(ws / "iso"), binary=q(lib.BIN / a["binary"]))
    for name, body in (("dtctl", wrapper), ("sort", SORT_SHIM)):
        (ws / "bin" / name).write_text(body)
        (ws / "bin" / name).chmod(0o555)
    # the agent cannot write files (no Write/Edit, redirection refused), but if
    # it ever could, replacing bin/dtctl would bypass every guard: lock it
    (ws / "bin").chmod(0o555)

    prompt = PREAMBLE + task["prompt"] + "\n"
    (ws / "prompt.md").write_text(prompt)
    return ws, prompt


def claude_env(ws):
    keep = ["USER", "LOGNAME", "LANG", "LC_ALL", "TERM", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS",
            "SHELL", "TZ"]
    e = {k: os.environ[k] for k in keep if k in os.environ}
    e.update(HOME=str(ws / "home"), CLAUDE_CONFIG_DIR=str(ws / "cfg"),
             PATH=f"{ws / 'bin'}:/usr/local/bin:/usr/bin:/bin",
             DISABLE_AUTOUPDATER="1", CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1",
             DISABLE_TELEMETRY="1")
    return e


def run_one(env, batch_dir, task, arm, rep, cfg_by_tenant, pointers=None):
    ws, prompt = setup_run(env, batch_dir, task, arm, rep, cfg_by_tenant, pointers)
    allowed = ["Bash(dtctl:*)"] + [f"Bash({f}:*)" for f in FILTERS] + [
        "Skill", f"Read(/{ws / 'cfg' / 'skills'}/**)", f"Read(/{ws / 'work'}/**)"]
    cmd = [shutil.which("claude") or "claude", "-p", prompt,
           "--model", env["EVAL_MODEL"],
           "--output-format", "stream-json", "--verbose",
           "--max-turns", str(MAX_TURNS),
           "--permission-mode", "dontAsk",
           "--tools", "Bash,Read,Skill",
           "--allowedTools", ",".join(allowed),
           "--disallowedTools", ",".join(DISALLOWED),
           "--strict-mcp-config", "--no-session-persistence"]
    if system_nudge(arm):
        cmd += ["--append-system-prompt", system_nudge(arm)]
    t0 = time.time()
    with open(ws / "transcript.jsonl", "w") as out, open(ws / "claude.err", "w") as err:
        try:
            p = subprocess.run(cmd, cwd=ws / "work", env=claude_env(ws), stdin=subprocess.DEVNULL,
                               stdout=out, stderr=err, timeout=TIMEOUT_S)
            rc = p.returncode
        except subprocess.TimeoutExpired:
            rc = "timeout"
        finally:
            # credentials do not stay in run dirs, even when the run is interrupted
            (ws / "cfg" / ".credentials.json").unlink(missing_ok=True)
    meta = dict(task=task["id"], arm=arm, rep=rep, rc=rc, wall_s=round(time.time() - t0, 1),
                model=env["EVAL_MODEL"], binary=ARMS[arm]["binary"], build=build_rev(ARMS[arm]["binary"]),
                started=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(t0)))
    (ws / "meta.json").write_text(json.dumps(meta, indent=2))
    return meta


def build_rev(binary):
    f = lib.BIN / ("main.rev" if binary == "dtctl-main" else "recipes.rev")
    return f.read_text().strip() if f.exists() else None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("batch")
    ap.add_argument("--arms", default="A,B")
    ap.add_argument("--tasks", default="")
    ap.add_argument("--taskset", default="", help="tasks/<name>.txt: one task id per line")
    ap.add_argument("--model", default="", help="investigator model (overrides EVAL_MODEL)")
    ap.add_argument("--reps", type=int, default=2)
    ap.add_argument("--rep-offset", type=int, default=0)
    ap.add_argument("--parallel", type=int, default=0)
    ap.add_argument("--no-gt", action="store_true")
    args = ap.parse_args()

    env = lib.load_env()
    if args.model:
        env["EVAL_MODEL"] = args.model
    only = set(filter(None, args.tasks.split(",")))
    if args.taskset:
        only |= lib.load_taskset(args.taskset)
    tasks = lib.load_tasks(env, only or None)
    arms = args.arms.split(",")
    batch_dir = lib.runs_dir(env) / args.batch
    check_no_ancestor_config(batch_dir)
    gt_dir = batch_dir / "_gt"
    gt_dir.mkdir(parents=True, exist_ok=True)

    cfg_by_tenant = {}
    for tenant in sorted({t["meta"]["tenant"] for t in tasks}):
        cfg_by_tenant[tenant] = lib.make_readonly_config(
            env, lib.tenant_context(env, tenant), batch_dir / "_cfg" / f"{tenant}.yaml")

    pointers = None
    if any(ARMS[a].get("dt_pointers") for a in arms):
        pointers = recipe_pointers("dtctl-recipes")
        (batch_dir / "_pointers.json").write_text(json.dumps(pointers, indent=2))

    ids = [t["id"] for t in tasks]
    if not args.no_gt:
        subprocess.run([sys.executable, str(lib.HERE / "ground_truth.py"), str(gt_dir), "start", *ids], check=True)

    jobs = [(t, a, r) for r in range(1 + args.rep_offset, args.reps + 1 + args.rep_offset)
            for t in tasks for a in arms]
    par = args.parallel or int(env["EVAL_PARALLEL"])
    print(f"{len(jobs)} runs, {par} in parallel -> {batch_dir}", file=sys.stderr)
    with cf.ThreadPoolExecutor(par) as ex:
        futs = {ex.submit(run_one, env, batch_dir, t, a, r, cfg_by_tenant, pointers): (t["id"], a, r)
                for t, a, r in jobs}
        for f in cf.as_completed(futs):
            tid, a, r = futs[f]
            try:
                m = f.result()
                print(f"  {tid} {a} r{r}: rc={m['rc']} {m['wall_s']}s", file=sys.stderr)
            except Exception as e:
                print(f"  {tid} {a} r{r}: FAILED {e}", file=sys.stderr)

    if not args.no_gt:
        subprocess.run([sys.executable, str(lib.HERE / "ground_truth.py"), str(gt_dir), "end", *ids], check=True)


if __name__ == "__main__":
    main()
