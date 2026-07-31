---
layout: docs
title: "Documents (any type)"
---

Dashboards and notebooks are the two most familiar kinds of Dynatrace *document*,
but the Document Service stores documents of **any type** — launchpads and
app-specific documents (for example `acme:config`) among them. dtctl can create,
export, apply, and update documents of any type, and attach classification
**labels** to them.

If you only work with dashboards and notebooks, see
[Dashboards & Notebooks]({{ '/docs/dashboards/' | relative_url }}) — it covers the
same commands with dashboard/notebook-specific detail. This page focuses on
**custom document types** and **labels**, which apply to every document type.

> **Token scope:** creating, updating, or applying documents requires
> `document:documents:write`. See
> [Token Scopes]({{ '/docs/token-scopes/' | relative_url }}).

## Document types

Every document carries a `type`. `dashboard` and `notebook` are built in; anything
else (`launchpad`, `acme:config`, …) is a custom type. dtctl treats a payload with
a non-empty `type` field as a document, so the export → edit → re-import round-trip
works for any type.

```bash
# List documents of a custom type with a raw Document API filter
dtctl get documents --filter "type == 'launchpad'"
```

## Creating a document

The type comes from `--type` or a `type` field in the file:

```bash
# Create a launchpad document
dtctl create document -f launchpad.json --type launchpad

# Create from a payload that already contains a "type" field
dtctl create document -f my-app-config.yaml

# Create with a custom ID and template variables
dtctl create document -f config.yaml --type acme:config --id acme-config --set env=prod
```

`create` always creates — it fails if the ID already exists. Use `apply` for
create-or-update, or `update document` for update-only.

## Round-trip: export, edit, re-import

Export a document, edit it locally, and re-apply. **Use `-o yaml`, not `-o json`:**
JSON output is wrapped in a result envelope that `apply`/`update` cannot read back,
whereas YAML is emitted as the plain document.

```bash
# Export (includes type, id, labels, and content)
dtctl get document acme-config -o yaml > doc.yaml

# Edit doc.yaml...

# Re-import — type and id are read from the file
dtctl apply -f doc.yaml            # create-or-update
dtctl update document -f doc.yaml  # update-only (fails if it doesn't exist)
```

### apply vs. update document

Both go through the same applier, so dry-run, `--show-diff`, template rendering
(`--set`), safety checks, and labels behave identically. They differ only in intent:

| | `apply -f` | `update document -f` |
|---|---|---|
| Target doesn't exist | creates it | **fails** (no accidental create on a typo'd ID) |
| Type resolution | payload `type`, or `--type` | `--type`, or payload `type` |
| ID resolution | payload `id`, or `--id` | `--id`, or payload `id` |

```bash
# Force a type for a file that carries only raw content
dtctl apply -f content.json --type acme:config --id acme-config
dtctl update document -f content.json --type acme:config --id acme-config

# Preview and diff before writing
dtctl update document -f doc.yaml --dry-run
dtctl update document -f doc.yaml --show-diff
```

`--type` cannot be combined with array (bulk) input.

## Labels

Labels are classification strings stored in a document's metadata. Attach them on
`create`, `apply`, or `update document` with a repeatable `--label` flag, or carry
them in the payload under a `labels` array (as produced by `get document -o yaml`).

```bash
# Set labels at creation
dtctl create document -f config.yaml --type acme:config --label team-a --label env:prod

# Replace labels on an existing document
dtctl update document -f doc.yaml --label team-a --label env:prod

# Labels in an exported file round-trip through apply untouched
dtctl get document acme-config -o yaml > doc.yaml
dtctl apply -f doc.yaml
```

Behavior to know:

- **Replace, not merge.** Passing `--label` replaces the document's entire label
  set. Omitting `--label` leaves existing labels unchanged (an exported file's
  `labels` array is preserved on a round-trip apply).
- **Labels cannot be cleared**, only replaced — the Document Service offers no
  clear-labels affordance.
- **Create sets labels via a follow-up update.** The create API cannot set labels
  directly, so dtctl issues a create and then a label update. This is not atomic:
  if the label step fails, the document already exists and the error names it.

Query labels back with the read side:

```bash
dtctl get documents --add-fields labels
dtctl get document acme-config -o yaml   # labels appear at the top level
```
