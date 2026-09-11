package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// Exit codes for --require. A signal with no verdict is deliberately not
// folded into "not live": failing a gate closed because dtctl ran out of scan
// budget would report a dtctl problem as a customer telemetry problem.
const (
	exitRequiredSignalNotLive = 1
	exitRequiredSignalUnknown = 2
)

// parseSinceFlag turns the --since value into a DQL timeframe expression and
// the window length.
//
// Both "15m" and "-15m" are accepted: the sign is what a user reaching for a
// lookback naturally writes either way. Note that a leading dash has to be
// passed as --since=-15m, since pflag would otherwise read it as a flag.
//
// The expression is emitted in whole minutes where possible and seconds
// otherwise, so a Go duration like "1h30m" — which DQL does not accept
// verbatim — still produces a valid timeframe.
func parseSinceFlag(v string) (expr string, window time.Duration, err error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", 0, nil
	}
	d, perr := time.ParseDuration(strings.TrimPrefix(v, "-"))
	if perr != nil {
		return "", 0, fmt.Errorf("invalid --since %q: expected a duration such as 15m, 2h, or 90s", v)
	}
	if d <= 0 {
		return "", 0, fmt.Errorf("invalid --since %q: the window must be a positive duration", v)
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("now()-%dm", int64(d/time.Minute)), d, nil
	}
	return fmt.Sprintf("now()-%ds", int64(d/time.Second)), d, nil
}

// defaultStaleAfter derives the freshness threshold from the window: a third
// of the window, floored at two minutes so a short window does not call
// normally-jittery ingest stale.
func defaultStaleAfter(window time.Duration) time.Duration {
	third := window / 3
	if third < 2*time.Minute {
		return 2 * time.Minute
	}
	return third
}

func roundWindow(d time.Duration) string {
	if d%time.Minute == 0 {
		return d.String()
	}
	return d.Round(time.Second).String()
}

// exitForRequiredSignals enforces --require after the report has been printed,
// so the operator sees the evidence for the failure rather than just a code.
func exitForRequiredSignals(inv *inventory.Inventory, require []string) {
	code, messages := requiredSignalsExitCode(inv, require)
	for _, m := range messages {
		fmt.Fprintf(os.Stderr, "\n%s\n", m)
	}
	if code != 0 {
		os.Exit(code)
	}
}

// requiredSignalsExitCode is the pure decision behind --require, kept separate
// from the exit so it is testable.
func requiredSignalsExitCode(inv *inventory.Inventory, require []string) (int, []string) {
	if len(require) == 0 || inv == nil || inv.Window == nil {
		return 0, nil
	}
	states := make(map[string]inventory.SignalState, len(inv.Signals))
	for _, sig := range inv.Signals {
		states[strings.ToLower(sig.Name)] = sig.State
	}
	code := 0
	var messages []string
	var notLive, unknown, missing []string
	for _, name := range require {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		state, ok := states[strings.ToLower(name)]
		switch {
		case !ok:
			missing = append(missing, name)
		case state == inventory.SignalLive:
		case state == inventory.SignalUnknown:
			unknown = append(unknown, name)
		default:
			notLive = append(notLive, fmt.Sprintf("%s (%s)", name, state))
		}
	}
	if len(missing) > 0 {
		messages = append(messages, fmt.Sprintf("required signal not probed: %s — check the name against the capability set, or drop it from --signals", strings.Join(missing, ", ")))
		code = exitRequiredSignalNotLive
	}
	if len(notLive) > 0 {
		messages = append(messages, fmt.Sprintf("required signals not live: %s", strings.Join(notLive, ", ")))
		code = exitRequiredSignalNotLive
	}
	// Unknown wins: it is the weaker claim, and reporting "no verdict" as a
	// telemetry failure is the mistake this whole command is built to avoid.
	if len(unknown) > 0 {
		messages = append(messages, fmt.Sprintf("required signals have no verdict: %s — dtctl could not establish their state; this is not evidence of absence", strings.Join(unknown, ", ")))
		code = exitRequiredSignalUnknown
	}
	return code, messages
}

// inventorySuggestions tailors agent-mode guidance to what the run actually
// found, so the advice is about this result rather than generic.
func inventorySuggestions(inv *inventory.Inventory) []string {
	if inv == nil || inv.Window == nil {
		return []string{
			"Run 'dtctl query \"fetch <object> | limit 10\"' to sample any listed data object",
			"Cite the evidence carried by absent capabilities instead of re-probing; unknown capabilities got no verdict and may still exist",
		}
	}
	var out []string
	for _, sig := range inv.Signals {
		switch sig.State {
		case inventory.SignalEmpty:
			out = append(out, fmt.Sprintf("%s is live tenant-wide but matched nothing for this scope — investigate the source's emission, not the tenant", sig.Name))
		case inventory.SignalStale:
			out = append(out, fmt.Sprintf("%s stopped %ds ago inside the window — the source emitted and then went quiet; this is not a query problem", sig.Name, sig.AgeSeconds))
		}
	}
	if inv.Summary != nil && inv.Summary.Unknown > 0 {
		out = append(out, "Unknown signals got no verdict: re-run with a narrower --since or a raised --scan-limit-gbytes rather than treating them as absent")
	}
	if len(out) == 0 {
		out = append(out, "Every required signal is arriving; re-run with --require to turn this into a gate")
	}
	return out
}

// printInventorySignalsHuman renders windowed arrival mode for a terminal: the
// per-signal state table first, then the evidence for everything that is not
// simply live.
func printInventorySignalsHuman(inv *inventory.Inventory) {
	const w = 12
	output.DescribeKV("Context:", w, "%s", inv.Context)
	output.DescribeKV("Generated:", w, "%s", inv.GeneratedAt)
	if inv.Window != nil {
		output.DescribeKV("Window:", w, "%s → now()  (stale after %s)", inv.Window.Since, inv.Window.StaleAfter)
		if inv.Window.Filter != "" {
			output.DescribeKV("Scope:", w, "%s", inv.Window.Filter)
		}
	}

	if len(inv.Signals) == 0 {
		fmt.Println("\nNo signal types were probed — every capability was excluded by --signals.")
		return
	}

	rows := make([][5]string, 0, len(inv.Signals))
	for _, sig := range inv.Signals {
		rows = append(rows, [5]string{
			sig.Name,
			string(sig.State),
			signalCount(sig),
			signalLastSeen(sig),
			signalAge(sig),
		})
	}
	// Most-alive first: the states a reader acts on sit together at the bottom.
	sort.SliceStable(rows, func(i, j int) bool {
		return stateRank(rows[i][1]) < stateRank(rows[j][1])
	})

	headers := [5]string{"SIGNAL", "STATE", "RECORDS", "LAST SEEN", "AGE"}
	width := [5]int{}
	for i, h := range headers {
		width[i] = len(h)
	}
	for _, r := range rows {
		for i, cell := range r {
			if len(cell) > width[i] {
				width[i] = len(cell)
			}
		}
	}
	fmt.Println()
	fmt.Printf("%-*s  %-*s  %*s  %-*s  %*s\n",
		width[0], headers[0], width[1], headers[1], width[2], headers[2], width[3], headers[3], width[4], headers[4])
	for _, r := range rows {
		fmt.Printf("%-*s  %-*s  %*s  %-*s  %*s\n",
			width[0], r[0], width[1], r[1], width[2], r[2], width[3], r[3], width[4], r[4])
	}

	if s := inv.Summary; s != nil {
		parts := []string{}
		for _, p := range []struct {
			n int
			l string
		}{{s.Live, "live"}, {s.Stale, "stale"}, {s.Empty, "empty"}, {s.NoData, "no-data"}, {s.Absent, "absent"}, {s.Unknown, "unknown"}} {
			if p.n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", p.n, p.l))
			}
		}
		if len(parts) > 0 {
			fmt.Printf("\n%s\n", strings.Join(parts, " · "))
		}
	}

	var evidence []inventory.Signal
	for _, sig := range inv.Signals {
		if sig.Evidence != "" {
			evidence = append(evidence, sig)
		}
	}
	if len(evidence) > 0 {
		output.DescribeSection("Evidence (what was checked)")
		for _, sig := range evidence {
			fmt.Printf("  %s — %s\n", sig.Name, sig.Evidence)
		}
	}
	for _, n := range inv.Notes {
		output.DescribeKV("Note:", w, "%s", n)
	}
	if r := inv.Discovery; r != nil {
		fmt.Fprintf(os.Stderr, "\nDiscovery: %d queries, %.1fs query time\n", r.Queries, r.Seconds)
		for _, n := range r.Notes {
			fmt.Fprintf(os.Stderr, "  note: %s\n", n)
		}
	}
}

// stateRank orders states from most to least alive.
func stateRank(state string) int {
	switch inventory.SignalState(state) {
	case inventory.SignalLive:
		return 0
	case inventory.SignalStale:
		return 1
	case inventory.SignalEmpty:
		return 2
	case inventory.SignalNoData:
		return 3
	case inventory.SignalAbsent:
		return 4
	default:
		return 5
	}
}

func signalCount(sig inventory.Signal) string {
	switch {
	case sig.Datapoints > 0:
		return strconv.FormatInt(sig.Datapoints, 10) + " pts"
	case sig.State == inventory.SignalAbsent || sig.State == inventory.SignalUnknown:
		return "—"
	default:
		return strconv.FormatInt(sig.Records, 10)
	}
}

func signalLastSeen(sig inventory.Signal) string {
	if sig.LastSeen == "" {
		return "—"
	}
	if ts, err := time.Parse(time.RFC3339, sig.LastSeen); err == nil {
		return ts.UTC().Format("15:04:05")
	}
	return sig.LastSeen
}

func signalAge(sig inventory.Signal) string {
	if sig.LastSeen == "" {
		return "—"
	}
	return (time.Duration(sig.AgeSeconds) * time.Second).String()
}
