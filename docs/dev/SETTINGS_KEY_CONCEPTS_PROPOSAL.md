# Settings API: Key-Concepts Alignment Proposal

**Status:** Draft / RFC
**Date:** 2026-05-05
**Scope:** `pkg/resources/settings/`, `pkg/apply/apply_settings.go`, `cmd/{create,edit,describe,get}_settings.go`
**Reference:** [Dynatrace Settings API — Key concepts](https://docs.dynatrace.com/docs/dynatrace-api/environment-api/settings/key-concepts)

## Summary

A review of dtctl's Settings 2.0 implementation against the documented key concepts surfaced one correctness bug (broken optimistic locking), two missing capabilities that materially impact day-to-day workflows (`externalId` upsert, `adminAccess`), and several smaller gaps. This document proposes a remediation plan, ordered by impact and risk.

## Conformance matrix

| Key concept | Status | Where |
|---|---|---|
| `schemaId` | ✅ followed | `settings.go` throughout |
| `objectId` as primary identifier | ✅ followed | `settings.go:39–54` |
| `scope` (required on create) | ✅ followed | `apply_settings.go:43`, scope-string parsed in `populateDisplayFields` |
| `validateOnly` | ✅ followed | `ValidateCreate`, `ValidateUpdate` |
| `fields` (sparse fieldsets) | ✅ followed | `Get`, first page of `ListObjects` |
| Pagination quirks (Settings-API-specific) | ✅ followed | `settings.go:167–230` per `AGENTS.md` rule |
| `multiObject` / `ordered` schema flags | ✅ surfaced | `Schema` struct |
| **`updateToken` (optimistic locking)** | ❌ **broken** | `If-Match: <schemaVersion>` used instead — see Proposal 1 |
| **`externalId`** | ⚠️ partial | Field exists; no upsert path through `apply` — see Proposal 2 |
| **`adminAccess`** | ❌ missing | No flag — see Proposal 3 |
| `insertAfter` / `insertBefore` | ❌ missing | Ordered schemas can only append — Proposal 4 |
| Effective-values view | ❌ missing | No way to see inherited config — Proposal 5 |
| `maxObjects` on schema | ❌ not surfaced | Add to `Schema` for `describe schema` — Proposal 6 |
| `forceSecretResubmission` | ❌ not exposed | Edge case — Proposal 7 |
| Batch-write partial failures | ⚠️ single-item only | `Create` always wraps `[obj]`; not a bug today — Proposal 8 |

## Proposals

Each proposal is sized to be its own PR (and, where useful, its own follow-up TDD plan). Files and code locations are given as anchors; full task decomposition is deferred to per-feature plans.

---

### Proposal 1 — Fix `updateToken` (correctness, P0)

**Problem.** `pkg/resources/settings/settings.go:353,390,423` send `If-Match: <SchemaVersion>` for update/delete. `schemaVersion` is the version of the schema (e.g. `"1.50"`), shared across every object on that schema — it is not a per-object ETag. The documented optimistic-lock field is `updateToken`, returned in `GET` responses and echoed back in the `PUT` body. Today, two parallel `dtctl edit` runs against the same object both succeed because every object on schema v1.50 satisfies `If-Match: 1.50`. Concurrency control is effectively off.

**Proposal.**

1. Add `UpdateToken string \`json:"updateToken,omitempty"\`` to:
   - `SettingsObject` (response)
   - The PUT request body builder in `Update` / `ValidateUpdate`
2. Include `updateToken` in the `fields` selectors used by `Get` and `ListObjects`.
3. Replace the `If-Match` header in `Update`, `ValidateUpdate`, and `Delete` with the body field (PUT) / query parameter (DELETE). Confirm exact wire format against the live API while implementing — the docs describe `updateToken` as a body/query field, not a header.
4. Update `handler_test.go` to assert the request carries `updateToken` and to reject stale tokens with HTTP 409, matching real API behaviour.

**Files.** `pkg/resources/settings/settings.go`, `pkg/resources/settings/handler_test.go`, `pkg/resources/settings/settings_test.go`.

**Backward compatibility.** External — none (the field is server-supplied and round-tripped). Internal — `SettingsObject` gains a field; YAML/JSON output of `dtctl get settings` will start including `updateToken`. Acceptable; document in `CHANGELOG.md`.

**Verification.** Manual concurrent-edit test against a development tenant: two terminals running `dtctl edit settings <id>` on the same object; second save should fail with a clear "object was modified" error. Integration test that simulates 409 from the mock server.

---

### Proposal 2 — `externalId` upsert through `apply` (P0)

**Problem.** `SettingsObjectCreate` already declares `ExternalID` (`settings.go:96`), but `apply_settings.go` never reads it from the manifest, and `cmd/create_settings.go` exposes no flag. The whole point of `externalId` is to make `apply` idempotent without storing the API-generated `objectId` in the manifest. Without it, re-applying a manifest that lacks `objectId` creates a duplicate object.

**Proposal.**

1. Teach `apply_settings.go` to extract `externalId` from the YAML/JSON.
2. When neither `objectId` nor an existing object via `externalId` is found, create with `externalId` set; the API will reject collisions (HTTP 409) which we treat as "found, then update".
3. Add a lookup helper: `Handler.GetByExternalID(externalID, schemaID string)` using `GET /settings/objects?externalIds=…&schemaIds=…`.
4. Surface `--external-id` on `dtctl create settings` for one-off creation.
5. `apply` decision tree:
   - manifest has `objectId` → existing logic
   - manifest has `externalId` only → look up; create if absent, update if present
   - neither → create (today's behaviour)

**Files.** `pkg/resources/settings/settings.go`, `pkg/apply/apply_settings.go`, `cmd/create_settings.go`, `pkg/apply/apply_integration_test.go`.

**Backward compatibility.** Pure additive. Existing manifests without `externalId` keep working.

**Verification.** Round-trip test: `dtctl apply` a manifest with `externalId` twice; second apply must update, not create. E2E covered in `test/e2e/`.

---

### Proposal 3 — `--admin-access` flag (P1)

**Problem.** Settings objects can carry owner-based access control (OBAC). Today there is no way for dtctl to bypass OBAC on objects it didn't create, blocking break-glass and CI flows. Documents already gained this flag in commit `bdb2288`; settings should follow the same pattern.

**Proposal.**

1. Add `--admin-access` to `dtctl get/describe/edit/delete/apply` for `settings`. Wire it as `?adminAccess=true` on every Settings API request.
2. Plumb the flag through `Handler` methods as a struct option (e.g. `RequestOptions{AdminAccess bool}`) rather than per-method booleans.
3. Document on the same page as the Documents flag; add to `--help` text.

**Files.** `pkg/resources/settings/settings.go`, `cmd/{get,describe,edit,delete,apply}_settings.go`, `cmd/flags.go` (or wherever the global flag lives), tests.

**Backward compatibility.** Purely additive flag. Default behaviour unchanged.

---

### Proposal 4 — `insertAfter` / `insertBefore` for ordered schemas (P2)

**Problem.** Some schemas are `ordered: true` (e.g. management-zone rules, request-naming rules). `Schema.Ordered` is surfaced, but `SettingsObjectCreate` has no `insertAfter`/`insertBefore` field, so users can only append. There's no CLI surface for repositioning.

**Proposal.**

1. Add `InsertAfter string \`json:"insertAfter,omitempty"\`` and `InsertBefore string \`json:"insertBefore,omitempty"\`` to `SettingsObjectCreate`.
2. Accept these fields from the manifest in `apply_settings.go`.
3. Add `--insert-after <objectId>` and `--insert-before <objectId>` to `dtctl create settings`.
4. Optionally: a `dtctl move settings <objectId> --before|--after <other>` shortcut. Defer until there's user demand.

**Files.** `pkg/resources/settings/settings.go`, `pkg/apply/apply_settings.go`, `cmd/create_settings.go`.

**Backward compatibility.** Additive.

---

### Proposal 5 — Effective-values view (P2)

**Problem.** `dtctl get settings --schema X --scope Y` only returns explicitly persisted objects. The most common settings question — "what configuration is actually active for this entity?" — requires walking the scope hierarchy plus defaults. The Settings API exposes this via `GET /settings/effectiveValues` (or schema-specific equivalents).

**Proposal.**

1. Add `Handler.GetEffectiveValues(schemaID, scope string) (...)`.
2. Surface as `dtctl describe settings --effective --schema X --scope Y` (or `--effective-only`). Output is read-only — no edit/apply path.
3. Confirm exact endpoint name and pagination behaviour against the API while implementing.

**Files.** `pkg/resources/settings/settings.go`, `cmd/describe_settings.go`.

**Backward compatibility.** Additive.

---

### Proposal 6 — Surface `maxObjects` on `Schema` (P3)

**Problem.** `maxObjects` is part of the schema definition and tells users the per-scope cap. Today users discover the cap by hitting HTTP 409.

**Proposal.** Add `MaxObjects int \`json:"maxObjects,omitempty" table:"MAX_OBJECTS,wide"\`` to `Schema`. Show in `dtctl describe schema`.

**Files.** `pkg/resources/settings/settings.go`, golden-test updates in `pkg/output/testdata/golden/`.

---

### Proposal 7 — `forceSecretResubmission` (P3)

**Problem.** When updating a settings object that contains secrets, modifying any non-secret property requires re-supplying the secret value (or setting `forceSecretResubmission=true`). Today the user can only work around this by passing the redacted secret back unchanged, which fails.

**Proposal.** Add `--force-secret-resubmission` flag to `dtctl edit/update settings` and the equivalent manifest field. Wire as `?forceSecretResubmission=true`.

**Files.** `pkg/resources/settings/settings.go`, `cmd/edit_settings.go`, `cmd/update.go`.

**Note.** Low priority; revisit if users report it.

---

### Proposal 8 — Batch-write semantics (parking lot)

**Problem.** `Create` always wraps a single object in `[…]` and treats the response as a single item. The Settings API supports multi-object batch create with per-item failures. Today `apply` calls one-by-one, so we don't benefit from batching.

**Proposal.** Defer. Revisit only if `apply` performance on large manifests becomes a problem; the API change required is non-trivial (per-item error reporting, partial-success UX).

---

## Out of scope

- Settings schema authoring (creating/modifying schemas) — not a v2 customer-facing API surface.
- Cross-environment settings copy. Already addressed by `dtctl apply`.
- Settings versioning/history. Distinct API surface; orthogonal to this proposal.

## Suggested execution order

1. **P0 — Proposal 1 (`updateToken`)** — correctness fix, small surface area.
2. **P0 — Proposal 2 (`externalId` upsert)** — eliminates a real footgun in `apply`.
3. **P1 — Proposal 3 (`--admin-access`)** — small, parallels the documents flag.
4. **P2 — Proposals 4, 5** — independent, can be parallelised.
5. **P3 — Proposals 6, 7** — bundle with whoever is in the file next.

## Open questions

1. **`updateToken` wire format.** Body field on PUT, or query param, or both? Confirm against the live API before settling Proposal 1's interface.
2. **`Schema` struct vs. raw map.** `Handler.GetSchema` returns `map[string]any`, while `ListSchemas` decodes into the typed `Schema`. Adding `MaxObjects` (Proposal 7) is an opportunity to align — but doing so mid-proposal is a distraction. Track separately.
3. **OBAC defaults.** Should `--admin-access` be implied for some operations (e.g. `delete` in CI contexts)? Default off matches documents; revisit only if friction is reported.

## References

- Dynatrace Settings API — Key concepts: https://docs.dynatrace.com/docs/dynatrace-api/environment-api/settings/key-concepts
- Pagination rules for Settings API: `AGENTS.md` § "Pagination Pattern (CRITICAL)"
- Documents `--admin-access` precedent: commit `bdb2288`
