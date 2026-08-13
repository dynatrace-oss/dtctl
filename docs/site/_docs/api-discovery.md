---
layout: docs
title: API Discovery
---

`dtctl get apis` and `dtctl describe api` show which Dynatrace APIs your environment publishes machine-readable specifications for, and what each one offers. Where [`dtctl inventory`]({{ '/docs/inventory/' | relative_url }}) answers *"what data is there?"*, API discovery answers *"what endpoints exist, and what does each one need?"*

The listing comes from the environment's own API index — the same document its Swagger UI reads — so it shows exactly what that environment publishes. dtctl neither adds nor hides entries.

```bash
# What does this environment publish?
dtctl get apis

# Which of those has no native dtctl command yet?
dtctl get apis --uncovered

# Operation counts and categories (one request per API)
dtctl get apis --ops-count -o wide

# Structured output
dtctl get apis -o json
```

## The DTCTL column

`dtctl get apis` marks every API that already has a native dtctl command (operation counts shown here with `--ops-count`, which is off by default because it costs one request per API):

```
NAME               BASE PATH               OPS   DTCTL
Widget Service     /platform/widget/v1     14    widget
Sprocket Service   /platform/sprocket/v1   3
Cog Service        /platform/cog/v1
Elsewhere API
```

(Names above are illustrative — what your environment lists is up to your environment.) A row with no base path at all, like `Elsewhere API`, is an entry whose specification is documented on another host; `-o wide` marks those with `EXTERNAL: true`.

A blank DTCTL cell means there is no native command for that API yet. `--uncovered` filters to exactly those rows, which turns the gap between the platform and dtctl into a concrete list — useful both for deciding what to contribute and for knowing when you have to fall back to a raw HTTP call.

If a specification cannot be read (some environments publish the index but restrict the documents), that row keeps its name and base path, loses its operation count, and dtctl prints a warning saying how many rows were affected. Use `-o json` to see the per-row `spec_error`.

## Describing one API

```bash
# The operation index for one API
dtctl describe api document

# Address it by base path instead of by name
dtctl describe api /platform/document/v1

# One operation in full
dtctl describe api document --operation 'GET /documents/{id}'

# The unprojected specification document
dtctl describe api document --raw
```

The default view is the **operation index**: every operation as `METHOD /path`, with its summary and the scope the specification declares for it. That is a complete view at a coarser grain, not a truncation — published specifications run to tens of thousands of tokens, which is unreadable for a human and unaffordable for an AI agent, so the detail is one drill-down away instead of always present.

`--operation` gives you everything you need to compose a call: parameters, the request-body schema, the responses, the required scopes, and a ready-to-run invocation.

> A blank SCOPE means the specification declares no scope for that operation — **not** that no scope is required.

`--raw` streams the document exactly as the environment served it (YAML for most APIs, JSON for the classic environment API). It is large: outside agent mode it goes to stdout so you can redirect it; in agent mode it spills to a file above the spill threshold, like a large query result.

## Calling an API without a native command

When an API has no native dtctl command, `dtctl exec api` sends a request to it directly:

```bash
dtctl exec api /platform/example/v1/things
dtctl exec api /platform/example/v1/things -X POST -d '{"name":"demo"}'
dtctl exec api /platform/example/v1/things -X POST -d @body.json
dtctl exec api /platform/example/v1/things/42 -X DELETE --dry-run
```

**Prefer a native command whenever one exists.** A native command validates input, resolves names to IDs, formats output, and cannot be pointed at the wrong endpoint; a raw call does none of that. `exec api` exists for the APIs dtctl does not wrap — and if you find yourself scripting against it, that API wants a native command instead. dtctl warns you when the path you passed is already covered natively, and names the command to use.

A few behaviors worth knowing:

- **The path is relative to your environment.** An absolute URL is refused: dtctl sends the active context's credentials, so it will not call another host.
- **Reads need no method; anything else needs an explicit `-X`.** dtctl will not infer a mutating method — and therefore a safety operation — from the presence of a body.
- **What the request may do is read from the API's specification, not from the HTTP method.** A POST may need only a read scope, or may delete data. When dtctl cannot resolve the operation it assumes the worst and gates the request as a delete, which your context's [safety level]({{ '/docs/configuration/#safety-levels' | relative_url }}) then permits or refuses. There is no flag to assert otherwise.
- **`--dry-run` shows the composed request and the safety verdict** without sending anything. Credential-bearing headers are redacted, so the output is safe to paste into a bug report.
- **JSON responses go through the normal printer**, so `-o json`/`-o yaml` and `--agent` behave as they do everywhere else. Anything else — YAML, CSV, text, a binary archive — is passed through verbatim.
- **`--check-scopes` resolves the scope for the path you passed** by reading the specification, instead of guessing from the verb.

## Required scopes

Discovery itself needs no special scope beyond what your context already uses to reach the environment; the index and the specification documents are served to any authenticated caller the environment allows. `exec api` needs whatever scope the endpoint itself requires — which is what `dtctl describe api <name> --operation '<METHOD> <path>'` tells you.

See [Token Scopes]({{ '/docs/token-scopes/' | relative_url }}) for the scopes the native commands need.
