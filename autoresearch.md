# Autoresearch: Test Coverage + Test Suite Speed

## Objective

Simultaneously raise test coverage (currently ~59.5%) and lower total test suite
wall-clock time (currently ~80-85s). Both are measured together via a composite score.

## Metrics

- **Primary**: `score` (unitless, higher is better) = `coverage_pct * 1000 / test_time_s`
- **Secondary**: `coverage_pct` (%, higher is better), `test_time_s` (s, lower is better)

Baseline score ≈ 59.5 × 1000 / 82 ≈ **726**

## How to Run

```bash
./autoresearch.sh   # outputs METRIC score=N, METRIC coverage_pct=N, METRIC test_time_s=N
```

Checks (correctness gate — must pass before keeping):
```bash
./autoresearch.checks.sh
```

## Root Cause of Slow Tests

The `pkg/client.New()` creates a resty client with:
- `SetRetryCount(3)` + `SetRetryWaitTime(1s)`

Resource package tests call `client.New(server.URL, "test-token")` directly but
**never override the retry settings**. So every `500` error test case triggers
3 retries × ~1s each = ~3s of unnecessary waiting per test case.

| Package              | 500-cases | Wasted time | Actual time |
|----------------------|-----------|-------------|-------------|
| pkg/resources/notification | 4   | ~12s        | 22s         |
| pkg/resources/bucket       | 2   | ~6s         | 13s         |
| pkg/resources/edgeconnect  | 2   | ~6s         | 11s         |
| pkg/resources/slo          | 2   | ~6s         | 11s         |

**Fix**: Add `NewForTesting(baseURL, token string) (*Client, error)` in `pkg/client`
that sets `RetryCount(0)`. Update all resource tests to use it.

## Low-Coverage Packages (biggest wins)

| Package                        | Coverage | Lines  | Priority |
|--------------------------------|----------|--------|----------|
| `pkg/apply/applier.go`         | 21.2%    | 1543   | HIGH — core apply logic, many 0% functions |
| `pkg/exec/dql.go`              | 28.5%    | 681    | HIGH — also slow (23s), DQL execution |
| `pkg/resources/workflow/`      | 35.3%    | 251    | MED  — reference pattern |
| `pkg/resources/settings/`      | 27.4%    | 613    | MED  |
| `pkg/resources/lookup/`        | 26.9%    | 587    | MED  |
| `pkg/resources/document/`      | 26.5%    | ~400   | MED  |
| `pkg/wait/`                    | 40.6%    | ~200   | MED  |
| `cmd/` (25.5%)                 | 25.5%    | large  | LOW  — hard to unit test without refactor |

## Files in Scope

**May modify:**
- `pkg/client/client.go` — add `NewForTesting()`
- `pkg/resources/*/` — update tests to use `NewForTesting()`, add new test cases
- `pkg/apply/applier_test.go` — add tests for uncovered apply functions
- `pkg/exec/dql_test.go` — add tests, fix slow patterns
- `pkg/wait/` — add tests
- `pkg/resources/document/` — add tests
- Any `*_test.go` file

**Off limits:**
- Production logic (only add `NewForTesting` helper, no behavior changes)
- `pkg/output/testdata/golden/` — only update if output structs change
- Integration tests in `test/e2e/`
- Anything requiring new external dependencies

## Constraints

1. All tests must pass (`go test ./...` exits 0)
2. Golden file tests must not regress (run `go test ./pkg/output/ -run TestGolden`)
3. No new external dependencies in `go.mod`
4. No real credentials, customer names, or env IDs in test data (use synthetic data)
5. Test data emails: use `@example.invalid` (RFC 2606)

## Strategy

**Phase 1 — Speed wins (high score impact, low risk):**
1. Add `client.NewForTesting()` in `pkg/client/client.go`
2. Update all resource test files to use it for error-case tests
3. Expected: ~30s reduction in test time → score jumps from ~726 to ~1200+

**Phase 2 — Coverage wins (one package at a time):**
4. Write tests for `pkg/resources/workflow/` uncovered functions
5. Write tests for `pkg/resources/settings/` uncovered functions
6. Write tests for `pkg/resources/lookup/` uncovered functions
7. Write tests for `pkg/apply/` core functions (NewApplier, Apply, applyWorkflow, applySLO, applyBucket)
8. Write tests for `pkg/exec/dql.go` uncovered functions

**Phase 3 — Tail coverage:**
9. `pkg/resources/document/`, `pkg/wait/`, `cmd/` helpers

## What's Been Tried

_Updated as experiments run._

- **Baseline**: score ≈ 726, coverage 59.5%, time ~82s
