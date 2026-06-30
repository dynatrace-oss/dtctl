# Spec: `dtctl describe analyzer` — Human- and Agent-Readable Analyzer Details

**Status**: Proposed
**Priority**: P2
**Effort**: Small (≈1 day)
**Impact**: Closes the "how do I run this analyzer?" gap between `get analyzer` and `exec analyzer`

---

## Problem

Today, discovering how to invoke a Davis analyzer is awkward:

- `dtctl get analyzers` lists analyzers (name, display name, category, type).
- `dtctl get analyzer <name>` returns the analyzer definition. In table mode it
  prints a one-row summary; in JSON/YAML it dumps the raw `AnalyzerDefinition`,
  whose `input`, `output`, and `analyzerCall` fields are `json.RawMessage` of the
  analyzer's **internal definition format** — *not* JSON Schema. There is no
  readable answer to "what inputs does this analyzer require?"
- `dtctl exec analyzer <name>` runs it — but you have to already know the input
  shape, or you discover it by trial-and-error 400s.

Meanwhile, the analyzer API exposes three endpoints that dtctl already wraps but
never surfaces. `grep` confirms `GetInputSchema`, `GetResultSchema`, and
`GetDocumentation` are called **nowhere** in `cmd/` or `test/` — they are dead
code in both `sdk/api/analyzer/analyzer.go` and `pkg/resources/analyzer/analyzer.go`:

| Endpoint | Handler method | Returns |
|---|---|---|
| `GET /analyzers/{name}/json-schema/input` | `GetInputSchema` | JSON Schema of inputs |
| `GET /analyzers/{name}/json-schema/result` | `GetResultSchema` | JSON Schema of result |
| `GET /analyzers/{name}/documentation` | `GetDocumentation` | Markdown docs |

So the data needed to answer "what does this analyzer take and return?" is one
HTTP call away and already plumbed — it's just not wired to a command.

---

## Design decision: `describe analyzer` vs. `get analyzer --schema`

There are two ways to surface this. They are not equivalent, and the choice
affects whether **agents** (the primary consumer, per the `LogPatternExtractor`
use case) benefit at all.

### The convention problem

Every existing `describe` command (`describe_slos.go`, `describe_anomalydetector.go`,
`describe.go`) follows the same shape: the rich, sectioned, cross-referenced
output is printed **only in the `outputFormat == "table"` branch**. The
JSON/YAML/agent path just calls `printer.Print(resource)` — returning the
*identical* object that `get` returns.

If `describe analyzer` follows that convention verbatim, then:

- a **human** at a terminal gets a nice schema layout, but
- an **agent** (which runs in `--agent`/JSON mode) gets exactly what
  `get analyzer` already returns — **nothing new**.

That would defeat the stated purpose. To serve agents, `describe analyzer` must
**merge the resolved input/result schemas into the structured output too**, which
is a deliberate departure from the established describe pattern.

### Options

**Option A — `dtctl describe analyzer <name>` (recommended).**
A new `describe` subcommand that fetches the definition + input schema + result
schema and renders an enriched view in *all* output modes (not table-only).
Pros: fits the verb-noun model, bundles human-readable framing (category, labels,
docs pointer) with the schema, natural place for an `exec` suggestion via
`enrichAgent`. Cons: requires intentionally breaking the "describe == get in JSON
mode" convention; 2 extra API calls.

**Option B — `dtctl get analyzer <name> --schema`.**
A flag on the existing `get` command that swaps the raw definition for the
resolved input/result schemas. Pros: smallest surface area, no convention break.
Cons: overloads `get` with a mode flag; no natural home for the docs/exec framing;
`get` is meant to return the resource as-is, and schemas are a different resource.

**Recommendation: Option A**, because the value is precisely the *bundling* —
metadata + required inputs + an actionable `exec` hint in one call — and because
agents are first-class consumers here, the structured (non-table) output must
carry the schema. This spec is written for Option A. If reviewers prefer to keep
`describe` strictly conventional, fall back to Option B.

---

## Design (Option A)

### Command

```
dtctl describe analyzer <name>            # enriched human view (table mode)
dtctl describe analyzer <name> -o json    # enriched structured view (agents)
dtctl describe analyzer <name> --doc      # append raw markdown documentation
```

Aliases: `az` (matching `get analyzers` / `exec analyzer`). Registered in
`describe.go`'s `init()` alongside the other describe subcommands.

### Data assembled

One `describe` invocation makes up to three calls:

1. `Get(name)` → `AnalyzerDefinition` (name, display name, category, type, labels).
2. `GetInputSchema(name)` → JSON Schema for inputs.
3. `GetResultSchema(name)` → JSON Schema for the result.

`--doc` adds a fourth call to `GetDocumentation(name)`.

Calls 2 and 3 are best-effort: a non-200 (e.g. an analyzer with no published
schema) degrades to a `(schema unavailable — use -o json on get, or --doc)`
marker rather than failing the command. This mirrors how
`describe_anomalydetector.go` silently skips its DQL cross-reference on error.

### Structured output (the agent contract)

In `-o json`/`-o yaml`/`--agent`, `describe analyzer` prints an enriched struct —
**this is the deliberate departure from get**:

```json
{
  "name": "dt.statistics.clustering.LogPatternExtractor",
  "displayName": "Log Pattern Extractor",
  "category": "Statistics",
  "type": "BUILT_IN",
  "description": "Extracts recurring patterns from log records...",
  "labels": ["logs", "clustering"],
  "inputSchema":  { "type": "object", "required": ["logQuery"], "properties": { ... } },
  "resultSchema": { "type": "object", "properties": { ... } }
}
```

In agent mode this is wrapped in the standard envelope and `enrichAgent` attaches
a suggestion:

```json
{
  "ok": true,
  "result": { ... },
  "context": {
    "verb": "describe",
    "resource": "analyzer",
    "suggestions": [
      "dtctl exec analyzer <name> --query <dql>  -- run this analyzer",
      "dtctl describe analyzer <name> --doc       -- full markdown docs"
    ]
  }
}
```

### Human (table) output

> **Note:** field names below (`logQuery`, `maxPatterns`, …) are *illustrative*.
> Real rendering depends on the live JSON Schema; the layout — not the fields —
> is what this spec fixes. Validate against a real schema before implementing
> (see Open Questions).

```
$ dtctl describe analyzer dt.statistics.clustering.LogPatternExtractor

Name:          dt.statistics.clustering.LogPatternExtractor
Display Name:  Log Pattern Extractor
Category:      Statistics
Type:          BUILT_IN
Description:   Extracts recurring patterns from log records using Davis AI clustering.
Labels:        logs, clustering

Input (required):
  logQuery     string    DQL query selecting the log records to analyze

Input (optional):
  maxPatterns  integer   Maximum number of patterns to return (default: 100)
  timeframe    string    Relative timeframe override (e.g. "now()-1h")

Output:
  patterns     array     Clustered log patterns with DPL expressions and counts
  status       string    Execution status

  Run it:  dtctl exec analyzer dt.statistics.clustering.LogPatternExtractor --query <dql>
  Docs:    dtctl describe analyzer dt.statistics.clustering.LogPatternExtractor --doc
```

Rendered with the existing `output.DescribeKV` / `output.DescribeSection` helpers,
identical to every other describe command.

### `--doc` output

Dumps the raw markdown from the documentation endpoint to stdout (no reformatting):

```
$ dtctl describe analyzer dt.statistics.clustering.LogPatternExtractor --doc

# Log Pattern Extractor

Clusters log lines into recurring DPL patterns using Davis AI statistical
clustering...
```

### JSON Schema flattening (scope guard)

JSON Schema permits arbitrary composition (`oneOf`, `anyOf`, `allOf`, `$ref`).
The renderer deliberately handles only the common case and does not build a
recursive schema walker:

- Read top-level `properties` + the `required` array.
- For each property show `name`, `type`, `required?`, and `description`.
- Any property whose schema uses `oneOf`/`anyOf`/`allOf`/`$ref` renders as
  `(composite — see -o json or --doc)`.
- If there is no top-level `properties` at all, print
  `(schema not introspectable — use --doc)`.

This covers built-in analyzers (the 80% case) and fails legibly for the rest.
The full schema is always available via `-o json`.

---

## Implementation Plan

No SDK work — all three endpoints already exist in the handler.

### Step 1: `cmd/describe_analyzers.go` (≈0.5 day)

1. Add `describeAnalyzerCmd` (`Use: "analyzer <name>"`, alias `az`, `ExactArgs(1)`).
2. `RunE`: `Setup()`, build handler, `Get` + `GetInputSchema` + `GetResultSchema`
   (best-effort on the two schema calls).
3. Table branch: render with `DescribeKV`/`DescribeSection` + the schema flattener.
4. Non-table branch: assemble the enriched struct and `printer.Print` it; call
   `enrichAgent(printer, "describe", "analyzer")` and set `Suggestions`.
5. `--doc` flag: when set, call `GetDocumentation` and print the markdown verbatim
   (short-circuits the normal render).
6. Register in `describe.go` `init()`; add to the describe `Long` help resource list.

### Step 2: Schema flattener helper (≈0.25 day)

1. Small helper in `cmd/describe_analyzers.go` (or `pkg/resources/analyzer/`) that
   turns a `map[string]interface{}` JSON Schema into ordered `(name, type,
   required, description)` rows, with the composite/`$ref` fallback.

### Step 3: Tests (≈0.25 day)

1. Golden tests for table + JSON output (`pkg/output/testdata/golden/describe/`),
   using a real `AnalyzerDefinition` struct plus representative schemas.
2. Unit test for the flattener: required vs optional, a composite property
   fallback, and an empty/missing `properties` schema.
3. Test that schema-call failure degrades gracefully (command still succeeds).

---

## Acceptance Criteria

- [ ] `dtctl describe analyzer <name>` prints metadata + required/optional inputs +
      outputs in table mode.
- [ ] `dtctl describe analyzer <name> -o json` includes `inputSchema` and
      `resultSchema` — i.e. returns strictly more than `get analyzer -o json`.
- [ ] Agent mode wraps the result in the envelope with an `exec analyzer` suggestion.
- [ ] `--doc` prints the raw markdown documentation.
- [ ] A missing/empty input or result schema degrades to a marker without failing.
- [ ] Composite (`oneOf`/`$ref`) properties render a legible fallback, not garbage.
- [ ] Golden tests cover table and JSON output and prevent format regressions.

---

## Open Questions

1. **Convention break** — are reviewers comfortable with `describe analyzer`
   returning more than `get analyzer` in JSON mode (Option A), or should this ship
   as `get analyzer --schema` (Option B) to preserve the describe convention?
2. **Real schema shape** — fetch a real input schema (e.g.
   `dt.statistics.clustering.LogPatternExtractor`) against the `fxz` test tenant
   and confirm the flattener assumptions before locking the table layout.
3. **Dead code** — if neither option ships, `GetInputSchema`/`GetResultSchema`/
   `GetDocumentation` should arguably be deleted as unused.

---

## References

- Existing describe patterns: `cmd/describe_slos.go`, `cmd/describe_anomalydetector.go`
- Analyzer handler (endpoints already implemented): `pkg/resources/analyzer/analyzer.go`,
  `sdk/api/analyzer/analyzer.go`
- Get/exec analyzer commands: `cmd/get_analyzers.go`, `cmd/exec_analyzers.go`
- Agent envelope + enrichment: `pkg/output/agent.go`, `enrichAgent` in `cmd/root.go`
- Describe output helpers: `pkg/output/messages.go` (`DescribeKV`, `DescribeSection`)
