<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# Lookup Tables

Lookup tables let you enrich DQL query results by mapping key values to additional information. They are stored in Grail and referenced in queries using the `lookup` operator. A lookup table is a structured dataset, typically loaded from CSV, that maps a key field to one or more value fields; common use cases include mapping error codes to descriptions, IP addresses to locations, or service names to owning teams.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | lookup | `dtctl create lookup` | Create resources from files | yes | write |
| delete | lookup | `dtctl delete lookup` | Delete resources | yes | delete |
| describe | lookup | `dtctl describe lookup` | Show details of a specific resource | no | read |
| get | lookups | `dtctl get lookups` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

| Safety level | Scopes |
| --- | --- |
| delete | `storage:files:delete` |
| read | `storage:files:read` |
| write | `storage:files:write` |


## Output

The path is the only identifier for a lookup table (there is no separate ID); `get lookup <path>` and `describe lookup <path>` both address it that way. `-o json` on `get lookup` is the way to back up a table before replacing it, since updates are full replacements rather than merges.

## Examples

```bash
# List all lookup tables
dtctl get lookups

# Get a specific lookup table
dtctl get lookup /lookups/production/error_codes
```

Create a lookup table from a CSV file; dtctl auto-detects column types from the data:

```bash
dtctl create lookup -f error_codes.csv \
  --path /lookups/production/error_codes \
  --lookup-field code
```

Example CSV format:

```csv
code,description,severity,action
ERR001,Database connection timeout,critical,page-oncall
ERR002,Rate limit exceeded,warning,notify-slack
```

For non-CSV formats, specify a custom parse pattern:

```bash
dtctl create lookup -f data.txt \
  --path /lookups/production/service_map \
  --lookup-field service_id \
  --parse-pattern "pipe"
```

Lookup tables are replaced in full — to update, delete the existing table and recreate it:

```bash
dtctl delete lookup /lookups/production/error_codes -y
dtctl create lookup -f error_codes_v2.csv \
  --path /lookups/production/error_codes \
  --lookup-field code
```

Reference a lookup table in a DQL query with the `lookup` operator to enrich results:

```bash
dtctl query 'fetch logs
  | filter loglevel == "ERROR"
  | lookup [/lookups/production/error_codes], sourceField:error_code, lookupField:code, prefix:"err_"
  | fields timestamp, error_code, err_description, err_severity, content'
```

Delete a lookup table:

```bash
# Delete with confirmation prompt
dtctl delete lookup /lookups/production/error_codes

# Skip confirmation
dtctl delete lookup /lookups/production/error_codes -y
```

Lookup table paths must start with `/lookups/`, use only alphanumeric characters, hyphens, underscores, and forward slashes, and stay under 500 characters. Organize paths by environment (`/lookups/production/`, `/lookups/staging/`) and keep source CSV files in version control.

