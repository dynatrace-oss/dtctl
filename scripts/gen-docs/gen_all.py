#!/usr/bin/env python3
"""
gen_all.py - project driver on top of gen_docs.py.

Maps dtctl's catalog resources onto the PLANNED ~21-file docs structure for
PRODUCT-18391 (including multi-resource grouped pages) and emits one .md per
planned file, plus COMMANDS.md and TOKEN_SCOPES.md. Generated sections
(supported operations + required token scopes) come from the catalog; prose
(overview / output / examples) is left as SME placeholders.

Input:  `dtctl commands --full -o json`
Output: <out-dir>/resources/<file>.md, <out-dir>/COMMANDS.md,
        <out-dir>/TOKEN_SCOPES.md, <out-dir>/INDEX.md (the mapping report)
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

import gen_docs  # reuse the proven core (md_table, matrix, token scopes)

# planned file -> (Title, [singular resource stems], authored_only, note)
PLAN = {
    "workflows":            ("Workflows", ["workflow", "workflow-execution", "wfe-task-result"], False, ""),
    "dql-queries":          ("DQL Queries", [], True,
                             "DQL is the `query`/`exec` verb, not a CRUD resource - author prose + examples; no per-resource ops to generate."),
    "dashboards-notebooks": ("Dashboards & Notebooks", ["dashboard", "notebook"], False, ""),
    "slos":                 ("SLOs", ["slo", "slo-template"], False, ""),
    "settings-api":         ("Settings", ["setting", "settings-schema"], False, ""),
    "cloud-integrations":   ("Cloud Integrations", ["aws", "azure", "gcp"], True,
                             "NOT a CRUD resource in the CLI - no verb exposes aws/azure/gcp; managed via the Settings API (scopes: settings:objects, extensions:configurations). Author prose + examples."),
    "grail-buckets":        ("Grail Buckets", ["bucket"], False, ""),
    "filter-segments":      ("Filter Segments", ["segment"], False, ""),
    "lookup-tables":        ("Lookup Tables", ["lookup"], False, ""),
    "extensions":           ("Extensions", ["extension", "extension-config", "hub-extension", "hub-extension-release"], False,
                             "hub-extensions folded in here per the plan."),
    "app-engine":           ("App Engine", ["app", "function", "intent"], False, ""),
    "analyzers":            ("Analyzers", ["analyzer"], False, ""),
    "copilot":              ("Davis CoPilot", ["copilot", "copilot-skill"], False,
                             "kept split from analyzers; grouped as 'Davis AI' on the docs.dynatrace.com page only."),
    "anomaly-detectors":    ("Anomaly Detectors", ["anomaly-detector"], False, ""),
    "live-debugger":        ("Live Debugger", ["breakpoint", "snapshot"], False, ""),
    "documents":            ("Documents & Trash", ["document", "trash"], False, ""),
    "edgeconnect":          ("EdgeConnect", ["edgeconnect"], False, ""),
    "notifications":        ("Notifications", ["notification"], False, ""),
    "api-discovery":        ("API Discovery", ["api"], False, ""),
    "platform-tokens":      ("Platform Tokens", ["platform-token", "account-token", "token"], False,
                             "verify: may have no resource_scopes entry in the catalog."),
    "users-groups":         ("Users & Groups", ["user", "group"], False, ""),
}


def deplural(x: str) -> str:
    if x.endswith("es") and len(x) > 3:
        return x[:-2]
    if x.endswith("s") and len(x) > 2:
        return x[:-1]
    return x


def matches(key: str, stems: set[str]) -> bool:
    """Does a catalog resource key (singular or plural) belong to this file?"""
    if key in stems:
        return True
    if deplural(key) in stems:
        return True
    if any(key == s + "s" or key == s + "es" for s in stems):
        return True
    return False


def render_grouped_page(title: str, stems: list[str], catalog: dict,
                        raw_index: dict[str, list[dict]], note: str) -> str:
    stemset = set(stems)
    out = [f"# {title}\n"]
    if note:
        out.append(f"<!-- NOTE: {note} -->\n")
    out.append("<!-- SME: Overview - what this resource is in the Dynatrace platform, "
               "when to use it, and how it relates to neighboring resources (1-2 sentences). -->\n")

    # Supported operations, merged across all matching catalog resource forms
    out.append("## Supported operations\n")
    rows = []
    seen = set()
    for canon in sorted(raw_index.keys()):
        if not matches(canon, stemset):
            continue
        for op in sorted(raw_index[canon], key=lambda o: (o["verb"], o["resource_as_written"])):
            k = (op["verb"], op["resource_as_written"])
            if k in seen:
                continue
            seen.add(k)
            syntax = f"`dtctl {op['verb']} {op['resource_as_written']}`"
            rows.append([op["verb"], op["resource_as_written"], syntax,
                         op["description"], "yes" if op["mutating"] else "no", op["access"] or ""])
    out.append(gen_docs.md_table(
        ["Operation", "Resource", "Command syntax", "Description", "Mutating", "Access"], rows))

    # Flags: verb-level only (catalog has no per-resource flags)
    out.append("\n## Flags\n")
    out.append("Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, "
               "`--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags "
               "(`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. "
               "The catalog exposes no per-resource flags.\n")

    # Required token scopes, unioned across matching resource_scopes keys
    out.append("\n## Required token scopes\n")
    resource_scopes = catalog.get("resource_scopes", {})
    by_level: dict[str, set[str]] = {}
    for key, levels in resource_scopes.items():
        if not matches(key, stemset):
            continue
        for level, scopes in levels.items():
            by_level.setdefault(level, set()).update(scopes)
    scope_rows = [[level, ", ".join(f"`{s}`" for s in sorted(by_level[level]))]
                  for level in sorted(by_level.keys())]
    out.append(gen_docs.md_table(["Safety level", "Scopes"], scope_rows))

    out.append("\n## Output\n")
    out.append("<!-- SME: describe the returned shape (key fields, id/name conventions) "
               "and how -o json / -o wide differ. -->\n")
    out.append("\n## Examples\n")
    out.append("<!-- SME: 3-5 real invocations with sample output. -->\n")
    return "\n".join(out) + "\n"


def render_authored_stub(title: str, note: str) -> str:
    return (f"# {title}\n\n<!-- NOTE: {note} -->\n\n"
            "## Overview\n<!-- SME -->\n\n"
            "## Usage\n<!-- SME: this page is authored, not generated -->\n\n"
            "## Examples\n<!-- SME -->\n")


def build_raw_index(catalog: dict) -> dict[str, list[dict]]:
    """Like gen_docs.build_resource_index but we keep it here to control merge."""
    return gen_docs.build_resource_index(catalog)


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: gen_all.py <commands-full.json> [out-dir]", file=sys.stderr)
        return 2
    catalog = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    out_dir = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("out-all")
    (out_dir / "resources").mkdir(parents=True, exist_ok=True)

    raw_index = build_raw_index(catalog)
    all_keys = set(raw_index.keys()) | set(catalog.get("resource_scopes", {}).keys())

    index_rows = []
    covered_keys: set[str] = set()
    for fname, (title, stems, authored, note) in PLAN.items():
        if authored:
            page = render_authored_stub(title, note)
            gen = "authored"
        else:
            page = render_grouped_page(title, stems, catalog, raw_index, note)
            gen = "generated"
            covered_keys.update(k for k in all_keys if matches(k, set(stems)))
        (out_dir / "resources" / f"{fname}.md").write_text(page, encoding="utf-8")
        index_rows.append([f"`resources/{fname}.md`", title, ", ".join(stems) or "-", gen])

    # cross-cutting generated files (reuse proven core)
    (out_dir / "COMMANDS.md").write_text(gen_docs.render_commands_matrix(catalog), encoding="utf-8")
    (out_dir / "TOKEN_SCOPES.md").write_text(gen_docs.render_token_scopes(catalog), encoding="utf-8")

    # coverage report: which catalog resource keys did NOT land in any file
    uncovered = sorted(k for k in all_keys if k not in covered_keys and deplural(k) not in {deplural(c) for c in covered_keys})
    idx = ["# INDEX - planned file -> catalog resources\n",
           gen_docs.md_table(["File", "Title", "Catalog stems", "Mode"], index_rows),
           "\n## Catalog resource keys not mapped to any file\n",
           "_(review these - either fold into a file or confirm intentionally excluded)_\n\n",
           gen_docs.md_table(["Unmapped key"], [[k] for k in uncovered]) if uncovered else "_none_\n"]
    (out_dir / "INDEX.md").write_text("\n".join(idx), encoding="utf-8")

    print(f"wrote {len(PLAN)} resource files + COMMANDS.md + TOKEN_SCOPES.md + INDEX.md to {out_dir}")
    print(f"unmapped catalog keys: {uncovered}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
