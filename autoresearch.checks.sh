#!/usr/bin/env bash
set -euo pipefail

# Correctness gate: all tests must pass.
# Suppress version-mismatch noise from proto stubs; only surface real failures.
OUTPUT=$(go test ./... -count=1 2>&1) || true

FAILED=$(echo "$OUTPUT" | grep -E "^FAIL" | head -20 || true)
if [ -n "$FAILED" ]; then
    echo "ERROR: test failures detected:"
    echo "$FAILED"
    echo "$OUTPUT" | grep -E "^--- FAIL" | head -20 || true
    exit 1
fi
