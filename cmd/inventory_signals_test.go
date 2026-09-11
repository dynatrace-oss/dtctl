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
	notApplicable := inventory.Signal{Name: "host-metrics", State: inventory.SignalNotApplicable}

	for _, tc := range []struct {
		name    string
		inv     *inventory.Inventory
		require []string
		want    int
	}{
		{"no require", windowedInv(live, empty), nil, 0},
		{"all live", windowedInv(live), []string{"logs"}, 0},
		{"empty fails", windowedInv(live, empty), []string{"logs", "bizevents"}, exitRequiredSignalNotLive},
		{"stale fails", windowedInv(live, stale), []string{"spans"}, exitRequiredSignalNotLive},
		// A signal that cannot carry the scope fails the gate, because the
		// gate as written can never pass — but it is reported as a question
		// the signal cannot answer, not as missing telemetry.
		{"n/a fails", windowedInv(live, notApplicable), []string{"host-metrics"}, exitRequiredSignalNotLive},
		// Unknown means dtctl could not establish the state. Failing a gate
		// closed on it would report a dtctl budget problem as a telemetry one.
		{"unknown outranks not-live-ish", windowedInv(live, unknown), []string{"rum"}, exitRequiredSignalUnknown},
		{"unknown outranks not-live", windowedInv(empty, unknown), []string{"bizevents", "rum"}, exitRequiredSignalUnknown},
		// A name that survived up-front validation but was never probed has
		// no verdict; it is not a telemetry failure.
		{"unprobed name has no verdict", windowedInv(live), []string{"nosuch"}, exitRequiredSignalUnknown},
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
		// n/a has no count to report: the probe was never asked.
		{inventory.Signal{State: inventory.SignalNotApplicable}, "—"},
	} {
		if got := signalCount(tc.sig); got != tc.want {
			t.Errorf("signalCount(%+v) = %q, want %q", tc.sig, got, tc.want)
		}
	}
}

func TestExitCodesDoNotCollideWithGenericFailure(t *testing.T) {
	// A CI gate has to be able to tell "logs are not arriving" from "dtctl
	// could not authenticate", and cobra returns 1 for the latter.
	for _, code := range []int{exitRequiredSignalNotLive, exitRequiredSignalUnknown} {
		if code <= 3 {
			t.Errorf("gate exit code %d sits in the band generic command failures use", code)
		}
	}
	if exitRequiredSignalNotLive == exitRequiredSignalUnknown {
		t.Error("not-live and no-verdict must be distinguishable by exit code")
	}
}

func TestRequiredSignalNotApplicableExplainsTheQuestion(t *testing.T) {
	inv := windowedInv(inventory.Signal{Name: "rum", State: inventory.SignalNotApplicable})
	_, messages := requiredSignalsExitCode(inv, []string{"rum"})
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "cannot carry this scope") {
		t.Errorf("n/a must be reported as an unanswerable question, not missing data: %q", joined)
	}
	if strings.Contains(joined, "not live") {
		t.Errorf("n/a must not be phrased as a liveness failure: %q", joined)
	}
}

func TestValidateSignalNames(t *testing.T) {
	defs := inventory.BuiltinDefinitions()

	if err := validateSignalNames(defs, nil, []string{"logs", "spans"}); err != nil {
		t.Errorf("known signal names should validate: %v", err)
	}
	if err := validateSignalNames(defs, nil, []string{"LOGS"}); err != nil {
		t.Errorf("name matching should be case-insensitive: %v", err)
	}
	// Entity-census capabilities are not signal types and cannot be required.
	if err := validateSignalNames(defs, nil, []string{"hosts"}); err == nil {
		t.Error("a non-signal capability should be rejected as a --require name")
	}
	err := validateSignalNames(defs, nil, []string{"logz"})
	if err == nil {
		t.Fatal("a misspelled signal name should be rejected before the run")
	}
	if !strings.Contains(err.Error(), "logs") {
		t.Errorf("the error should list the available signals: %v", err)
	}
	// Requiring what --signals excludes is a gate that can never pass.
	if err := validateSignalNames(defs, []string{"logs"}, []string{"spans"}); err == nil {
		t.Error("--require outside --signals should be rejected")
	}
	if err := validateSignalNames(defs, []string{"logs", "spans"}, []string{"spans"}); err != nil {
		t.Errorf("--require inside --signals should validate: %v", err)
	}
}

func TestInventorySuggestionsFlagNotApplicable(t *testing.T) {
	inv := windowedInv(
		inventory.Signal{Name: "logs", State: inventory.SignalLive},
		inventory.Signal{Name: "rum", State: inventory.SignalNotApplicable},
	)
	joined := strings.Join(inventorySuggestions(inv), "\n")
	if !strings.Contains(joined, "cannot carry this scope") {
		t.Errorf("agents must be told not to report n/a as missing telemetry: %q", joined)
	}
}

func TestStateRankOrdersNotApplicableAfterData(t *testing.T) {
	// n/a says something about the question, not the data, so it sorts past
	// every state that does say something about the data.
	for _, state := range []inventory.SignalState{
		inventory.SignalLive, inventory.SignalStale,
		inventory.SignalEmpty, inventory.SignalNoData, inventory.SignalAbsent,
	} {
		if stateRank(string(inventory.SignalNotApplicable)) <= stateRank(string(state)) {
			t.Errorf("n/a should rank after %s", state)
		}
	}
	if stateRank(string(inventory.SignalUnknown)) <= stateRank(string(inventory.SignalNotApplicable)) {
		t.Error("unknown should rank last")
	}
}
