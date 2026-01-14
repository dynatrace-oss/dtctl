# Settings UID Resolution

## Summary

dtctl now supports using either **objectId** or **UID** when working with settings objects, making it much more user-friendly.

## Quick Start

```bash
# Both formats work:
dtctl get settings vu9U3hXa3q0AAAA...                          # objectId (fast)
dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074        # UID (user-friendly)

# Works with all commands:
dtctl delete settings e1cd3543-8603-3895-bcee-34d20c700074
dtctl update settings e1cd3543-8603-3895-bcee-34d20c700074 -f settings.yaml
```

## What's the difference?

### ObjectID (Base64-encoded composite key)

**Example**: `vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQALQVBQTElDQVRJT04AEDVDOUI5QkIxQjQ1NDY4NTUAJGU0YzY3NDJmLTQ3ZjktM2IxNC04MzQ4LTU5Y2JlMzJmNzk4ML7vVN4V2t6t`

- ✅ **Fast** - Direct API call (no extra lookup)
- ❌ **Not user-friendly** - Long, cryptic string
- ✅ **Always works** - API native format

**When to use**: Scripting, automation, performance-critical operations

### UID (UUID format)

**Example**: `e4c6742f-47f9-3b14-8348-59cbe32f7980`

- ✅ **User-friendly** - Short, readable UUID
- ✅ **Matches DebugUI** - Same format you see in Dynatrace UI
- ⚠️ **Slower** - Requires listing all settings to resolve (one extra API call)

**When to use**: Interactive use, copying from DebugUI, human readability

## How it works

The Dynatrace Settings API only accepts `objectId`, not `UID`. When you provide a UID, dtctl automatically:

1. **Detects** the format (UUID pattern matching)
2. **Lists** all settings objects (with pagination)
3. **Finds** the object with matching UID
4. **Uses** its objectId for the actual API call

This happens transparently - you don't need to do anything special.

## Performance Considerations

### UID Resolution Cost

- **API calls**: 1 extra call (list operation)
- **Time**: Typically <1 second for most schemas
- **Memory**: Minimal (streaming with pagination)

### When it matters

UID resolution is slower in these cases:
- Large schemas with thousands of settings
- Batch operations on many settings
- Scripts that run frequently

**Recommendation**: Use objectId in performance-critical scripts, UID for interactive use.

## Examples

### Interactive Use (UID recommended)

```bash
# Copy UID from DebugUI and use directly
dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074

# Human-readable output
SCHEMA_ID              UID                                   SCOPE_TYPE    SCOPE_ID           SUMMARY
builtin:rum.web.name   e1cd3543-8603-3895-bcee-34d20c700074  APPLICATION   FAAEB28910ABC22   My App
```

### Scripting (objectId recommended)

```bash
#!/bin/bash
# List settings and extract objectIds for fast batch operations
dtctl get settings --schema builtin:my-schema -o json | \
  jq -r '.[] | .objectId' | \
  while read objectId; do
    dtctl delete settings "$objectId" -y
  done
```

### Mixed Approach

```bash
# List with wide output to see both UID and objectId
dtctl get settings --schema builtin:rum.web.name -o wide

# Copy the objectId (from OBJECT_ID column) for fast operations
# Or copy the UID (from UID column) for readability
```

## Default Table Output

By default, dtctl shows **decoded fields** (UID, SCOPE_TYPE, SCOPE_ID) and hides the objectId:

```bash
$ dtctl get settings --schema builtin:rum.web.name

SCHEMA_ID              UID                                   SCOPE_TYPE    SCOPE_ID           SUMMARY
builtin:rum.web.name   e4c6742f-47f9-3b14-8348-59cbe32f7980  APPLICATION   5C9B9BB1B4546855   My App
```

Use `-o wide` to see the objectId:

```bash
$ dtctl get settings --schema builtin:rum.web.name -o wide

OBJECT_ID           SCHEMA_ID              UID                          SCOPE_TYPE  SCOPE_ID          SCOPE                        VERSION
vu9U3hXa3q0AAAA...  builtin:rum.web.name   e4c6742f-47f9-3b14-8348...  APPLICATION 5C9B9BB1B4546855  APPLICATION-5C9B9BB1B45...   1.0
```

## Implementation Details

### UUID Detection

A string is treated as a UID if it matches the UUID pattern:
- Format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`
- Case insensitive
- Hyphens optional: `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` also works

### Code Location

- **Decoder**: `pkg/resources/settings/decoder.go`
- **UID Resolution**: `pkg/resources/settings/settings.go` (`Get()`, `getByUID()`)
- **UUID Detection**: `pkg/resources/settings/settings.go` (`isUUID()`)

### Testing

Run tests:
```bash
go test ./pkg/resources/settings/ -v
```

Key test files:
- `decoder_test.go` - ObjectID decoding
- `uid_resolution_test.go` - UUID detection
- `integration_test.go` - End-to-end decoding

## Troubleshooting

### Empty UID in output

Some settings objects may show an empty UID column:

```bash
$ dtctl get settings --schema builtin:internal.open-pipeline.stage.processing

SCHEMA_ID                                     UID    SCOPE_TYPE     SCOPE_ID    SUMMARY
builtin:internal.open-pipeline.stage.processing      environment               Pipeline config
```

**Why this happens**:

Not all settings objectIds contain a UID field. The objectId format varies:
- **Application-scoped settings**: Usually have all 4 fields (schemaId, scopeType, scopeId, UID)
- **Environment-scoped settings**: May only have 2-3 fields (schemaId, scopeType, maybe scopeId)
- **Legacy settings**: Older settings might use different encoding

This is **normal and expected**. Settings without UIDs can still be:
- Retrieved using their full objectId
- Displayed with `-o wide` to see the objectId
- Managed with all dtctl commands

**Workaround**: Use the objectId instead:
```bash
# Show objectId for settings without UID
dtctl get settings --schema <schema> -o wide

# Use objectId directly
dtctl get settings <objectId>
```

### UID not found

```bash
$ dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074
Error: settings object with UID "e1cd3543-8603-3895-bcee-34d20c700074" not found
```

**Possible causes**:
1. Wrong UID - check DebugUI or list settings
2. Settings object was deleted
3. Insufficient permissions to list settings
4. Setting doesn't have a UID (see "Empty UID" above)

**Solution**: Try listing settings to see available UIDs:
```bash
dtctl get settings --schema <your-schema>
```

### Slow UID resolution

If UID resolution is consistently slow:

1. **Use objectId instead** for better performance
2. **Check schema size**: `dtctl get settings --schema <schema> | wc -l`
3. **Consider caching**: Save objectIds from initial lookup

## See Also

- [SETTINGS_OBJECTID_DECODING.md](./SETTINGS_OBJECTID_DECODING.md) - Full ObjectID format specification
- [API_DESIGN.md](./API_DESIGN.md) - dtctl design principles
- Dynatrace Settings API: `/platform/classic/environment-api/v2/settings`
