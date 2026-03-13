#!/usr/bin/env bash
set -euo pipefail

# Correctness gate: all tests must pass, golden files must be consistent
# Output is suppressed on success; only errors are shown.
go test ./... -count=1 2>&1 | grep -E "^(FAIL|---\s+FAIL)" || true

# Fail if any package failed
if go test ./... -count=1 2>&1 | grep -q "^FAIL"; then
    echo "ERROR: one or more test packages failed"
    exit 1
fi
