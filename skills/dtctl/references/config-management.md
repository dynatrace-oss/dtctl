# dtctl Configuration Management

## Configuration Discovery

dtctl checks three locations in priority order:
1. Command-line flags
2. Local project `.dtctl.yaml`
3. Global `$XDG_CONFIG_HOME/dtctl/config`

Recommendation: local `.dtctl.yaml` for project-specific contexts, global config for personal tenants.

## .dtctl.yaml Files

Can be committed to git (no secrets). Credentials stored separately in OS keyring.

**Team conflict note:** `use-context` modifies the config file. Workarounds:
- Exclude from version control
- Use `--context` flag per command instead of switching
- Accept individual context preferences in commits

## Credential Management

```bash
# Store token (use --token flag, not stdin)
dtctl config set-credentials "prod-token" --token "$TOKEN"

# Create context
dtctl config set-context "prod" \
  --environment "https://tenant.apps.dynatrace.com" \
  --token-ref "prod-token" \
  --safety-level readwrite-mine

# Switch context
dtctl config use-context "prod"

# Per-command context override
dtctl get workflows --context staging --plain
```

## Safety Levels

| Level | Use Case |
|-------|----------|
| `readonly` | Production monitoring |
| `readwrite-mine` | Development (recommended default) |
| `readwrite-all` | Shared environments |
| `dangerously-unrestricted` | Emergency admin |

Actual permissions depend on API token scopes, not just safety level.
