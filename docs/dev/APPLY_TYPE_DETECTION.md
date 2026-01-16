# Apply Command: Resource Type Detection

How `dtctl apply -f` detects resource types from YAML/JSON files.

## Overview

**Key Point**: Type detection happens **entirely client-side** in `pkg/apply/applier.go:108-208`. The Dynatrace API does not provide a unified apply endpoint - dtctl must route to the correct API based on resource type.

## Detection Rules (Priority Order)

dtctl uses heuristic detection based on field presence:

| Resource | Detection Fields | Example |
|----------|------------------|---------|
| **Workflow** | `tasks` + `trigger` | `tasks: [...], trigger: { schedule: {...} }` |
| **Dashboard** | `tiles` OR `content.tiles` OR `metadata` + `type: dashboard` | `content: { tiles: [...] }` |
| **Notebook** | `sections` OR `content.sections` OR `metadata` + `type: notebook` | `content: { sections: [...] }` |
| **SLO** | `criteria` + `name` + (`customSli` OR `sliReference`) | `criteria: [...], customSli: {...}` |
| **Bucket** | `bucketName` + `table` | `bucketName: "logs", table: "logs"` |
| **Settings** | `schemaId` + `scope` + `value` | `schemaId: "...", scope: "env", value: {...}` |

### Dashboard/Notebook Formats

dtctl handles three document formats:

```yaml
# Format 1: Direct content
tiles: [...]
version: "1"

# Format 2: Nested content
content:
  tiles: [...]

# Format 3: With metadata (from API)
type: dashboard
metadata:
  version: 1
content:
  tiles: [...]
```

## Why Client-Side Detection?

Each resource type requires:
- Different API endpoint (workflows, documents, SLOs, buckets, settings)
- Different create/update logic (versions, UUID handling, field requirements)
- Proper round-trip support (`dtctl get -o yaml` → edit → `apply -f`)

## Implementation

See `pkg/apply/applier.go:108-208` for the detection logic.

When adding new resources:
1. Add detection heuristic to `detectResourceType()`
2. Add handler in `Apply()` switch statement
3. Add test cases to `pkg/apply/applier_test.go`

## Limitations

- **No explicit type flag**: Users cannot override detection (potential enhancement: `--type=dashboard`)
- **Order matters**: Detection checks run sequentially; ambiguous fields may match wrong type
- **API validation only**: Invalid resource structure is caught by API, not detection

---

**Implementation**: `pkg/apply/applier.go:108-208` | **Tests**: `pkg/apply/applier_test.go`
