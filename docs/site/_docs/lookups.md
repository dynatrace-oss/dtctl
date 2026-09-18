---
layout: docs
title: Lookup Tables
---

Lookup tables let you enrich DQL query results by mapping key values to additional information. They are stored in Grail and referenced in queries using the `lookup` operator.

## Overview

A lookup table is a structured dataset (typically loaded from CSV) that maps a key field to one or more value fields. Common use cases include mapping error codes to descriptions, IP addresses to locations, or service names to owning teams.

## Listing Lookup Tables

```bash
# List all lookup tables
dtctl get lookups

# Get a specific lookup table
dtctl get lookup /lookups/production/error_codes
```

## Creating Lookup Tables

Create a lookup table from a CSV file. dtctl auto-detects the columns from the header
row and generates the DPL parse pattern for them — including empty cells, quoted cells
containing the delimiter, and column names that need quoting:

```bash
dtctl create lookup -f error_codes.csv \
  --path /lookups/production/error_codes \
  --lookup-field code

# Detected pattern, without uploading anything
dtctl create lookup -f error_codes.csv \
  --path /lookups/production/error_codes \
  --lookup-field code --dry-run
```

After the upload, the stored record count is checked against the input: dtctl warns
when rows were dropped and exits non-zero when the pattern matched nothing.

### Example CSV Format

```csv
code,description,severity,action
ERR001,Database connection timeout,critical,page-oncall
ERR002,Rate limit exceeded,warning,notify-slack
ERR003,Authentication failed,high,review-logs
ERR004,Disk space low,warning,expand-volume
```

### Custom Parse Patterns

For non-CSV formats, specify a custom parse pattern:

```bash
# Pipe-delimited file
dtctl create lookup -f data.txt \
  --path /lookups/production/service_map \
  --lookup-field service_id \
  --parse-pattern "LD*:service_id '|' LD*:name" \
  --skip-records 1

# Tab-delimited file
dtctl create lookup -f data.tsv \
  --path /lookups/production/regions \
  --lookup-field region_code \
  --parse-pattern "LD*:region_code '\t' LD*:region_name" \
  --skip-records 1
```

`LD*` matches a column that may be empty; a bare `LD` requires at least one character,
so rows with an empty cell would be dropped without an error.

## Updating Lookup Tables

Lookup tables are replaced in full. To update, delete the existing table and recreate it:

```bash
dtctl delete lookup /lookups/production/error_codes -y
dtctl create lookup -f error_codes_v2.csv \
  --path /lookups/production/error_codes \
  --lookup-field code
```

## Using Lookup Tables in DQL

Reference lookup tables in DQL queries with the `lookup` operator to enrich results:

```bash
# Enrich logs with error descriptions
dtctl query 'fetch logs
  | filter loglevel == "ERROR"
  | lookup [/lookups/production/error_codes], sourceField:error_code, lookupField:code, prefix:"err_"
  | fields timestamp, error_code, err_description, err_severity, content'
```

### Practical Examples

**Error code enrichment:**

```dql
fetch logs
  | filter loglevel == "ERROR"
  | lookup [/lookups/production/error_codes], sourceField:error_code, lookupField:code, prefix:"err_"
  | summarize count(), by: {err_description, err_severity}
```

**IP-to-location mapping:**

```dql
fetch logs
  | lookup [/lookups/production/ip_locations], sourceField:client_ip, lookupField:ip, prefix:"geo_"
  | summarize count(), by: {geo_country, geo_city}
```

**Service-to-team ownership:**

```dql
fetch events
  | lookup [/lookups/production/service_owners], sourceField:dt.entity.service, lookupField:service_id, prefix:"team_"
  | fields timestamp, dt.entity.service, team_name, team_slack_channel
```

**Country code resolution:**

```dql
fetch bizevents
  | lookup [/lookups/production/country_codes], sourceField:country, lookupField:iso_code, prefix:"country_"
  | summarize revenue = sum(amount), by: {country_name}
```

## Deleting Lookup Tables

```bash
# Delete with confirmation prompt
dtctl delete lookup /lookups/production/error_codes

# Skip confirmation
dtctl delete lookup /lookups/production/error_codes -y
```

## Path Requirements

Lookup table paths must follow these rules:

- Must start with `/lookups/`
- Allowed characters: alphanumeric, hyphens, underscores, forward slashes
- Maximum length: 500 characters

Organize paths with a clear hierarchy, for example:

```
/lookups/production/error_codes
/lookups/production/service_owners
/lookups/staging/error_codes
```

## Tips

- **Organize paths by environment** -- use `/lookups/production/`, `/lookups/staging/`, etc. to keep tables separated.
- **Use descriptive names** -- the path is the only identifier, so make it meaningful.
- **Back up before replacing** -- export existing tables with `dtctl get lookup <path> -o json` before deleting.
- **Version your CSV files** -- keep source CSV files in version control alongside your other configuration.

## Required Scopes

| Operation | Required Scope |
|-----------|---------------|
| List / Get | `storage:files:read` |
| Create | `storage:files:write` |
| Delete | `storage:files:delete` |
