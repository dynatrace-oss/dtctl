# Feature Flags API Design for dtctl

A comprehensive kubectl-style CLI for managing Dynatrace Feature Flags.

## Table of Contents

- [Overview](#overview)
- [Resource Hierarchy](#resource-hierarchy)
- [Command Structure](#command-structure)
- [Resource Types](#resource-types)
  - [Projects](#1-projects)
  - [Stages](#2-stages)
  - [Feature Flags](#3-feature-flags)
  - [Feature Flag Stage Definitions](#4-feature-flag-stage-definitions)
  - [Context Attributes](#5-context-attributes)
  - [Change Requests](#6-change-requests)
- [Common Workflows](#common-workflows)
- [Advanced Features](#advanced-features)
- [Examples](#examples)

## Overview

The Feature Flags API enables progressive rollouts, A/B testing, and controlled feature releases. The CLI provides a kubectl-inspired interface for managing feature flags across multiple projects and stages (environments).

### Design Principles

- **Project-based organization**: Feature flags are organized within projects
- **Stage-based deployment**: Different configurations per environment (dev, staging, prod)
- **Targeting rules**: Conditional flag evaluation based on context attributes
- **Change management**: Built-in approval workflows via change requests
- **Optimistic locking**: Version-based conflict prevention for updates

### API Endpoint

```
https://{environment}.apps.dynatrace.com/platform/feature-flag-management/v0.3
```

## Resource Hierarchy

```
Project (e.g., "mobile-app")
├── Feature Flag Definitions (e.g., "new-checkout-flow")
│   └── Stage Definitions (e.g., "prod", "dev")
│       ├── enabled: true/false
│       ├── defaultVariant: "on"
│       └── targeting: {...}
├── Context Attributes (e.g., "user-tier", "region")
└── Stages (linked, e.g., "production", "development")

Change Requests (approval workflow for stage definition changes)
```

## Command Structure

### Verb-Resource Pattern

```bash
dtctl <verb> <resource> [flags]

# Examples:
dtctl get projects
dtctl describe feature-flag my-flag --project mobile-app
dtctl apply -f flag-config.yaml
dtctl exec feature-flag my-flag --project mobile-app --stage prod
```

### Global Flags

```
--project string      # Specify project (can be set in context)
--stage string        # Specify stage (can be set in context)
--context string      # Use specific dtctl context
-o, --output string   # Output format: json|yaml|table|wide
--plain              # Plain output (no colors, no interactive)
--dry-run            # Preview without applying changes
```

## Resource Types

### 1. Projects

**API Spec**: `feature-flags.yaml` - Projects section

Projects are containers for feature flags, context attributes, and stage associations.

#### Operations

```bash
# Resource name: project/projects (short: proj)

# List all projects
dtctl get projects
dtctl get projects -o wide                    # Show owner info
dtctl get projects -o json

# Get specific project
dtctl get project <project-id>
dtctl describe project <project-id>           # Detailed view with stages

# Create project
dtctl create project -f project.yaml
dtctl create project my-app \
  --name "My Application" \
  --description "Main application feature flags" \
  --owner user@example.com

# Update project
dtctl apply -f project.yaml
dtctl edit project my-app                     # Edit in $EDITOR

# Delete project
dtctl delete project my-app
dtctl delete project my-app --force           # Skip confirmation

# List stages linked to project
dtctl get stages --project my-app

# Link stage to project
dtctl link stage prod --project my-app
dtctl link stage --project my-app --stage dev

# Unlink stage from project
dtctl unlink stage prod --project my-app
```

#### Project Manifest

```yaml
apiVersion: feature-flag/v0.3
kind: Project
metadata:
  id: my-app                    # Optional for create, required for update
spec:
  name: My Application
  description: Feature flags for the main application
  owners:
    - user1@example.com
    - user2@example.com
```

#### Table Output

```
NAME        DESCRIPTION                     OWNERS                  STAGES    MODIFIED
my-app      Main application feature flags  user1@example.com       3         2d ago
mobile-app  Mobile app features             user2@example.com       2         1w ago
```

---

### 2. Stages

**API Spec**: `feature-flags.yaml` - Stages section

Stages represent deployment environments (development, staging, production, etc.).

#### Operations

```bash
# Resource name: stage/stages (short: stg)

# List all stages
dtctl get stages
dtctl get stages --project my-app              # Stages linked to project

# Get specific stage
dtctl get stage production
dtctl describe stage production                # Detailed view

# Create stage
dtctl create stage -f stage.yaml
dtctl create stage production \
  --name "Production" \
  --description "Production environment" \
  --owner sre-team@example.com

# Update stage
dtctl apply -f stage.yaml
dtctl edit stage production

# Delete stage (only if not linked to any project)
dtctl delete stage old-stage
```

#### Stage Manifest

```yaml
apiVersion: feature-flag/v0.3
kind: Stage
metadata:
  id: production
spec:
  name: Production
  description: Production environment
  owners:
    - sre-team@example.com
```

#### Table Output

```
NAME        DESCRIPTION            OWNERS                PROJECTS    MODIFIED
production  Production environment sre-team@example.com  5           3d ago
staging     Staging environment    platform-team         3           1w ago
dev         Development            dev-team              8           2h ago
```

---

### 3. Feature Flags

**API Spec**: `feature-flags.yaml` - Feature Flag Definitions section

Feature flag definitions are the blueprint for a flag, defining its type, variants, and default behavior.

#### Operations

```bash
# Resource name: feature-flag/feature-flags (short: ff, flag)

# List feature flags in a project
dtctl get feature-flags --project my-app
dtctl get ff --project my-app
dtctl get ff --project my-app --filter 'name~checkout'

# Get specific flag
dtctl get feature-flag new-checkout --project my-app
dtctl describe ff new-checkout --project my-app

# Create feature flag
dtctl create feature-flag -f flag.yaml --project my-app
dtctl create ff new-checkout --project my-app \
  --name "New Checkout Flow" \
  --type BOOLEAN \
  --variants '{"on":true,"off":false}'

# Update feature flag
dtctl apply -f flag.yaml --project my-app
dtctl edit ff new-checkout --project my-app

# Delete feature flag (must not be active in any stage)
dtctl delete ff new-checkout --project my-app
dtctl delete ff new-checkout --project my-app --force

# Evaluate flag (check current state)
dtctl exec ff new-checkout --project my-app --stage prod
dtctl exec ff new-checkout --project my-app --stage prod \
  --context '{"user-tier":"premium","region":"us-east"}'
```

#### Feature Flag Manifest

```yaml
apiVersion: feature-flag/v0.3
kind: FeatureFlag
metadata:
  id: new-checkout-flow
  project: my-app
spec:
  name: New Checkout Flow
  description: Enable the redesigned checkout experience
  type: BOOLEAN  # STRING, NUMBER, BOOLEAN
  variants:
    on: true
    off: false

# Multi-variant example
---
apiVersion: feature-flag/v0.3
kind: FeatureFlag
metadata:
  id: checkout-theme
  project: my-app
spec:
  name: Checkout Theme
  type: STRING
  variants:
    red: "theme-red"
    blue: "theme-blue"
    green: "theme-green"
```

#### Table Output

```
NAME               TYPE     VARIANTS    STAGES ACTIVE    MODIFIED
new-checkout-flow  BOOLEAN  on, off     2/3              1d ago
checkout-theme     STRING   red, blue   1/3              3h ago
beta-features      BOOLEAN  on, off     0/3              2w ago
```

#### Wide Output

```
NAME               TYPE     VARIANTS    PROD     STAGING  DEV      MODIFIED
new-checkout-flow  BOOLEAN  on, off     enabled  enabled  enabled  1d ago
checkout-theme     STRING   3 variants  -        enabled  enabled  3h ago
```

---

### 4. Feature Flag Stage Definitions

**API Spec**: `feature-flags.yaml` - Feature Flag Stage Definitions section

Stage definitions are stage-specific configurations of a feature flag (enabled state, default variant, targeting rules).

#### Operations

```bash
# Resource name: feature-flag-stage/feature-flag-stages (short: ffs, flag-stage)

# List stage definitions for a flag
dtctl get feature-flag-stages \
  --project my-app \
  --stage production

# Get specific stage definition
dtctl get ffs new-checkout \
  --project my-app \
  --stage production
dtctl describe ffs new-checkout \
  --project my-app \
  --stage production

# Create/update stage definition
dtctl apply -f flag-stage.yaml
dtctl edit ffs new-checkout --project my-app --stage prod

# Enable/disable flag in a stage
dtctl patch ffs new-checkout \
  --project my-app \
  --stage production \
  --enabled=true
dtctl patch ffs new-checkout \
  --project my-app \
  --stage production \
  --enabled=false

# Set default variant
dtctl patch ffs new-checkout \
  --project my-app \
  --stage production \
  --default-variant=on

# Delete stage definition (falls back to project-level definition)
dtctl delete ffs new-checkout --project my-app --stage prod
```

#### Stage Definition Manifest

```yaml
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: new-checkout-flow
  project: my-app
  stage: production
spec:
  enabled: true
  defaultVariant: on
  targeting: |
    {
      "if": [
        {"==": [{"var": "user-tier"}, "premium"]},
        "on",
        "off"
      ]
    }

# Targeting explanation:
# If user-tier == "premium", return "on", else "off"
# Uses json-logic format: https://jsonlogic.com/
```

#### Advanced Targeting Examples

```yaml
# Percentage rollout
targeting: |
  {
    "if": [
      {"<": [{"var": "$flagd.flagKey"}, 0.25]},
      "on",
      "off"
    ]
  }
# 25% of users get "on"

# Multi-condition
targeting: |
  {
    "if": [
      {
        "and": [
          {"==": [{"var": "region"}, "us-east"]},
          {"in": [{"var": "user-tier"}, ["premium", "enterprise"]]}
        ]
      },
      "on",
      "off"
    ]
  }
```

#### Table Output

```
PROJECT  FLAG              STAGE       ENABLED  DEFAULT   TARGETING
my-app   new-checkout-flow production  true     on        yes
my-app   new-checkout-flow staging     true     on        yes
my-app   checkout-theme    dev         true     blue      no
```

---

### 5. Context Attributes

**API Spec**: `feature-flags.yaml` - Contexts section

Context attributes are variables used in targeting rules (e.g., user-tier, region, version).

#### Operations

```bash
# Resource name: context/contexts (short: ctx)

# List contexts for a project
dtctl get contexts --project my-app
dtctl get ctx --project my-app

# Get specific context
dtctl get context user-tier --project my-app
dtctl describe ctx user-tier --project my-app

# Create context
dtctl create context -f context.yaml --project my-app
dtctl create ctx user-tier --project my-app \
  --name "User Tier" \
  --type STRING \
  --description "User subscription level"

# Update context
dtctl apply -f context.yaml --project my-app
dtctl edit ctx user-tier --project my-app

# Delete context
dtctl delete ctx user-tier --project my-app
```

#### Context Manifest

```yaml
apiVersion: feature-flag/v0.3
kind: Context
metadata:
  id: user-tier
  project: my-app
spec:
  name: User Tier
  description: User subscription level
  type: STRING  # STRING, NUMBER, BOOLEAN, VERSION

# Other examples
---
apiVersion: feature-flag/v0.3
kind: Context
metadata:
  id: app-version
  project: my-app
spec:
  name: App Version
  type: VERSION  # Supports semantic version comparisons

---
apiVersion: feature-flag/v0.3
kind: Context
metadata:
  id: user-count
  project: my-app
spec:
  name: Active User Count
  type: NUMBER
```

#### Table Output

```
NAME          TYPE     DESCRIPTION
user-tier     STRING   User subscription level
region        STRING   Geographic region
app-version   VERSION  Application version
beta-user     BOOLEAN  Enrolled in beta program
```

---

### 6. Change Requests

**API Spec**: `feature-flags.yaml` - Change Requests section

Change requests provide an approval workflow for modifying feature flag stage definitions.

#### Operations

```bash
# Resource name: change-request/change-requests (short: cr)

# List change requests
dtctl get change-requests
dtctl get cr --project my-app
dtctl get cr --filter 'projectId=my-app and stageId=production'

# Get specific change request
dtctl get cr <request-id>
dtctl describe cr <request-id>

# Create change request
dtctl create cr -f change-request.yaml
dtctl create cr --project my-app \
  --feature-flag new-checkout \
  --stage production \
  --enabled=true \
  --comment "Enabling for production launch"

# Update change request
dtctl apply -f change-request.yaml
dtctl edit cr <request-id>

# Apply (approve) change request
dtctl apply cr <request-id> \
  --comment "Approved by SRE team"

# Close (reject) change request
dtctl close cr <request-id> \
  --comment "Not ready for production"
```

#### Change Request Manifest

```yaml
apiVersion: feature-flag/v0.3
kind: ChangeRequest
metadata:
  id: cr-123  # Optional for create
spec:
  projectId: my-app
  featureFlagId: new-checkout-flow
  stageId: production
  featureFlagStageDefinitionVersion: "v1.2.3"
  comment: Enabling new checkout for production
  enabled: true
  defaultVariant: on
  targeting: |
    {
      "if": [
        {"==": [{"var": "user-tier"}, "premium"]},
        "on",
        "off"
      ]
    }
```

#### Table Output

```
ID      PROJECT  FLAG              STAGE       STATUS   CREATED   REQUESTER
cr-123  my-app   new-checkout-flow production  pending  2h ago    user@example.com
cr-124  my-app   checkout-theme    staging     applied  1d ago    dev@example.com
cr-125  mobile   beta-features     prod        closed   3d ago    pm@example.com
```

## Common Workflows

### 1. Create a New Feature Flag

```bash
# Step 1: Create the project (if needed)
dtctl create project my-app --name "My Application"

# Step 2: Link stages to project
dtctl link stage dev --project my-app
dtctl link stage staging --project my-app
dtctl link stage production --project my-app

# Step 3: Create context attributes
dtctl create context user-tier --project my-app \
  --type STRING \
  --description "User subscription tier"

# Step 4: Create the feature flag
dtctl create ff new-feature --project my-app \
  --name "New Feature" \
  --type BOOLEAN \
  --variants '{"on":true,"off":false}'

# Step 5: Configure for dev (100% enabled)
dtctl apply -f - <<EOF
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: new-feature
  project: my-app
  stage: dev
spec:
  enabled: true
  defaultVariant: on
EOF

# Step 6: Configure for staging (premium users only)
dtctl apply -f - <<EOF
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: new-feature
  project: my-app
  stage: staging
spec:
  enabled: true
  defaultVariant: off
  targeting: |
    {
      "if": [
        {"==": [{"var": "user-tier"}, "premium"]},
        "on",
        "off"
      ]
    }
EOF

# Step 7: Create change request for production
dtctl create cr --project my-app \
  --feature-flag new-feature \
  --stage production \
  --enabled=true \
  --default-variant=off \
  --comment "Requesting production rollout"
```

### 2. Progressive Rollout

```bash
# Start with 10% rollout
dtctl patch ffs new-feature \
  --project my-app \
  --stage production \
  --enabled=true \
  --targeting '{
    "if": [
      {"<": [{"var": "$flagd.flagKey"}, 0.10]},
      "on",
      "off"
    ]
  }'

# Increase to 25%
dtctl patch ffs new-feature \
  --project my-app \
  --stage production \
  --targeting '{
    "if": [
      {"<": [{"var": "$flagd.flagKey"}, 0.25]},
      "on",
      "off"
    ]
  }'

# Full rollout
dtctl patch ffs new-feature \
  --project my-app \
  --stage production \
  --default-variant=on
```

### 3. Emergency Disable

```bash
# Quickly disable a flag in production
dtctl patch ffs problematic-feature \
  --project my-app \
  --stage production \
  --enabled=false \
  --force

# Or edit interactively
dtctl edit ffs problematic-feature --project my-app --stage production
```

### 4. Multi-Environment Deployment

```bash
# Apply flag configuration to all stages
for stage in dev staging production; do
  dtctl apply -f flag-config.yaml --stage $stage
done

# Or use templating
dtctl apply -f flag-config.yaml \
  --set stage=dev \
  --set enabled=true

dtctl apply -f flag-config.yaml \
  --set stage=production \
  --set enabled=false
```

### 5. Audit and Review

```bash
# List all flags in production
dtctl get ffs --project my-app --stage production

# Show flags enabled in production
dtctl get ffs --project my-app --stage production \
  --filter 'enabled=true'

# Get detailed flag configuration
dtctl describe ff my-flag --project my-app

# Export all flags for backup
dtctl get ff --project my-app -o yaml > flags-backup.yaml

# View change history (via change requests)
dtctl get cr --project my-app --filter 'featureFlagId=my-flag'
```

## Advanced Features

### Context-Based Configuration

Set default project and stage in your dtctl context:

```bash
# Set default project
dtctl config set-context my-context --project my-app

# Set default stage
dtctl config set-context my-context --stage production

# Now commands use these defaults
dtctl get ff                    # Lists flags in my-app
dtctl get ffs                   # Lists stage defs in my-app/production

# Override when needed
dtctl get ff --project other-app
```

### Validation

```bash
# Validate flag definition before applying
dtctl validate feature-flag -f flag.yaml

# Dry-run to preview changes
dtctl apply -f flag.yaml --dry-run

# Check targeting syntax
dtctl validate targeting -f targeting.json
```

### Filtering and Sorting

```bash
# Filter flags by name
dtctl get ff --project my-app --filter 'name~checkout'

# Filter by type
dtctl get ff --project my-app --filter 'type=BOOLEAN'

# Sort by modification time
dtctl get ff --project my-app --sort 'modificationInfo.lastModifiedTime'

# Combine filters
dtctl get cr \
  --filter 'projectId=my-app and stageId=production' \
  --sort 'modificationInfo.createdTime'
```

### Watch Mode

```bash
# Watch for changes to feature flags
dtctl get ff --project my-app --watch

# Watch change requests
dtctl get cr --project my-app --watch --interval 10s
```

### Bulk Operations

```bash
# Disable all flags in a stage
dtctl get ff --project my-app -o json | \
  jq -r '.items[].id' | \
  xargs -I {} dtctl patch ffs {} \
    --project my-app \
    --stage staging \
    --enabled=false

# Export all project configurations
dtctl get projects -o json | \
  jq -r '.items[].id' | \
  xargs -I {} sh -c 'dtctl get ff --project {} -o yaml > {}-flags.yaml'
```

## Examples

### Example 1: A/B Test Configuration

```yaml
apiVersion: feature-flag/v0.3
kind: FeatureFlag
metadata:
  id: checkout-variant
  project: ecommerce
spec:
  name: Checkout Page Variant
  description: A/B test for checkout redesign
  type: STRING
  variants:
    control: "original"
    variant-a: "redesign-v1"
    variant-b: "redesign-v2"

---
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: checkout-variant
  project: ecommerce
  stage: production
spec:
  enabled: true
  defaultVariant: control
  targeting: |
    {
      "if": [
        {"<": [{"var": "$flagd.flagKey"}, 0.33]},
        "variant-a",
        {
          "if": [
            {"<": [{"var": "$flagd.flagKey"}, 0.66]},
            "variant-b",
            "control"
          ]
        }
      ]
    }
```

### Example 2: Beta Feature for Premium Users

```yaml
apiVersion: feature-flag/v0.3
kind: FeatureFlag
metadata:
  id: advanced-analytics
  project: mobile-app
spec:
  name: Advanced Analytics
  type: BOOLEAN
  variants:
    enabled: true
    disabled: false

---
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: advanced-analytics
  project: mobile-app
  stage: production
spec:
  enabled: true
  defaultVariant: disabled
  targeting: |
    {
      "if": [
        {
          "or": [
            {"==": [{"var": "user-tier"}, "premium"]},
            {"==": [{"var": "user-tier"}, "enterprise"]},
            {"==": [{"var": "beta-user"}, true]}
          ]
        },
        "enabled",
        "disabled"
      ]
    }
```

### Example 3: Version-Based Rollout

```yaml
apiVersion: feature-flag/v0.3
kind: Context
metadata:
  id: app-version
  project: mobile-app
spec:
  name: App Version
  type: VERSION

---
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: new-ui
  project: mobile-app
  stage: production
spec:
  enabled: true
  defaultVariant: disabled
  targeting: |
    {
      "if": [
        {">=": [{"var": "app-version"}, "2.5.0"]},
        "enabled",
        "disabled"
      ]
    }
```

### Example 4: Regional Rollout

```yaml
apiVersion: feature-flag/v0.3
kind: FeatureFlagStage
metadata:
  featureFlagId: payment-provider-stripe
  project: ecommerce
  stage: production
spec:
  enabled: true
  defaultVariant: disabled
  targeting: |
    {
      "if": [
        {
          "in": [
            {"var": "region"},
            ["us-east", "us-west", "eu-west"]
          ]
        },
        "enabled",
        "disabled"
      ]
    }
```

## Shell Completion

```bash
# Enable completion for project and stage names
dtctl completion bash > /etc/bash_completion.d/dtctl

# Now you can tab-complete
dtctl get ff --project <TAB>
dtctl get ffs --project my-app --stage <TAB>
```

## Configuration File

```yaml
# ~/.config/dtctl/config
apiVersion: v1
kind: Config
current-context: prod

contexts:
- name: prod
  context:
    environment: https://abc123.apps.dynatrace.com
    token-ref: prod-token
    project: my-app        # Default project
    stage: production      # Default stage

- name: dev
  context:
    environment: https://abc123.apps.dynatrace.com
    token-ref: dev-token
    project: my-app
    stage: development

tokens:
- name: prod-token
  token: dt0s16.***
- name: dev-token
  token: dt0s16.***
```

## Error Handling

### Common Errors

```bash
# Flag not found
Error: feature flag "my-flag" not found in project "my-app"
  Run 'dtctl get ff --project my-app' to list available flags

# Cannot delete active flag
Error: cannot delete feature flag "my-flag": still active in stages [production, staging]
  Run 'dtctl get ffs --project my-app' to view stage definitions
  Disable in all stages first, or use --force

# Optimistic locking conflict
Error: version conflict when updating "my-flag"
  The resource has been modified since you last read it
  Run 'dtctl get ff my-flag --project my-app' to get the latest version
  Exit code: 409

# Invalid targeting syntax
Error: invalid targeting expression
  Expected valid JSON Logic format
  See https://jsonlogic.com/ for syntax documentation
```

## Implementation Notes

### Resource Scopes

- **Global**: Projects, Stages
- **Project-scoped**: Feature Flags, Contexts
- **Project+Stage-scoped**: Feature Flag Stage Definitions
- **Global with filters**: Change Requests

### Pagination

All list operations support pagination:

```bash
dtctl get ff --project my-app --page-size 50 --page 2
```

### Optimistic Locking

Update operations use optimistic locking via version fields. The CLI handles this transparently:

```bash
# CLI fetches current version, applies edit, sends with version
dtctl edit ff my-flag --project my-app
```

### Rate Limiting

The CLI implements exponential backoff for rate-limited requests and shows progress for long operations.

## Future Enhancements

- Flag usage analytics (evaluation counts, trends)
- Flag dependency tracking
- Scheduled flag changes
- Flag archival and history
- Template-based flag creation
- Integration with workflow automation
- Rollback capabilities
- Flag comparison across stages
