package inventory

// These exercise the gate and window helpers that a caller drives directly:
// they are the SDK's half of what used to be CLI-only logic, so they are
// tested here rather than through a command.

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeSince(t *testing.T) {
	for _, tc := range []struct {
		in     string
		expr   string
		window time.Duration
	}{
		// An omitted window falls back to the default rather than erroring:
		// the scope is the required input, the window has a sane default.
		{"", "now()-15m", 15 * time.Minute},
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
		expr, window, err := NormalizeSince(tc.in)
		if err != nil {
			t.Errorf("NormalizeSince(%q): unexpected error %v", tc.in, err)
			continue
		}
		if expr != tc.expr || window != tc.window {
			t.Errorf("NormalizeSince(%q) = %q/%s, want %q/%s", tc.in, expr, window, tc.expr, tc.window)
		}
	}

	// "-1h" is deliberately absent: a signed lookback is valid input.
	for _, bad := range []string{"yesterday", "15", "0m", "-0s", "2 hours"} {
		if _, _, err := NormalizeSince(bad); err == nil {
			t.Errorf("NormalizeSince(%q) should have failed", bad)
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
		if got := DefaultStaleAfter(tc.window); got != tc.want {
			t.Errorf("DefaultStaleAfter(%s) = %s, want %s", tc.window, got, tc.want)
		}
	}
}

func windowedInv(signals ...Signal) *Inventory {
	return &Inventory{
		Window:  &ArrivalWindow{Since: "now()-15m", Filter: `x == "y"`, StaleAfter: "5m0s"},
		Signals: signals,
		Summary: Summarize(signals),
	}
}

func TestCheckRequiredExitCode(t *testing.T) {
	live := Signal{Name: "logs", State: SignalLive}
	stale := Signal{Name: "spans", State: SignalStale}
	empty := Signal{Name: "bizevents", State: SignalEmpty}
	unknown := Signal{Name: "rum", State: SignalUnknown}
	notApplicable := Signal{Name: "host-metrics", State: SignalNotApplicable}

	for _, tc := range []struct {
		name    string
		inv     *Inventory
		require []string
		want    int
	}{
		{"no require", windowedInv(live, empty), nil, 0},
		{"all live", windowedInv(live), []string{"logs"}, 0},
		{"empty fails", windowedInv(live, empty), []string{"logs", "bizevents"}, ExitRequiredSignalNotLive},
		{"stale fails", windowedInv(live, stale), []string{"spans"}, ExitRequiredSignalNotLive},
		// A signal that cannot carry the scope fails the gate, because the
		// gate as written can never pass — but it is reported as a question
		// the signal cannot answer, not as missing telemetry.
		{"n/a fails", windowedInv(live, notApplicable), []string{"host-metrics"}, ExitRequiredSignalNotLive},
		// Unknown means dtctl could not establish the state. Failing a gate
		// closed on it would report a dtctl budget problem as a telemetry one.
		{"unknown outranks not-live-ish", windowedInv(live, unknown), []string{"rum"}, ExitRequiredSignalUnknown},
		{"unknown outranks not-live", windowedInv(empty, unknown), []string{"bizevents", "rum"}, ExitRequiredSignalUnknown},
		// A name that survived up-front validation but was never probed has
		// no verdict; it is not a telemetry failure.
		{"unprobed name has no verdict", windowedInv(live), []string{"nosuch"}, ExitRequiredSignalUnknown},
		{"case insensitive", windowedInv(live), []string{"LOGS"}, 0},
		// Plain (unwindowed) inventory keeps its always-zero exit.
		{"unwindowed ignores require", &Inventory{}, []string{"logs"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckRequired(tc.inv, tc.require).ExitCode()
			if got != tc.want {
				t.Errorf("exit code = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRequiredSignalsMessagesExplain(t *testing.T) {
	inv := windowedInv(
		Signal{Name: "bizevents", State: SignalEmpty},
		Signal{Name: "rum", State: SignalUnknown},
	)
	messages := CheckRequired(inv, []string{"bizevents", "rum"}).Messages
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "bizevents (empty)") {
		t.Errorf("message should name the failing signal and its state: %q", joined)
	}
	if !strings.Contains(joined, "not evidence of absence") {
		t.Errorf("unknown message must not read as an absence claim: %q", joined)
	}
}

func TestRequiredSignalNotApplicableExplainsTheQuestion(t *testing.T) {
	inv := windowedInv(Signal{Name: "rum", State: SignalNotApplicable})
	messages := CheckRequired(inv, []string{"rum"}).Messages
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "cannot carry this scope") {
		t.Errorf("n/a must be reported as an unanswerable question, not missing data: %q", joined)
	}
	if strings.Contains(joined, "not live") {
		t.Errorf("n/a must not be phrased as a liveness failure: %q", joined)
	}
}

func TestValidateSignalNames(t *testing.T) {
	defs := BuiltinDefinitions()

	if err := ValidateSignalNames(defs, nil, []string{"logs", "spans"}); err != nil {
		t.Errorf("known signal names should validate: %v", err)
	}
	if err := ValidateSignalNames(defs, nil, []string{"LOGS"}); err != nil {
		t.Errorf("name matching should be case-insensitive: %v", err)
	}
	// Entity-census capabilities are not signal types and cannot be required.
	if err := ValidateSignalNames(defs, nil, []string{"hosts"}); err == nil {
		t.Error("a non-signal capability should be rejected as a --require name")
	}
	err := ValidateSignalNames(defs, nil, []string{"logz"})
	if err == nil {
		t.Fatal("a misspelled signal name should be rejected before the run")
	}
	if !strings.Contains(err.Error(), "logs") {
		t.Errorf("the error should list the available signals: %v", err)
	}
	// Requiring what --signals excludes is a gate that can never pass.
	if err := ValidateSignalNames(defs, []string{"logs"}, []string{"spans"}); err == nil {
		t.Error("--require outside --signals should be rejected")
	}
	if err := ValidateSignalNames(defs, []string{"logs", "spans"}, []string{"spans"}); err != nil {
		t.Errorf("--require inside --signals should validate: %v", err)
	}
}
