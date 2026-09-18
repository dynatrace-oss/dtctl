---
layout: docs
title: Grail Buckets
---

Grail buckets are storage containers in Dynatrace that hold observability data such as logs, events, and business analytics records. Each bucket defines its data table, retention period, and status.

## Listing Buckets

```bash
# List all buckets
dtctl get buckets

# Describe a specific bucket
dtctl describe bucket logs-production
```

## Creating and Applying Buckets

Define a bucket in YAML and create or update it:

```yaml
# bucket.yaml
bucketName: logs-production
displayName: Production Logs
table: logs
retentionDays: 90
status: active
```

```bash
# Create a new bucket
dtctl create bucket -f bucket.yaml

# Apply (create or update)
dtctl apply -f bucket.yaml
```

## Watch Mode

Monitor bucket status changes in real time:

```bash
dtctl get buckets --watch
```

## Deleting Buckets

```bash
# Delete with confirmation prompt
dtctl delete bucket logs-staging

# Skip confirmation
dtctl delete bucket logs-staging -y
```
