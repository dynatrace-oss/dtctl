package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

func TestParseSinceFlag(t *testing.T) {
	for _, tc := range []struct {
		in     string
		expr   string
		window time.Duration
	}{
		{"", "", 0},
		{"15m", "now()-15m", 15 * time.Minute},
		// A lookback reads naturally with or without the sign.
		{"-15m", "now()-15m", 15 * time.Minute},
		{"2h", "now()-120m", 2 * time.Hour},
		// DQL rejects a Go-style compound duration, so sub-minute precision
		// falls back to seconds rather than emitting "1h30m".
		{"90s", "now()-90s", 90 * time.Second},
		{"1h30m", "now()-90m", 90 * time.Minute},
		{"45s", "now()-45s", 45 * time.Second},
	} {
		expr, window, err := parseSinceFlag(tc.in)
		if err != nil {
			t.Errorf("parseSinceFlag(%q): unexpected error %v", tc.in, err)
			continue
		}
		if expr != tc.expr || window != tc.window {
			t.Errorf("parseSinceFlag(%q) = %q/%s, want %q/%s", tc.in, expr, window, tc.expr, tc.window)
		}
	}

	// "-1h" is deliberately absent: a signed lookback is valid input.
	for _, bad := range []string{"yesterday", "15", "0m", "-0s", "2 hours"} {
		if _, _, err := parseSinceFlag(bad); err == nil {
			t.Errorf("parseSinceFlag(%q) should have failed", bad)
		}
	}
}

func TestDefaultStaleAfter(t *testing.T) {
	for _, tc := range []struct {
		window time.Duration
		want   time.Duration
	}{
		// Floored at 2m so ordinary ingest jitter is not called stale.
		{1 * time.Minute, 2 * time.Minute},
		{5 * time.Minute, 2 * time.Minute},
		{15 * time.Minute, 5 * time.Minute},
		{1 * time.Hour, 20 * time.Minute},
	} {
		if got := defaultStaleAfter(tc.window); got != tc.want {
			t.Errorf("defaultStaleAfter(%s) = %s, want %s", tc.window, got, tc.want)
		}
	}
}

func windowedInv(signals ...inventory.Signal) *inventory.Inventory {
	return &inventory.Inventory{
		Window:  &inventory.ArrivalWindow{Since: "now()-15m", Filter: `x == "y"`, StaleAfter: "5m0s"},
		Signals: signals,
		Summary: inventory.Summarize(signals),
	}
}

func TestRequiredSignalsExitCode(t *testing.T) {
	live := inventory.Signal{Name: "logs", State: inventory.SignalLive}
	stale := inventory.Signal{Name: "spans", State: inventory.SignalStale}
	empty := inventory.Signal{Name: "bizevents", State: inventory.SignalEmpty}
	unknown := inventory.Signal{Name: "rum", State: inventory.SignalUnknown}

	for _, tc := range []struct {
		name    string
		inv     *inventory.Inventory
		require []string
		want    int
	}{
		{"no require", windowedInv(live, empty), nil, 0},
		{"all live", windowedInv(live), []string{"logs"}, 0},
		{"empty fails", windowedInv(live, empty), []string{"logs", "bizevents"}, 1},
		{"stale fails", windowedInv(live, stale), []string{"spans"}, 1},
		// Unknown means dtctl could not establish the state. Failing a gate
		// closed on it would report a dtctl budget problem as a telemetry one.
		{"unknown is 2", windowedInv(live, unknown), []string{"rum"}, 2},
		{"unknown outranks not-live", windowedInv(empty, unknown), []string{"bizevents", "rum"}, 2},
		{"unprobed name fails", windowedInv(live), []string{"nosuch"}, 1},
		{"case insensitive", windowedInv(live), []string{"LOGS"}, 0},
		// Plain (unwindowed) inventory keeps its always-zero exit.
		{"unwindowed ignores require", &inventory.Inventory{}, []string{"logs"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := requiredSignalsExitCode(tc.inv, tc.require)
			if got != tc.want {
				t.Errorf("exit code = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRequiredSignalsMessagesExplain(t *testing.T) {
	inv := windowedInv(
		inventory.Signal{Name: "bizevents", State: inventory.SignalEmpty},
		inventory.Signal{Name: "rum", State: inventory.SignalUnknown},
	)
	_, messages := requiredSignalsExitCode(inv, []string{"bizevents", "rum"})
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "bizevents (empty)") {
		t.Errorf("message should name the failing signal and its state: %q", joined)
	}
	if !strings.Contains(joined, "not evidence of absence") {
		t.Errorf("unknown message must not read as an absence claim: %q", joined)
	}
}

func TestInventorySuggestionsAreResultSpecific(t *testing.T) {
	inv := windowedInv(
		inventory.Signal{Name: "bizevents", State: inventory.SignalEmpty},
		inventory.Signal{Name: "spans", State: inventory.SignalStale, AgeSeconds: 1620},
	)
	joined := strings.Join(inventorySuggestions(inv), "\n")
	if !strings.Contains(joined, "bizevents is live tenant-wide") {
		t.Errorf("empty signal should steer to the source, not the tenant: %q", joined)
	}
	if !strings.Contains(joined, "spans stopped") {
		t.Errorf("stale signal should be called out: %q", joined)
	}

	// Unwindowed runs keep the original catalog-oriented advice.
	plain := inventorySuggestions(&inventory.Inventory{})
	if !strings.Contains(strings.Join(plain, "\n"), "fetch <object>") {
		t.Errorf("unwindowed suggestions changed: %v", plain)
	}
}

func TestSignalCountRendering(t *testing.T) {
	for _, tc := range []struct {
		sig  inventory.Signal
		want string
	}{
		{inventory.Signal{State: inventory.SignalLive, Records: 42}, "42"},
		{inventory.Signal{State: inventory.SignalLive, Datapoints: 400}, "400 pts"},
		{inventory.Signal{State: inventory.SignalAbsent}, "—"},
		{inventory.Signal{State: inventory.SignalUnknown}, "—"},
		{inventory.Signal{State: inventory.SignalEmpty}, "0"},
	} {
		if got := signalCount(tc.sig); got != tc.want {
			t.Errorf("signalCount(%+v) = %q, want %q", tc.sig, got, tc.want)
		}
	}
}
