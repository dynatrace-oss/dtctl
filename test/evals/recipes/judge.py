#!/usr/bin/env python3
"""Blind judge: score every run of a batch 0-3 against ground truth.

    judge.py <batch> [--force] [--parallel N]

The judge sees the question, the rubric, the ground truth (start and end of
the batch) and the agent's final message — never the arm, the binary, the
skills or the tool calls. Writes judge.json next to each run's transcript.
"""
import argparse
import concurrent.futures as cf
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import lib

JUDGE_PROMPT = """You are grading an answer to a question about a monitoring environment.
Score it strictly with the rubric, against the ground truth. The ground truth
was measured twice, at the start and at the end of the evaluation window;
values that moved between the two are volatile, and an answer within or near
that range is correct. Judge only the final answer and the claims it makes,
not its style. Numbers that are rounded or abbreviated (e.g. "~221k") count as
the value they denote. Do not reward hedging: a list of possibilities that
happens to contain the right answer without committing to it scores as if the
right answer were missing.

## Question
{question}

## Rubric
{rubric}

## Ground truth (start of batch)
```json
{gt_start}
```

## Ground truth (end of batch)
```json
{gt_end}
```

## Answer to grade
<answer>
{answer}
</answer>

Reply with only a JSON object, no prose around it:
{{"score": <0-3>, "notes": "<one or two sentences: what was right, what was wrong>",
 "errors": ["<short tag per mistake, e.g. counted-duplicates, wrong-window, unit, array-eq, capped-list, wrong-entity, no-answer>"]}}
"""


def final_answer(ws):
    """The agent's last message (the result event), or the last assistant text."""
    text, last = None, None
    p = ws / "transcript.jsonl"
    if not p.exists():
        return None
    for line in p.read_text().splitlines():
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("type") == "result":
            text = e.get("result")
        elif e.get("type") == "assistant":
            for c in e["message"]["content"]:
                if c.get("type") == "text" and c["text"].strip():
                    last = c["text"]
    return text or last


def judge_one(env, task, gt_start, gt_end, ws, force):
    out = ws / "judge.json"
    if out.exists() and not force:
        return json.loads(out.read_text())
    answer = final_answer(ws)
    if not answer:
        res = dict(score=0, notes="no final answer (run produced no message)", errors=["no-answer"])
        out.write_text(json.dumps(res, indent=2))
        return res
    prompt = JUDGE_PROMPT.format(
        question=task["prompt"], rubric=task["rubric"],
        gt_start=json.dumps(gt_start.get(task["id"]), indent=1, default=str),
        gt_end=json.dumps(gt_end.get(task["id"], gt_start.get(task["id"])), indent=1, default=str),
        answer=answer.strip())
    with tempfile.TemporaryDirectory(prefix="judge-") as tmp:
        cfg = Path(tmp) / "cfg"
        cfg.mkdir()
        cred = Path.home() / ".claude" / ".credentials.json"
        if cred.exists():
            shutil.copy(cred, cfg / ".credentials.json")
        e = {k: os.environ[k] for k in ("USER", "LANG", "TERM", "PATH") if k in os.environ}
        e.update(HOME=tmp, CLAUDE_CONFIG_DIR=str(cfg), DISABLE_AUTOUPDATER="1")
        last_err = None
        for _ in range(3):
            p = subprocess.run(
                ["claude", "-p", "--model", env["EVAL_JUDGE_MODEL"], "--output-format", "json",
                 "--tools", "", "--strict-mcp-config", "--no-session-persistence", "--max-turns", "1"],
                input=prompt, capture_output=True, text=True, cwd=tmp, env=e, timeout=600)
            try:
                r = json.loads(p.stdout)
                m = re.search(r"\{.*\}", r["result"], re.S)
                res = json.loads(m.group(0))
                res["score"] = int(res["score"])
                res["judge_cost_usd"] = r.get("total_cost_usd")
                break
            except Exception as ex:  # retry on a malformed reply
                last_err = f"{ex}: {p.stdout[:300]} {p.stderr[:300]}"
        else:
            raise RuntimeError(last_err)
    out.write_text(json.dumps(res, indent=2))
    (ws / "judge_prompt.md").write_text(prompt)
    return res


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("batch")
    ap.add_argument("--force", action="store_true")
    ap.add_argument("--parallel", type=int, default=6)
    args = ap.parse_args()
    env = lib.load_env()
    bd = lib.runs_dir(env) / args.batch
    gt_start = json.loads((bd / "_gt" / "gt-start.json").read_text())
    gt_end_p = bd / "_gt" / "gt-end.json"
    gt_end = json.loads(gt_end_p.read_text()) if gt_end_p.exists() else gt_start
    tasks = {t["id"]: t for t in lib.load_tasks(env)}
    jobs = [(tasks[ws.parent.name], ws) for ws in sorted(bd.glob("t*/*-r*")) if (ws / "meta.json").exists()]
    with cf.ThreadPoolExecutor(args.parallel) as ex:
        futs = {ex.submit(judge_one, env, t, gt_start, gt_end, ws, args.force): ws for t, ws in jobs}
        for f in cf.as_completed(futs):
            ws = futs[f]
            try:
                r = f.result()
                print(f"{ws.parent.name} {ws.name}: {r['score']}  {r['notes'][:110]}", file=sys.stderr)
            except Exception as e:
                print(f"{ws.parent.name} {ws.name}: JUDGE FAILED {e}", file=sys.stderr)


if __name__ == "__main__":
    main()
