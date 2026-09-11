package inventory

import (
	"fmt"
	"strings"
)

// Exit codes for a --require style gate, exported because they are the
// contract a caller scripts against.
//
// They deliberately start at 10. 1 is what a CLI returns for any command
// error, and a gate that cannot tell "logs are not arriving" from "could not
// authenticate" is not a gate. A signal with no verdict is likewise kept
// separate from "not live": failing a build closed because a probe ran out of
// scan budget reports a tooling problem as a customer telemetry problem.
const (
	ExitRequiredSignalNotLive = 10
	ExitRequiredSignalUnknown = 11
)

// RequireVerdict is the outcome of gating on a set of required signals. The
// three buckets are kept apart rather than reduced to a bool because they
// warrant different responses: not-live is a telemetry finding, not-applicable
// is a malformed question, and unknown is an admission that nothing was
// established.
type RequireVerdict struct {
	// NotLive names required signals that were probed and are not live, each
	// with the state it did reach.
	NotLive []string
	// NotApplicable names required signals the scope cannot select on, so the
	// gate can never pass as written.
	NotApplicable []string
	// Unknown names required signals that got no verdict, whether because a
	// probe was cut short or because they were never probed.
	Unknown []string
	// Messages explain the verdict in the order it should be reported.
	Messages []string
}

// OK reports whether every required signal is live.
func (v RequireVerdict) OK() bool {
	return len(v.NotLive) == 0 && len(v.NotApplicable) == 0 && len(v.Unknown) == 0
}

// ExitCode maps the verdict onto the gate's exit contract.
//
// Unknown outranks not-live: it is the weaker claim, and reporting "no
// verdict" as a telemetry failure is the mistake this whole feature exists to
// avoid.
func (v RequireVerdict) ExitCode() int {
	switch {
	case len(v.Unknown) > 0:
		return ExitRequiredSignalUnknown
	case len(v.NotLive) > 0, len(v.NotApplicable) > 0:
		return ExitRequiredSignalNotLive
	}
	return 0
}

// CheckRequired evaluates a set of required signal names against a windowed
// inventory. Names are matched case-insensitively; blanks are ignored. An
// inventory that is not windowed cannot fail the gate, because it never
// answered the question the gate asks.
func CheckRequired(inv *Inventory, require []string) RequireVerdict {
	var v RequireVerdict
	if len(require) == 0 || inv == nil || inv.Window == nil {
		return v
	}
	states := make(map[string]SignalState, len(inv.Signals))
	for _, sig := range inv.Signals {
		states[strings.ToLower(sig.Name)] = sig.State
	}
	for _, name := range require {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		state, ok := states[strings.ToLower(name)]
		switch {
		case !ok:
			// Validated names reaching here were simply never probed — no
			// verdict, which is not the same as a failure.
			v.Unknown = append(v.Unknown, name)
		case state == SignalLive:
		case state == SignalUnknown:
			v.Unknown = append(v.Unknown, name)
		case state == SignalNotApplicable:
			v.NotApplicable = append(v.NotApplicable, name)
		default:
			v.NotLive = append(v.NotLive, fmt.Sprintf("%s (%s)", name, state))
		}
	}
	if len(v.NotLive) > 0 {
		v.Messages = append(v.Messages,
			"required signals not live: "+strings.Join(v.NotLive, ", "))
	}
	// A signal that cannot carry the scope was never really required — the
	// gate asks a question of it that has no answer. Saying so beats both
	// passing silently and failing as if the telemetry were missing.
	if len(v.NotApplicable) > 0 {
		v.Messages = append(v.Messages,
			"required signals cannot carry this scope: "+strings.Join(v.NotApplicable, ", ")+
				" — none of the scope's fields exist on them, so this gate can never pass; drop them from the required set or widen the scope")
	}
	if len(v.Unknown) > 0 {
		v.Messages = append(v.Messages,
			"required signals have no verdict: "+strings.Join(v.Unknown, ", ")+
				" — their state could not be established; this is not evidence of absence")
	}
	return v
}

// ValidateSignalNames rejects a probe-selection or required name that no
// definition provides, and a required name that the selection excludes.
//
// It exists to be called before a run rather than after: both mistakes are
// usage errors, and discovering them once the battery has spent its budget
// both wastes the run and dresses a typo up as a telemetry verdict.
func ValidateSignalNames(defs map[string]*CapabilityDef, signals, require []string) error {
	known := SignalNames(defs)
	set := make(map[string]bool, len(known))
	for _, n := range known {
		set[strings.ToLower(n)] = true
	}
	var unknown []string
	for _, n := range append(append([]string{}, signals...), require...) {
		n = strings.TrimSpace(n)
		if n != "" && !set[strings.ToLower(n)] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown signal %s: this capability set provides %s",
			strings.Join(unknown, ", "), strings.Join(known, ", "))
	}
	if len(signals) == 0 {
		return nil
	}
	selected := make(map[string]bool, len(signals))
	for _, n := range signals {
		selected[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var excluded []string
	for _, n := range require {
		n = strings.TrimSpace(n)
		if n != "" && !selected[strings.ToLower(n)] {
			excluded = append(excluded, n)
		}
	}
	if len(excluded) > 0 {
		return fmt.Errorf("%s is required but excluded from probing: the gate could never pass",
			strings.Join(excluded, ", "))
	}
	return nil
}
