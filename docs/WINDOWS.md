# Installing dtctl on Windows

This guide covers everything you need to get dtctl running on Windows, including installation, configuration, shell completion, and tips for working with PowerShell.

## Quick Install (PowerShell)

Open PowerShell and run:

```powershell
irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
```

This downloads the latest release, extracts it to `%LOCALAPPDATA%\dtctl`, and adds it to your PATH. Restart your terminal afterwards.

## Step-by-Step Install

If you prefer to install manually:

### 1. Download

Go to the [releases page](https://github.com/dynatrace-oss/dtctl/releases/latest) and download the Windows zip file for your architecture:

| Architecture    | File                              |
| --------------- | --------------------------------- |
| 64-bit (x86_64) | `dtctl_VERSION_windows_amd64.zip` |
| ARM64           | `dtctl_VERSION_windows_arm64.zip` |

Most Windows PCs use the **amd64** variant. ARM64 is for devices like the Surface Pro X or Copilot+ PCs with Snapdragon processors.

### 2. Extract

Right-click the downloaded zip file and select **Extract All**, or use PowerShell:

```powershell
Expand-Archive dtctl_*_windows_*.zip -DestinationPath "$env:LOCALAPPDATA\dtctl"
```

This places `dtctl.exe` in `%LOCALAPPDATA%\dtctl` (typically `C:\Users\<you>\AppData\Local\dtctl`).

### 3. Add to PATH

Add the directory to your user PATH so you can run `dtctl` from any terminal:

**Option A -- PowerShell (recommended):**

```powershell
$binPath = "$env:LOCALAPPDATA\dtctl"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$binPath*") {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$binPath", 'User')
}
```

**Option B -- System Settings UI:**

1. Press `Win + R`, type `sysdm.cpl`, and press Enter
2. Go to the **Advanced** tab and click **Environment Variables**
3. Under **User variables**, select `Path` and click **Edit**
4. Click **New** and add: `%LOCALAPPDATA%\dtctl`
5. Click **OK** to save

After changing PATH, restart your terminal (or run `refreshenv` if you have Chocolatey installed).

### 4. Verify

```powershell
dtctl version
```

Expected output:

```
dtctl version 0.x.x
commit: abc1234
built: 2025-01-01T00:00:00Z
```

## Verify Download Integrity

Each release includes a `checksums.txt` file signed with [cosign](https://github.com/sigstore/cosign). To verify your download:

```powershell
# Download checksums
Invoke-WebRequest "https://github.com/dynatrace-oss/dtctl/releases/latest/download/checksums.txt" -OutFile checksums.txt

# Compute the SHA256 of your zip file
$hash = (Get-FileHash dtctl_*_windows_*.zip -Algorithm SHA256).Hash.ToLower()

# Check it matches
Select-String $hash checksums.txt
```

If the `Select-String` command prints a matching line, the download is intact.

## Building from Source

Requires **Go 1.24+** and **Git**.

```powershell
git clone https://github.com/dynatrace-oss/dtctl.git
cd dtctl
go build -o bin\dtctl.exe .

# Verify
.\bin\dtctl.exe version
```

To install to your Go bin directory:

```powershell
go install .

# Verify (ensure $env:GOPATH\bin is in your PATH)
dtctl version
```

## Shell Completion

### PowerShell

Tab completion makes dtctl much faster to use. Add this line to your PowerShell profile so it loads automatically:

```powershell
# Find your profile path
echo $PROFILE

# Edit it (creates the file if it doesn't exist)
if (!(Test-Path -Path $PROFILE)) { New-Item -ItemType File -Path $PROFILE -Force }
notepad $PROFILE
```

Add this line to the profile file:

```powershell
dtctl completion powershell | Out-String | Invoke-Expression
```

Save, close, and reopen PowerShell. You can now use `Tab` to complete commands:

```powershell
dtctl get <Tab>        # cycles through resource types
dtctl get workflows -o <Tab>   # cycles through output formats
```

### Git Bash / MSYS2

If you use Git Bash, you can set up bash completions:

```bash
# Generate completion script
dtctl completion bash > ~/.dtctl-completion.bash

# Add to your ~/.bashrc
echo 'source ~/.dtctl-completion.bash' >> ~/.bashrc
source ~/.bashrc
```

## Configuration

dtctl stores configuration and credentials under `%LOCALAPPDATA%\dtctl`:

| Item        | Path                          |
| ----------- | ----------------------------- |
| Config file | `%LOCALAPPDATA%\dtctl\config` |
| Cached data | `%LOCALAPPDATA%\dtctl\cache`  |
| Credentials | Windows Credential Manager    |

Credentials are stored securely in **Windows Credential Manager** (viewable via Control Panel > Credential Manager > Windows Credentials).

### Set Up Your First Environment

```powershell
# OAuth login (recommended -- opens browser)
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"

# Or use a token
dtctl config set-context my-env `
  --environment "https://abc12345.apps.dynatrace.com" `
  --token-ref my-token

dtctl config set-credentials my-token `
  --token "dt0s16.XXXXXXXX.YYYYYYYY"

# Verify connectivity
dtctl doctor
```

Note: In PowerShell, use the backtick (`` ` ``) for line continuation instead of backslash (`\`).

## PowerShell Tips

### Quoting

PowerShell handles quotes differently from bash/zsh. This matters most with DQL queries that contain double quotes.

**Use here-strings** for DQL queries to avoid quoting issues:

```powershell
# Here-string preserves all quotes exactly
dtctl query -f - -o json @'
fetch logs
| filter status = "ERROR"
| limit 10
'@
```

**Or use query files** to sidestep quoting entirely:

```powershell
# Save query to file
@"
fetch logs
| filter status = "ERROR"
| limit 10
"@ | Out-File -Encoding UTF8 query.dql

# Execute
dtctl query -f query.dql
```

See the [DQL Queries](QUICK_START.md#powershell-quoting-issues-and-solutions) section in the Quick Start guide for more examples.

### Line Continuation

PowerShell uses the backtick (`` ` ``) for line continuation, not backslash:

```powershell
# Correct (PowerShell)
dtctl config set-context my-env `
  --environment "https://abc12345.apps.dynatrace.com" `
  --token-ref my-token

# Wrong (bash syntax -- won't work in PowerShell)
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token
```

### JSON Output and Piping

PowerShell works well with JSON output for scripting:

```powershell
# Parse JSON output
$workflows = dtctl get workflows -o json | ConvertFrom-Json

# Filter in PowerShell
$workflows | Where-Object { $_.title -like "*daily*" }

# Count resources
(dtctl get slos -o json | ConvertFrom-Json).Count
```

### Environment Variables

Set environment variables for dtctl in PowerShell:

```powershell
# Temporary (current session)
$env:DTCTL_ENVIRONMENT = "https://abc12345.apps.dynatrace.com"

# Persistent (current user)
[Environment]::SetEnvironmentVariable('DTCTL_ENVIRONMENT', 'https://abc12345.apps.dynatrace.com', 'User')
```

## Windows Terminal and cmd.exe

### Windows Terminal

dtctl works in all Windows Terminal profiles (PowerShell, cmd.exe, Git Bash, WSL). For the best experience, use **Windows Terminal** with PowerShell and enable completions as described above.

### cmd.exe

dtctl runs in cmd.exe, but shell completion is not available. Use PowerShell or Git Bash for the full experience.

```cmd
rem Verify installation
dtctl version

rem Run a query (use double quotes only)
dtctl query "fetch logs | limit 5"
```

### WSL (Windows Subsystem for Linux)

If you use WSL, follow the [Linux installation instructions](INSTALLATION.md#homebrew-recommended--macos-and-linux) instead -- install the Linux binary inside your WSL distribution, not the Windows binary.

## Updating

Re-run the install script to update to the latest version:

```powershell
irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
```

Or download the latest zip manually and extract to `%LOCALAPPDATA%\dtctl`.

## Uninstalling

```powershell
# Remove the binary
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\dtctl"

# Remove from PATH
$binPath = "$env:LOCALAPPDATA\dtctl"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$newPath = ($userPath -split ';' | Where-Object { $_ -ne $binPath }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $newPath, 'User')

# Remove configuration (optional -- also removes the binary if still present)
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\dtctl"

# Remove stored credentials (optional)
# Open Control Panel > Credential Manager > Windows Credentials
# Delete entries starting with "dtctl"
```

## Troubleshooting

### "dtctl is not recognized as an internal or external command"

The binary is not in your PATH. Either:

1. Use the full path: `& "$env:LOCALAPPDATA\dtctl\dtctl.exe"`
2. Add the bin directory to your PATH (see step 3 above)
3. Restart your terminal after modifying PATH

Check your PATH:

```powershell
$env:Path -split ';' | Select-String dtctl
```

### "Access is denied"

If extracting to `Program Files` or another protected location, run PowerShell as Administrator. Or install to `%LOCALAPPDATA%\dtctl\bin` (user-writable, no admin needed).

### Antivirus blocks dtctl.exe

Some antivirus software may flag unsigned binaries. You can:

1. Verify the checksum matches the official release (see [Verify Download Integrity](#verify-download-integrity))
2. Add an exclusion for `%LOCALAPPDATA%\dtctl\dtctl.exe` in your antivirus settings

### PowerShell execution policy blocks profile loading

If PowerShell completions don't load, your execution policy may block profile scripts:

```powershell
# Check current policy
Get-ExecutionPolicy

# Allow local scripts (recommended)
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### WSL vs native Windows

If you have both WSL and native Windows installs, make sure you're running the right binary:

```powershell
# Check which dtctl you're running
Get-Command dtctl | Select-Object Source
```

Inside WSL, use `which dtctl` instead.

## Next Steps

- [Quick Start Guide](QUICK_START.md) -- configure your environment and run your first commands
- [Installation](INSTALLATION.md) -- cross-platform installation reference
- [Token Scopes](TOKEN_SCOPES.md) -- required API permissions
