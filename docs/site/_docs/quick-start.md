---
layout: docs
title: Quick Start
---

Get up and running with dtctl in under five minutes.

## 1. Authenticate

### OAuth Login (Recommended)

Uses your Dynatrace SSO credentials -- no token management needed:

```bash
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"
```

This opens your browser for login. Tokens are stored securely and refreshed automatically.

### Token-Based Auth (CI/CD)

For headless environments, use a platform API token:

```bash
dtctl config set-context my-env \
  --environment "https://abc12345.apps.dynatrace.com" \
  --token-ref my-token

dtctl config set-credentials my-token \
  --token "dt0s16.XXXXXXXX.YYYYYYYY"
```

See [Configuration]({{ '/docs/configuration/' | relative_url }}) for details on creating tokens and managing multiple environments.

## 2. Verify Setup

```bash
dtctl doctor
```

This checks connectivity, authentication, and API permissions.

## 3. Common Commands

### List resources

```bash
dtctl get workflows
dtctl get dashboards
dtctl get slos
```

### Query with DQL

```bash
dtctl query "fetch logs | limit 10"
```

### Apply configuration from YAML

```bash
dtctl apply -f workflow.yaml
```

### Output as JSON

```bash
dtctl get dashboards -o json
```

### Execute a workflow and wait for results

```bash
dtctl exec workflow "Daily Health Check" --wait --show-results
```

### Ask Davis CoPilot

```bash
dtctl exec copilot nl2dql "error logs from last hour"
```

## What's Next?

- [Configuration]({{ '/docs/configuration/' | relative_url }}) -- multiple environments, safety levels, aliases
- [Pre-apply hooks]({{ '/docs/configuration/#pre-apply-hooks' | relative_url }}) -- validate resources with external tools before applying
- Individual resource guides for workflows, dashboards, DQL, SLOs, and more
