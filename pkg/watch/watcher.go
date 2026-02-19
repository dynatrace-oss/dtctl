package watch

import (
	"context"
	"log"
	"reflect"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

func NewWatcher(opts WatcherOptions) *Watcher {
	if opts.Interval < time.Second {
		opts.Interval = 2 * time.Second
	}

	return &Watcher{
		interval:    opts.Interval,
		client:      opts.Client,
		fetcher:     opts.Fetcher,
		differ:      NewDiffer(),
		printer:     opts.Printer,
		stopCh:      make(chan struct{}),
		showInitial: opts.ShowInitial,
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	if err := w.poll(ctx, true); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.stopCh:
			return nil
		case <-ticker.C:
			if err := w.poll(ctx, false); err != nil {
				if isTransient(err) {
					log.Printf("Warning: Temporary error, retrying: %v\n", err)
					continue
				}
				if isRateLimited(err) {
					backoff := extractRetryAfter(err)
					if backoff > 0 {
						time.Sleep(backoff)
					} else {
						time.Sleep(w.interval * 2)
					}
					continue
				}
				if isNetworkError(err) {
					log.Printf("Warning: Connection lost, retrying...\n")
					time.Sleep(w.interval * 2)
					continue
				}
				return err
			}
		}
	}
}

func (w *Watcher) Stop() {
	close(w.stopCh)
}

func (w *Watcher) poll(ctx context.Context, initial bool) error {
	result, err := w.fetcher()
	if err != nil {
		return err
	}

	resources, err := normalizeToSlice(result)
	if err != nil {
		return err
	}

	if initial && w.showInitial {
		// Initialize differ state so next poll doesn't see everything as "added"
		w.differ.Detect(resources)
		if w.printer != nil {
			return w.printer.PrintList(resources)
		}
		return nil
	}

	changes := w.differ.Detect(resources)

	// Always print the full table with change indicators
	if w.printer != nil {
		watchPrinter, ok := w.printer.(output.WatchPrinterInterface)
		if ok {
			return watchPrinter.PrintChanges(changes)
		}
		// Fallback for non-watch printers - only print actual changes
		for _, change := range changes {
			if change.Type != "" {
				if err := w.printer.Print(change.Resource); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func normalizeToSlice(result interface{}) ([]interface{}, error) {
	if result == nil {
		return []interface{}{}, nil
	}

	switch v := result.(type) {
	case []interface{}:
		return v, nil
	case []map[string]interface{}:
		slice := make([]interface{}, len(v))
		for i, item := range v {
			slice[i] = item
		}
		return slice, nil
	default:
		// Use reflection to handle any slice type
		rv := reflect.ValueOf(result)
		if rv.Kind() == reflect.Slice {
			slice := make([]interface{}, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				slice[i] = rv.Index(i).Interface()
			}
			return slice, nil
		}
		return []interface{}{result}, nil
	}
}

func isTransient(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "timeout") ||
		contains(errStr, "temporary") ||
		contains(errStr, "connection reset")
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "rate limit") ||
		contains(errStr, "429") ||
		contains(errStr, "too many requests")
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "connection refused") ||
		contains(errStr, "no such host") ||
		contains(errStr, "network unreachable")
}

func extractRetryAfter(err error) time.Duration {
	return 0
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
