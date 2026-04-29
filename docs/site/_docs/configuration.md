---
layout: docs
title: Configuration
---

## Authentication

### OAuth Login (Recommended)

Browser-based SSO login with automatic token refresh:

```bash
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"
```

Tokens are stored securely in your OS keyring. To log out:

```bash
dtctl auth logout
```

### Token-Based Auth

For CI/CD or headless environments, use a platform API token:

```bash
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token

dtctl config set-credentials my-token \
  --token "dt0s16.XXXXXXXX.YYYYYYYY"
```

### Creating a Platform Token

1. In Dynatrace, navigate to **Identity & Access Management > Access Tokens**
2. Select **Generate new token** and choose **Platform token**
3. Add the required scopes for your use case
4. Copy the token immediately -- it's only shown once

See the [Dynatrace Platform Tokens documentation](https://docs.dynatrace.com/docs/manage/identity-access-management/access-tokens-and-oauth-clients/platform-tokens) for detailed instructions.

### Current User Identity

Check who you're authenticated as:

```bash
dtctl auth whoami
```

Use `dtctl auth whoami -o json` for machine-readable output, or `--id-only` to get just the user ID.

## Multiple Environments

### Create Contexts

```bash
# Development
dtctl config set-context dev \
  --environment "https://dev.apps.dynatrace.com" \
  --token-ref dev-token \
  --safety-level dangerously-unrestricted

# Production (read-only)
dtctl config set-context prod \
  --environment "https://prod.apps.dynatrace.com" \
  --token-ref prod-token \
  --safety-level readonly
```

### Switch Contexts

```bash
dtctl config use-context dev

# Or use the shortcut:
dtctl ctx dev

# List all contexts
dtctl ctx
```

### One-Time Context Override

Run a single command against a different context without switching:

```bash
dtctl get workflows --context prod
```

## Per-Project Configuration

Create a `.dtctl.yaml` in your project root for team or CI/CD configuration:

```bash
dtctl config init
```

This generates a template with environment variable placeholders:

```yaml
apiVersion: dtctl.io/v1
kind: Config
current-context: production
contexts:
  - name: production
    context:
      environment: ${DT_ENVIRONMENT_URL}
      token-ref: my-token
      safety-level: readwrite-all
tokens:
  - name: my-token
    token: ${DT_API_TOKEN}
```

Commit the file to version control without secrets -- each developer or CI system provides values via environment variables.

### Config Search Order

1. `--config` flag (explicit path)
2. `.dtctl.yaml` in the current directory or any parent (walks up to root)
3. Global config (`~/.config/dtctl/config`)

## Safety Levels

Safety levels provide **client-side** protection against accidental destructive operations:

| Level | Description |
|-------|-------------|
| `readonly` | No modifications allowed |
| `readwrite-mine` | Modify your own resources only |
| `readwrite-all` | Modify all resources (default) |
| `dangerously-unrestricted` | All operations including bucket deletion |

```bash
dtctl config set-context prod \
  --environment "https://prod.apps.dynatrace.com" \
  --token-ref prod-token \
  --safety-level readonly
```

View context details including safety level:

```bash
dtctl config describe-context prod
```

Safety levels are client-side only. For actual security, configure your API tokens with minimum required scopes.

## Apply Hooks

Apply hooks run external commands around `dtctl apply`:

- **Pre-apply** (`pre-apply`): runs **before** the resource is sent to the API. Receives the processed JSON on stdin and can reject the apply (non-zero exit aborts).
- **Post-apply** (`post-apply`): runs **after** a successful apply. Receives the apply result as JSON on stdin. Useful for cleanup, notifications, or writing metadata to disk. A non-zero exit is reported as a warning — the resource is already persisted.

Both hooks are invoked the same way: the command string is tokenized using POSIX-style shell quoting (so `"path with spaces"` and `'quoted args'` are honoured) and executed directly — there is **no shell interpretation** of the command line itself. The resource type and source file are appended as the final two positional arguments.

### Configuration

Hooks are configured globally in `preferences` or per-context:

```yaml
# ~/.config/dtctl/config
preferences:
  hooks:
    pre-apply:  "node /opt/dtctl-hooks/validate.js"
    post-apply: "bash /opt/dtctl-hooks/notify.sh"

contexts:
  - name: production
    context:
      environment: https://abc12345.apps.dynatrace.com
      token-ref: prod-token
      hooks:
        pre-apply: "opa eval --bundle /policies -i /dev/stdin"
  - name: dev
    context:
      environment: https://dev.apps.dynatrace.com
      token-ref: dev-token
      hooks:
        pre-apply:  "none"  # explicitly disable the global hook
        post-apply: "none"
```

Per-context hooks take precedence over global hooks. The special value `"none"` disables the global hook for a specific context.

### Hook Contract

| Aspect | Pre-apply | Post-apply |
|--------|-----------|------------|
| **Invocation** | direct exec of the tokenized command, with `<resource-type>` and `<source-file>` appended as args | same |
| **$1** | Resource type (e.g., `dashboard`, `workflow`, `slo`) | same |
| **$2** | Source filename from `-f` | same |
| **Stdin** | Processed resource JSON (YAML→JSON + template rendering applied) | Apply result JSON (array of per-resource result objects: `action`, `resourceType`, `id`, `name`, etc.) |
| **Stdout** | Always forwarded to the user's stdout | Forwarded to the user's stdout |
| **Stderr** | Always forwarded to the user's stderr | Always forwarded to the user's stderr |
| **Exit 0** | Proceed with apply | Reported as success |
| **Exit non-zero** | Abort apply, show stdout+stderr | Warning only — apply already succeeded; stdout+stderr are shown and a warning line is printed |
| **Timeout** | 30 seconds | 30 seconds |
| **Dry-run** | Pre-apply runs (validates before the preview) | Post-apply is skipped |
| **Array apply (partial failure)** | Runs once on the full array | Runs once on the resources that *were* persisted, even when later items in the batch fail |

Because the command is tokenized but then executed directly (no `sh -c`), pipes, redirections, glob expansion, and environment-variable expansion in the command string itself are **not** supported — put any shell logic inside the script the hook invokes.

> **Agent mode (`--agent` / `-A`).** When dtctl writes its JSON envelope to stdout, both pre-apply and post-apply hook output is redirected to stderr automatically so the envelope on stdout stays clean for machine consumers. Use stderr for any human-readable hook diagnostics.
>
> **Shell positional parameters in config values.** The config loader expands `$VAR` / `${VAR}` against the process environment but preserves shell positional parameters (`$1`, `$2`, `$@`, …) verbatim. You can write `pre-apply: "bash validate.sh \"$1\" \"$2\""` and have those tokens reach the hook unchanged.

### Writing Hooks

**Pre-apply** — reject dashboards without a title:

```bash
#!/bin/bash
# validate.sh
resource_type="$1"

if [ "$resource_type" = "dashboard" ]; then
  title=$(cat | jq -r '.title // empty')
  if [ -z "$title" ]; then
    echo "Error: dashboard must have a title" >&2
    exit 1
  fi
fi
```

**Post-apply** — delete the source file after a successful dashboard deploy, forcing the user to re-download before the next edit:

```bash
#!/bin/bash
# cleanup.sh
resource_type="$1"
source_file="$2"

if [ "$resource_type" = "dashboard" ] && [ -f "$source_file" ]; then
  rm -- "$source_file"
  id=$(cat | jq -r '.[0].id')
  echo "Deployed dashboard $id. Local file removed; re-download with 'dtctl get dashboard $id' before the next edit."
fi
```

### Usage

```bash
dtctl apply -f dashboard.yaml            # pre- and post-apply both run
dtctl apply -f dashboard.yaml --no-hooks # skip both hooks
dtctl apply -f dashboard.yaml --dry-run  # pre-apply runs, post-apply is skipped
dtctl apply -f dashboard.yaml -v         # verbose: logs hook command and duration
```

## Command Aliases

Create shortcuts for frequently used commands.

### Simple Aliases

```bash
dtctl alias set wf "get workflows"
dtctl wf
# Expands to: dtctl get workflows
```

### Parameterized Aliases

Use `$1`-`$9` for positional parameters:

```bash
dtctl alias set logs-errors "query 'fetch logs | filter status=\$1 | limit 100'"
dtctl logs-errors ERROR
# Expands to: dtctl query 'fetch logs | filter status=ERROR | limit 100'
```

### Shell Aliases

Prefix with `!` to execute through the system shell (enables pipes and external tools):

```bash
dtctl alias set wf-names "!dtctl get workflows -o json | jq -r '.workflows[].title'"
dtctl wf-names
```

### Import and Export

Share aliases with your team:

```bash
dtctl alias export -f team-aliases.yaml
dtctl alias import -f team-aliases.yaml
```

### Managing Aliases

```bash
dtctl alias list         # List all aliases
dtctl alias delete wf    # Delete an alias
```

### Alias Safety

Aliases cannot shadow built-in commands:

```bash
dtctl alias set get "query 'fetch logs'"
# Error: alias name "get" conflicts with built-in command
```

---

Previous: [Quick Start]({{ '/docs/quick-start/' | relative_url }})
