---
name: dtctl-release
description: Ship a new dtctl release — bump version, write changelog entries, run tests, commit, tag, push, and write GitHub release notes. Use this skill whenever the user says "release", "ship it", "cut a release", "new version", "bump version", "publish", or asks about the dtctl release process. Also use when the user wants to update CHANGELOG.md for a release or write GitHub release notes.
---

# dtctl Release Process

This skill walks through shipping a new dtctl release end-to-end. The process has six phases: analyze, version, changelog, test, commit/tag/push, and GitHub release notes.

## Prerequisites

- You must be on the `main` branch with all feature branches merged
- The working tree must be clean (`git status` shows no uncommitted changes)
- If the user is on a feature branch, merge to main first (or ask them to)

**Pre-flight check:**

```bash
git branch --show-current   # Must be "main"
git status                  # Must be clean
git pull origin main        # Must be up to date with remote
```

If any check fails, stop and resolve before continuing.

## Phase 1: Analyze Changes Since Last Release

Identify what changed since the last release to determine the version bump and write good release notes.

```bash
# Find the latest release tag
git tag --sort=-version:refname | head -5

# List all commits since that tag
git log <last-tag>..HEAD --oneline

# Check the current Unreleased section in CHANGELOG.md
```

Read the commits carefully and group them into:
- **Features** (new commands, new flags, new integrations)
- **Bug fixes** (corrected behavior)
- **Documentation** (new or updated docs)
- **Security** (dependency updates, vulnerability fixes)
- **Breaking changes** (removed flags, changed defaults)

For each significant feature, explore the actual implementation files to understand what it does. Don't just rely on commit messages — read the code so you can write accurate, detailed release notes.

## Phase 2: Determine Version Number

dtctl follows [Semantic Versioning](https://semver.org/) (currently pre-1.0, so 0.MINOR.PATCH):

| Change type | Bump | Example |
|-------------|------|---------|
| New features, new commands | MINOR | 0.23.0 -> 0.24.0 |
| Bug fixes only, no new features | PATCH | 0.24.0 -> 0.24.1 |
| Breaking changes (pre-1.0) | MINOR | 0.24.0 -> 0.25.0 |

## Phase 3: Update Version and Changelog

### 3a. Bump version in code

Edit `pkg/version/version.go` — change the `Version` variable:

```go
var Version = "X.Y.Z"  // update this
```

This is the only place the version is hardcoded. GoReleaser injects it at build time via `-ldflags`, but the fallback value here should always match the latest release.

### 3b. Update CHANGELOG.md

The changelog follows [Keep a Changelog](https://keepachangelog.com/) format. Move items from `[Unreleased]` into a new version section:

```markdown
## [Unreleased]

## [X.Y.Z] - YYYY-MM-DD

### Added
- **Feature name** — description with enough detail that users understand what it does and how to use it; include CLI examples where relevant

### Fixed
- **Bug summary** — what was broken and how it's fixed now

### Changed
- **Change summary** — what changed and why

### Security
- **Security fix** — what was vulnerable and what was upgraded

### Documentation
- **Doc name** — what was added or updated
```

Also add the comparison link at the bottom of the file:

```markdown
[X.Y.Z]: https://github.com/dynatrace-oss/dtctl/compare/vPREVIOUS...vX.Y.Z
```

If a comparison link for the previous version is missing (check!), add that too.

### Writing style for changelog entries

- Start each entry with a bold feature/fix name in `**double asterisks**`
- Follow with an em dash `—` and a description
- Be specific: mention command names, flag names, file paths, environment variables
- For features, explain both what it does and how to use it
- For fixes, explain what was broken and what the correct behavior is now
- Use semicolons to chain related details in a single entry
- Keep entries to 1-3 lines each

## Phase 4: Run Tests

Run the full test suite and build to catch any issues before releasing:

```bash
# Run all tests
go test ./...

# Build the binary
make build

# Verify the build works
./bin/dtctl version
```

All tests must pass. If any fail, fix them before proceeding.

## Phase 5: Commit, Tag, and Push

### 5a. Commit the release

```bash
git add CHANGELOG.md pkg/version/version.go
git commit -m "release vX.Y.Z: short summary of key features"
```

The commit message should follow the pattern: `release vX.Y.Z: feature1, feature2, feature3`

### 5b. Push and tag

```bash
git push origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

Pushing the tag triggers the GitHub Actions release workflow (`.github/workflows/release.yml`), which automatically:
- Builds cross-platform binaries (linux/darwin/windows, amd64/arm64)
- Signs checksums with cosign (keyless, via OIDC)
- Generates SBOMs with syft
- Creates a GitHub Release with auto-generated changelog
- Pushes an updated Homebrew cask to `dynatrace-oss/homebrew-tap`

### Homebrew

No manual Homebrew update is needed. GoReleaser's `homebrew_casks` config in `.goreleaser.yaml` automatically pushes the updated cask to the tap repository using a GitHub App token. The `skip_upload: auto` setting prevents pre-release tags from being published.

## Phase 6: Write GitHub Release Notes

After the tag is pushed, update the GitHub release with polished release notes. The auto-generated changelog from GoReleaser is a raw commit list — replace it with proper release notes.

```bash
gh release edit vX.Y.Z --notes "$(cat <<'EOF'
... release notes here ...
EOF
)"
```

### Release notes format

Follow this exact structure (see previous releases for reference — `gh release view vPREVIOUS`):

```markdown
## What's New

### Feature Name

Paragraph explaining the feature with context on why it's useful.

\`\`\`bash
# Concrete usage example
dtctl some-command --some-flag
\`\`\`

Additional detail about configuration, behavior, or edge cases. Keep it practical — show users exactly how to use the feature.

### Another Feature

...repeat for each major feature...

## Bug Fixes

- **Short fix title** — one-line explanation of what was broken and what's fixed.
- **Another fix** — description.

## Documentation

- **Doc title** — what was added, with a link if applicable.

## Install / Upgrade

\`\`\`bash
# Homebrew
brew update && brew upgrade dtctl

# Direct install
curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.sh | bash

# Go install
go install github.com/dynatrace-oss/dtctl@vX.Y.Z
\`\`\`

**Full Changelog**: https://github.com/dynatrace-oss/dtctl/compare/vPREVIOUS...vX.Y.Z
```

### Release notes writing style

- Each major feature gets its own `### Heading` with a paragraph + code example
- Code examples should be copy-pasteable and realistic
- Bug fixes go in a single bulleted list under `## Bug Fixes`
- Always end with the `## Install / Upgrade` block
- Always end with the `**Full Changelog**` comparison link
- Omit sections that don't apply (e.g., no `## Security` if there are no security fixes)
- Study previous releases for tone and detail level: `gh release view v0.23.0`, `gh release view v0.22.0`

## Checklist

Use this to track progress:

1. [ ] On `main` branch, clean working tree, up to date with remote
2. [ ] Analyzed all commits since last release
3. [ ] Determined version number (semver)
4. [ ] Updated `pkg/version/version.go`
5. [ ] Updated `CHANGELOG.md` (entries + comparison link)
6. [ ] Tests pass (`go test ./...`)
7. [ ] Build succeeds (`make build`)
8. [ ] Committed release changes on `main`
9. [ ] Pushed to `origin/main`
10. [ ] Created and pushed tag `vX.Y.Z`
11. [ ] GitHub release notes written via `gh release edit`
