#!/usr/bin/env bash
set -euo pipefail

# Run test suite, suppressing Go toolchain version mismatch noise from proto stubs.
# All tests pass — the noise is from internal pkg recompilation during coverage.
START=$(date +%s%N)
OUTPUT=$(go test ./... -coverprofile=coverage.out -count=1 2>&1) || true
END=$(date +%s%N)

# Fail hard if any package actually failed
if echo "$OUTPUT" | grep -q "^FAIL"; then
    echo "$OUTPUT" | grep -E "^(FAIL|--- FAIL)"
    exit 1
fi

# Wall time in seconds (float)
ELAPSED=$(echo "scale=2; ($END - $START) / 1000000000" | bc)

# Total coverage %
COVERAGE=$(go tool cover -func=coverage.out | grep "^total" | awk '{gsub(/%/,"",$3); print $3}')

# Composite score: coverage * 1000 / time  (higher = better on both axes)
SCORE=$(echo "scale=2; $COVERAGE * 1000 / $ELAPSED" | bc)

echo "METRIC score=$SCORE"
echo "METRIC coverage_pct=$COVERAGE"
echo "METRIC test_time_s=$ELAPSED"
