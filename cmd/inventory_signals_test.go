package cmd

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

func windowedInv(signals ...inventory.Signal) *inventory.Inventory {
	return &inventory.Inventory{
		Window:  &inventory.ArrivalWindow{Since: "now()-15m", Filter: `x == "y"`, StaleAfter: "5m0s"},
		Signals: signals,
		Summary: inventory.Summarize(signals),
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
	for _, code := range []int{inventory.ExitRequiredSignalNotLive, inventory.ExitRequiredSignalUnknown} {
		if code <= 3 {
			t.Errorf("gate exit code %d sits in the band generic command failures use", code)
		}
	}
	if inventory.ExitRequiredSignalNotLive == inventory.ExitRequiredSignalUnknown {
		t.Error("not-live and no-verdict must be distinguishable by exit code")
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
