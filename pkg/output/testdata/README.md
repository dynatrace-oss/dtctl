# Golden File Tests

This directory contains golden files (`.golden`) used for snapshot testing of dtctl's
output formatting. Each golden file captures the expected output of a specific printer
and resource type combination.

## How It Works

Tests in `pkg/output/golden_test.go` render test data through each output printer and
compare the result byte-for-byte against the corresponding `.golden` file. If the output
doesn't match, the test fails and prints a diff.

Golden files are committed to git. They serve as the **contract** for dtctl's output
format, ensuring that changes to output code don't silently break formatting.

## Directory Structure

```
golden/
  get/           # List output for each resource type (table, wide, json, yaml, csv, agent, watch)
  describe/      # Single-item detail output (table, json, yaml)
  query/         # DQL query results including visual formats (chart, sparkline, barchart, braille)
  errors/        # Error message output (auth, not-found, permission)
  empty/         # Empty result sets
```

## Updating Golden Files

When you intentionally change output formatting, regenerate the golden files:

```bash
# Using make
make test-update-golden

# Or directly
go test ./... -update
```

Then review the changes:

```bash
git diff pkg/output/testdata/
```

If the changes look correct, commit the updated golden files along with your code changes.

## Adding New Golden Tests

1. Add a new test case in `pkg/output/golden_test.go`
2. Run `go test ./pkg/output/ -update` to generate the golden file
3. Review the generated `.golden` file
4. Commit both the test case and golden file

## Key Design Decisions

- **No external dependencies**: Uses a custom helper in `cmd/testutil/golden.go` built on
  Go's standard `testing` and `os` packages.
- **ANSI stripping**: Visual format tests (chart, sparkline, barchart, braille) strip ANSI
  escape codes before comparison using `AssertGoldenStripped`, ensuring deterministic output
  regardless of terminal capabilities.
- **Fixed dimensions**: Visual format tests use fixed width (80) and height (20) to produce
  deterministic output independent of terminal size.
- **Colors disabled**: Tests run with `SetPlainMode(true)` to disable color output.
- **Static fixtures**: Test data uses fixed IDs, timestamps, and values to prevent false diffs.

## Golden File Helper API

The helper lives in `cmd/testutil/golden.go`:

- `AssertGolden(t, name, actual)` — compare output against `testdata/golden/<name>.golden`
- `AssertGoldenStripped(t, name, actual)` — same, but strips ANSI escape codes first
- `StripANSI(s)` — remove ANSI escape sequences from a string

Pass `-update` to `go test` to regenerate golden files instead of comparing.
