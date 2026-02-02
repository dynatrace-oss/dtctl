# Watch Mode Design Proposal

**Status:** Design Proposal  
**Created:** 2026-02-02  
**Author:** dtctl team

## Overview

Watch mode enables real-time monitoring of Dynatrace resources by continuously polling and displaying changes. This is similar to `kubectl get pods --watch` or `watch` command behavior.

## Goals

1. **Real-time monitoring** - Continuously display resource state changes
2. **Minimal output** - Only show changes, not full refreshes
3. **Flexible polling** - Configurable intervals with smart defaults
4. **Resource-agnostic** - Work with any `get` command
5. **Graceful exit** - Clean shutdown on Ctrl+C

## Non-Goals

- WebSocket/streaming connections (use polling for simplicity)
- Historical change tracking (use `history` command)
- Complex filtering beyond existing `get` filters

---

## User Experience

### Basic Usage

```bash
# Watch workflows
dtctl get workflows --watch

# Watch with custom interval (default: 2s)
dtctl get workflows --watch --interval 5s

# Watch specific resource
dtctl get workflow my-workflow --watch

# Watch with filters
dtctl get dashboards --mine --watch

# Live mode for DQL query results
dtctl query "fetch logs | filter status == 'ERROR'" --live
```

### Output Behavior

**Initial state:**
```
NAME              STATUS    UPDATED
error-handler     RUNNING   2m ago
data-processor    RUNNING   5m ago
backup-job        STOPPED   1h ago
```

**After change detected (kubectl-style prefixes):**
```
NAME              STATUS    UPDATED
error-handler     RUNNING   2m ago
~data-processor   FAILED    just now
backup-job        STOPPED   1h ago
```

**When resources are added or deleted:**
```
NAME              STATUS    UPDATED
error-handler     RUNNING   2m ago
+new-workflow     RUNNING   just now
~data-processor   FAILED    30s ago
-backup-job       STOPPED   1h ago
```

**Change indicators (kubectl-style):**
- `+` prefix for added resources (green)
- `~` prefix for modified resources (yellow)
- `-` prefix for deleted resources (red, shown briefly before removal)
- No prefix for unchanged resources

---

## Technical Design

### Flag Definition

```go
// Global flag available on all get/query commands
--watch              Enable watch mode (default: false)
--interval duration  Polling interval (default: 2s, min: 1s)
--watch-only         Only show changes, not initial state (default: false)
```

### Implementation Architecture

```go
// pkg/watch/watcher.go
package watch

type Watcher struct {
    interval     time.Duration
    client       *client.Client
    fetcher      ResourceFetcher
    differ       *Differ
    printer      output.Printer
    stopCh       chan struct{}
    showInitial  bool
}

type ResourceFetcher func() (interface{}, error)

type Change struct {
    Type     ChangeType  // Added, Modified, Deleted
    Resource interface{}
    Field    string      // Which field changed
    OldValue interface{}
    NewValue interface{}
}

type ChangeType string
const (
    ChangeTypeAdded    ChangeType = "ADDED"
    ChangeTypeModified ChangeType = "MODIFIED"
    ChangeTypeDeleted  ChangeType = "DELETED"
)

func NewWatcher(opts WatcherOptions) *Watcher
func (w *Watcher) Start(ctx context.Context) error
func (w *Watcher) Stop()
```

### Change Detection Algorithm

```go
// pkg/watch/differ.go
type Differ struct {
    previous map[string]interface{}
}

func (d *Differ) Detect(current []interface{}) []Change {
    changes := []Change{}
    
    // Build current state map by ID
    currentMap := make(map[string]interface{})
    for _, item := range current {
        id := extractID(item)
        currentMap[id] = item
    }
    
    // Detect additions and modifications
    for id, item := range currentMap {
        if prev, exists := d.previous[id]; !exists {
            changes = append(changes, Change{Type: ChangeTypeAdded, Resource: item})
        } else if !reflect.DeepEqual(prev, item) {
            changes = append(changes, Change{
                Type:     ChangeTypeModified,
                Resource: item,
                Field:    detectChangedField(prev, item),
            })
        }
    }
    
    // Detect deletions
    for id, item := range d.previous {
        if _, exists := currentMap[id]; !exists {
            changes = append(changes, Change{Type: ChangeTypeDeleted, Resource: item})
        }
    }
    
    d.previous = currentMap
    return changes
}
```

### Integration Points

#### 1. Get Commands

```go
// cmd/get.go
func addWatchFlags(cmd *cobra.Command) {
    cmd.Flags().Bool("watch", false, "Watch for changes")
    cmd.Flags().Duration("interval", 2*time.Second, "Polling interval")
    cmd.Flags().Bool("watch-only", false, "Only show changes")
}

func executeGetWithWatch(cmd *cobra.Command, fetcher watch.ResourceFetcher) error {
    if watchMode, _ := cmd.Flags().GetBool("watch"); watchMode {
        interval, _ := cmd.Flags().GetDuration("interval")
        watchOnly, _ := cmd.Flags().GetBool("watch-only")
        
        watcher := watch.NewWatcher(watch.WatcherOptions{
            Interval:    interval,
            Fetcher:     fetcher,
            ShowInitial: !watchOnly,
        })
        
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()
        
        // Handle Ctrl+C gracefully
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
        go func() {
            <-sigCh
            cancel()
        }()
        
        return watcher.Start(ctx)
    }
    
    // Normal get execution
    return executeGet(cmd, fetcher)
}
```

#### 2. Query Command

```go
// cmd/query.go
// Note: Query command uses --live mode instead of --watch
func executeQueryWithLive(query string, opts QueryOptions) error {
    if opts.Live {
        fetcher := func() (interface{}, error) {
            return executeQuery(query, opts)
        }
        
        watcher := watch.NewWatcher(watch.WatcherOptions{
            Interval: opts.WatchInterval,
            Fetcher:  fetcher,
        })
        
        return watcher.Start(context.Background())
    }
    
    return executeQuery(query, opts)
}
```

### Output Formatting

```go
// pkg/output/watch.go
type WatchPrinter struct {
    basePrinter Printer
    colorize    bool
}

func (p *WatchPrinter) PrintChanges(changes []watch.Change) error {
    for _, change := range changes {
        var prefix string
        var color string
        
        switch change.Type {
        case watch.ChangeTypeAdded:
            prefix = "+"
            color = colorGreen
        case watch.ChangeTypeModified:
            prefix = "~"
            color = colorYellow
        case watch.ChangeTypeDeleted:
            prefix = "-"
            color = colorRed
        default:
            prefix = " "  // No change
        }
        
        p.printWithPrefix(change.Resource, prefix, color)
    }
    return nil
}
```

---

## Error Handling

### Transient Errors

```go
// Retry on temporary failures
if err := fetcher(); err != nil {
    if isTransient(err) {
        log.Warn("Temporary error, retrying: %v", err)
        continue
    }
    return err
}
```

### Rate Limiting

```go
// Respect API rate limits
if err := fetcher(); err != nil {
    if isRateLimited(err) {
        backoff := extractRetryAfter(err)
        time.Sleep(backoff)
        continue
    }
}
```

### Connection Loss

```go
// Handle network interruptions
if err := fetcher(); err != nil {
    if isNetworkError(err) {
        log.Warn("Connection lost, retrying...")
        time.Sleep(interval * 2) // Exponential backoff
        continue
    }
}
```

---

## Performance Considerations

### Memory Management

- **Bounded history**: Only keep last state, not full history
- **Efficient diffing**: Use map-based comparison, not O(n²)
- **Lazy evaluation**: Only compute diffs when changes detected

### API Efficiency

- **Conditional requests**: Use `If-None-Match` headers where supported
- **Field filtering**: Only fetch necessary fields
- **Pagination**: Handle large result sets efficiently

### Terminal Performance

- **Incremental updates**: Only redraw changed rows
- **Buffer management**: Flush output periodically, not per change
- **Screen management**: Use terminal control codes for in-place updates

---

## Testing Strategy

### Unit Tests

```go
// pkg/watch/watcher_test.go
func TestWatcher_DetectChanges(t *testing.T) {
    tests := []struct {
        name     string
        previous []interface{}
        current  []interface{}
        expected []Change
    }{
        {
            name:     "detect addition",
            previous: []interface{}{},
            current:  []interface{}{workflow{ID: "1", Status: "RUNNING"}},
            expected: []Change{{Type: ChangeTypeAdded}},
        },
        // ... more test cases
    }
}
```

### Integration Tests

```go
// test/integration/watch_test.go
func TestWatch_Workflows(t *testing.T) {
    // Start watch in background
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    go func() {
        err := executeWatch(ctx, "workflows")
        require.NoError(t, err)
    }()
    
    // Trigger change
    time.Sleep(2 * time.Second)
    updateWorkflow("test-workflow", "STOPPED")
    
    // Verify change detected
    // ... assertions
}
```

### E2E Tests

```bash
# test/e2e/watch_test.sh
dtctl get workflows --watch --interval 1s &
WATCH_PID=$!

sleep 2
dtctl apply -f workflow-update.yaml

sleep 2
kill $WATCH_PID

# Verify output contains "CHANGED" indicator
```

---

## Examples

### Monitor Workflow Executions

```bash
# Watch for new executions
dtctl get executions --watch

# Watch specific workflow's executions
dtctl get executions --workflow error-handler --watch
```

### Monitor SLO Status

```bash
# Watch SLO evaluations
dtctl get slos --watch --interval 10s

# Live mode to monitor failures
dtctl query "fetch dt.entity.slo | filter status == 'FAILURE'" --live
```

### Monitor Dashboard Changes

```bash
# Watch for dashboard modifications
dtctl get dashboards --mine --watch

# Watch specific dashboard
dtctl get dashboard my-dashboard --watch
```

### CI/CD Integration

```bash
# Wait for workflow to complete
dtctl get execution $EXEC_ID --watch | grep -q "COMPLETED"
echo "Workflow completed!"

# Monitor deployment
dtctl query "fetch logs | filter deployment_id == '$DEPLOY_ID'" --live
```

---

## Alternatives Considered

### 1. WebSocket Streaming

**Pros:**
- Real-time updates without polling
- Lower latency
- Reduced API calls

**Cons:**
- Not all Dynatrace APIs support WebSockets
- More complex implementation
- Connection management overhead

**Decision:** Use polling for simplicity and universal compatibility

### 2. Server-Sent Events (SSE)

**Pros:**
- One-way streaming
- Simpler than WebSockets

**Cons:**
- Limited API support
- Browser-focused technology

**Decision:** Polling is more reliable

### 3. Full Screen Refresh

**Pros:**
- Simpler implementation
- No diff logic needed

**Cons:**
- Poor UX (flickering)
- Hard to track changes
- Wastes terminal space

**Decision:** Incremental updates provide better UX

---

## Migration Path

### Phase 1: Basic Implementation
- Implement core watcher with polling
- Add `--watch` flag to `get` commands
- Basic change detection (added/deleted only)

### Phase 2: Enhanced Detection
- Field-level change detection
- Color-coded output
- Performance optimizations

### Phase 3: Advanced Features
- `--watch-only` flag
- Custom change filters
- Export watch events to file

---

## Documentation Requirements

### User Documentation

- Add watch mode section to QUICK_START.md
- Update command reference with `--watch` flag
- Add watch mode examples to README.md

### Developer Documentation

- Document watcher architecture in ARCHITECTURE.md
- Add watch mode patterns to API_DESIGN.md
- Update CONTRIBUTING.md with watch testing guidelines

---

## Open Questions

1. **Should watch mode support multiple resources simultaneously?**
   - Example: `dtctl get workflows,dashboards --watch`
   - Decision: Start with single resource, add multi-resource in Phase 2

2. **Should we support watch filters?**
   - Example: `--watch-filter "status==FAILED"`
   - Decision: Use DQL query filters directly (get commands use --watch, query uses --live)

3. **Should we persist watch state across restarts?**
   - Decision: No, watch is ephemeral

4. **Maximum watch duration?**
   - Decision: No limit, user controls with Ctrl+C

---

## Success Metrics

- Watch mode works with all `get` commands
- Change detection latency < 100ms
- Memory usage < 50MB for 1000 resources
- No false positives in change detection
- Graceful handling of API errors
- User adoption in CI/CD pipelines

---

## References

- kubectl watch implementation: https://github.com/kubernetes/kubectl
- Dynatrace API rate limits: https://docs.dynatrace.com/docs/dynatrace-api/basics/rate-limiting
- Terminal control codes: https://en.wikipedia.org/wiki/ANSI_escape_code
