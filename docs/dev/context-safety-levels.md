# Context Safety Levels

## Overview

Context safety levels provide client-side protection against accidental destructive operations by binding safety constraints to connection contexts. This allows you to configure production contexts with strict safety while keeping development contexts permissive.

**Key Principle**: The safety level determines **what operations are allowed**. Confirmation behavior is **consistent** across all levels.

## Safety Levels

From safest to most permissive:

| Level | Description | Use Case |
|-------|-------------|----------|
| `read-only` | No modifications allowed | Production monitoring, troubleshooting, read-only API tokens |
| `read-write` | Can create/update/delete own resources only | Personal development, sandbox environments |
| `collaborative` | Can modify resources owned by others | Team environments, shared staging |
| `administrative` | Full resource management, no data operations | Production administration, most operations |
| `unrestricted` | All operations including data deletion | Development, emergency recovery, bucket management |

**Default**: If no safety level is specified, `read-write` is used.

## Configuration

### Context Structure

```yaml
contexts:
- name: production
  context:
    environment: https://abc123.apps.dynatrace.com
    token-ref: prod-token
    safety-level: administrative
    description: "Production environment - handle with care"

- name: dev-sandbox
  context:
    environment: https://dev789.live.dynatrace.com
    token-ref: dev-token
    safety-level: unrestricted
    description: "Personal dev environment - anything goes"
```

## Operation Permission Matrix

| Safety Level | Read | Create | Update Own | Update Shared | Delete Own | Delete Shared | Delete Bucket |
|-------------|------|--------|------------|---------------|------------|---------------|---------------|
| `read-only` | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `read-write` | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| `collaborative` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| `administrative` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| `unrestricted` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

**Note**: "Own" vs "Shared" distinction requires ownership detection (see Implementation Notes).

## Confirmation Behavior

**Consistent across all safety levels** - once an operation is permitted by the safety level:

### Standard Operations (create, update, delete resources)

```bash
# Interactive prompt with details
dtctl delete dashboard my-dashboard

# Output:
# Resource Type: dashboard
# Name: My Dashboard
# ID: abc-123-def
# Are you sure? [y/N]:

# Skip confirmation with flag
dtctl delete dashboard my-dashboard -y
```

### Data Destruction Operations (buckets, purge)

```bash
# Requires typing the resource name exactly
dtctl delete bucket logs-bucket

# Output:
# ⚠️  WARNING: This operation is IRREVERSIBLE and will delete all data
# Type the bucket name 'logs-bucket' to confirm: _

# Or use confirmation flag
dtctl delete bucket logs-bucket --confirm=logs-bucket
```

### Dry-Run Support

All destructive operations support `--dry-run`:

```bash
dtctl delete dashboard my-dashboard --dry-run
# Output: Would delete dashboard 'My Dashboard' (abc-123-def)

dtctl delete bucket logs-bucket --dry-run
# Output: Would permanently delete bucket 'logs-bucket' and all its data
```

## Usage Examples

### Example 1: Production Read-Only Access

```bash
# Setup read-only production access
dtctl config set-context prod-viewer \
  --environment https://prod.dynatrace.com \
  --token-ref readonly-token \
  --safety-level read-only

dtctl config use-context prod-viewer

# Allowed
dtctl get dashboards
dtctl query "fetch logs | limit 100"
dtctl describe workflow deploy-pipeline

# Blocked
dtctl delete dashboard old-dash
# Error: Context 'prod-viewer' (read-only) does not allow delete operations
```

### Example 2: Production Administration

```bash
# Setup production admin
dtctl config set-context prod-admin \
  --environment https://prod.dynatrace.com \
  --token-ref admin-token \
  --safety-level administrative

dtctl config use-context prod-admin

# Allowed (with confirmation)
dtctl delete dashboard old-dashboard
dtctl delete workflow deprecated-workflow
dtctl edit settings app-config

# Blocked - requires unrestricted
dtctl delete bucket temp-bucket
# Error: Context 'prod-admin' (administrative) does not allow bucket deletion
# Bucket operations require 'unrestricted' safety level
```

### Example 3: Team Staging Environment

```bash
# Setup shared staging access
dtctl config set-context staging \
  --environment https://staging.dynatrace.com \
  --token-ref staging-token \
  --safety-level collaborative

dtctl config use-context staging

# All team members can modify shared resources
dtctl edit dashboard team-dashboard
dtctl delete notebook experiment-01
dtctl apply -f shared-workflow.yaml

# Still protected from data loss
dtctl delete bucket staging-logs
# Error: Context 'staging' (collaborative) does not allow bucket deletion
```

### Example 4: Development Sandbox

```bash
# Setup unrestricted dev access
dtctl config set-context dev \
  --environment https://dev.dynatrace.com \
  --token-ref dev-token \
  --safety-level unrestricted

dtctl config use-context dev

# Everything allowed (with appropriate confirmations)
dtctl delete bucket test-bucket --confirm=test-bucket
dtctl delete dashboard any-dashboard -y
```

## Bypass Mechanisms

### --override-safety Flag

Bypass safety level checks for a single operation:

```bash
dtctl delete bucket logs-bucket --override-safety --confirm=logs-bucket
# ⚠️  Safety check bypassed: bucket deletion requires 'unrestricted' level
# Type the bucket name 'logs-bucket' to confirm: logs-bucket
```

**Use Cases**:
- Emergency operations
- Exceptional circumstances
- When you know what you're doing

**Behavior**:
- Prints a warning showing what safety was bypassed
- Still requires normal confirmation (typing name for buckets, -y for others)

## Context Management Commands

```bash
# Set or update safety level
dtctl config set-context <name> --safety-level <level>

# List contexts with safety info
dtctl config get-contexts
dtctl config get-contexts -o wide  # Shows safety level details

# View detailed context info
dtctl config describe-context <name>
```

## Implementation Notes

### Ownership Detection

For `read-write` level (own vs shared resources):

1. **Attempt 1**: Call `/platform/metadata/v1/user` to get current user ID
2. **Attempt 2**: Extract user ID from JWT token (no API call)
3. **Compare**: Check if resource owner matches current user ID
4. **Fallback**: If ownership cannot be determined, assume shared (safer)

User can override with: `--assume-mine` flag

### Safety Check Flow

```
Operation Requested
        ↓
[Check Safety Level]
        ↓ (allowed)
[Check Ownership if needed]
        ↓ (permitted)
[Confirmation Prompt]
        ↓ (confirmed)
[Execute Operation]
```

### Error Messages

Clear, actionable error messages:

```
❌ Operation not allowed:
   Context: production (administrative)
   Reason: Bucket deletion requires 'unrestricted' safety level

Suggestions:
  • Switch to an unrestricted context
  • Use --override-safety (if you have permission)
  • Contact your administrator
```

### Audit Logging (Future)

Context safety provides a foundation for audit logging:

```json
{
  "timestamp": "2026-01-15T10:30:00Z",
  "context": "production",
  "safety_level": "administrative",
  "operation": "delete",
  "resource_type": "dashboard",
  "resource_id": "abc-123",
  "user": "user@company.com",
  "bypassed_safety": false,
  "confirmation_method": "interactive"
}
```

## Migration Path

### Existing Configurations

Existing contexts without a safety level will default to `read-write`:

```yaml
# Existing config (no changes needed)
contexts:
- name: production
  context:
    environment: https://prod.dynatrace.com
    token-ref: prod-token
    # safety-level defaults to: read-write
```

### Gradual Adoption

1. **Phase 1**: Add safety levels to critical contexts (production)
2. **Phase 2**: Review and adjust safety levels based on usage
3. **Phase 3**: Enable audit logging (future)

### Backward Compatibility

- All existing commands continue to work without changes
- Safety checks are additive (don't break existing workflows)
- Flags (`-y`, `--force`) continue to work as before
- No breaking changes to config format

## Design Principles

1. **Client-side protection** - No server changes required
2. **Explicit over implicit** - Safety levels must be consciously set
3. **Fail safe** - Default to more restrictive when uncertain
4. **Consistent UX** - Same confirmation patterns across all operations
5. **Clear feedback** - Always explain why something was blocked
6. **Escape hatches** - Provide override mechanisms for exceptional cases
7. **Auditability** - Design enables future audit logging

## Related Documents

- `config-example.yaml` - Example configuration with safety levels
- `pkg/config/config.go` - Config structure definition
- `pkg/safety/checker.go` - Safety validation logic
- `cmd/config.go` - Context management commands
