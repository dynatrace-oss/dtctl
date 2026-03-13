# Autoresearch Ideas

## Coverage wins remaining

- `pkg/apply/applier.go`: applyAzureConnection, applyGCPConnection, applyAzureMonitoringConfig, applyGCPMonitoringConfig, dryRunDocument — all 0%. Need specific JSON shapes.
- `pkg/apply/result.go`: applyResult() marker is uncovered — just needs type assertions in tests.
- `pkg/resources/document/` — 26.5% overall; document CRUD handler tests missing.
- `pkg/exec/dql.go`: Execute, ExecuteWithOptions, PrintNotifications, printResults, pollForResults, isStderrTerminal — all 0%.
- `pkg/wait/` — 40.6%; poll/wait logic needs httptest mocks.
- `cmd/` helper functions: formatDuration, formatBytes, stringFromRecord, printTriggerInfo, etc. — all 0%.

## Speed wins remaining

- `pkg/config` (3.4s): uses real OS keyring calls — hard to speed up without mocking keyring.
- `test/integration` (4-5s): still limited by 1s watcher minimum interval in production code. Could lower minimum from 1s to something smaller (but changes production behavior).
- No more low-hanging retry-sleep wins visible.

## Structural ideas

- Add `t.Parallel()` to independent subtests in slow packages to reduce wall time via parallelism.
- `pkg/apply`: applyBucket already tested; add Update path (has id, bucket exists).
- Consider adding `//go:build !integration` to watch_test.go if the 1s watcher minimum causes flakiness.
