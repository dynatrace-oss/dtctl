# Wait Query API Design

## Overview

The `wait query` feature enables polling for observability data to become available and queryable in Dynatrace. This is essential for testing scenarios where you need to verify that instrumented data (spans, logs, metrics) has arrived before proceeding.

## Use Case

When running instrumented tests, you often need to:
1. Execute a test that generates observability data
2. Wait for that data to arrive in Dynatrace
3. Verify the data is queryable before proceeding
4. Handle ingestion delays gracefully with retry logic

## Command Structure

Following kubectl patterns, this feature is implemented as a `wait` verb:

```bash
dtctl wait query <dql-query> [flags]
dtctl wait query -f <query-file> [flags]
```

## Flags

### Success Condition
- `--for <condition>`: **Required**. Defines when the wait succeeds.

**Supported Conditions:**
| Condition | Description | Example |
|-----------|-------------|---------|
| `count=N` | Exactly N records | `--for=count=1` |
| `count-gte=N` | At least N records (>=) | `--for=count-gte=5` |
| `count-gt=N` | More than N records (>) | `--for=count-gt=0` |
| `count-lte=N` | At most N records (<=) | `--for=count-lte=10` |
| `count-lt=N` | Fewer than N records (<) | `--for=count-lt=100` |
| `any` | Any records returned (count > 0) | `--for=any` |
| `none` | No records returned (count == 0) | `--for=none` |

### Timing & Backoff
- `--timeout <duration>`: Maximum time to wait (default: `5m`)
- `--initial-delay <duration>`: Delay before first query attempt (default: `0s`)
- `--min-interval <duration>`: Minimum interval between retries (default: `1s`)
- `--max-interval <duration>`: Maximum interval between retries (default: `10s`)
- `--backoff-multiplier <float>`: Exponential backoff multiplier (default: `2.0`)
- `--max-attempts <int>`: Maximum retry attempts, 0 = unlimited (default: `0`)

### Query Options
All standard `query` command flags are supported:
- `--set <key=value>`: Template variable substitution
- `--max-result-records <int>`: Limit result records
- `--default-timeframe-start/end <timestamp>`: Query timeframe
- `--timezone <tz>`: Query timezone
- `--locale <locale>`: Query locale
- (See `dtctl query --help` for complete list)

### Output Options
- `-o, --output <format>`: Output format when condition is met (`json`|`yaml`|`table`)
- `--quiet, -q`: Suppress progress messages (only show final result)
- `--verbose, -v`: Show detailed progress including query execution times

## Examples

### Basic Usage

```bash
# Wait for a specific test span to arrive
dtctl wait query "fetch spans | filter test_id == 'test-abc-123'" \
  --for=count=1 \
  --timeout 5m

# Wait for any error logs in the last 5 minutes
dtctl wait query "fetch logs | filter status == 'ERROR' | filter timestamp > now() - 5m" \
  --for=any \
  --timeout 2m

# Wait for at least 10 metrics records
dtctl wait query "fetch metrics | filter metric.key == 'custom.test.metric'" \
  --for=count-gte=10 \
  --timeout 1m
```

### With Template Variables

```bash
# Query file: wait-for-span.dql
# fetch spans
# | filter test_id == "{{.test_id}}"
# | filter span.name == "{{.span_name | default "root"}}"

dtctl wait query -f wait-for-span.dql \
  --set test_id=my-test-123 \
  --set span_name="http.server.request" \
  --for=count=1 \
  --timeout 5m
```

### Custom Backoff Strategy

```bash
# Fast initial retries, slower later (good for CI/CD)
dtctl wait query "fetch spans | filter test_id == 'ci-build-456'" \
  --for=count-gte=1 \
  --timeout 10m \
  --min-interval 500ms \
  --max-interval 15s \
  --backoff-multiplier 1.5

# Conservative retry strategy (lower load)
dtctl wait query -f query.dql \
  --for=any \
  --timeout 30m \
  --min-interval 10s \
  --max-interval 2m \
  --backoff-multiplier 2.0
```

### Return Results on Success

```bash
# Get the span data as JSON when it arrives
dtctl wait query "fetch spans | filter test_id == 'test-789'" \
  --for=count=1 \
  --timeout 5m \
  -o json > span-data.json

# Display results as table
dtctl wait query "fetch logs | filter test_id == 'test-789'" \
  --for=count-gte=1 \
  --timeout 3m \
  -o table
```

### With Initial Delay

```bash
# Wait 30 seconds before first attempt (allow ingestion pipeline time)
dtctl wait query "fetch spans | filter test_id == 'test-xyz'" \
  --for=count=1 \
  --timeout 5m \
  --initial-delay 30s
```

### Limit Attempts

```bash
# Try maximum 10 times before giving up
dtctl wait query "fetch logs | filter test_id == 'flaky-test'" \
  --for=any \
  --timeout 10m \
  --max-attempts 10
```

### Real-World Integration Testing

```bash
# Capture trace ID from HTTP request and wait for trace to be ingested
TRACE_ID=$(curl -s -A "Mozilla/5.0" https://example.com/your-app \
  -D - -o /dev/null | grep "dtTrId" | sed -E 's/.*dtTrId;desc="([^"]+)".*/\1/')

echo "Trace ID: $TRACE_ID"

# Wait for the trace spans to arrive in Dynatrace
dtctl wait query "fetch spans | filter trace.id == \"$TRACE_ID\"" \
  --for=any \
  --timeout 3m \
  -o json

# Use in CI/CD pipeline to validate deployments
#!/bin/bash
test_endpoint() {
  local url=$1

  echo "Testing endpoint: $url"

  # Execute request and capture trace ID
  local trace_id=$(curl -s -A "Mozilla/5.0" "$url" \
    -D - -o /dev/null | grep "dtTrId" | sed -E 's/.*dtTrId;desc="([^"]+)".*/\1/')

  if [ -z "$trace_id" ]; then
    echo "Failed to capture trace ID"
    return 1
  fi

  echo "Captured trace: $trace_id"

  # Wait for trace data to be queryable
  if dtctl wait query "fetch spans | filter trace.id == \"$trace_id\"" \
    --for=any --timeout 2m -o json > /tmp/trace.json; then

    # Validate no errors in trace
    error_count=$(jq '[.records[] | select(.status == "ERROR")] | length' /tmp/trace.json)
    if [ "$error_count" -gt 0 ]; then
      echo "❌ Found $error_count errors in trace"
      return 1
    fi

    echo "✅ Trace validated successfully"
    return 0
  else
    echo "❌ Timeout waiting for trace data"
    return 1
  fi
}

# Run tests
test_endpoint "https://example.com/api/checkout" || exit 1
test_endpoint "https://example.com/api/payment" || exit 1
echo "All tests passed!"
```

## Behavior

### Execution Flow

```
1. Parse condition and validate query
2. Wait for initial-delay (if specified)
3. Execute query
4. Evaluate condition against result count
5. If condition met:
   - Print success message to stderr
   - Print results to stdout (if -o specified)
   - Exit with code 0
6. If condition not met:
   - Calculate next retry interval (exponential backoff)
   - Check timeout and max-attempts limits
   - Wait for interval
   - Go to step 3
7. If timeout or max-attempts exceeded:
   - Print failure message to stderr
   - Exit with code 1
```

### Exponential Backoff Algorithm

```go
interval = min(min_interval * (backoff_multiplier ^ attempt), max_interval)
```

**Example with defaults** (min=1s, max=10s, multiplier=2.0):
- Attempt 1: 1s
- Attempt 2: 2s
- Attempt 3: 4s
- Attempt 4: 8s
- Attempt 5: 10s (capped at max-interval)
- Attempt 6+: 10s (capped at max-interval)

### Progress Output

**Default mode** (progress to stderr):
```
Waiting for condition: count=1
Attempt 1/∞: 0 records found, retrying in 1s...
Attempt 2/∞: 0 records found, retrying in 2s...
Attempt 3/∞: 0 records found, retrying in 4s...
Attempt 4/∞: 1 record found, condition met!
Elapsed: 7.2s
```

**Quiet mode** (`--quiet`):
```
(no output unless -o is specified, or error occurs)
```

**Verbose mode** (`--verbose`):
```
Waiting for condition: count=1
Query: fetch spans | filter test_id == 'test-123'
Timeout: 5m0s, Max attempts: unlimited

Attempt 1/∞ at 2026-01-10T10:00:00Z
  Query executed in 234ms
  Result: 0 records
  Condition not met, retrying in 1s...

Attempt 2/∞ at 2026-01-10T10:00:01Z
  Query executed in 189ms
  Result: 0 records
  Condition not met, retrying in 2s...

Attempt 3/∞ at 2026-01-10T10:00:03Z
  Query executed in 201ms
  Result: 1 record
  Condition met!

Success! Condition 'count=1' satisfied after 3 attempts
Elapsed: 3.7s
```

### Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success - condition met |
| `1` | Timeout reached |
| `2` | Max attempts exceeded |
| `3` | Query execution error |
| `4` | Invalid condition syntax |
| `5` | Invalid arguments |

## Implementation Considerations

### Package Structure

```
pkg/wait/
  ├── condition.go       # Condition parsing and evaluation
  ├── condition_test.go
  ├── backoff.go         # Exponential backoff logic
  ├── backoff_test.go
  ├── query_waiter.go    # Main polling logic
  └── query_waiter_test.go

cmd/
  └── wait.go            # CLI command definition
```

### Core Types

```go
// Condition represents a success condition
type Condition struct {
    Type     ConditionType
    Operator Operator
    Value    int64
}

type ConditionType string
const (
    ConditionTypeCount ConditionType = "count"
    ConditionTypeAny   ConditionType = "any"
    ConditionTypeNone  ConditionType = "none"
)

type Operator string
const (
    OpEqual        Operator = "=="
    OpGreaterEqual Operator = ">="
    OpGreater      Operator = ">"
    OpLessEqual    Operator = "<="
    OpLess         Operator = "<"
)

// BackoffConfig configures retry backoff
type BackoffConfig struct {
    MinInterval        time.Duration
    MaxInterval        time.Duration
    Multiplier         float64
    InitialDelay       time.Duration
}

// WaitConfig configures the wait operation
type WaitConfig struct {
    Query              string
    Condition          Condition
    Timeout            time.Duration
    MaxAttempts        int
    Backoff            BackoffConfig
    QueryOptions       exec.DQLExecuteOptions
    OutputFormat       string
    Quiet              bool
    Verbose            bool
}

// QueryWaiter polls a query until a condition is met
type QueryWaiter struct {
    executor *exec.DQLExecutor
    config   WaitConfig
}

// Result contains the wait operation result
type Result struct {
    Success      bool
    Attempts     int
    Elapsed      time.Duration
    RecordCount  int64
    Records      []map[string]interface{}
    FailureReason string
}
```

### Key Methods

```go
// NewQueryWaiter creates a new query waiter
func NewQueryWaiter(executor *exec.DQLExecutor, config WaitConfig) *QueryWaiter

// Wait executes the wait operation
func (w *QueryWaiter) Wait(ctx context.Context) (*Result, error)

// ParseCondition parses a condition string
func ParseCondition(s string) (Condition, error)

// Evaluate evaluates a condition against a record count
func (c Condition) Evaluate(recordCount int64) bool

// CalculateNextInterval calculates the next retry interval
func CalculateNextInterval(attempt int, config BackoffConfig) time.Duration
```

### Error Handling

- Query execution errors should be retried (transient failures)
- Malformed queries should fail immediately (permanent errors)
- Timeout context should be respected throughout
- Network errors should be retried with backoff

### Testing Strategy

1. **Unit tests**:
   - Condition parsing and evaluation
   - Backoff calculation
   - Edge cases (zero records, negative values, etc.)

2. **Integration tests**:
   - Mock DQL executor with predictable responses
   - Test timeout behavior
   - Test max-attempts behavior
   - Test various condition types

3. **E2E tests**:
   - Real Dynatrace environment
   - Insert test data and wait for it
   - Verify actual ingestion delay handling

## Future Enhancements

### Complex Conditions (v2)
```bash
# Support for field-based conditions
--for=jsonpath='{.records[0].duration}'>1000
--for=jsonpath='{.records[*].status}'=ERROR

# Multiple conditions (all must match)
--for=count-gt=0 --for=jsonpath='{.records[0].test_id}'=abc
```

### Callback Hooks (v2)
```bash
# Execute command on success
--on-success "echo 'Data arrived!' | notify-send"

# Execute command on each attempt
--on-attempt "echo 'Attempt {{.attempt}}: {{.count}} records'"
```

### Watch Mode (v2)
```bash
# Continuously watch and report when condition becomes true/false
dtctl wait query "..." --for=count-gt=0 --watch
```

### JQ-style Filtering (v2)
```bash
# Use jq-like syntax for complex conditions
--for ".records | length > 0 and .[0].duration > 1000"
```

## Related Commands

- `dtctl query`: Execute DQL queries
- `dtctl query --live`: Live updating query results (different use case - continuous refresh)
- Future `dtctl wait` for other resources (e.g., `dtctl wait workflow <id> --for complete`)

## References

- **kubectl wait**: https://kubernetes.io/docs/reference/kubectl/generated/kubectl_wait/
- **Exponential backoff**: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
- **DQL Documentation**: Dynatrace Query Language reference
