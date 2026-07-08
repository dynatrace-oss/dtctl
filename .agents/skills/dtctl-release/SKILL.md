---
name: dtctl-release
description: Explain and operate the dtctl release process, which is automated by release-please. Use this skill whenever the user says "release", "ship it", "cut a release", "new version", "bump version", "publish", or asks how dtctl releases work, why a release PR exists, or how to trigger/finish a release.
---

# dtctl Release Process (release-please)

dtctl releases use [release-please](https://github.com/googleapis/release-please) but are **triggered manually** — **nothing releases on ordinary merges to `main`**. There is no manual version bump, no `CHANGELOG.md` to edit, and no tag to push by hand. The release notes and version are computed from the [Conventional Commits](https://www.conventionalcommits.org/) since the last release.

## How it works

The `.github/workflows/release.yml` workflow runs **only when you trigger it** (`workflow_dispatch` — the "Run workflow" button on the Actions tab, or `gh workflow run`). A full release is **two dispatches with a merge in between**:

1. **Dispatch #1** — the `release-please` job inspects the conventional commits since the last release and opens/updates a **release PR** titled like `chore(main): release 0.31.0`. That PR:
   - bumps `.release-please-manifest.json`,
   - bumps the fallback version in `pkg/version/version.go` (via the `// x-release-please-version` annotation),
   - and contains the pending release notes (in the PR body).
2. **Merge the release PR** once you're happy with the version + notes.
3. **Dispatch #2** — release-please detects the merged release PR, creates the git tag (`vX.Y.Z`) and the **GitHub Release** with generated notes, and `release_created == true` gates the `goreleaser` job, which builds cross-platform binaries, signs checksums with cosign, generates SBOMs with syft, attaches everything to the release (without overwriting the notes — `release.mode: keep-existing`), and pushes the updated Homebrew cask to `dynatrace-oss/homebrew-tap`.

> ⚠️ Don't forget **Dispatch #2**: merging the release PR alone does **not** publish anything, because the workflow only runs on manual dispatch.

There is **no `CHANGELOG.md` file** in the repo by design — the GitHub Release is the canonical changelog (`skip-changelog: true` in `release-please-config.json`).

## Version bumps are determined by commit type

| Commit prefix | Bump (pre-1.0) | Example |
|---------------|----------------|---------|
| `feat:` | MINOR | 0.30.3 → 0.31.0 |
| `fix:` / `perf:` | PATCH | 0.30.3 → 0.30.4 |
| `feat!:` / `fix!:` or `BREAKING CHANGE:` footer | MINOR (pre-1.0) | 0.30.3 → 0.31.0 |
| `docs:` / `test:` / `chore:` / `ci:` / `refactor:` | no release on its own | — |

So the way to "control the version" is to **write good conventional commits** (see CONTRIBUTING.md). PRs are squash-merged, so the **PR title** becomes the release-relevant commit.

## To cut a release

```bash
# 1. Dispatch the workflow to build/update the release PR from commits since the last release
gh workflow run release.yml
gh run watch    # wait for the release-please run to finish

# 2. Review the proposed version + notes, then merge the release PR
gh pr list --search "chore(main): release in:title" --state open
gh pr merge <number> --squash

# 3. Dispatch AGAIN to create the tag + GitHub Release and publish artifacts
gh workflow run release.yml
gh run watch

# 4. Confirm
gh release view "$(gh release list --limit 1 --json tagName -q '.[0].tagName')"
```

The tag, GitHub Release, binaries, signatures, SBOMs, and Homebrew cask are produced by the second dispatch.

## Polishing release notes (optional)

release-please generates notes grouped by commit type with PR links. They're good by default. If you want richer prose, edit the GitHub Release after it's published — GoReleaser uses `keep-existing`, so it won't clobber your edits:

```bash
gh release edit vX.Y.Z --notes-file notes.md
```

## Troubleshooting

- **No release PR appeared** — did you dispatch the workflow? It does **not** run on merges; run `gh workflow run release.yml`. If it ran and still no PR, there are no releasable commits since the last release (only `docs`/`chore`/`test`/etc.), or a commit isn't a valid conventional commit. Check `gh run list --workflow=release.yml`.
- **Merged the release PR but nothing published** — that's expected: dispatch the workflow a **second time** so release-please creates the tag/Release and GoReleaser runs.
- **Wrong version bump** — caused by the commit types. To force a specific bump, use a `Release-As: x.y.z` footer in a commit on `main`.
- **Release created but no binaries** — inspect the `goreleaser` job in the release workflow run; the `release-please` job must have output `release_created: true` for it to run.
- **Homebrew didn't update** — check the `HOMEBREW_TAP_APP_ID` / `HOMEBREW_TAP_APP_PRIVATE_KEY` secrets and the GoReleaser step logs; `skip_upload: auto` intentionally skips pre-release tags.

## Checklist

1. [ ] All intended changes merged to `main` as conventional commits
2. [ ] **Dispatch #1**: `gh workflow run release.yml` → release PR created/updated
3. [ ] Open release PR reflects the expected version and notes
4. [ ] Merge the release PR (squash)
5. [ ] **Dispatch #2**: `gh workflow run release.yml` → tag + Release created
6. [ ] `release.yml` run is green for both `release-please` and `goreleaser` jobs
7. [ ] GitHub Release shows binaries, checksums, signatures, and SBOMs
8. [ ] (Optional) Polished the release notes via `gh release edit`
