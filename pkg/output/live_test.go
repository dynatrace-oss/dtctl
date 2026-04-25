package output

import (
	"bytes"
	"context"
	"testing"
)

// TestFetchAndPrint_NilDataReturnsNil verifies that fetchAndPrint returns nil
// without clearing the screen or calling printer.Print when the fetcher returns
// (nil, nil) — the signal used by the DQL executor to indicate context cancellation.
func TestFetchAndPrint_NilDataReturnsNil(t *testing.T) {
	printCalled := false
	printer := &recordingPrinter{onPrint: func(data interface{}) error {
		printCalled = true
		return nil
	}}

	var buf bytes.Buffer
	p := &LivePrinter{
		printer:  printer,
		interval: DefaultLiveInterval,
		writer:   &buf,
	}

	fetcher := func(_ context.Context) (interface{}, error) {
		return nil, nil // simulates context-cancelled path
	}

	err := p.fetchAndPrint(context.Background(), fetcher)
	if err != nil {
		t.Fatalf("fetchAndPrint returned unexpected error: %v", err)
	}
	if printCalled {
		t.Error("printer.Print should not be called when fetcher returns nil data")
	}
	if buf.Len() != 0 {
		t.Errorf("no output expected for nil data, got: %q", buf.String())
	}
}

// TestFetchAndPrint_FetcherError propagates the error from the fetcher without
// touching the printer or writing any output.
func TestFetchAndPrint_FetcherError(t *testing.T) {
	printCalled := false
	printer := &recordingPrinter{onPrint: func(data interface{}) error {
		printCalled = true
		return nil
	}}

	var buf bytes.Buffer
	p := &LivePrinter{
		printer:  printer,
		interval: DefaultLiveInterval,
		writer:   &buf,
	}

	wantErr := context.Canceled
	fetcher := func(_ context.Context) (interface{}, error) {
		return nil, wantErr
	}

	err := p.fetchAndPrint(context.Background(), fetcher)
	if err != wantErr {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if printCalled {
		t.Error("printer.Print should not be called when fetcher returns an error")
	}
}

// recordingPrinter is a minimal Printer implementation for testing.
type recordingPrinter struct {
	onPrint func(data interface{}) error
}

func (r *recordingPrinter) Print(data interface{}) error     { return r.onPrint(data) }
func (r *recordingPrinter) PrintList(data interface{}) error { return nil }
