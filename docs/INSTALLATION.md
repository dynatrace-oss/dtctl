# Installing dtctl

This guide covers building and installing dtctl from source.

## Prerequisites

- Go 1.24 or later
- Git
- Make

## Building from Source

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/dynatrace/dtctl.git
cd dtctl

# Build the binary
make build

# Verify the build
./bin/dtctl version
```

Expected output:
```
dtctl version dev
commit: unknown
built: unknown
```

### Test the Binary

Try a few commands to ensure everything works:

```bash
# Show help
./bin/dtctl --help

# View available commands
./bin/dtctl get --help
./bin/dtctl query --help
```

## Installation Options

### Option 1: Use from bin/ Directory

The simplest approach - use `./bin/dtctl` directly:

```bash
# From the project directory
./bin/dtctl config set-context my-env \
  --environment "https://YOUR_ENV.apps.dynatrace.com" \
  --token-ref my-token
```

### Option 2: Install to GOPATH

Install to your Go binary directory:

```bash
make install

# Verify
dtctl version
```

This installs to `$GOPATH/bin/dtctl` (typically `~/go/bin/dtctl`). Ensure `$GOPATH/bin` is in your `$PATH`.

### Option 3: Copy to System PATH

Install system-wide:

```bash
# Linux/macOS
sudo cp bin/dtctl /usr/local/bin/

# Verify
dtctl version
```

### Option 4: Add to PATH

Add the bin directory to your PATH:

```bash
# Add to ~/.bashrc, ~/.zshrc, or ~/.profile
export PATH="$PATH:/path/to/dtctl/bin"

# Reload your shell
source ~/.bashrc  # or ~/.zshrc
```

## Shell Completion (Optional)

Enable tab completion for faster workflows.

### Bash

```bash
# Generate completion script
dtctl completion bash > /tmp/dtctl-completion.bash

# Test it
source /tmp/dtctl-completion.bash

# Make it permanent
sudo mkdir -p /etc/bash_completion.d
sudo cp /tmp/dtctl-completion.bash /etc/bash_completion.d/dtctl

# Reload your shell
source ~/.bashrc
```

### Zsh

```bash
# Create completions directory
mkdir -p ~/.zsh/completions

# Generate completion script
dtctl completion zsh > ~/.zsh/completions/_dtctl

# Add to your ~/.zshrc (if not already present)
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
echo 'autoload -U compinit && compinit' >> ~/.zshrc

# Clear completion cache and reload
rm -f ~/.zcompdump*
source ~/.zshrc
```

**For oh-my-zsh users**: Place the completion file in `~/.oh-my-zsh/completions/_dtctl`:

```bash
mkdir -p ~/.oh-my-zsh/completions
dtctl completion zsh > ~/.oh-my-zsh/completions/_dtctl
rm -f ~/.zcompdump*
source ~/.zshrc
```

### Fish

```bash
# Create completions directory (if needed)
mkdir -p ~/.config/fish/completions

# Generate completion script
dtctl completion fish > ~/.config/fish/completions/dtctl.fish

# Reload shell
source ~/.config/fish/config.fish
```

### PowerShell

```powershell
# Temporary (current session)
dtctl completion powershell | Out-String | Invoke-Expression

# Permanent - add to your PowerShell profile
# First, find your profile location:
echo $PROFILE

# Then add this line to your profile:
dtctl completion powershell | Out-String | Invoke-Expression
```

## Verify Installation

After installation, verify everything works:

```bash
# Check version
dtctl version

# View help
dtctl --help

# Test tab completion (if enabled)
dtctl get <TAB><TAB>
```

## Updating dtctl

To update to the latest version:

```bash
# Navigate to the repository
cd /path/to/dtctl

# Pull latest changes
git pull

# Rebuild
make build

# Reinstall (if using Option 2 or 3)
make install
# or
sudo cp bin/dtctl /usr/local/bin/
```

## Uninstalling

To remove dtctl:

```bash
# If installed via Option 2 (make install)
rm $GOPATH/bin/dtctl

# If installed via Option 3 (system-wide)
sudo rm /usr/local/bin/dtctl

# Remove configuration (optional)
rm -rf ~/.config/dtctl    # Linux
# or
rm -rf ~/Library/Application\ Support/dtctl    # macOS
```

## Next Steps

Now that dtctl is installed, see the [Quick Start Guide](QUICK_START.md) to learn how to:
- Configure your Dynatrace environment
- Execute commands
- Work with workflows, dashboards, DQL queries, and more

## Troubleshooting

### "command not found: dtctl"

The binary is not in your PATH. Either:
1. Use the full path: `./bin/dtctl` or `/path/to/dtctl/bin/dtctl`
2. Add the bin directory to your PATH (see Option 4 above)
3. Install to a directory already in PATH (see Options 2 or 3 above)

Check your PATH:
```bash
echo $PATH
```

### "permission denied"

Make the binary executable:
```bash
chmod +x bin/dtctl
```

### Build fails

Ensure you have the required prerequisites:
```bash
# Check Go version (needs 1.24+)
go version

# Check Make
make --version

# Try cleaning and rebuilding
make clean
make build
```

### Shell completion not working

After setting up completion:
1. Ensure you reloaded your shell or sourced the config file
2. Clear completion cache (Zsh: `rm -f ~/.zcompdump*`)
3. Verify the completion file exists in the correct location
4. Check file permissions: `ls -la ~/.zsh/completions/_dtctl`

## Getting Help

- **Quick Start**: See [QUICK_START.md](QUICK_START.md) for usage examples
- **API Reference**: See [API_DESIGN.md](API_DESIGN.md) for complete command reference
- **Architecture**: Read [ARCHITECTURE.md](ARCHITECTURE.md) for implementation details
- **Issues**: Report bugs at [GitHub Issues](https://github.com/dynatrace/dtctl/issues)

### macOS: "zsh: exec format error"

If you built the binary inside a Linux-based devcontainer (for example on an ARM container) and then try to run `bin/dtctl` natively on macOS, you may see:

```
zsh: exec format error: bin/dtctl
```

This happens because the compiled binary's OS/architecture don't match your host. To fix it, rebuild the binary for macOS on your host or produce a cross-compiled macOS binary.

Rebuild locally on macOS (recommended):

```bash
# From the project root
make clean
# Build for the host (native macOS build)
make build-host
# Or explicitly build for darwin/arm64
make build-darwin-arm64

# Run the built binary
./bin/dtctl-host version    # from `make build-host`
# or
./bin/dtctl-darwin-arm64 version
```

Cross-build from Linux (requires Go on the build machine):

```bash
# Create a darwin/arm64 binary from Linux
env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/dtctl-darwin-arm64 .
```

Notes:
- If you built the binary inside a container using a different OS (Linux) and then copied it to macOS, the binary won't run on macOS. Always build for the target OS/arch.
- For Apple Silicon (arm64) Macs, target `darwin/arm64`. For older Intel Macs target `darwin/amd64`.
- If you need universal binaries or native macOS toolchain features, build on macOS directly.

