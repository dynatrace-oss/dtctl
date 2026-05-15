# SDK Migration Guide

When should code live in `sdk/` vs `pkg/`?

## Decision criteria

| Question | If yes → `sdk/` | If no → `pkg/` |
|----------|-----------------|-----------------|
| Is it needed by at least two CLIs? | ✅ | Stay in `pkg/` |
| Is it leaf-shaped (no upward imports into CLI code)? | ✅ | Stay in `pkg/` |
| Can it work without Cobra, Viper, logrus, or OTel? | ✅ | Stay in `pkg/` |
| Does it avoid global state and side effects? | ✅ | Stay in `pkg/` |

All four conditions must be true for code to belong in `sdk/`.

## What belongs in `sdk/`

- Platform-level primitives shared across Dynatrace CLIs
- Pure functions and explicitly-constructed types
- Code with minimal, carefully chosen dependencies

## What stays in `pkg/`

- Cobra command definitions and CLI-specific wiring
- Product-specific logic (apply, exec, watch, safety, tracing)
- Code that imports heavy dependencies (OTel, charting libraries)
- Output formatters tied to dtctl's specific rendering conventions

## Moving code from `pkg/` to `sdk/`

1. **Check the criteria above.** If any answer is "no", stop.
2. **Design the public API first.** Write a `doc.go` and key type signatures before moving implementation. Use functional options for constructors.
3. **Keep the old `pkg/` path as a thin wrapper** that delegates to the SDK. This avoids breaking all call sites in one PR.
4. **Add tests in `sdk/`** that cover the functionality independently of the CLI.
5. **Verify constraints pass:** `make sdk-check-deps sdk-check-imports`
6. **Migrate callers** in the CLI to import from `sdk/` directly (can be done incrementally).
7. **Remove the wrapper** once all callers have migrated.

## Rules

- `sdk/` must never import from `pkg/` or `cmd/` (enforced by CI).
- `sdk/go.mod` must not add forbidden dependencies (enforced by `make sdk-check-deps`).
- All exported types use unexported fields + functional options.
- Logging is injected via `Logger` interface, never imported directly.
- Errors are typed — use sentinels and `errors.Is`/`errors.As`.

## Versioning

- SDK is versioned separately: tags use `sdk/vX.Y.Z` format.
- During `v0.x`, breaking changes are allowed in minor bumps.
- After `v1.0.0`, standard Go semver rules apply.
