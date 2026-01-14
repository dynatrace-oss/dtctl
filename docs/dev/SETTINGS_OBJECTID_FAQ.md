# Settings ObjectID and UID - Frequently Asked Questions

## Why do some settings not have a UID?

**Short answer**: Not all Dynatrace settings objects include a UID in their objectId encoding.

**Long answer**: The objectId is a composite key that can contain:
1. Schema ID (always present)
2. Scope Type (always present)
3. Scope ID (optional)
4. UID (optional)

The structure varies based on:
- **Scope type**: Environment-scoped settings often don't have separate UIDs
- **Schema design**: Some schemas don't use UIDs
- **Historical reasons**: Legacy settings may use simpler formats

### Examples

**With UID** (application-scoped RUM setting):
```
ObjectID: vu9U3hXa3q0AAAA... (base64)
Decoded:
  - Schema ID: builtin:rum.web.name
  - Scope Type: APPLICATION
  - Scope ID: 5C9B9BB1B4546855
  - UID: e4c6742f-47f9-3b14-8348-59cbe32f7980 ✅
```

**Without UID** (environment-scoped pipeline setting):
```
ObjectID: vu9U3hXa3q0AAAA... (base64)
Decoded:
  - Schema ID: builtin:internal.open-pipeline.stage.processing
  - Scope Type: environment
  - Scope ID: (none)
  - UID: (none) ❌
```

## Can I still use settings without UIDs?

**Yes!** Settings without UIDs work perfectly. You just need to use the full objectId:

```bash
# This works for ALL settings
dtctl get settings <objectId>
dtctl delete settings <objectId>
dtctl update settings <objectId> -f file.yaml

# This only works for settings that have a UID
dtctl get settings <uid>
```

## How do I know which settings have UIDs?

List settings and check the UID column:

```bash
$ dtctl get settings --schema builtin:rum.web.name

SCHEMA_ID              UID                                   SCOPE_TYPE    SCOPE_ID
builtin:rum.web.name   e4c6742f-47f9-3b14-8348-59cbe32f7980  APPLICATION   5C9B9BB1B4546855  ✅ Has UID

$ dtctl get settings --schema builtin:internal.open-pipeline.stage.processing

SCHEMA_ID                                     UID    SCOPE_TYPE     SCOPE_ID
builtin:internal.open-pipeline.stage.processing      environment               ❌ No UID
```

If the UID column is empty, that setting doesn't have a UID.

## Why can't I search by UID for settings without UIDs?

Because the UID doesn't exist in the objectId! When you provide a UID:

```bash
dtctl get settings e1cd3543-8603-3895-bcee-34d20c700074
```

dtctl:
1. Lists all settings objects
2. Decodes each objectId
3. Searches for a matching UID
4. Returns "not found" if no match

If a setting doesn't have a UID in its objectId, it can never match your search.

## What should I use - objectId or UID?

### Use UID when:
- ✅ Working interactively
- ✅ Copying from DebugUI
- ✅ You know the setting has a UID
- ✅ Human readability matters

### Use objectId when:
- ✅ Working with any setting (always works)
- ✅ Performance matters (faster)
- ✅ Scripting/automation
- ✅ You're not sure if UID exists

### Best practice for scripts:

```bash
# Always use objectId in scripts
dtctl get settings --schema <schema> -o json | \
  jq -r '.[] | .objectId' | \
  while read objectId; do
    dtctl delete settings "$objectId" -y
  done
```

## How do I get the objectId for a setting?

Use `-o wide` or `-o json`:

```bash
# Wide output shows objectId
dtctl get settings --schema <schema> -o wide

# JSON output includes objectId
dtctl get settings --schema <schema> -o json | jq -r '.[].objectId'
```

## Does DebugUI always show UIDs?

DebugUI decodes objectIds the same way dtctl does. If a setting doesn't have a UID in its objectId:
- DebugUI shows just the objectId or other fields
- dtctl shows an empty UID column
- Both work correctly

## Summary

| Aspect | Settings WITH UID | Settings WITHOUT UID |
|--------|------------------|---------------------|
| **Can use objectId?** | ✅ Yes | ✅ Yes |
| **Can use UID?** | ✅ Yes | ❌ No (UID doesn't exist) |
| **Shown in table?** | ✅ UID column populated | ⚠️ UID column empty |
| **Works with commands?** | ✅ All commands | ✅ All commands (use objectId) |
| **Common scope types** | APPLICATION, HOST | environment, tenant |

**Key takeaway**: The objectId always works. The UID is a convenience feature when available.

## Related Documentation

- [SETTINGS_OBJECTID_DECODING.md](./SETTINGS_OBJECTID_DECODING.md) - Technical details of objectId format
- [SETTINGS_UID_RESOLUTION.md](./SETTINGS_UID_RESOLUTION.md) - Using UIDs vs objectIds
