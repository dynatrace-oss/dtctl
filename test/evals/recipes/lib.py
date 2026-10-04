"""Shared helpers for the recipes-vs-skills eval harness.

Everything tenant-specific comes from env.sh (git-ignored); nothing here names a
tenant, a URL or a service.
"""
import json
import os
import re
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent.parent
TASKS = HERE / "tasks"
BIN = HERE / "bin"


def load_env():
    """Source env.sh in bash and return the EVAL_* variables."""
    env_file = HERE / "env.sh"
    if not env_file.exists():
        sys.exit(f"missing {env_file} - copy env.example.sh and fill it in")
    out = subprocess.run(
        ["bash", "-c", f'set -a; source "{env_file}"; env -0'],
        check=True, capture_output=True, text=True).stdout
    env = {}
    for kv in out.split("\0"):
        if kv.startswith("EVAL_") and "=" in kv:
            k, v = kv.split("=", 1)
            env[k] = v
    env.setdefault("EVAL_MODEL", "sonnet")
    env.setdefault("EVAL_JUDGE_MODEL", "opus")
    env.setdefault("EVAL_PARALLEL", "6")
    # not under $HOME: claude loads .claude/ and CLAUDE.md from every ancestor of its cwd
    env.setdefault("EVAL_RUNS_DIR", str(Path(os.environ.get("TMPDIR", "/tmp")) / "dtctl-recipes-evals"))
    return env


def runs_dir(env):
    p = Path(env["EVAL_RUNS_DIR"])
    p.mkdir(parents=True, exist_ok=True)
    return p


def tenant_context(env, tenant):
    """Map a task's tenant label (T1/T2) to the dtctl context name."""
    return env["EVAL_" + tenant]


# ---------------------------------------------------------------- read-only config

def make_readonly_config(env, context, dest):
    """Write a single-context dtctl config for `context` with safety-level readonly.

    Reads the user's real config through `dtctl config view` (so this never
    parses YAML by hand) and keeps only environment + token-ref: the token
    itself stays in the keyring and is looked up by that name.
    """
    src = subprocess.run(
        [str(BIN / "dtctl-main"), "--plain", "--no-agent", "config", "view", "-o", "json"],
        check=True, capture_output=True, text=True,
        env={k: v for k, v in os.environ.items() if k != "DTCTL_CONFIG"}).stdout
    cfg = json.loads(src)
    ctx = next((c for c in cfg["Contexts"] if c["Name"] == context), None)
    if ctx is None:
        sys.exit(f"context {context!r} not in your dtctl config")
    c = ctx["Context"]
    dest = Path(dest)
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists():
        dest.chmod(0o644)
    dest.write_text(
        "apiVersion: v1\nkind: Config\n"
        f"current-context: {context}\n"
        "contexts:\n"
        f"    - name: {context}\n"
        "      context:\n"
        f"        environment: {c['Environment']}\n"
        f"        token-ref: {c['TokenRef']}\n"
        "        safety-level: readonly\n")
    dest.chmod(0o444)
    return dest


def isolated_env(cfg_path, iso_dir):
    """Environment for a dtctl process: read-only config, private XDG dirs.

    The private XDG_CONFIG_HOME matters: dtctl loads user recipe books from
    $XDG_CONFIG_HOME/dtctl/recipes, so a developer's own recipes would leak into
    the recipes arm otherwise. XDG_RUNTIME_DIR / DBUS stay, so the keyring works.
    """
    iso = Path(iso_dir)
    e = dict(os.environ)
    e.pop("DTCTL_CONTEXT", None)
    e["DTCTL_CONFIG"] = str(cfg_path)
    for k in ("CONFIG", "CACHE", "STATE", "DATA"):
        d = iso / k.lower()
        d.mkdir(parents=True, exist_ok=True)
        e[f"XDG_{k}_HOME"] = str(d)
    return e


# ---------------------------------------------------------------- queries

class QueryError(RuntimeError):
    pass


def query(cfg_path, iso_dir, dql, binary="dtctl-main", timeout=300):
    """Run DQL with the main-branch binary, return the list of records."""
    p = subprocess.run(
        [str(BIN / binary), "--plain", "--no-agent", "query", dql, "-o", "json"],
        capture_output=True, text=True, timeout=timeout,
        env=isolated_env(cfg_path, iso_dir))
    if p.returncode != 0:
        raise QueryError(f"{p.stderr.strip()[:500]}\nDQL: {dql}")
    out = p.stdout.strip()
    if not out:
        return []
    d = json.loads(out)
    return d.get("records", d) if isinstance(d, dict) else d


READ_VERBS = ("get", "describe")
READ_EXECS = ("slo",)  # `exec slo` evaluates an SLO: OperationRead


def command_json(cfg_path, iso_dir, args, binary="dtctl-main", timeout=300):
    """Run a read-only dtctl command with `-o json` and parse its output.

    Only get/describe and `exec slo` are allowed. Some commands print a
    progress line before the JSON document, so parsing starts at the first
    line that opens one.
    """
    args = list(args)
    if not (args[0] in READ_VERBS or (args[0] == "exec" and args[1] in READ_EXECS)):
        raise ValueError(f"not a read-only command: {args}")
    p = subprocess.run(
        [str(BIN / binary), "--plain", "--no-agent", *args, "-o", "json"],
        capture_output=True, text=True, timeout=timeout,
        env=isolated_env(cfg_path, iso_dir))
    if p.returncode != 0:
        raise QueryError(f"{p.stderr.strip()[:500]}\nargs: {args}")
    lines = p.stdout.splitlines(keepends=True)
    start = next((i for i, ln in enumerate(lines) if ln.lstrip()[:1] in ("{", "[")), None)
    if start is None:
        return None
    return json.loads("".join(lines[start:]))


def num(v):
    """dtctl renders long numbers as strings in JSON; coerce."""
    if v is None:
        return None
    if isinstance(v, (int, float)):
        return v
    try:
        f = float(v)
        return int(f) if f.is_integer() else f
    except (TypeError, ValueError):
        return v


# ---------------------------------------------------------------- tasks

def load_tasks(env=None, only=None):
    tasks = []
    for f in sorted(TASKS.glob("t*.md")):
        tid = f.stem
        if only and tid not in only:
            continue
        text = f.read_text()
        m = re.match(r"---\n(.*?)\n---\n(.*)", text, re.S)
        meta = {}
        for line in m.group(1).splitlines():
            k, _, v = line.partition(":")
            meta[k.strip()] = v.strip()
        body = m.group(2)
        prompt = re.search(r"## Prompt\n(.*?)\n## Rubric", body, re.S).group(1).strip()
        rubric = re.search(r"## Rubric\n(.*)", body, re.S).group(1).strip()
        if env is not None:
            prompt = expand(prompt, env)
        tasks.append(dict(id=tid, meta=meta, prompt=prompt, rubric=rubric))
    return tasks


def load_taskset(name):
    """Task ids listed in tasks/<name>.txt (one per line, '#' comments)."""
    f = TASKS / f"{name}.txt"
    if not f.exists():
        sys.exit(f"no task set {f}")
    return {ln.split("#")[0].strip() for ln in f.read_text().splitlines()} - {""}


def expand(text, env):
    def sub(m):
        k = m.group(1)
        if k not in env:
            sys.exit(f"{k} is not set in env.sh")
        return env[k]
    return re.sub(r"\$\{(EVAL_[A-Z0-9_]+)\}", sub, text)
