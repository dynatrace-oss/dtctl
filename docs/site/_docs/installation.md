---
layout: docs
title: Installation
---

## Homebrew (macOS and Linux)

```bash
brew install dynatrace-oss/tap/dtctl
```

This installs the binary and sets up shell completions automatically.

## Shell Script (macOS and Linux)

If you don't use Homebrew:

```bash
curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.sh | sh
```

This downloads the latest release, extracts it to `~/.local/bin` (Linux) or `/usr/local/bin` (macOS), and verifies the installation. Override the install directory with `DTCTL_INSTALL_DIR`:

```bash
curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.sh | DTCTL_INSTALL_DIR=~/bin sh
```

## Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
```

This downloads the latest release, extracts it to `%LOCALAPPDATA%\dtctl`, and adds it to your PATH. Restart your terminal afterwards.

**Manual install:** Download the zip for your architecture from the [releases page](https://github.com/dynatrace-oss/dtctl/releases/latest), extract it, and add the directory to your PATH. See the [Windows guide](https://github.com/dynatrace-oss/dtctl/blob/main/docs/WINDOWS.md) for detailed steps, PowerShell tips, and troubleshooting.

## Binary Download (Manual)

Download a pre-built binary from the [GitHub releases page](https://github.com/dynatrace-oss/dtctl/releases/latest).

**Linux/macOS:**

```bash
# Extract the archive
tar -xzf dtctl_*.tar.gz

# Make it executable
chmod +x dtctl

# macOS only: remove quarantine attribute
sudo xattr -r -d com.apple.quarantine dtctl

# Move to a directory in your PATH
sudo mv dtctl /usr/local/bin/

# Verify
dtctl version
```

## Building from Source

Requires **Go 1.24+**, Git, and Make.

```bash
git clone https://github.com/dynatrace-oss/dtctl.git
cd dtctl
make build
./bin/dtctl version
```

To install system-wide:

```bash
make install          # installs to $GOPATH/bin
# or
sudo cp bin/dtctl /usr/local/bin/
```

Ensure `$GOPATH/bin` is in your `$PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Shell Completion

Enable tab completion for faster workflows.

### Bash

```bash
dtctl completion bash > /tmp/dtctl-completion.bash
sudo mkdir -p /etc/bash_completion.d
sudo cp /tmp/dtctl-completion.bash /etc/bash_completion.d/dtctl
source ~/.bashrc
```

### Zsh

```bash
mkdir -p ~/.zsh/completions
dtctl completion zsh > ~/.zsh/completions/_dtctl

# Add to ~/.zshrc (if not already present)
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
echo 'autoload -U compinit && compinit' >> ~/.zshrc
rm -f ~/.zcompdump*
source ~/.zshrc
```

**oh-my-zsh users:** place the file in `~/.oh-my-zsh/completions/_dtctl` instead.

### Fish

```bash
mkdir -p ~/.config/fish/completions
dtctl completion fish > ~/.config/fish/completions/dtctl.fish
```

### PowerShell

```powershell
# Add to your PowerShell profile ($PROFILE):
dtctl completion powershell | Out-String | Invoke-Expression
```

## Updating

```bash
# Homebrew
brew update && brew upgrade dtctl

# Shell script (re-run)
curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.sh | sh

# From source
git pull && make build
```

**Windows:**

```powershell
irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
```

## Uninstalling

```bash
# Homebrew
brew uninstall dtctl
brew untap dynatrace-oss/tap   # optional

# Manual install
sudo rm /usr/local/bin/dtctl

# Remove configuration (optional)
rm -rf ~/.config/dtctl          # Linux
rm -rf ~/Library/Application\ Support/dtctl   # macOS
```

## Troubleshooting

### "command not found: dtctl"

The binary is not in your PATH. Either use the full path (`./bin/dtctl`) or add the binary's directory to your PATH:

```bash
export PATH="$PATH:/path/to/dtctl/bin"
```

### "permission denied"

Make the binary executable:

```bash
chmod +x dtctl
```

### macOS: "Apple could not verify dtctl is free of malware"

This is expected for unsigned open-source binaries. Remove the quarantine attribute:

```bash
sudo xattr -r -d com.apple.quarantine dtctl
```

Or allow it via **System Settings > Privacy & Security > Allow Anyway**.

### "exec format error"

You're running a binary built for a different OS/architecture (e.g., a Linux binary on macOS). Rebuild for your platform:

```bash
make clean && make build
```

For Apple Silicon Macs, target `darwin/arm64`. For Intel Macs, target `darwin/amd64`.

### Windows: "dtctl is not recognized"

The binary is not in your PATH. Restart your terminal after installation, or check your PATH:

```powershell
$env:Path -split ';' | Select-String dtctl
```

See the [Windows guide](https://github.com/dynatrace-oss/dtctl/blob/main/docs/WINDOWS.md) for more Windows-specific troubleshooting.

---

Next: [Quick Start]({{ '/docs/quick-start/' | relative_url }}) -- set up your first environment and run your first commands.
