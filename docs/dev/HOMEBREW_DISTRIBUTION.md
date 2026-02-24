# Homebrew Distribution

Spec for distributing `dtctl` via Homebrew using a custom tap.

## Status

- **Current**: Binary downloads from GitHub Releases only
- **Goal**: `brew install dynatrace-oss/tap/dtctl`
- **Tap repo**: <https://github.com/dynatrace-oss/homebrew-tap>

## Overview

Homebrew uses "taps" (third-party repos) to distribute packages. GoReleaser
has built-in support for generating and pushing a Cask to a tap repo on every
release.

We use `homebrew_casks` (not the deprecated `brews`/Formulas) because:

- GoReleaser deprecated `brews` in v2.10 in favor of `homebrew_casks`
- Casks are the recommended distribution mechanism for pre-built binaries
- Since `dtctl` binaries are not code-signed/notarized, a post-install hook
  strips the macOS quarantine attribute automatically

User experience after implementation:

```bash
brew install dynatrace-oss/tap/dtctl

# Upgrades work automatically:
brew upgrade dtctl
```

## Implementation Plan

### Step 1: Initialize the tap repository

The tap repo exists at <https://github.com/dynatrace-oss/homebrew-tap>.

> **Note:** Homebrew requires the `homebrew-` prefix. When resolving
> `dynatrace-oss/tap`, Homebrew looks for `dynatrace-oss/homebrew-tap`.

Repository structure:

```text
dynatrace-oss/homebrew-tap/
  README.md
  Casks/
    .gitkeep        # Placeholder -- GoReleaser creates dtctl.rb on first release
```

GoReleaser will create `Casks/dtctl.rb` automatically on the first release.

### Step 2: Create a GitHub token for tap pushes

GoReleaser needs a token with write access to `dynatrace-oss/homebrew-tap`. The default
`GITHUB_TOKEN` from GitHub Actions is scoped to the current repo only, so a
separate token is required.

Options (pick one):

| Method | Scope | Recommendation |
|--------|-------|----------------|
| **Fine-grained PAT** | `Contents: write` on `dynatrace-oss/homebrew-tap` only | Preferred -- least privilege |
| Classic PAT | `repo` scope | Works but overly broad |
| GitHub App installation token | Custom app with repo-scoped permissions | Best for orgs, more setup |

Store the token as a repository secret named `HOMEBREW_TAP_GITHUB_TOKEN` in the
`dynatrace-oss/dtctl` repository settings.

### Step 3: Update `.goreleaser.yaml`

Use `homebrew_casks` (not the deprecated `brews`):

```yaml
homebrew_casks:
  - repository:
      owner: dynatrace-oss
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    homepage: https://github.com/dynatrace-oss/dtctl
    description: >-
      kubectl-inspired CLI for Dynatrace - manage workflows, dashboards,
      SLOs, queries, and more from your terminal
    skip_upload: auto
    completions:
      bash: completions/dtctl.bash
      zsh: completions/dtctl.zsh
      fish: completions/dtctl.fish
    hooks:
      post:
        install: |
          if OS.mac?
            system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "#{staged_path}/dtctl"]
          end
```

Key decisions:

- **`homebrew_casks`** -- The non-deprecated GoReleaser mechanism (since v2.10).
  Generates a Cask file in `Casks/` directory.
- **`skip_upload: auto`** -- GoReleaser skips the tap push for pre-release
  tags. This prevents unstable versions from being published to Homebrew.
- **Post-install hook** -- Strips the macOS quarantine attribute since dtctl
  binaries are not code-signed. This prevents the "cannot be opened because
  Apple cannot verify" error.
- **No `directory` key** -- Defaults to `Casks/`, the standard Homebrew
  convention for casks.

Also fix other deprecations in the same change:

- `archives.format` -> `archives.formats` (list)
- `archives.format_overrides.format` -> `formats` (list)
- `snapshot.name_template` -> `snapshot.version_template`

### Step 4: Update the release workflow

Add `HOMEBREW_TAP_GITHUB_TOKEN` to the env block in
`.github/workflows/release.yml`:

```yaml
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

If the secret is not set (e.g., in a fork), GoReleaser will skip the cask step
and log a warning -- it won't fail the release.

### Step 5: Update README and docs

- **README.md** -- Add `brew install dynatrace-oss/tap/dtctl` as the primary
  install method.
- **docs/INSTALLATION.md** -- Add Homebrew section before binary download.
- **docs/dev/IMPLEMENTATION_STATUS.md** -- Check the `Homebrew tap` box.

### Step 6: Test locally before merging

Verify the cask generation works before cutting a release:

```bash
# 1. Generate a snapshot release (no upload)
make release-snapshot

# 2. Inspect the generated cask
cat dist/homebrew_casks/Casks/dtctl.rb

# 3. Verify the cask is valid Ruby
ruby -c dist/homebrew_casks/Casks/dtctl.rb

# 4. Optionally test with a local tap
mkdir -p /tmp/test-tap/Casks
cp dist/homebrew_casks/Casks/dtctl.rb /tmp/test-tap/Casks/
brew tap --force local/tap /tmp/test-tap
brew install local/tap/dtctl
dtctl version
brew uninstall dtctl
brew untap local/tap
```

### Step 7: First release with Homebrew

1. Merge the goreleaser + workflow changes to `main`
2. Tag a new release: `git tag v0.X.0 && git push origin v0.X.0`
3. Verify:
   - GitHub Release is created with binaries
   - `dynatrace-oss/homebrew-tap` has a new commit with `Casks/dtctl.rb`
   - `brew install dynatrace-oss/tap/dtctl` works on a clean machine

## Generated Cask

For reference, GoReleaser will generate something like this:

```ruby
cask "dtctl" do
  version "0.12.0"
  sha256 "abc123..."

  url "https://github.com/dynatrace-oss/dtctl/releases/download/v0.12.0/dtctl_0.12.0_darwin_arm64.tar.gz"
  name "dtctl"
  desc "kubectl-inspired CLI for Dynatrace - manage workflows, dashboards, SLOs, queries, and more from your terminal"
  homepage "https://github.com/dynatrace-oss/dtctl"

  binary "dtctl"

  postflight do
    if OS.mac?
      system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "#{staged_path}/dtctl"]
    end
  end
end
```

## Maintenance

- **Cask updates are automatic** -- every tagged release triggers GoReleaser,
  which pushes an updated cask.
- **Pre-releases are skipped** -- `skip_upload: auto` ensures only stable
  releases are published.
- **No manual cask editing** -- The cask is fully generated. Don't edit
  `dtctl.rb` in the tap repo by hand.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `HOMEBREW_TAP_GITHUB_TOKEN` not set | GoReleaser skips cask step, logs warning, release still succeeds |
| Pre-release tag (e.g., `v1.0.0-rc1`) | Cask not uploaded (`skip_upload: auto`) |
| Fork creates a tag | No tap token available, cask step skipped |
| Tap repo doesn't exist | GoReleaser fails the cask step |
| Casks/dtctl.rb already exists | GoReleaser overwrites it (normal behavior) |

## Out of Scope (for now)

- **homebrew-core submission** -- Requires meaningful user base and follows a
  different process (PR to Homebrew/homebrew-core). Consider later.
- **Code signing / notarization** -- Would remove the need for the xattr hook.
  Requires an Apple Developer Program membership ($99/year).
- **Linux-specific package managers** (apt/yum) -- Separate effort, tracked in
  IMPLEMENTATION_STATUS.md.
- **Scoop (Windows)** -- GoReleaser supports this too, but separate effort.
- **Docker image** -- Related but separate distribution channel.

## Checklist

- [x] Initialize `dynatrace-oss/homebrew-tap` with README and empty `Casks/` dir
- [ ] Create fine-grained PAT with `Contents: write` on the tap repo
- [ ] Store PAT as `HOMEBREW_TAP_GITHUB_TOKEN` secret in `dynatrace-oss/dtctl`
- [x] Add `homebrew_casks` section to `.goreleaser.yaml`
- [x] Fix all GoReleaser deprecation warnings (formats, version_template)
- [x] Add `HOMEBREW_TAP_GITHUB_TOKEN` env to `.github/workflows/release.yml`
- [x] Test with `make release-snapshot` locally
- [x] Update README.md with Homebrew install instructions
- [x] Update docs/INSTALLATION.md
- [ ] Tag a release and verify end-to-end
- [x] Update IMPLEMENTATION_STATUS.md
