package watch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewWatcher(t *testing.T) {
	fetcher := func() (interface{}, error) {
		return []interface{}{}, nil
	}

	opts := WatcherOptions{
		Interval:    2 * time.Second,
		Fetcher:     fetcher,
		ShowInitial: true,
	}

	watcher := NewWatcher(opts)

	if watcher == nil {
		t.Fatal("Expected watcher to be created")
	}

	if watcher.interval != 2*time.Second {
		t.Errorf("Expected interval 2s, got %v", watcher.interval)
	}

	if watcher.showInitial != true {
		t.Error("Expected showInitial to be true")
	}
}

func TestNewWatcher_MinInterval(t *testing.T) {
	fetcher := func() (interface{}, error) {
		return []interface{}{}, nil
	}

	opts := WatcherOptions{
		Interval: 500 * time.Millisecond,
		Fetcher:  fetcher,
	}

	watcher := NewWatcher(opts)

	if watcher.interval != time.Second {
		t.Errorf("Expected sub-1s interval to clamp to documented 1s minimum, got %v", watcher.interval)
	}
}

func TestWatcher_StopIsIdempotent(t *testing.T) {
	w := NewWatcher(WatcherOptions{
		Interval: time.Second,
		Fetcher:  func() (interface{}, error) { return []interface{}{}, nil },
	})

	// Multiple calls must not panic with "close of closed channel".
	w.Stop()
	w.Stop()
	w.Stop()
}

func TestWatcher_Stop(t *testing.T) {
	fetcher := func() (interface{}, error) {
		return []interface{}{}, nil
	}

	opts := WatcherOptions{
		Interval: 2 * time.Second,
		Fetcher:  fetcher,
	}

	watcher := NewWatcher(opts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		watcher.Stop()
	}()

	err := watcher.Start(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestWatcher_ContextCancellation(t *testing.T) {
	fetcher := func() (interface{}, error) {
		return []interface{}{}, nil
	}

	opts := WatcherOptions{
		Interval: 2 * time.Second,
		Fetcher:  fetcher,
	}

	watcher := NewWatcher(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := watcher.Start(ctx)
	if err != nil {
		t.Errorf("Expected no error on context cancellation, got %v", err)
	}
}

// recordingPrinter captures Print/PrintList/PrintChanges calls so tests can
// assert on what watch mode actually emits.
type recordingPrinter struct {
	listCalls   int
	printCalls  int
	changeCalls int
	lastChanges []Change
}

func (p *recordingPrinter) Print(_ interface{}) error     { p.printCalls++; return nil }
func (p *recordingPrinter) PrintList(_ interface{}) error { p.listCalls++; return nil }
func (p *recordingPrinter) PrintChanges(c []Change) error {
	p.changeCalls++
	p.lastChanges = append([]Change(nil), c...)
	return nil
}

func TestWatcher_WatchOnly_DoesNotEmitInitialState(t *testing.T) {
	// Reproduces the bug where --watch-only (ShowInitial=false) caused every
	// existing resource to be reported as ChangeTypeAdded on the first poll
	// because the differ's baseline was never seeded.
	resources := []interface{}{
		map[string]interface{}{"id": "a", "v": 1},
		map[string]interface{}{"id": "b", "v": 1},
	}
	calls := 0
	fetcher := func() (interface{}, error) {
		calls++
		return resources, nil
	}

	printer := &recordingPrinter{}
	w := NewWatcher(WatcherOptions{
		Interval:    time.Second,
		Fetcher:     fetcher,
		Printer:     printer,
		ShowInitial: false,
	})

	if err := w.poll(context.Background(), true); err != nil {
		t.Fatalf("initial poll: %v", err)
	}

	if printer.listCalls != 0 {
		t.Errorf("--watch-only must suppress initial PrintList; got %d calls", printer.listCalls)
	}
	if printer.changeCalls != 0 {
		t.Errorf("--watch-only must not invoke PrintChanges on first poll; got %d", printer.changeCalls)
	}
	if printer.printCalls != 0 {
		t.Errorf("--watch-only must not emit individual prints on first poll; got %d", printer.printCalls)
	}

	// A subsequent poll with the same data must report no actual changes -
	// proving the differ was seeded silently on the initial poll. Pre-fix,
	// it would report two ChangeTypeAdded entries here.
	if err := w.poll(context.Background(), false); err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(printer.lastChanges) != 0 {
		t.Errorf("expected no changes on identical second poll; got %d: %v", len(printer.lastChanges), printer.lastChanges)
	}
}

func TestWatcher_ShowInitial_PrintsThenSeedsBaseline(t *testing.T) {
	resources := []interface{}{map[string]interface{}{"id": "a", "v": 1}}
	fetcher := func() (interface{}, error) { return resources, nil }

	printer := &recordingPrinter{}
	w := NewWatcher(WatcherOptions{
		Interval:    time.Second,
		Fetcher:     fetcher,
		Printer:     printer,
		ShowInitial: true,
	})

	if err := w.poll(context.Background(), true); err != nil {
		t.Fatalf("initial poll: %v", err)
	}
	if printer.listCalls != 1 {
		t.Errorf("ShowInitial=true must call PrintList once; got %d", printer.listCalls)
	}

	// Identical second poll must produce no actual changes.
	if err := w.poll(context.Background(), false); err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(printer.lastChanges) != 0 {
		t.Errorf("expected no changes on identical second poll; got %v", printer.lastChanges)
	}
}

func TestWatcher_TransientError_HonoursContextDuringBackoff(t *testing.T) {
	// Verify that context cancellation is honoured during the transient-error
	// backoff sleep (i.e. sleep uses w.sleep, not time.Sleep).
	call := 0
	fetcher := func() (interface{}, error) {
		call++
		if call == 2 {
			return nil, errors.New("connection timeout")
		}
		return []interface{}{}, nil
	}

	const interval = 50 * time.Millisecond
	w := NewWatcher(WatcherOptions{
		Interval: interval,
		Fetcher:  fetcher,
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel mid-backoff: after initial poll + first ticker + a bit into backoff sleep.
	go func() {
		time.Sleep(interval + 10*time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_ = w.Start(ctx)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("watcher did not stop promptly on context cancel during transient backoff; took %v", elapsed)
	}
}

func TestExtractRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected time.Duration
	}{
		{"nil", nil, 0},
		{"no header", errors.New("rate limit exceeded"), 0},
		{"basic", errors.New("HTTP 429: Retry-After: 30"), 30 * time.Second},
		{"case insensitive", errors.New("retry-after=12"), 12 * time.Second},
		{"capped", errors.New("Retry-After: 9999"), 300 * time.Second},
		{"zero is ignored", errors.New("Retry-After: 0"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRetryAfter(tt.err)
			if got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWatcher_Sleep_HonoursContextCancel(t *testing.T) {
	w := NewWatcher(WatcherOptions{
		Interval: time.Second,
		Fetcher:  func() (interface{}, error) { return []interface{}{}, nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	ok := w.sleep(ctx, 5*time.Second)
	elapsed := time.Since(start)

	if ok {
		t.Error("sleep must return false when ctx is cancelled")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("sleep should return promptly on cancel; took %v", elapsed)
	}
}

func TestNormalizeToSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: 0,
		},
		{
			name:     "slice of interfaces",
			input:    []interface{}{"a", "b", "c"},
			expected: 3,
		},
		{
			name: "slice of maps",
			input: []map[string]interface{}{
				{"id": "1"},
				{"id": "2"},
			},
			expected: 2,
		},
		{
			name:     "single item",
			input:    map[string]interface{}{"id": "1"},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeToSlice(tt.input)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if len(result) != tt.expected {
				t.Errorf("Expected length %d, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "temporary error",
			err:      errors.New("temporary failure"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTransient(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "429 error",
			err:      errors.New("HTTP 429 Too Many Requests"),
			expected: true,
		},
		{
			name:     "too many requests",
			err:      errors.New("too many requests"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRateLimited(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "no such host",
			err:      errors.New("no such host"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network unreachable"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
