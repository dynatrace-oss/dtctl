#!/usr/bin/env python3
"""Reject a change that withdraws stable surface.

The checked-in manifest is gated for *freshness*: `go test ./test/stability/`
fails when `docs/STABILITY.md` disagrees with the command tree. That catches an
undeclared change, but it cannot catch a declared one -- delete a stable flag,
run `make stability-manifest`, commit the regenerated file, and the gate is
green again. The promise the manifest exists to record would then be broken by
the very commit that updated the record of it.

So this compares the manifest against its state on another ref (the PR base, or
the last release) and refuses two transitions:

  * a stable entry disappearing -- unless it was deprecated on the base, which
    is exactly the cycle a stable removal is supposed to go through;
  * a stable entry coming back weaker, which is a demotion and always needs a
    major release rather than a manifest regeneration.

Promotions, additions, and anything below stable are free: the tiers below
stable promise nothing, and this check exists to protect a promise.

Usage:
    check_compat.py [--base REF_OR_FILE] [--head FILE]
"""

import argparse
import re
import subprocess
import sys

MANIFEST = "docs/STABILITY.md"
ROW = re.compile(
    r"^(?P<indent>\s*)(?P<name>\S+(?: \S+)*?)\s{2,}"
    r"(?P<level>stable|experimental|development)\b(?P<rest>.*)$"
)


def surface(text):
    """Parse the manifest's Surface block into {entry key: (level, deprecated)}.

    A flag is keyed by the command it hangs off, so `query --spill` vanishing is
    a different finding from `query` vanishing.
    """
    _, _, body = text.partition("## Surface")
    if not body:
        sys.exit("manifest has no '## Surface' section: refusing to compare")

    entries, command = {}, None
    for line in body.split("\n"):
        m = ROW.match(line)
        if not m:
            continue
        name, level, rest = m["name"], m["level"], m["rest"]
        if not m["indent"]:
            command = name
            key = name
        elif command and name.startswith("--"):
            key = "%s %s" % (command, name)
        else:
            continue
        entries[key] = (level, "deprecated" in rest)
    if not entries:
        sys.exit("parsed no entries out of the Surface section: refusing to compare")
    return entries


def read(ref):
    if "/" in ref or ref.endswith(".md"):
        try:
            with open(ref, encoding="utf-8") as fh:
                return fh.read()
        except OSError:
            pass  # not a path after all; try it as a git ref
    out = subprocess.run(
        ["git", "show", "%s:%s" % (ref, MANIFEST)],
        capture_output=True, text=True, check=False,
    )
    if out.returncode != 0:
        sys.exit("cannot read %s at %r: %s" % (MANIFEST, ref, out.stderr.strip()))
    return out.stdout


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="origin/main",
                    help="git ref or file to compare against (default: origin/main)")
    ap.add_argument("--head", default=MANIFEST, help="manifest to check")
    args = ap.parse_args()

    base, head = surface(read(args.base)), surface(read(args.head))

    removed, demoted = [], []
    for key, (level, deprecated) in sorted(base.items()):
        if level != "stable":
            continue
        if key not in head:
            if not deprecated:
                removed.append(key)
            continue
        if head[key][0] != "stable":
            demoted.append((key, head[key][0]))

    if not removed and not demoted:
        print("OK Stable surface intact against %s (%d stable entries checked)."
              % (args.base, sum(1 for lvl, _ in base.values() if lvl == "stable")))
        return 0

    print("Stable surface withdrawn relative to %s:\n" % args.base)
    for key in removed:
        print("  removed   %s" % key)
    for key, now in demoted:
        print("  demoted   %s  stable -> %s" % (key, now))
    print(
        "\nStable means additive-only, so neither is a cleanup:\n"
        "  * to remove it, deprecate it first (stability.Deprecate) and remove it\n"
        "    in a later release -- a deprecated entry may then disappear;\n"
        "  * to weaken it, that is a breaking change and needs a major release,\n"
        "    not a regenerated manifest.\n"
        "If the entry was never really stable, say so in the commit and in\n"
        "dtctl-contrib breaking-changes/ before overriding this check."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
