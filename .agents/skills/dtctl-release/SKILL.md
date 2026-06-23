---
name: dtctl-release
description: Explain and operate the dtctl release process, which is automated by release-please. Use this skill whenever the user says "release", "ship it", "cut a release", "new version", "bump version", "publish", or asks how dtctl releases work, why a release PR exists, or how to trigger/finish a release.
---

# dtctl Release Process (release-please)

dtctl releases are **automated by [release-please](https://github.com/googleapis/release-please)**. There is no manual version bump, no `CHANGELOG.md` to edit, and no tag to push by hand. Releasing is just **merging the release PR**.

## How it works

1. Every push to `main` runs `.github/workflows/release.yml`.
2. The `release-please` job inspects the [Conventional Commits](https://www.conventionalcommits.org/) since the last release and maintains a **release PR** titled like `chore(main): release 0.31.0`. That PR:
   - bumps `.release-please-manifest.json`,
   - bumps the fallback version in `pkg/version/version.go` (via the `// x-release-please-version` annotation),
   - and accumulates the pending release notes (in the PR body).
3. **Merging the release PR** makes release-please create the git tag (`vX.Y.Z`) and the **GitHub Release** with generated notes.
4. The same workflow then runs the gated `goreleaser` job (`release_created == true`), which builds cross-platform binaries, signs checksums with cosign, generates SBOMs with syft, attaches everything to the release (without overwriting the notes — `release.mode: keep-existing`), and pushes the updated Homebrew cask to `dynatrace-oss/homebrew-tap`.

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

Usually nothing to do but merge:

```bash
# Find the open release PR
gh pr list --search "release" --state open

# Review the version bump and notes it proposes, then merge it
gh pr merge <number> --squash
```

After the merge, watch the release workflow finish both jobs:

```bash
gh run watch
gh release view "$(gh release list --limit 1 --json tagName -q '.[0].tagName')"
```

That's it — the tag, GitHub Release, binaries, signatures, SBOMs, and Homebrew cask are all produced automatically.

## Polishing release notes (optional)

release-please generates notes grouped by commit type with PR links. They're good by default. If you want richer prose, edit the GitHub Release after it's published — GoReleaser uses `keep-existing`, so it won't clobber your edits:

```bash
gh release edit vX.Y.Z --notes-file notes.md
```

## Troubleshooting

- **No release PR appeared** — there are no releasable commits since the last release (only `docs`/`chore`/`test`/etc.), or a commit isn't a valid conventional commit. Check `gh run list --workflow=release.yml`.
- **Wrong version bump** — caused by the commit types. To force a specific bump, use a `Release-As: x.y.z` footer in a commit on `main`.
- **Release created but no binaries** — inspect the `goreleaser` job in the release workflow run; the `release-please` job must have output `release_created: true` for it to run.
- **Homebrew didn't update** — check the `HOMEBREW_TAP_APP_ID` / `HOMEBREW_TAP_APP_PRIVATE_KEY` secrets and the GoReleaser step logs; `skip_upload: auto` intentionally skips pre-release tags.

## Checklist

1. [ ] All intended changes merged to `main` as conventional commits
2. [ ] Open release PR reflects the expected version and notes
3. [ ] Merge the release PR (squash)
4. [ ] `release.yml` run is green for both `release-please` and `goreleaser` jobs
5. [ ] GitHub Release shows binaries, checksums, signatures, and SBOMs
6. [ ] (Optional) Polished the release notes via `gh release edit`
