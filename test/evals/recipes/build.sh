#!/usr/bin/env bash
# Build the two binaries the arms use:
#   bin/dtctl-main     from origin/main (control: no recipes)       [BASE=<ref> to override]
#   bin/dtctl-recipes  from the working tree (recipes branch)       [RECIPES_REF=<ref> to build a commit]
# and copy each tree's skills/dtctl next to them.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../../.." && pwd)
base=${BASE:-origin/main}
mkdir -p "$here/bin"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

rm -rf "$here/bin/skill-main" "$here/bin/skill-recipes"
mkdir -p "$here/bin/skill-main" "$here/bin/skill-recipes"

if [ -n "${RECIPES_REF:-}" ]; then
  echo "building dtctl-recipes from $RECIPES_REF"
  mkdir -p "$tmp/recipes"
  git -C "$repo" archive "$RECIPES_REF" | tar -x -C "$tmp/recipes"
  (cd "$tmp/recipes" && go build -o "$here/bin/dtctl-recipes" .)
  cp -r "$tmp/recipes/skills/dtctl/." "$here/bin/skill-recipes/"
  git -C "$repo" rev-parse --short "$RECIPES_REF" > "$here/bin/recipes.rev"
else
  echo "building dtctl-recipes from the working tree"
  (cd "$repo" && go build -o "$here/bin/dtctl-recipes" .)
  cp -r "$repo/skills/dtctl/." "$here/bin/skill-recipes/"
  echo "$(git -C "$repo" rev-parse --short HEAD)+worktree" > "$here/bin/recipes.rev"
fi

echo "building dtctl-main from $base"
mkdir -p "$tmp/main"
git -C "$repo" archive "$base" | tar -x -C "$tmp/main"
(cd "$tmp/main" && go build -o "$here/bin/dtctl-main" .)
cp -r "$tmp/main/skills/dtctl/." "$here/bin/skill-main/"
git -C "$repo" rev-parse --short "$base" > "$here/bin/main.rev"

"$here/bin/dtctl-main" version 2>/dev/null | head -1 || true
"$here/bin/dtctl-recipes" version 2>/dev/null | head -1 || true
