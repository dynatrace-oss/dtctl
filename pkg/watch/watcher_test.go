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

	if watcher.interval < time.Second {
		t.Errorf("Expected interval to be at least 1s, got %v", watcher.interval)
	}
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
