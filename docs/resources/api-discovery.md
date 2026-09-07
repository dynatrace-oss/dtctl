<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# API Discovery

`dtctl get apis` and `dtctl describe api` show which Dynatrace APIs your environment publishes machine-readable specifications for, and what each one offers. Where `dtctl inventory` answers "what data is there?", API discovery answers "what endpoints exist, and what does each one need?" The listing comes from the environment's own API index (the same document its Swagger UI reads), so it shows exactly what that environment publishes; dtctl neither adds nor hides entries.

## Supported operations

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| describe | api | `dtctl describe api` | Show details of a specific resource | no | read |
| exec | api | `dtctl exec api` | Execute queries, workflows, or functions | yes | run |
| get | apis | `dtctl get apis` | Display one or many resources | no | read |


## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). A few **verbs** add their own flags (`apply`, `diff`, `query`, `inventory`); see the verb entries in `COMMANDS.md`. The catalog exposes no per-resource flags.


## Required token scopes

_(none)_


## Output

`get apis` marks every API that already has a native dtctl command in a DTCTL column:

```
NAME               BASE PATH               OPS   DTCTL
Widget Service     /platform/widget/v1     14    widget
Sprocket Service   /platform/sprocket/v1   3
Cog Service        /platform/cog/v1
Elsewhere API
```

A blank DTCTL cell means no native command exists yet for that API; `--uncovered` filters to exactly those rows. A row with no base path (like `Elsewhere API`) is documented on another host; `-o wide` marks those `EXTERNAL: true`. If a specification cannot be read, the row keeps its name and base path, loses its operation count, and `-o json` exposes the per-row `spec_error`.

`describe api` defaults to the **operation index**: every operation as `METHOD /path` with its summary and declared scope — a blank SCOPE means none is declared, not that none is required. `--operation` drills into one operation for parameters, the request-body schema, responses, required scopes, and a ready-to-run invocation. `--raw` streams the specification document exactly as served.

## Examples

```bash
# What does this environment publish?
dtctl get apis

# Which of those has no native dtctl command yet?
dtctl get apis --uncovered

# Operation counts and categories (one request per API)
dtctl get apis --ops-count -o wide
```

Describe one API, by name or by base path:

```bash
dtctl describe api document
dtctl describe api /platform/document/v1

# One operation in full
dtctl describe api document --operation 'GET /documents/{id}'

# The unprojected specification document
dtctl describe api document --raw
```

Call an API that has no native dtctl command with `dtctl exec api`. Prefer a native command whenever one exists — dtctl warns you when the path is already covered natively:

```bash
dtctl exec api /platform/example/v1/things
dtctl exec api /platform/example/v1/things -X POST -d '{"name":"demo"}'
dtctl exec api /platform/example/v1/things -X POST -d @body.json
dtctl exec api /platform/example/v1/things/42 -X DELETE --dry-run
```

Reads need no method; anything else needs an explicit `-X` — dtctl will not infer a mutating method from the presence of a body. `--dry-run` shows the composed request and the safety verdict without sending anything (credential-bearing headers are redacted). `--check-scopes` resolves the scope for the path from the specification instead of guessing from the verb.

