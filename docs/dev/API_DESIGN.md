# dtctl API Design

A kubectl-inspired CLI tool for managing Dynatrace platform resources.

## Table of Contents

- [Design Principles](#design-principles)
- [Command Structure](#command-structure)
- [Resource Types](#resource-types)
- [Common Operations](#common-operations)
- [Configuration & Context](#configuration--context)
  - [Current User Context](#current-user-context)
  - [Filtering Resources by Owner](#filtering-resources-by-owner---mine)
- [Output Formats](#output-formats)
- [Examples](#examples)

## Design Principles

### Core Philosophy

**"Kubectl for Dynatrace"**: Leverage existing DevOps muscle memory. If a user knows kubectl, they should intuitively know how to use dtctl.

**Developer Experience (DX) First**: Prioritize speed, interactivity, and "human-readable" inputs over 1:1 API purity.

**AI-Native**: The tool must function as an efficient backend for AI Agents, providing structured discovery and error recovery.

### 1. Command Structure (The Grammar)

Strictly follow the **verb + noun** pattern.

**Management (CRUD)**: `dtctl get | describe | edit | delete | apply [resource]`
- Resources: dashboard, notebook, alert, slo, workflow, etc.

**Execution (Data)**: `dtctl query [dql-string]` and `dtctl exec [resource]`

**Separation of Concerns**: Never mix configuration flags with data query logic.

### 2. The "No Leaky Abstractions" Rule (Data Querying)

Do not invent a new query language via CLI flags.

**Passthrough**: The query command is a "dumb pipe" for DQL.
- ✅ `dtctl query "fetch logs | filter ..."`
- ❌ `dtctl get logs --filter-status=ERROR` (Anti-pattern)

**File Support**: Always support reading queries from files (`-f query.dql`) to handle complex, multi-line logic.

**Templating**: Allow basic variable substitution in DQL files (`--set host=h-123`) to make them reusable.

### 3. Configuration Management (The "Monaco Bridge")

dtctl is the "Runtime" companion to Monaco's "Build time."

**Unit Testing**: Enable rapid testing of Monaco-style JSON templates without running a full pipeline.

**Apply Logic**:
- **Strict Mode**: Validate payloads against the API schema before sending.
- **Template Injection**: Support `dtctl apply -f template.json --set name="Dev"` to render variables client-side.
- **Idempotency**: `apply` must handle the logic of "Create (POST) if new, Update (PUT) if exists."

### 4. Input/Output Standards (Human vs. Machine)

**Input (Write)**: Support YAML with Comments.
- **Principle**: "Humans write YAML, Machines speak JSON."
- **Implementation**: Automatically convert user YAML to API-compliant JSON on the fly.

**Output (Read)**:
- **Default**: Human-readable TUI tables (ASCII).
- **JSON** (`-o json`): Raw API response for piping (jq).
- **YAML** (`-o yaml`): Reconstructed YAML for copy-pasting.
- **Charts**: Sparklines, bar charts, and line charts for timeseries data.

### 5. Handling Identity (Naming)

**Resolution Loop**: Never assume names are unique.
- If a user asks for `dtctl get dashboard "Production"` and multiple exist, stop and ask (Interactive Disambiguation).
- Display NAME and ID side-by-side in lists.

**Safety**: Destructive actions (delete, apply) on ambiguous names must require confirmation or an exact UUID.

### 6. AI Enablement Strategy

Design features specifically to help LLMs drive the tool.

**Self-Discovery**: Ensure `--help` provides exhaustive, structured context.

**Machine Output**: Implement a `--plain` flag that strips ANSI colors, spinners, and interactive prompts, returning strict JSONL/YAML for agents.

**Error Messages**: Errors should be descriptive and suggest the fix (e.g., "Unknown flag --ownr, did you mean --owner?") to allow Agentic auto-correction.

### 7. kubectl-like User Experience
- **Verb-noun pattern**: `dtctl <verb> <resource> [options]`
- **Consistent flags**: Same flags work across similar operations
- **Multiple output formats**: Table (human), JSON, YAML, charts
- **Declarative configuration**: Apply YAML/JSON files to create/update resources
- **Context management**: Switch between environments easily

### 8. Resource-Oriented Design
- Every Dynatrace API concept is exposed as a resource
- Resources have standard CRUD operations where applicable
- Resources can have sub-resources (e.g., `workflow/executions`)
- Resources support filtering, sorting, and field selection

### 9. Progressive Disclosure
- Simple commands for common tasks
- Advanced options available via flags
- Comprehensive help at every level

## Command Structure

### Core Verbs

```
get         - List or retrieve resources
describe    - Show detailed information about a resource
create      - Create a resource from file or arguments
delete      - Delete resources
edit        - Edit a resource interactively (supports YAML and JSON)
apply       - Apply configuration from file (create or update, supports templates)
logs        - Print logs for a resource
query       - Execute a DQL query (with template support)
exec        - Execute a workflow or function
history     - Show version history (snapshots) of a document
restore     - Restore a document to a previous version
wait        - Wait for a specific condition (query results, resource state)

# (not implemented yet)
# patch       - Update specific fields of a resource
# explain     - Show documentation for a resource type
# diff        - Show differences between local and remote resources
```

### Syntax Pattern

```bash
dtctl [verb] [resource-type] [resource-name] [flags]

# Examples:
dtctl get dashboards
dtctl get notebooks --name "analysis"
dtctl describe dashboard "Production Dashboard"
dtctl delete workflow my-workflow-id
dtctl apply -f workflow.yaml
dtctl query "fetch logs | limit 10"
```

### Global Flags

```
--context string      # Use a specific context
-o, --output string   # Output format: json|yaml|table|wide|name|custom-columns=...
--plain               # Plain output for machine processing (no colors, no interactive prompts)
--no-headers          # Omit headers in table output
-v, --verbose         # Verbose output
--dry-run             # Print what would be done without doing it
--field-selector string # Filter by fields (e.g., owner=me,type=notebook)

# (not implemented yet)
# -w, --watch           # Watch for changes
```

## Resource Types

> **Note**: Like kubectl, dtctl supports both singular and plural resource names (e.g., `dashboard` or `dashboards`, `notebook` or `notebooks`), as well as short aliases for convenience.
> 
> **Important**: There is no generic `documents` command. Dashboards and notebooks are accessed via their specific resource types (`dtctl get dashboards` and `dtctl get notebooks`), even though they share the underlying Document API.

### 1. Dashboards
**API Spec**: `document.yaml`

Dashboards are visual documents for monitoring and analysis.

```bash
# Resource name: dashboard/dashboards (short: dash, db)
dtctl get dashboards                             # List all dashboards
dtctl get dashboard <id>                         # Get specific dashboard by ID
dtctl get dashboards --name "production"         # Filter by name
dtctl describe dashboard <id>                    # Detailed view with metadata
dtctl describe dashboard "Production Dashboard"  # Describe by name
dtctl edit dashboard <id>                        # Edit in $EDITOR (YAML by default)
dtctl edit dashboard "My Dashboard"              # Edit by name
dtctl edit dashboard <id> --format=json          # Edit in JSON format
dtctl delete dashboard <id>                      # Move to trash
dtctl delete dashboard "Old Dashboard" -y        # Delete by name, skip confirmation
dtctl create dashboard -f dashboard.yaml         # Create new dashboard
dtctl apply -f dashboard.yaml                    # Create or update

# Sharing
dtctl share dashboard <id> --user <user-sso-id>  # Share with user (read access)
dtctl share dashboard <id> --user <id> --access read-write  # Read-write access
dtctl share dashboard <id> --group <group-sso-id> # Share with group
dtctl unshare dashboard <id> --user <user-sso-id> # Remove user access
dtctl unshare dashboard <id> --all               # Remove all shares

# (not implemented yet)
# dtctl lock dashboard <id>                        # Acquire active lock
# dtctl unlock dashboard <id>                      # Release active lock
```

### 2. Notebooks
**API Spec**: `document.yaml`

Notebooks are interactive documents for data exploration and analysis.

```bash
# Resource name: notebook/notebooks (short: nb)
dtctl get notebooks                              # List all notebooks
dtctl get notebook <id>                          # Get specific notebook by ID
dtctl get notebooks --name "analysis"            # Filter by name
dtctl describe notebook <id>                     # Detailed view with metadata
dtctl describe notebook "Analysis Notebook"      # Describe by name
dtctl edit notebook <id>                         # Edit in $EDITOR (YAML by default)
dtctl edit notebook "My Notebook"                # Edit by name
dtctl edit notebook <id> --format=json           # Edit in JSON format
dtctl delete notebook <id>                       # Move to trash
dtctl delete notebook "Old Notebook" -y          # Delete by name, skip confirmation
dtctl create notebook -f notebook.yaml           # Create new notebook
dtctl apply -f notebook.yaml                     # Create or update

# Sharing
dtctl share notebook <id> --user <user-sso-id>   # Share with user (read access)
dtctl share notebook <id> --user <id> --access read-write  # Read-write access
dtctl share notebook <id> --group <group-sso-id> # Share with group
dtctl unshare notebook <id> --user <user-sso-id> # Remove user access
dtctl unshare notebook <id> --all                # Remove all shares

# (not implemented yet)
# dtctl lock notebook <id>                         # Acquire active lock
```

### 3. Document Version History (Snapshots)
**API Spec**: `document.yaml`

These operations apply to both dashboards and notebooks. Snapshots capture document content at specific points in time and can be used to restore previous versions.

```bash
# View version history
dtctl history dashboard <id-or-name>             # List dashboard snapshots
dtctl history notebook <id-or-name>              # List notebook snapshots
dtctl history dashboard "Production Dashboard"   # By name
dtctl history notebook "Analysis Notebook" -o json  # Output as JSON

# Restore to previous version
dtctl restore dashboard <id-or-name> <version>   # Restore dashboard to version
dtctl restore notebook <id-or-name> <version>    # Restore notebook to version
dtctl restore dashboard "My Dashboard" 5         # Restore by name to version 5
dtctl restore notebook "My Notebook" 3 --force   # Skip confirmation

# Notes:
# - Snapshots are created when updating documents with create-snapshot option
# - Maximum 50 snapshots per document (oldest deleted when exceeded)
# - Snapshots auto-delete after 30 days
# - Only document owner can restore snapshots
# - Restoring creates a snapshot of current state before restoring

# Trash management (not implemented yet)
# dtctl get trash                                  # List deleted documents
# dtctl restore trash <id>                         # Restore from trash
# dtctl delete trash <id> --permanent              # Permanently delete
```

### 4. Service Level Objectives (SLOs)
**API Spec**: `slo.yaml`

```bash
# Resource name: slo/slos
dtctl get slos                                   # List all SLOs
dtctl get slos --filter 'name~my-service'        # Filter by name
dtctl describe slo <id>                          # Show SLO details
dtctl create slo -f slo-definition.yaml          # Create SLO
dtctl delete slo <id>                            # Delete SLO
dtctl apply -f slo-definition.yaml               # Create or update

# SLO Templates
dtctl get slo-templates                          # List templates
dtctl describe slo-template <id>                 # Template details
dtctl create slo --from-template <template-id>   # Create from template

# Evaluation
dtctl exec slo <id>                              # Evaluate SLO now
dtctl exec slo <id> --timeout 60                 # Custom timeout (seconds)
dtctl exec slo <id> -o json                      # Output as JSON
```

### 5. Automation Workflows
**API Spec**: `automation.yaml`

```bash
# Resource name: workflow/workflows (short: wf)
dtctl get workflows                              # List workflows
dtctl get workflow <id>                          # Get specific workflow
dtctl describe workflow <id>                     # Workflow details by ID
dtctl describe workflow "My Workflow"            # Workflow details by name
dtctl edit workflow <id>                         # Edit in $EDITOR (YAML by default)
dtctl edit workflow "My Workflow"                # Edit by name
dtctl edit workflow <id> --format=json           # Edit in JSON format
dtctl delete workflow <id>                       # Delete workflow by ID
dtctl delete workflow "Old Workflow"             # Delete by name (with confirmation)
dtctl delete workflow "Old Workflow" -y          # Delete by name, skip confirmation
dtctl apply -f workflow.yaml                     # Create or update

# Workflow execution
dtctl exec workflow <id>                         # Run workflow
dtctl exec workflow <id> --params key=value      # Run with parameters
dtctl exec workflow <id> --wait                  # Run and wait for completion
dtctl exec workflow <id> --wait --timeout 10m    # Run with custom timeout

# Workflow Executions (sub-resource)
dtctl get workflow-executions                    # List all executions
dtctl get workflow-executions -w <workflow-id>   # List executions for workflow
dtctl get wfe <execution-id>                     # Get specific execution
dtctl describe workflow-execution <execution-id> # Execution details with tasks
dtctl describe wfe <execution-id>                # Short alias

# Execution Logs
dtctl logs workflow-execution <execution-id>     # View execution logs
dtctl logs wfe <execution-id>                    # Short alias
dtctl logs wfe <execution-id> --follow           # Stream logs in real-time
dtctl logs wfe <execution-id> --all              # Full logs for all tasks
dtctl logs wfe <execution-id> --task <name>      # Logs for specific task

# Version History
dtctl history workflow <id-or-name>              # List workflow versions
dtctl history workflow "My Workflow"             # By name
dtctl history workflow <id> -o json              # Output as JSON

# Restore Previous Version
dtctl restore workflow <id-or-name> <version>    # Restore to version
dtctl restore workflow "My Workflow" 5           # Restore by name
dtctl restore workflow <id> 3 --force            # Skip confirmation
```

### 6. Identity & Access Management (IAM)
**API Specs**: `iam.yaml`, `appengine-registry.yaml`

```bash
# Users
dtctl get users                                  # List users
dtctl describe user <id>                         # User details
dtctl get users --group <group-id>               # Users in group

# Groups
dtctl get groups                                 # List groups
dtctl describe group <id>                        # Group details

# (not implemented yet)
# dtctl create group -f group.yaml                 # Create group
# dtctl delete group <id>                          # Delete group

# Permissions & Policies (not implemented yet)
# dtctl get policies                               # List policies
# dtctl describe policy <id>                       # Policy details
# dtctl create policy -f policy.yaml               # Create policy
# dtctl get permissions --user <id>                # User's permissions
```

### 7. Grail Data & Queries
**API Specs**: `grail-query.yaml`, `grail-storage-management.yaml`, `grail-fieldsets.yaml`, `grail-filter-segments.yaml`

```bash
# DQL Queries
dtctl query "fetch logs | limit 100"             # Execute DQL query
dtctl query -f query.dql                         # Execute from file
dtctl query "fetch logs" -o json                 # Output as JSON
dtctl query "fetch logs" -o yaml                 # Output as YAML
dtctl query "fetch logs" -o table                # Output as table

# DQL with template variables
dtctl query -f query.dql --set host=h-123        # With variable substitution
dtctl query -f query.dql --set host=h-123 --set timerange=2h

# Template Syntax:
#   Use {{.variable}} to reference variables
#   Use {{.variable | default "value"}} for default values

# Wait for Query Results
# Poll a query until a specific condition is met
dtctl wait query "fetch spans | filter test_id == 'test-123'" --for=count=1 --timeout 5m
dtctl wait query "fetch logs | filter status == 'ERROR'" --for=any --timeout 2m
dtctl wait query -f query.dql --set test_id=my-test --for=count-gte=1

# Wait conditions:
#   count=N       - Exactly N records
#   count-gte=N   - At least N records (>=)
#   count-gt=N    - More than N records (>)
#   count-lte=N   - At most N records (<=)
#   count-lt=N    - Fewer than N records (<)
#   any           - Any records (count > 0)
#   none          - No records (count == 0)

# Wait with custom backoff strategy
dtctl wait query "..." --for=any \
  --min-interval 500ms --max-interval 15s --backoff-multiplier 1.5

# Wait and output results when condition is met
dtctl wait query "..." --for=count=1 -o json > result.json

# Storage Buckets
# Resource name: bucket/buckets (short: bkt)
dtctl get buckets                                # List storage buckets
dtctl get bucket <bucket-name>                   # Get specific bucket
dtctl describe bucket <bucket-name>              # Bucket details
dtctl create bucket -f bucket.yaml               # Create bucket
dtctl delete bucket <bucket-name>                # Delete bucket
dtctl apply -f bucket.yaml                       # Create or update bucket

# Fieldsets (not implemented yet)
# dtctl get fieldsets                              # List fieldsets
# dtctl describe fieldset <id>                     # Fieldset details
# dtctl create fieldset -f fieldset.yaml           # Create fieldset

# Filter Segments (not implemented yet)
# dtctl get filter-segments                        # List filter segments
# dtctl describe filter-segment <id>               # Details
# dtctl create filter-segment -f segment.yaml      # Create segment

# Storage usage info (not implemented yet)
# dtctl get bucket-usage                           # Storage usage info
```

### 8. Settings
**API Spec**: `settings.yaml`

```bash
# Settings Schemas
dtctl get settings-schemas                       # List all settings schemas
dtctl get settings-schema <schema-id>            # Get schema definition

# Settings Objects
dtctl get settings --schema <schema-id>          # List settings for schema
dtctl get settings --schema <schema-id> --scope environment  # Filter by scope
dtctl get settings <object-id>                   # Get specific settings object
dtctl create settings -f value.yaml --schema <schema-id> --scope environment
dtctl delete settings <object-id>                # Delete settings object
dtctl apply -f settings.yaml                     # Apply settings (create or update)

# (not implemented yet)
# dtctl validate setting -f setting.yaml           # Validate without applying
```

### 9. Notifications
**API Specs**: `notification-v2.yaml`

```bash
# Resource name: notification/notifications (short: notif)
dtctl get notifications                          # List event notifications
dtctl get notification <id>                      # Get specific notification
dtctl get notifications --type <type>            # Filter by notification type
dtctl delete notification <id>                   # Delete notification

# (not implemented yet)
# dtctl create notification -f notif.yaml          # Create notification
```

### 10. App Engine
**API Specs**: `appengine-app-functions.yaml`, `appengine-edge-connect.yaml`, `appengine-function-executor.yaml`, `appengine-registry.yaml`

```bash
# Apps (Registry)
dtctl get apps                                   # List installed apps
dtctl describe app <id>                          # App details
dtctl delete app <id>                            # Uninstall app

# App Functions (from installed apps)
# Resource name: function/functions (short: fn, func)
dtctl get functions --app <app-id>               # List functions in an app
dtctl describe function <app-id>/<function-name> # Function details
dtctl exec function <app-id>/<function-name>     # Execute function (GET)
dtctl exec function <app-id>/<function-name> --method POST --payload '{"key":"value"}'
dtctl exec function <app-id>/<function-name> --method POST --data @payload.json
dtctl exec function <app-id>/<function-name> -o json  # JSON output

# Deferred (async) execution for resumable functions (not implemented yet)
# dtctl exec function <app-id>/<function-name> --defer
# dtctl get deferred-executions                    # List deferred executions
# dtctl describe deferred-execution <execution-id> # Execution details

# Function Executor (ad-hoc code execution)
dtctl exec function -f script.js                 # Execute JavaScript file
dtctl exec function -f script.js --payload '{"input":"data"}'
dtctl exec function --code 'export default async function() { return "hello" }'
dtctl get sdk-versions                           # List available SDK versions

# EdgeConnect
# Resource name: edgeconnect/edgeconnects (short: ec)
dtctl get edgeconnects                           # List EdgeConnect configs
dtctl get edgeconnect <id>                       # Get specific EdgeConnect
dtctl describe edgeconnect <id>                  # EdgeConnect details
dtctl create edgeconnect -f edgeconnect.yaml     # Create EdgeConnect
dtctl delete edgeconnect <id>                    # Delete EdgeConnect
```

### 11. OpenPipeline
**API Specs**: `openpipeline-config.yaml`, `openpipeline-ingest.json`

```bash
# Pipeline configurations (read-only)
# Resource name: openpipeline/openpipelines (short: opp, pipeline, pipelines)
dtctl get openpipelines                          # List pipelines
dtctl get openpipeline <id>                      # Get specific pipeline (e.g., logs, events, bizevents)
dtctl describe openpipeline <id>                 # Pipeline details
dtctl get openpipelines -o json                  # Output as JSON

# Write operations (not implemented yet)
# dtctl create pipeline -f pipeline.yaml         # Create pipeline
# dtctl apply -f pipeline.yaml                   # Update pipeline

# Validation (not implemented yet)
# dtctl validate pipeline -f pipeline.yaml       # Validate config

# Ingest (not implemented yet)
# dtctl ingest --pipeline <id> -f data.json      # Test ingest
```

### 12. Vulnerabilities
**API Spec**: `vulnerabilities.yaml`

```bash
# Resource name: vulnerability/vulnerabilities (short: vuln)
dtctl get vulnerabilities                        # List vulnerabilities
dtctl get vulnerabilities --severity critical    # Filter by severity
dtctl describe vulnerability <id>                # Vulnerability details
dtctl get vulnerabilities --affected <entity-id> # By affected entity
```

### 13. Davis AI
**API Specs**: `davis-analyzers.yaml`, `davis-copilot.yaml`

Davis AI provides predictive/causal analysis (Analyzers) and generative AI chat (CoPilot).

```bash
# Analyzers
# Resource name: analyzer/analyzers (short: az)
dtctl get analyzers                              # List all available analyzers
dtctl get analyzer dt.statistics.GenericForecastAnalyzer  # Get analyzer definition
dtctl get analyzers --filter "name contains 'forecast'"   # Filter analyzers
dtctl get analyzers -o json                      # Output as JSON

# Execute Analyzers
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer -f input.json
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --input '{"query":"timeseries avg(dt.host.cpu.usage)"}'
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer --query "timeseries avg(dt.host.cpu.usage)"

# Analyzer execution options
dtctl exec analyzer <name> -f input.json --validate  # Validate input without executing
dtctl exec analyzer <name> -f input.json --wait      # Wait for completion (default)
dtctl exec analyzer <name> -f input.json --timeout 600  # Custom timeout (seconds)
dtctl exec analyzer <name> -f input.json -o json     # Output result as JSON

# Davis CoPilot Skills
dtctl get copilot-skills                         # List available CoPilot skills

# Davis CoPilot Chat
# Resource name: copilot (short: cp, chat)
dtctl exec copilot "What caused the CPU spike?"  # Ask a question
dtctl exec copilot -f question.txt               # Read question from file
dtctl exec copilot "Explain errors" --stream     # Stream response in real-time

# CoPilot chat options
dtctl exec copilot "Analyze this" --context "Additional context here"
dtctl exec copilot "What is DQL?" --no-docs      # Disable Dynatrace docs retrieval
dtctl exec copilot "List errors" --instruction "Answer in bullet points"

# NL to DQL
dtctl exec copilot nl2dql "show me error logs from the last hour"
dtctl exec copilot nl2dql "find hosts with high CPU usage"
dtctl exec copilot nl2dql -f prompt.txt          # Read prompt from file
dtctl exec copilot nl2dql "..." -o json          # Output as JSON (includes messageToken)

# DQL to NL
dtctl exec copilot dql2nl "fetch logs | filter status='ERROR' | limit 10"
dtctl exec copilot dql2nl -f query.dql           # Read query from file
dtctl exec copilot dql2nl "..." -o json          # Output as JSON (includes summary + explanation)

# Document Search
dtctl exec copilot document-search "CPU analysis" --collections notebooks
dtctl exec copilot document-search "error monitoring" --collections dashboards,notebooks
dtctl exec copilot document-search "performance" --exclude doc-123,doc-456
```

**Analyzer Input Example** (`input.json`):
```json
{
  "query": "timeseries avg(dt.host.cpu.usage)",
  "forecastHorizon": 100,
  "generalParameters": {
    "timeframe": {
      "startTime": "now-7d",
      "endTime": "now"
    }
  }
}
```

### 14. Platform Management
**API Spec**: `platform-management.yaml`

```bash
# Environments and accounts (not implemented yet)
# dtctl get environments                           # List environments
# dtctl describe environment <id>                  # Environment details
# dtctl get accounts                               # List accounts (if multi-account)
```

### 15. Hub (Extensions)
**API Specs**: `hub.yaml`, `hub-certificates.yaml`

```bash
# Extensions (not implemented yet)
# dtctl get extensions                             # List installed extensions
# dtctl describe extension <id>                    # Extension details
# dtctl install extension <extension-id>           # Install from Hub
# dtctl uninstall extension <id>                   # Uninstall extension

# Certificates (not implemented yet)
# dtctl get certificates                           # List certificates
```

### 16. Feature Flags
**API Spec**: `feature-flags.yaml`
**Detailed Design**: [FEATURE_FLAGS_API_DESIGN.md](FEATURE_FLAGS_API_DESIGN.md)

Feature flags enable progressive rollouts, A/B testing, and controlled feature releases. The API is organized around **Projects** (containers), **Stages** (environments), **Feature Flag Definitions**, and **Stage Definitions** (stage-specific configs).

```bash
# All feature flags commands (not implemented yet)

# Projects - containers for feature flags
# Resource name: project/projects (short: proj)
# dtctl get projects                               # List projects
# dtctl describe project my-app                    # Project details with stages
# dtctl create project my-app --name "My Application"
# dtctl delete project my-app

# Link/unlink stages to projects
# dtctl link stage production --project my-app     # Link stage to project
# dtctl unlink stage dev --project my-app          # Unlink stage

# Stages - deployment environments
# Resource name: stage/stages (short: stg)
# dtctl get stages                                 # List all stages
# dtctl get stages --project my-app                # Stages linked to project
# dtctl create stage production --name "Production"
# dtctl delete stage old-stage                     # Only if not linked

# Feature Flags - flag definitions
# Resource name: feature-flag/feature-flags (short: ff, flag)
# dtctl get ff --project my-app                    # List flags in project
# dtctl describe ff new-checkout --project my-app  # Flag details
# dtctl create ff new-feature --project my-app \
#   --type BOOLEAN \
#   --variants '{"on":true,"off":false}'
# dtctl edit ff new-feature --project my-app       # Edit in $EDITOR
# dtctl delete ff old-feature --project my-app

# Feature Flag Stage Definitions - stage-specific configs
# Resource name: feature-flag-stage/feature-flag-stages (short: ffs)
# dtctl get ffs --project my-app --stage production
# dtctl describe ffs new-checkout --project my-app --stage prod
# dtctl patch ffs new-checkout \
#   --project my-app \
#   --stage production \
#   --enabled=true                                 # Enable flag
# dtctl patch ffs new-checkout \
#   --project my-app \
#   --stage production \
#   --default-variant=on                           # Set default
# dtctl edit ffs new-checkout --project my-app --stage prod
# dtctl delete ffs new-checkout --project my-app --stage dev

# Context Attributes - variables for targeting rules
# Resource name: context/contexts (short: ctx)
# dtctl get contexts --project my-app              # List context attributes
# dtctl create ctx user-tier --project my-app \
#   --type STRING \
#   --description "User subscription level"
# dtctl delete ctx old-context --project my-app

# Change Requests - approval workflow for changes
# Resource name: change-request/change-requests (short: cr)
# dtctl get cr --project my-app                    # List change requests
# dtctl create cr --project my-app \
#   --feature-flag new-checkout \
#   --stage production \
#   --enabled=true \
#   --comment "Enabling for production"
# dtctl apply cr <request-id> \
#   --comment "Approved by SRE"                    # Approve request
# dtctl close cr <request-id> \
#   --comment "Not ready"                          # Reject request

# Evaluate flag (check current state)
# dtctl exec ff new-checkout --project my-app --stage prod
# dtctl exec ff new-checkout --project my-app --stage prod \
#   --context '{"user-tier":"premium"}'            # With context

# Common workflows
# Progressive rollout
# dtctl patch ffs new-feature --project my-app --stage prod \
#   --enabled=true \
#   --targeting '{"if":[{"<":[{"var":"$flagd.flagKey"},0.10]},"on","off"]}'

# Set project/stage defaults in context
# dtctl config set-context prod --project my-app --stage production
# dtctl get ff                                     # Uses default project
# dtctl get ffs                                    # Uses default project+stage
```

**See [FEATURE_FLAGS_API_DESIGN.md](FEATURE_FLAGS_API_DESIGN.md) for:**
- Complete command reference for all resource types
- Manifest examples (YAML/JSON)
- Targeting rule examples (percentage rollouts, A/B tests, user segments)
- Common workflows (progressive rollout, emergency disable)
- Context-based configuration
- Change request workflows

### 17. Email (Templates)
**API Spec**: `email.yaml`

```bash
# Email templates and sending (not implemented yet)
# dtctl get email-templates                        # List templates
# dtctl send email --template <id> --to user@ex.com # Send email
```

### 18. State Management
**API Spec**: `state-management.yaml`

```bash
# State storage for apps/extensions (not implemented yet)
# dtctl get state <key>                            # Get state value
# dtctl set state <key> <value>                    # Set state
# dtctl delete state <key>                         # Delete state
```

## Common Operations

### Create Resources

```bash
# From file with create command
dtctl create workflow -f workflow.yaml
dtctl create dashboard -f dashboard.yaml
dtctl create notebook -f notebook.yaml
dtctl create slo -f slo.yaml
dtctl create settings -f settings.yaml --schema <schema-id> --scope <scope>
dtctl create bucket -f bucket.yaml
dtctl create edgeconnect -f edgeconnect.yaml

# From file with apply (create or update)
dtctl apply -f resource.yaml
dtctl apply -f directory/                        # Multiple files

# From stdin
cat resource.yaml | dtctl apply -f -
cat workflow.yaml | dtctl create workflow -f -

# With template variables
dtctl create workflow -f workflow.yaml --set env=prod --set owner=team-a

# Inline creation (not implemented yet)
# dtctl create document --name "My Notebook" --type notebook
```

### Update Resources

```bash
# Declarative update (apply)
dtctl apply -f resource.yaml                     # Create if not exists, update if exists

# Imperative update (patch - not implemented yet)
# dtctl patch document <id> --name "New Name"

# Interactive edit
dtctl edit document <id>                         # Opens in $EDITOR
```

### Delete Resources

```bash
# Single resource by ID
dtctl delete document <id>

# Single resource by name (with name resolution)
dtctl delete workflow "My Workflow"
dtctl delete dashboard "Production Dashboard"

# From file
dtctl delete -f resource.yaml

# Multiple resources (supported for some resource types)
dtctl delete document <id1> <id2> <id3>

# Skip confirmation prompt
dtctl delete document <id> -y
dtctl delete document <id> --yes

# Note: Deletion requires confirmation by default (shows resource details)
# Use -y/--yes to skip, or --plain to disable interactive prompts
```

### List & Filter

```bash
# Basic list
dtctl get dashboards
dtctl get notebooks

# Filter by field (limited support - depends on resource type)
dtctl get dashboards --mine                      # Filter to current user's dashboards
dtctl get notebooks --mine                       # Filter to current user's notebooks

# Limit results
dtctl get workflows --chunk-size 10              # Control pagination

# Wide output (more columns)
dtctl get dashboards -o wide
dtctl get notebooks -o wide

# (not implemented yet)
# dtctl get dashboards --sort-by=.metadata.modified  # Sort results
# dtctl get slos --filter 'name~production'          # Advanced filters
# dtctl get dashboards --output custom-columns=NAME:.name,TYPE:.type,OWNER:.owner
```

## Configuration & Context

### Configuration File Structure

**Location** (platform-specific):
- **Linux**: `$XDG_CONFIG_HOME/dtctl/config` (default: `~/.config/dtctl/config`)
- **macOS**: `~/Library/Application Support/dtctl/config`
- **Windows**: `%LOCALAPPDATA%\dtctl\config`

**Note**: dtctl follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html) and adapts to platform conventions.

```yaml
apiVersion: v1
kind: Config
current-context: prod

contexts:
- name: dev
  context:
    environment: https://dev.apps.dynatrace.com
    token-ref: dev-token

- name: prod
  context:
    environment: https://prod.apps.dynatrace.com
    token-ref: prod-token

tokens:
- name: dev-token
  token: dt0s16.***

- name: prod-token
  token: dt0s16.***

preferences:
  output: table
  editor: vim
```

### Context Management Commands

```bash
# View configuration
dtctl config view                                # View full config
dtctl config view --minify                       # View without defaults

# View contexts
dtctl config get-contexts                        # List all contexts
dtctl config current-context                     # Show current context

# Switch context
dtctl config use-context prod                    # Switch to prod

# Set context properties
dtctl config set-context dev --environment https://...

# Set credentials
dtctl config set-credentials dev --token dt0s16.***

# Delete context
dtctl config delete-context dev

# Rename context
dtctl config rename-context old-name new-name
```

### Authentication

```bash
# Set token for current context
dtctl config set-credentials --token dt0s16.***

# Login interactively (OAuth flow, if supported)
dtctl login

# View current auth info
dtctl auth whoami

# Test authentication
dtctl auth can-i create documents
dtctl auth can-i delete slo my-slo-id
dtctl auth can-i '*' '*'                         # Test all permissions
```

### Current User Context

The `auth whoami` command displays information about the currently authenticated user.

```bash
# View current user info
dtctl auth whoami
# Output:
# User ID:    621321d-1231-dsad-652321829b50
# User Name:  John Doe
# Email:      john.doe@example.com
# Context:    prod
# Environment: https://abc12345.apps.dynatrace.com

# Machine-readable output
dtctl auth whoami -o json
# {"userId":"621321d-...","userName":"John Doe","emailAddress":"john.doe@example.com"}

dtctl auth whoami -o yaml

# Get just the user ID (useful for scripting)
dtctl auth whoami --id-only
# 621321d-1231-dsad-652321829b50
```

**Implementation Notes:**
- Primary: Calls `/platform/metadata/v1/user` API (requires `app-engine:apps:run` scope)
- Fallback: Decodes JWT token's `sub` claim (works offline, but only provides user ID)

### Filtering Resources by Owner (`--mine`)

Many resources support ownership. The `--mine` flag filters to show only resources owned by the current user.

```bash
# List only my dashboards
dtctl get dashboards --mine

# List only my notebooks  
dtctl get notebooks --mine

# List only my workflows
dtctl get workflows --mine

# Combine with other filters
dtctl get dashboards --mine --name "production"
dtctl get notebooks --mine -o json
```

**Supported Resources:**
| Resource | `--mine` Support | Filter Field |
|----------|------------------|--------------|
| `dashboards` | ✅ | `owner` |
| `notebooks` | ✅ | `owner` |
| `workflows` | ✅ | `owner` |
| `slos` | ✅ | `owner` |
| `filter-segments` | ✅ | `owner` |
| `settings` | ❌ | N/A (scope-based) |
| `apps` | ❌ | N/A (environment-wide) |

**How it works:**
1. dtctl fetches the current user ID (via metadata API or JWT)
2. Adds `owner=='<user-id>'` to the API filter parameter
3. Returns only resources owned by the authenticated user

**Alternative: Explicit owner filter**

For more control, use the `--field-selector` flag:

```bash
# Filter by specific owner
dtctl get dashboards --field-selector owner=<user-id>

# Filter by creator (different from owner for transferred docs)
dtctl get notebooks --field-selector modificationInfo.createdBy=<user-id>

# The special value "me" resolves to current user ID
dtctl get dashboards --field-selector owner=me
dtctl get notebooks --field-selector modificationInfo.createdBy=me
```

**Caching User ID:**

To avoid repeated API calls, dtctl caches the user ID for the current context:
- Cache location: `~/.cache/dtctl/<context>/user.json`
- Cache TTL: 24 hours (configurable via `preferences.user-cache-ttl`)
- Force refresh: `dtctl auth whoami --refresh`

## Output Formats

### Table (default)
```bash
dtctl get documents
# NAME              TYPE       OWNER    MODIFIED
# my-notebook       notebook   me       2h ago
# prod-dashboard    dashboard  team     1d ago
```

### JSON
```bash
dtctl get documents -o json
# {"items": [{"id": "...", "name": "my-notebook", ...}]}

# Pretty print
dtctl get documents -o json | jq .
```

### YAML
```bash
dtctl get document my-notebook -o yaml
# apiVersion: document/v1
# kind: Document
# metadata:
#   id: abc-123
#   name: my-notebook
# ...
```

### Custom Output
```bash
# JSONPath query
dtctl get documents -o jsonpath='{.items[*].name}'

# Custom columns
dtctl get documents -o custom-columns=NAME:.name,ID:.id
```

### Chart (Timeseries Visualization)
```bash
# Visualize timeseries data as ASCII line charts in the terminal
dtctl query "timeseries avg(dt.host.cpu.usage)" -o chart

# Forecast analyzer with chart output
dtctl exec analyzer dt.statistics.GenericForecastAnalyzer \
  --input '{"timeSeriesData":"timeseries avg(dt.host.cpu.usage)","forecastHorizon":50}' \
  -o chart

# Multiple series (grouped by dimension) - limited to 10 series
dtctl query "timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}" -o chart
```

### Sparkline (Compact Timeseries)
```bash
# Compact single-line visualization with stats
dtctl query "timeseries avg(dt.host.cpu.usage)" -o sparkline

# Compare multiple series compactly (alias: -o spark)
dtctl query "timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}" -o spark
```

Output example:
```
HOST-A  ▁▂▃▄▅▆▇█▇▆▅▄▃▂▁  (min: 22.7, max: 38.4, avg: 33.2)
HOST-B  ▅▅▅▅▅▅▅▅▅▅▅▅▅▅▅  (min: 99.0, max: 100.0, avg: 99.5)
```

### Bar Chart (Aggregated Comparison)
```bash
# Compare average values across series as horizontal bars
dtctl query "timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}" -o barchart

# Short alias
dtctl query "timeseries avg(dt.host.cpu.usage), by:{dt.entity.host}" -o bar
```

Output example:
```
HOST-A  ██████████░░░░░░░░░░  33.2
HOST-B  ██████████████████████████████████████████████████  100.0
```

**Note**: All timeseries output formats (`chart`, `sparkline`, `barchart`) require timeseries data 
(records with `timeframe` and `interval` fields). If the data is not timeseries, they fall back 
to JSON output with a warning. When more than 10 series are present, only the first 10 are displayed.

## Examples

### Working with Dashboards and Notebooks

```bash
# List dashboards and notebooks
dtctl get dashboards
dtctl get notebooks

# Filter by name
dtctl get dashboards --name "production"
dtctl get notebooks --name "analysis"

# View details
dtctl describe dashboard "Production Dashboard"
dtctl describe notebook "Analysis Notebook"

# Edit a dashboard (opens in $EDITOR)
dtctl edit dashboard <dashboard-id>
dtctl edit dashboard "Production Dashboard"

# Edit in JSON format instead of YAML
dtctl edit notebook <notebook-id> --format=json

# Delete (moves to trash)
dtctl delete dashboard <dashboard-id>
dtctl delete notebook "Old Notebook" --force  # Skip confirmation

# Apply changes from file
dtctl apply -f dashboard.yaml
dtctl apply -f notebook.yaml --set environment=prod
```

### Managing SLOs

```bash
# Create SLO from template
dtctl get slo-templates --filter 'name~availability'
dtctl describe slo-template template-id
dtctl create slo --from-template template-id \
  --name "API Availability" \
  --target 99.9

# Check SLO status
dtctl get slos
dtctl describe slo my-slo-id

# Evaluate SLO performance
dtctl exec slo my-slo-id                         # Evaluate and show results
dtctl exec slo my-slo-id -o json | jq '.evaluationResults[].errorBudget'
```

### Automation Workflows

```bash
# List and view workflows
dtctl get workflows
dtctl describe workflow <workflow-id>

# Edit a workflow
dtctl edit workflow <workflow-id>
dtctl edit workflow "My Workflow"

# Apply workflow from file (create or update)
dtctl apply -f workflow.yaml
dtctl apply -f workflow.yaml --set environment=prod

# Execute workflow
dtctl exec workflow <workflow-id>
dtctl exec workflow <workflow-id> --params severity=high --params env=prod

# Execute and wait for completion
dtctl exec workflow <workflow-id> --wait
dtctl exec workflow <workflow-id> --wait --timeout 10m

# Monitor executions
dtctl get workflow-executions
dtctl get wfe -w <workflow-id>              # Filter by workflow
dtctl describe wfe <execution-id>           # Detailed view with tasks

# View execution logs
dtctl logs wfe <execution-id>
dtctl logs wfe <execution-id> --follow      # Stream in real-time
dtctl logs wfe <execution-id> --all         # All tasks with headers
dtctl logs wfe <execution-id> --task <name> # Specific task
```

### Querying Grail Data

```bash
# Simple query
dtctl query "fetch logs | filter status='ERROR' | limit 100"

# Query with output formatting
dtctl query "fetch logs | summarize count(), by: {status}" -o json
dtctl query "fetch logs | limit 10" -o yaml
dtctl query "fetch logs | limit 10" -o table

# Execute query from file
dtctl query -f analysis.dql
dtctl query -f analysis.dql -o json > results.json

# Query with template variables
dtctl query -f logs-by-host.dql --set host=h-123 --set timerange=2h

# Template example (logs-by-host.dql):
# fetch logs
# | filter host = "{{.host}}"
# | filter timestamp > now() - {{.timerange | default "1h"}}
# | limit {{.limit | default 100}}
```

### Waiting for Query Results

```bash
# Wait for test data to arrive (common in CI/CD)
dtctl wait query "fetch spans | filter test_id == 'integration-test-123'" \
  --for=count=1 \
  --timeout 5m

# Wait for any error logs in the last 5 minutes
dtctl wait query "fetch logs | filter status == 'ERROR' | filter timestamp > now() - 5m" \
  --for=any \
  --timeout 2m

# Wait for at least 10 metrics records
dtctl wait query "fetch metrics | filter metric.key == 'custom.test.metric'" \
  --for=count-gte=10 \
  --timeout 1m

# Wait with template variables
dtctl wait query -f wait-for-span.dql \
  --set test_id=my-test-456 \
  --set span_name="http.server.request" \
  --for=count=1 \
  --timeout 5m

# Custom backoff for fast CI/CD pipelines
dtctl wait query "fetch spans | filter test_id == 'ci-build-789'" \
  --for=any \
  --timeout 10m \
  --min-interval 500ms \
  --max-interval 15s \
  --backoff-multiplier 1.5

# Conservative retry strategy (lower load on system)
dtctl wait query -f query.dql \
  --for=any \
  --timeout 30m \
  --min-interval 10s \
  --max-interval 2m

# Wait and capture results as JSON
dtctl wait query "fetch spans | filter test_id == 'test-xyz'" \
  --for=count=1 \
  --timeout 5m \
  -o json > span-data.json

# Wait with initial delay (allow ingestion pipeline time to process)
dtctl wait query "fetch logs | filter test_id == 'load-test'" \
  --for=count-gte=100 \
  --timeout 10m \
  --initial-delay 30s

# Limit retry attempts (prevent infinite loops)
dtctl wait query "fetch logs | filter test_id == 'flaky-test'" \
  --for=any \
  --timeout 10m \
  --max-attempts 20

# Use in shell scripts with exit codes
if dtctl wait query "..." --for=count=1 --timeout 2m --quiet; then
  echo "Data arrived successfully"
  # Continue with test assertions
else
  echo "Timeout waiting for data" >&2
  exit 1
fi

# Real-world example: Capture trace ID from HTTP request and wait for trace data
TRACE_ID=$(curl -s -A "Mozilla/5.0" https://example.com/your-app \
  -D - -o /dev/null | grep "dtTrId" | sed -E 's/.*dtTrId;desc="([^"]+)".*/\1/')
echo "Trace ID: $TRACE_ID"

# Wait for the trace to be ingested and queryable
dtctl wait query "fetch spans | filter trace.id == \"$TRACE_ID\"" \
  --for=any \
  --timeout 3m \
  -o json | jq '.records[] | {name: .span.name, duration: .duration}'

# Use in automated tests
test_endpoint() {
  local url=$1

  # Make request and capture trace ID
  local trace_id=$(curl -s -A "Mozilla/5.0" "$url" \
    -D - -o /dev/null | grep "dtTrId" | sed -E 's/.*dtTrId;desc="([^"]+)".*/\1/')

  echo "Testing trace: $trace_id"

  # Wait for trace data with 2 minute timeout
  if dtctl wait query "fetch spans | filter trace.id == \"$trace_id\"" \
    --for=any --timeout 2m -o json > /tmp/trace.json; then

    # Run assertions on the trace
    local error_count=$(jq '[.records[] | select(.status == "ERROR")] | length' /tmp/trace.json)
    if [ "$error_count" -gt 0 ]; then
      echo "❌ Found $error_count errors in trace"
      return 1
    fi

    echo "✅ Trace validated successfully"
    return 0
  else
    echo "❌ Timeout waiting for trace data"
    return 1
  fi
}

test_endpoint "https://example.com/api/checkout"
```

### Apply Operations

```bash
# Apply workflow (create or update)
dtctl apply -f workflow.yaml

# Apply with template variable substitution
dtctl apply -f workflow.yaml --set environment=production --set owner=team-a

# Dry run to preview changes
dtctl apply -f workflow.yaml --dry-run

# Apply dashboard or notebook
dtctl apply -f dashboard.yaml
dtctl apply -f notebook.yaml --set environment=prod

# Export resources for backup
dtctl get workflows -o yaml > workflows-backup.yaml
dtctl get dashboards -o json > dashboards-backup.json
```

### Pipeline Operations

```bash
# All pipeline operations (not implemented yet)
# View current pipeline config
# dtctl get pipelines

# Update pipeline
# dtctl apply -f logs-pipeline.yaml

# Validate before applying
# dtctl validate pipeline -f logs-pipeline.yaml

# Test pipeline with sample data
# dtctl ingest --pipeline logs-pipeline --file test-data.json --dry-run
```

### IAM Operations

```bash
# List users and their groups
dtctl get users -o wide
dtctl get groups

# (not implemented yet)
# dtctl get permissions --user user@example.com
# dtctl create policy -f service-account.yaml
# dtctl get policies -o yaml > iam-audit.yaml
```

## Advanced Features

### Wait for Conditions

```bash
# Wait for query results
dtctl wait query "fetch logs" --for=any --timeout=5m
dtctl wait query "fetch logs" --for=count-gte=100

# (not implemented yet)
# dtctl wait --for=condition=complete execution <id>  # Wait for workflow/resource conditions
# dtctl wait --for=condition=evaluated slo <id>
```

### Watch Mode

```bash
# (not implemented yet)
# dtctl get documents --watch
# dtctl get executions <workflow-id> --watch
# dtctl get slos --watch --interval 30s
```

### Dry Run

```bash
# Preview changes without applying
dtctl apply -f resource.yaml --dry-run
dtctl delete document <id> --dry-run
```

### Diff

```bash
# (not implemented yet)
# dtctl diff -f resource.yaml
# dtctl diff document <id> local-copy.yaml
```

### Explain Resources

```bash
# (not implemented yet)
# dtctl explain document
# dtctl explain slo
# dtctl explain workflow
```

### Shell Completion

```bash
# Generate completion script
dtctl completion bash > /etc/bash_completion.d/dtctl
dtctl completion zsh > /usr/local/share/zsh/site-functions/_dtctl

# Enable for current session
source <(dtctl completion bash)
```

## Error Handling

### Exit Codes
- `0`: Success
- `1`: General error
- `2`: Usage error (invalid flags/arguments)
- `3`: Authentication error
- `4`: Not found
- `5`: Permission denied

### Error Output
```bash
Error: document "my-doc" not found
  Run 'dtctl get documents' to list available documents
  Run 'dtctl get trash' to check if document is in trash

Exit code: 4
```

## Implementation Notes

### API Mapping
- Each resource type maps to one or more OpenAPI specs in `api-spec/`
- Resource operations should generate appropriate REST API calls
- Handle pagination automatically for list operations
- Support filtering and sorting via query parameters

### Rate Limiting
- Implement exponential backoff for rate-limited requests
- Show progress for long-running operations
- Support `--wait` flag for async operations

### Caching
- Cache settings schemas and templates locally
- Cache document metadata for fast listing
- Invalidate cache on create/update/delete
- Provide `--no-cache` flag to force refresh

### Validation
- Validate manifests against OpenAPI specs before applying
- Provide helpful error messages with suggestions
- Support `--validate=false` to skip validation

## Future Enhancements

- Interactive mode with prompts for resource creation
- Resource templates and generators
- Bulk operations (e.g., delete multiple filtered resources)
- Resource diffing and change previews
- Integration with CI/CD pipelines
- Plugin system for custom commands
- Shell integration (kubectl-like autocompletion)
- Resource usage analytics and cost estimation
