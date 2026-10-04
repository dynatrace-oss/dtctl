#!/usr/bin/env bash
# Build the two binaries the arms use:
#   bin/dtctl-main     from origin/main (control: no recipes)       [BASE=<ref> to override]
#   bin/dtctl-recipes  from the working tree (recipes branch)
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
base=${BASE:-origin/main}
mkdir -p "$here/bin"

echo "building dtctl-recipes from the working tree"
(cd "$repo" && go build -o "$here/bin/dtctl-recipes" .)

echo "building dtctl-main from $base"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git -C "$repo" archive "$base" | tar -x -C "$tmp"
(cd "$tmp" && go build -o "$here/bin/dtctl-main" .)

# the dtctl skill each arm gets, taken from the same two trees
mkdir -p "$here/bin/skill-main" "$here/bin/skill-recipes"
cp -r "$tmp/skills/dtctl/." "$here/bin/skill-main/"
cp -r "$repo/skills/dtctl/." "$here/bin/skill-recipes/"
git -C "$repo" rev-parse --short "$base" > "$here/bin/main.rev"
git -C "$repo" rev-parse --short HEAD > "$here/bin/recipes.rev"
"$here/bin/dtctl-main" version 2>/dev/null | head -1 || true
"$here/bin/dtctl-recipes" version 2>/dev/null | head -1 || true
