package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// Exit codes for --require.
//
// Deliberately outside the 0-3 band: 1 is what cobra returns for any command
// error, and a gate that cannot tell "logs are not arriving" from "dtctl
// could not authenticate" is not a gate. A signal with no verdict is likewise
// kept separate from "not live" — failing a build closed because dtctl ran
// out of scan budget would report a dtctl problem as a customer telemetry
// problem.
const (
	exitRequiredSignalNotLive = 10
	exitRequiredSignalUnknown = 11
)

// defaultArrivalWindow is the window used when --since is not given. Short
// enough that every probe stays cheap, long enough to survive ordinary ingest
// jitter.
const defaultArrivalWindow = "15m"

var inventoryArrivalsCmd = &cobra.Command{
	Use:   "arrivals",
	Short: "Is data arriving for this source right now? Per-signal ingest state over a window",
	Long: `Report, per signal type, whether data matching one scope is arriving inside a
window — the question you actually have after instrumenting a service, pointing a
collector at the tenant, or running an ingest.

'dtctl inventory' cannot answer it: its verdicts are retention-scoped, so a stream
that received data once last week reads as present. This command windows the
question and narrows it to one scope.

States:
  live    records matched and the newest is fresh
  stale   records matched, but the source stopped inside the window
  empty   nothing matched, yet the stream holds data within retention —
          the stream works, this scope is not producing into it
  no-data nothing matched and the stream is empty tenant-wide
  n/a     the signal cannot carry this scope: none of the fields the scope
          names exist on it (there is no k8s.namespace.name on RUM data, and
          no service.name dimension on Kubernetes metrics)
  absent  the stream is not in this environment's data-object catalog
  unknown no verdict — the probe was truncated, capped, or failed. Never to
          be read as absence.

--scope is required. An unscoped windowed count is the single most expensive query
available, and the unscoped question is already answered for free, without a window,
by plain 'dtctl inventory'. ('--scope true' is legal DQL and will be honoured — it is
simply the expensive query this default is steering you away from.)

Examples:
  # Is anything arriving for this namespace?
  dtctl inventory arrivals --scope 'k8s.namespace.name == "payments"'

  # Widen the window
  dtctl inventory arrivals --since 1h --scope 'service.name == "checkout"'

  # Gate a pipeline on it (exit 10 if a required signal is not live, 11 if unknown)
  dtctl inventory arrivals --scope 'service.name == "checkout"' --require logs,spans

  # Confirm an ingest landed, probing only what can answer
  dtctl inventory arrivals --since 5m --scope 'log.source == "batch-import"' \
      --signals logs --require logs
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		scope, _ := cmd.Flags().GetString("scope")
		since, _ := cmd.Flags().GetString("since")
		signals, _ := cmd.Flags().GetStringSlice("signals")
		require, _ := cmd.Flags().GetStringSlice("require")
		staleAfterFlag, _ := cmd.Flags().GetDuration("stale-after")

		if strings.TrimSpace(scope) == "" {
			return fmt.Errorf("--scope is required: see 'dtctl inventory arrivals --help' for why an unscoped window is not offered")
		}
		sinceExpr, windowLen, err := parseSinceFlag(since)
		if err != nil {
			return err
		}

		defs, err := inventoryDefinitions(cmd)
		if err != nil {
			return err
		}
		// Names are checked before a single query runs: a typo is a usage
		// error, and discovering it after the battery has spent the budget
		// would both waste the run and dress the mistake up as a telemetry
		// verdict.
		if err := validateSignalNames(defs, signals, require); err != nil {
			return err
		}

		staleAfter := staleAfterFlag
		if staleAfter == 0 {
			staleAfter = defaultStaleAfter(windowLen)
		}
		if windowLen > time.Hour {
			fmt.Fprintf(os.Stderr, "warning: a %s window makes each probe scan proportionally more; probes cut short by the scan cap report as unknown, not absent — narrow --since if that happens\n", roundWindow(windowLen))
		}

		runner := newInventoryRunner(cmd, cfg, c)
		ctx, cancel := inventoryCancelContext()
		defer cancel()

		budgetQueries, budgetSeconds := inventoryBudget(cmd)
		inv, err := inventory.Discover(ctx, runner, defs, inventory.DiscoverOptions{
			ContextName:   cfg.CurrentContext,
			BudgetQueries: budgetQueries,
			BudgetSeconds: budgetSeconds,
			Since:         sinceExpr,
			Scope:         scope,
			StaleAfter:    staleAfter,
			Signals:       signals,
		})
		if err != nil {
			return err
		}

		if outputFormat == "table" && !agentMode {
			printInventorySignalsHuman(inv)
			exitForRequiredSignals(inv, require)
			return nil
		}
		printer := NewPrinter()
		if ap := enrichAgent(printer, "inventory", "arrivals"); ap != nil {
			ap.SetSuggestions(inventorySuggestions(inv))
		}
		if err := printer.Print(inv); err != nil {
			return err
		}
		exitForRequiredSignals(inv, require)
		return nil
	},
}

// validateSignalNames rejects a --signals or --require name that no
// definition provides, and a --require name that --signals excluded — both
// are mistakes the run itself could only report as a missing verdict.
func validateSignalNames(defs map[string]*inventory.CapabilityDef, signals, require []string) error {
	known := inventory.SignalNames(defs)
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
		return fmt.Errorf("--require names %s, which --signals excludes from probing: the gate could never pass", strings.Join(excluded, ", "))
	}
	return nil
}

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
		v = defaultArrivalWindow
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
	var notLive, unknown, notApplicable []string
	for _, name := range require {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		state, ok := states[strings.ToLower(name)]
		switch {
		case !ok:
			// The name was validated up front, so reaching here means the
			// signal was never probed — no verdict, not a failure.
			unknown = append(unknown, name)
		case state == inventory.SignalLive:
		case state == inventory.SignalUnknown:
			unknown = append(unknown, name)
		case state == inventory.SignalNotApplicable:
			notApplicable = append(notApplicable, name)
		default:
			notLive = append(notLive, fmt.Sprintf("%s (%s)", name, state))
		}
	}
	if len(notLive) > 0 {
		messages = append(messages, fmt.Sprintf("required signals not live: %s", strings.Join(notLive, ", ")))
		code = exitRequiredSignalNotLive
	}
	// A signal that cannot carry the scope was never really required — the
	// gate asks a question of it that has no answer. Saying so beats both
	// passing silently and failing as if the telemetry were missing.
	if len(notApplicable) > 0 {
		messages = append(messages, fmt.Sprintf("required signals cannot carry this scope: %s — none of the scope's fields exist on them, so this gate can never pass; drop them from --require or widen --scope", strings.Join(notApplicable, ", ")))
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
	if inv.Summary != nil && inv.Summary.NotApplicable > 0 {
		out = append(out, "Signals marked n/a cannot carry this scope at all — do not report them as missing telemetry, and do not re-probe them")
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

	// "Volume", not "Records": metric families are counted in datapoints.
	headers := [5]string{"SIGNAL", "STATE", "VOLUME", "LAST SEEN", "AGE"}
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
		}{{s.Live, "live"}, {s.Stale, "stale"}, {s.Empty, "empty"}, {s.NoData, "no-data"}, {s.NotApplicable, "n/a"}, {s.Absent, "absent"}, {s.Unknown, "unknown"}} {
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

// stateRank orders states from most to least alive. n/a sits past the states
// that say something about the data, because it says something about the
// question instead.
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
	case inventory.SignalNotApplicable:
		return 5
	default:
		return 6
	}
}

func signalCount(sig inventory.Signal) string {
	switch {
	case sig.Datapoints > 0:
		return strconv.FormatInt(sig.Datapoints, 10) + " pts"
	case sig.State == inventory.SignalAbsent ||
		sig.State == inventory.SignalUnknown ||
		sig.State == inventory.SignalNotApplicable:
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

func init() {
	addInventoryDiscoveryFlags(inventoryArrivalsCmd)
	inventoryArrivalsCmd.Flags().String("scope", "", "DQL filter fragment scoping every probe, e.g. 'k8s.namespace.name == \"payments\"' (required)")
	inventoryArrivalsCmd.Flags().String("since", defaultArrivalWindow, "Window to report arrivals over (e.g. 15m, 1h)")
	inventoryArrivalsCmd.Flags().StringSlice("signals", nil, "Restrict probing to these signals (default: all signal streams and metric families)")
	inventoryArrivalsCmd.Flags().StringSlice("require", nil, "Exit non-zero unless every named signal is live (10 = not live, 11 = no verdict)")
	inventoryArrivalsCmd.Flags().Duration("stale-after", 0, "Age past which a matched signal is stale rather than live (default max(2m, window/3))")
	_ = inventoryArrivalsCmd.MarkFlagRequired("scope")
}
