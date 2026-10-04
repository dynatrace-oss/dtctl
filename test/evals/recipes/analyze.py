#!/usr/bin/env python3
"""Aggregate a judged batch: per-arm and per-task tables, paired differences.

    analyze.py <batch> [<batch> ...] [--json out.json]

Several batches can be pooled (e.g. a pilot plus the full matrix, or a later
batch with more reps): runs are keyed by task/arm/rep within each batch.
Prints Markdown with no tenant data in it — task ids, arms and numbers only —
so the tables can go into the write-up as they are.
"""
import argparse
import json
import random
import re
import shlex
import statistics as st
import sys
from collections import defaultdict
from pathlib import Path

import lib

ALLOWED_BASH = ("dtctl", "jq", "head", "tail", "wc", "sort", "uniq", "grep", "cut", "sed", "awk", "tr", "cat",
                "column", "cd", "echo", "printf", "date", "ls", "true")


def argv_of(f):
    return [a for a in f.read_bytes().split(b"\0") if a != b""]


def verb_of(args):
    skip = False
    pos = []
    for a in args:
        a = a.decode(errors="replace")
        if skip:
            skip = False
            continue
        if a in ("-o", "--output", "--context", "-c", "--jq", "--config"):
            skip = True
            continue
        if a.startswith("-"):
            continue
        pos.append(a)
    return pos


def is_empty(out_text):
    """An ok envelope with nothing in it."""
    try:
        d = json.loads(out_text)
    except (json.JSONDecodeError, ValueError):
        return out_text.strip() == ""
    if not isinstance(d, dict) or not d.get("ok", True):
        return False
    ctx = d.get("context") or {}
    if ctx.get("total") == 0 or ctx.get("empty_reason"):
        return True
    r = d.get("result")
    if r in ([], None, ""):
        return True
    if isinstance(r, dict):
        recs = r.get("records")
        if recs in ([], "", None) and r.get("kind") == "records":
            return True
    return False


def call_metrics(ws):
    calls = sorted((ws / "calls").glob("*.argv"))
    m = dict(calls=len(calls), errors=0, refused=0, empties=0, runs=0, recipe_list=0, recipe_describe=0,
             recipes_run=[], queries=0, exec_ms=0)
    for f in calls:
        stem = f.with_suffix("")
        args = argv_of(f)
        pos = verb_of(args)
        if stem.with_suffix(".refused").exists():
            m["refused"] += 1
            continue
        rc = (stem.with_suffix(".rc").read_text().strip() if stem.with_suffix(".rc").exists() else "?")
        if rc != "0":
            m["errors"] += 1
        out = stem.with_suffix(".out")
        if rc == "0" and out.exists() and is_empty(out.read_text(errors="replace")):
            m["empties"] += 1
        ms = stem.with_suffix(".ms")
        if ms.exists() and ms.read_text().strip().isdigit():
            m["exec_ms"] += int(ms.read_text().strip())
        if pos[:1] == ["run"] and len(pos) > 1:
            m["runs"] += 1
            m["recipes_run"].append(pos[1])
        elif pos[:2] == ["get", "recipes"] or pos[:2] == ["get", "recipe"]:
            m["recipe_list"] += 1
        elif pos[:2] == ["describe", "recipe"]:
            m["recipe_describe"] += 1
        elif pos[:1] == ["query"] or pos[:2] == ["exec", "dql"]:
            m["queries"] += 1
    return m


def command_words(cmd):
    """The program names of a shell command line (quotes respected)."""
    try:
        toks = list(shlex.shlex(cmd, posix=True, punctuation_chars=True))
    except ValueError:
        return [cmd.split()[0]] if cmd.split() else []
    words, expect = [], True
    for t in toks:
        if t in ("|", "||", "&&", ";", "&", "(", ")"):
            expect = True
        elif expect:
            if "=" in t and not t.startswith("="):  # VAR=value prefix
                continue
            words.append(t)
            expect = False
        elif t in (">", ">>", "<", ">&", "2>&1"):
            pass
    return words


def transcript_metrics(ws):
    m = dict(cost=None, turns=None, duration_s=None, in_tokens=None, out_tokens=None, skills=[],
             skill_refs=0, bash=0, off_policy=[], result_error=None)
    p = ws / "transcript.jsonl"
    if not p.exists():
        return m
    for line in p.read_text().splitlines():
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("type") == "assistant":
            for c in e["message"]["content"]:
                if c.get("type") != "tool_use":
                    continue
                if c["name"] == "Skill":
                    m["skills"].append(c["input"].get("skill") or c["input"].get("command") or "?")
                elif c["name"] == "Read":
                    m["skill_refs"] += 1
                    fp = c["input"].get("file_path", "")
                    if "/cfg/skills/" not in fp:
                        m["off_policy"].append("Read " + fp[-60:])
                elif c["name"] == "Bash":
                    m["bash"] += 1
                    cmd = c["input"].get("command", "")
                    for path in re.findall(r"(?:~|/)[\w./-]+", cmd):
                        outside_ws = path.startswith(("~", "/home", "/etc", "/usr", "/var", "/opt", "/root")) or \
                            (str(ws.parent.parent.parent) in path and "/cfg/skills/" not in path
                             and str(ws) not in path)
                        if outside_ws:
                            m["off_policy"].append("Path " + path[-60:])
                        if "_gt" in path or "/repos/" in path or "/test/evals" in path:
                            m["off_policy"].append("FORBIDDEN " + path[-60:])
                    for w in command_words(cmd):
                        if w not in ALLOWED_BASH:
                            m["off_policy"].append("Bash " + w)
                else:
                    m["off_policy"].append(c["name"])
        elif e.get("type") == "result":
            u = e.get("usage") or {}
            m.update(cost=e.get("total_cost_usd"), turns=e.get("num_turns"),
                     duration_s=round((e.get("duration_ms") or 0) / 1000, 1),
                     in_tokens=sum(u.get(k) or 0 for k in ("input_tokens", "cache_creation_input_tokens",
                                                           "cache_read_input_tokens")),
                     out_tokens=u.get("output_tokens"), result_error=e.get("is_error"))
    return m


def collect(batches):
    env = lib.load_env()
    rows = []
    for b in batches:
        bd = lib.runs_dir(env) / b
        for ws in sorted(bd.glob("t*/*-r*")):
            if not (ws / "meta.json").exists():
                continue
            meta = json.loads((ws / "meta.json").read_text())
            j = json.loads((ws / "judge.json").read_text()) if (ws / "judge.json").exists() else {}
            r = dict(batch=b, task=meta["task"], arm=meta["arm"], rep=meta["rep"], wall_s=meta["wall_s"],
                     rc=meta["rc"], score=j.get("score"), notes=j.get("notes"), err_tags=j.get("errors", []))
            r.update(call_metrics(ws))
            r.update(transcript_metrics(ws))
            rows.append(r)
    return rows


def mean(xs):
    xs = [x for x in xs if x is not None]
    return st.mean(xs) if xs else float("nan")


def sd(xs):
    xs = [x for x in xs if x is not None]
    return st.stdev(xs) if len(xs) > 1 else 0.0


def paired(rows, a, b, n_boot=10000, seed=1):
    """Per-task mean score difference b - a, bootstrap CI over tasks, sign counts."""
    per = defaultdict(lambda: defaultdict(list))
    for r in rows:
        if r["score"] is not None:
            per[r["task"]][r["arm"]].append(r["score"])
    diffs = {t: mean(v[b]) - mean(v[a]) for t, v in per.items() if v.get(a) and v.get(b)}
    if not diffs:
        return None
    d = list(diffs.values())
    rnd = random.Random(seed)
    boots = sorted(mean([rnd.choice(d) for _ in d]) for _ in range(n_boot))
    return dict(mean=mean(d), lo=boots[int(0.025 * n_boot)], hi=boots[int(0.975 * n_boot)],
                better=sum(x > 0 for x in d), worse=sum(x < 0 for x in d), same=sum(x == 0 for x in d),
                per_task=diffs)


def fmt(x, nd=2):
    return "-" if x is None or x != x else f"{x:.{nd}f}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("batches", nargs="+")
    ap.add_argument("--json")
    args = ap.parse_args()
    rows = collect(args.batches)
    arms = sorted({r["arm"] for r in rows})
    tasks = sorted({r["task"] for r in rows})
    meta = {t["id"]: t["meta"] for t in lib.load_tasks()}

    print("## Per arm\n")
    print("| arm | runs | mean score | sd (runs) | sd of task means across reps | fully correct (3) | zero | "
          "dtctl calls | errored | empty | recipe runs | cost $ | tokens in (k) | tokens out (k) | wall s |")
    print("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    for a in arms:
        rs = [r for r in rows if r["arm"] == a]
        sc = [r["score"] for r in rs]
        # run-to-run spread: sd of the per-rep mean score over tasks
        by_rep = defaultdict(list)
        for r in rs:
            by_rep[(r["batch"], r["rep"])].append(r["score"])
        rep_means = [mean(v) for v in by_rep.values() if len(v) == len(tasks)]
        print(f"| {a} | {len(rs)} | {fmt(mean(sc))} | {fmt(sd(sc))} | {fmt(sd(rep_means))} | "
              f"{sum(s == 3 for s in sc)} | {sum(s == 0 for s in sc)} | "
              f"{fmt(mean([r['calls'] for r in rs]), 1)} | {fmt(mean([r['errors'] for r in rs]), 1)} | "
              f"{fmt(mean([r['empties'] for r in rs]), 1)} | {fmt(mean([r['runs'] for r in rs]), 1)} | "
              f"{fmt(mean([r['cost'] for r in rs]), 3)} | {fmt(mean([r['in_tokens'] for r in rs]) / 1000, 0)} | "
              f"{fmt(mean([r['out_tokens'] for r in rs]) / 1000, 1)} | {fmt(mean([r['wall_s'] for r in rs]), 0)} |")
    print("\nTotals: " + ", ".join(
        f"{a}: ${sum(r['cost'] or 0 for r in rows if r['arm'] == a):.2f}" for a in arms))

    print("\n## Per task (scores per rep; mean calls)\n")
    print("| task | domain | coverage | kind | " + " | ".join(f"{a} scores | {a} calls" for a in arms) + " |")
    print("|---|---|---|---|" + "---|---|" * len(arms))
    for t in tasks:
        m = meta.get(t, {})
        cells = []
        for a in arms:
            rs = sorted([r for r in rows if r["task"] == t and r["arm"] == a], key=lambda r: (r["batch"], r["rep"]))
            cells.append(" ".join(str(r["score"]) for r in rs) or "-")
            cells.append(fmt(mean([r["calls"] for r in rs]), 1))
        print(f"| {t} | {m.get('domain', '')} | {m.get('coverage', '')} | {m.get('kind', '')} | " +
              " | ".join(cells) + " |")

    print("\n## By coverage\n")
    print("| coverage | " + " | ".join(arms) + " |")
    print("|---|" + "---|" * len(arms))
    for cov in ("covered", "partial", "none"):
        cells = [fmt(mean([r["score"] for r in rows if r["arm"] == a and meta.get(r["task"], {}).get("coverage") == cov]))
                 for a in arms]
        print(f"| {cov} | " + " | ".join(cells) + " |")

    print("\n## Paired differences (per-task mean, bootstrap 95% CI over tasks)\n")
    for a, b in (("A", "B"), ("A", "C"), ("B", "C"), ("A2", "B2"), ("A2", "B3"), ("A", "A2"), ("B", "B2")):
        if a in arms and b in arms:
            p = paired(rows, a, b)
            if p:
                print(f"- {b} - {a}: {p['mean']:+.2f} [{p['lo']:+.2f}, {p['hi']:+.2f}]; "
                      f"{b} better on {p['better']}, worse on {p['worse']}, tied on {p['same']} tasks")
                big = {t: round(d, 2) for t, d in sorted(p["per_task"].items()) if abs(d) >= 1}
                if big:
                    print(f"  - |diff| >= 1: {big}")

    print("\n## Recipe and skill usage\n")
    for a in arms:
        rs = [r for r in rows if r["arm"] == a]
        used = sum(1 for r in rs if r["runs"])
        listed = sum(1 for r in rs if r["recipe_list"] or r["recipe_describe"])
        skill_runs = sum(1 for r in rs if r["skills"])
        sk = defaultdict(int)
        for r in rs:
            for s_ in set(r["skills"]):
                sk[s_] += 1
        rec = defaultdict(int)
        for r in rs:
            for n in r["recipes_run"]:
                rec[n] += 1
        print(f"- {a}: runs using >=1 recipe {used}/{len(rs)}, browsing recipes {listed}/{len(rs)}, "
              f"loading >=1 skill {skill_runs}/{len(rs)}; skills: {dict(sorted(sk.items(), key=lambda x: -x[1]))}")
        if rec:
            print(f"  - recipes run: {dict(sorted(rec.items(), key=lambda x: -x[1]))}")
    sc_with = [r["score"] for r in rows if r["arm"] != "A" and r["runs"]]
    sc_without = [r["score"] for r in rows if r["arm"] != "A" and not r["runs"]]
    print(f"- recipes arms, runs that used a recipe: mean {fmt(mean(sc_with))} (n={len(sc_with)}); "
          f"that did not: {fmt(mean(sc_without))} (n={len(sc_without)})")

    off = [(r["task"], r["arm"], r["rep"], r["off_policy"]) for r in rows if r["off_policy"]]
    print("\n## Policy audit\n")
    print(f"- runs with off-policy tool use: {len(off)}")
    for o in off[:20]:
        print(f"  - {o}")
    print(f"- runs refused by the call budget or verb guard: {sum(1 for r in rows if r['refused'])}")
    print(f"- runs not finishing cleanly: {[(r['task'], r['arm'], r['rep'], r['rc']) for r in rows if r['rc'] != 0]}")

    if args.json:
        Path(args.json).write_text(json.dumps(rows, indent=1, default=str))


if __name__ == "__main__":
    main()
