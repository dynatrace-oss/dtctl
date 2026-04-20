# dqlbench — DQL cost regression harness

Compares DQL produced by dtctl (via Davis CoPilot nl2dql, and as embedded in
dashboard/notebook templates) against cost-quality expectations. v1 runs two
modes:

1. **Unit / snapshot mode** (no tenant required) — loads every fixture in
   `fixtures/`, applies the `pkg/dqlcost` linter + rewriter, and asserts
   that:
   - The fixture's expected lint rule IDs (if any) fire.
   - The rewritten query no longer trips any error-severity rule.
   - The fixture's `expect.must_contain` / `must_not_contain` strings match.

2. **Tenant mode** (`//go:build dqlbench`) — executes the fixture query via
   `DQLExecutor.ExecuteQueryWithOptions(IncludeContributions=true,
   MetadataFields=["all"])`, captures `GrailMetadata.ScannedBytes`, and
   compares against `budget.max_scanned_bytes`. Writes a markdown report to
   `reports/<ts>.md`.

## Run

```
# Snapshot mode (no tenant)
make test-dqlbench-unit

# Tenant mode (requires DTCTL_INTEGRATION_ENV + DTCTL_INTEGRATION_TOKEN)
make test-dqlbench
```

## Add a fixture

Drop a YAML file into `fixtures/`:

```yaml
id: errors-last-24h
description: "Errors by service over the last 24h"
query: |
  fetch logs | filter loglevel == "ERROR" | summarize c = count(), by:{dt.entity.service}
expect_rules: [COST001, COST008]        # these should fire on the raw query
after_rewrite_no_rules: [COST001, COST008]  # none of these should fire after Rewrite()
must_contain: ["fetch logs"]
must_not_contain: ["sort timestamp desc | filter"]
budget:
  max_scanned_bytes: 5_000_000_000
```

The harness walks `fixtures/` and runs one subtest per fixture.
