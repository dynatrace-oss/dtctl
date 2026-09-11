package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/segment"
	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// inventoryCmd probes the current environment for what data actually exists
// there. `dtctl commands` answers "what can I run?"; `dtctl inventory` answers
// "what is there to query?".
var inventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Probe the environment: which data, entity types, and capabilities exist here",
	Long: `Probe the current context's environment and report what data is available:
which Grail data objects are fetchable (and which are queried through other
commands), which buckets and filter segments exist, the live entity-type
census, and which capabilities (spans, logs, RUM, k8s, cloud integrations,
metric families, ...) are backed by evidence — with the evidence cited for
every absent capability. A capability that could not be checked (failed
probe, exhausted budget) is reported as unknown, never as absent.

This is about the data in the environment, not the resources you manage —
for dashboards, workflows, SLOs, and the rest, use 'dtctl get <resource>'.

Discovery is read-only and budgeted: it runs a small battery of DQL queries
(data-object catalog, buckets, entity census, metric catalog when needed,
plus any probe-shaped definitions) and stops with a partial inventory rather
than overrunning the budget. Nothing is persisted.

The capability set is customizable. dtctl ships a built-in, structural-only
set; --definitions merges your own definitions over it (see
docs/dev/examples/inventory-definitions.example.yaml for the format and the
four discovery shapes).

Examples:
  # The environment inventory for the current context
  dtctl inventory
  dtctl inventory -o json

  # Add organization-specific capability definitions
  dtctl inventory --definitions ./our-capabilities.yaml

  # Only your definitions, without the built-in set
  dtctl inventory --definitions ./our-capabilities.yaml --no-builtin-definitions

  # Onboarding verification: is data arriving for this scope right now?
  dtctl inventory --since 15m --where 'k8s.namespace.name == "payments"'

  # Gate a pipeline on it (exit 1 if a required signal is not live)
  dtctl inventory --since 10m --where 'service.name == "checkout"' --require logs,spans

  # Confirm an ingest actually landed
  dtctl inventory --since 5m --where 'log.source == "batch-import"' --require logs

Windowed arrival mode (--since) reports a per-signal ingest state instead of the
environment-wide capability verdicts, because those are retention-scoped: a stream
that received data once last week is "present" but not arriving. States are live,
stale (matched, but stopped inside the window), empty (stream is live tenant-wide
but nothing matched this scope), no-data, absent, and unknown.

--since requires --where. An unscoped windowed count is the most expensive query in
the battery, and the unscoped question is already answered for free without a window.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		since, _ := cmd.Flags().GetString("since")
		where, _ := cmd.Flags().GetString("where")
		signals, _ := cmd.Flags().GetStringSlice("signals")
		require, _ := cmd.Flags().GetStringSlice("require")
		staleAfterFlag, _ := cmd.Flags().GetDuration("stale-after")

		sinceExpr, windowLen, err := parseSinceFlag(since)
		if err != nil {
			return err
		}
		if sinceExpr != "" && strings.TrimSpace(where) == "" {
			return fmt.Errorf("--since requires --where: an unscoped windowed count is the most expensive query available, and plain 'dtctl inventory' already answers the unscoped question for free")
		}
		if sinceExpr == "" && (strings.TrimSpace(where) != "" || len(signals) > 0 || len(require) > 0) {
			return fmt.Errorf("--where, --signals, and --require apply to windowed arrival mode only: add --since <duration>")
		}
		staleAfter := staleAfterFlag
		if staleAfter == 0 {
			staleAfter = defaultStaleAfter(windowLen)
		}
		if windowLen > time.Hour {
			fmt.Fprintf(os.Stderr, "warning: a %s window makes each probe scan proportionally more; probes cut short by the scan cap report as unknown, not absent — narrow --since if that happens\n", roundWindow(windowLen))
		}

		noBuiltin, _ := cmd.Flags().GetBool("no-builtin-definitions")
		defFiles, _ := cmd.Flags().GetStringArray("definitions")
		base := inventory.BuiltinDefinitions()
		if noBuiltin {
			base = map[string]*inventory.CapabilityDef{}
		}
		overlays := make([]*inventory.Definitions, 0, len(defFiles))
		for _, f := range defFiles {
			d, derr := loadDefinitionsFile(f)
			if derr != nil {
				return derr
			}
			overlays = append(overlays, d)
		}
		defs := inventory.MergeDefinitions(base, overlays...)

		budgetQueries, _ := cmd.Flags().GetInt("budget-queries")
		budgetSeconds, _ := cmd.Flags().GetFloat64("budget-seconds")
		scanLimitGB, _ := cmd.Flags().GetFloat64("scan-limit-gbytes")

		runner := &inventoryRunner{
			executor:    NewDQLExecutorFromConfig(cfg, c),
			scanLimitGB: scanLimitGB,
		}

		// Segments come from the API, not DQL — fetched here, best-effort. A
		// failure must stay distinguishable from "no segments exist".
		var segs []inventory.SegmentInfo
		var segNote string
		if sinceExpr != "" {
			// Windowed mode reports arrival state for one scope; the segment
			// catalog is neither asked for nor consumed there.
		} else if list, serr := segment.NewHandler(c).List(); serr == nil {
			for _, s := range list.FilterSegments {
				segs = append(segs, inventory.SegmentInfo{UID: s.UID, Name: s.Name, Description: s.Description})
			}
		} else {
			segNote = fmt.Sprintf("segment discovery failed: %v — the segment list is unknown, not empty", serr)
		}

		// Cancel cleanly on Ctrl+C: discovery aborts, nothing is half-reported.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigCh)
		go func() {
			<-sigCh
			cancel()
		}()

		inv, err := inventory.Discover(ctx, runner, defs, inventory.DiscoverOptions{
			ContextName:   cfg.CurrentContext,
			Segments:      segs,
			BudgetQueries: budgetQueries,
			BudgetSeconds: budgetSeconds,
			Since:         sinceExpr,
			Where:         where,
			StaleAfter:    staleAfter,
			Signals:       signals,
		})
		if err != nil {
			return err
		}
		if segNote != "" {
			inv.Notes = append(inv.Notes, segNote)
		}

		if outputFormat == "table" && !agentMode {
			if inv.Window != nil {
				printInventorySignalsHuman(inv)
			} else {
				printInventoryHuman(inv)
			}
			exitForRequiredSignals(inv, require)
			return nil
		}
		printer := NewPrinter()
		if ap := enrichAgent(printer, "inventory", ""); ap != nil {
			ap.SetSuggestions(inventorySuggestions(inv))
		}
		if err := printer.Print(inv); err != nil {
			return err
		}
		exitForRequiredSignals(inv, require)
		return nil
	},
}

// loadDefinitionsFile reads one capability-definitions file. File I/O stays in
// the CLI layer — the SDK parses bytes (ParseDefinitions) and never sees paths.
func loadDefinitionsFile(path string) (*inventory.Definitions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read definitions %s: %w", path, err)
	}
	defs, err := inventory.ParseDefinitions(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return defs, nil
}

// inventoryMaxResultRecords must exceed the largest `| limit` in the discovery
// battery (10000, the metric catalog): the executor's default client cap is
// 1000 records, which silently under-cuts the catalog queries on big tenants —
// on one, the metric catalog lost every dt.* key to the cut and turned live
// metric families into fabricated absences.
const inventoryMaxResultRecords = 20000

// inventoryRunner adapts the DQL executor to the discovery Runner interface.
// Every probe carries the scan cap; queries are tagged for observability.
type inventoryRunner struct {
	executor    *exec.DQLExecutor
	scanLimitGB float64
}

func (r *inventoryRunner) RunQuery(ctx context.Context, dql string) (*inventory.RunResult, error) {
	start := time.Now()
	resp, err := r.executor.ExecuteQueryWithContext(ctx, dql, exec.DQLExecuteOptions{
		DefaultScanLimitGbytes: r.scanLimitGB,
		MaxResultRecords:       inventoryMaxResultRecords,
		ClientContext:          "inventory",
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, context.Canceled
	}
	truncated := false
	for _, n := range resp.GetNotifications() {
		if exec.ResultIsPartial(n) {
			truncated = true
			break
		}
	}
	return &inventory.RunResult{
		Records:   resp.GetRecords(),
		Seconds:   time.Since(start).Seconds(),
		Truncated: truncated,
	}, nil
}

// printInventoryHuman renders the inventory for a terminal.
func printInventoryHuman(inv *inventory.Inventory) {
	const w = 14
	output.DescribeKV("Context:", w, "%s", inv.Context)
	output.DescribeKV("Generated:", w, "%s", inv.GeneratedAt)
	if len(inv.Capabilities) > 0 {
		output.DescribeKV("Capabilities:", w, "%s", strings.Join(inv.Capabilities, ", "))
	}
	if len(inv.Absent) > 0 {
		output.DescribeSection("Absent (what was checked)")
		for _, a := range inv.Absent {
			fmt.Printf("  %s — %s\n", a.Name, a.Evidence)
		}
	}
	if len(inv.Unknown) > 0 {
		output.DescribeSection("Unknown (no verdict — not evidence of absence)")
		for _, u := range inv.Unknown {
			fmt.Printf("  %s — %s\n", u.Name, u.Evidence)
		}
	}
	if len(inv.EntityTypes) > 0 {
		output.DescribeKV("Entities:", w, "%s", topCensusTypes(inv.EntityTypes, 12))
	}
	if len(inv.DataObjects) > 0 {
		line := strings.Join(inv.DataObjects, ", ")
		if inv.EntityViews > 0 {
			line += fmt.Sprintf(" (+%d dt.entity.* lookback views)", inv.EntityViews)
		}
		output.DescribeKV("Data objects:", w, "%s", line)
	}
	if len(inv.QueryOnly) > 0 {
		output.DescribeKV("Query-only:", w, "%s (no fetch — see notes)", strings.Join(inv.QueryOnly, ", "))
	}
	if len(inv.Buckets) > 0 {
		output.DescribeKV("Buckets:", w, "%s", strings.Join(capNames(inv.Buckets, 20), ", "))
	}
	if len(inv.Segments) > 0 {
		output.DescribeSection("Segments (apply with -S <name>)")
		const maxSegments = 10
		for i, s := range inv.Segments {
			if i >= maxSegments {
				fmt.Printf("  (+%d more — full list with -o json)\n", len(inv.Segments)-maxSegments)
				break
			}
			desc := ""
			if s.Description != "" {
				desc = " — " + s.Description
			}
			fmt.Printf("  %s%s\n", s.Name, desc)
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

// capNames limits a name list to n entries for the human view, marking how
// many were cut; the full list stays available via -o json|yaml.
func capNames(names []string, n int) []string {
	if len(names) <= n {
		return names
	}
	capped := append([]string{}, names[:n]...)
	return append(capped, fmt.Sprintf("(+%d more — see -o json)", len(names)-n))
}

// topCensusTypes formats the census as the top-n types by count.
func topCensusTypes(census map[string]int64, n int) string {
	type kv struct {
		k string
		v int64
	}
	entries := make([]kv, 0, len(census))
	for k, v := range census {
		entries = append(entries, kv{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].v != entries[j].v {
			return entries[i].v > entries[j].v
		}
		return entries[i].k < entries[j].k
	})
	var parts []string
	for i, e := range entries {
		if i >= n {
			parts = append(parts, fmt.Sprintf("(+%d more types)", len(entries)-n))
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d", e.k, e.v))
	}
	return strings.Join(parts, " ")
}

func init() {
	rootCmd.AddCommand(inventoryCmd)
	inventoryCmd.Flags().StringArray("definitions", nil, "Capability-definitions file merged over the built-in set (repeatable, later files win)")
	inventoryCmd.Flags().Bool("no-builtin-definitions", false, "Start from an empty capability set instead of the built-in one")
	inventoryCmd.Flags().Int("budget-queries", 100, "Discovery budget: max queries")
	inventoryCmd.Flags().Float64("budget-seconds", 300, "Discovery budget: max cumulative query seconds")
	inventoryCmd.Flags().Float64("scan-limit-gbytes", 25, "Scan cap applied to every discovery probe")
	inventoryCmd.Flags().String("since", "", "Windowed arrival mode: report per-signal ingest state over this window (e.g. 15m, 1h). Requires --where")
	inventoryCmd.Flags().String("where", "", "DQL filter fragment scoping every windowed probe (e.g. 'k8s.namespace.name == \"payments\"')")
	inventoryCmd.Flags().StringSlice("signals", nil, "Restrict windowed probing to these signals (default: all signal streams and metric families)")
	inventoryCmd.Flags().StringSlice("require", nil, "Exit non-zero unless every named signal is live (1 = not live, 2 = no verdict)")
	inventoryCmd.Flags().Duration("stale-after", 0, "Age past which a matched signal is stale rather than live (default max(2m, window/3))")
}
