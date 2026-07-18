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
	"github.com/dynatrace-oss/dtctl/pkg/inventory"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/segment"
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
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		noBuiltin, _ := cmd.Flags().GetBool("no-builtin-definitions")
		defFiles, _ := cmd.Flags().GetStringArray("definitions")
		base := inventory.BuiltinDefinitions()
		if noBuiltin {
			base = map[string]*inventory.CapabilityDef{}
		}
		overlays := make([]*inventory.Definitions, 0, len(defFiles))
		for _, f := range defFiles {
			d, derr := inventory.LoadDefinitionsFile(f)
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
		if list, serr := segment.NewHandler(c).List(); serr == nil {
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
		})
		if err != nil {
			return err
		}
		if segNote != "" {
			inv.Notes = append(inv.Notes, segNote)
		}

		if outputFormat == "table" && !agentMode {
			printInventoryHuman(inv)
			return nil
		}
		printer := NewPrinter()
		if ap := enrichAgent(printer, "inventory", ""); ap != nil {
			ap.SetSuggestions([]string{
				"Run 'dtctl query \"fetch <object> | limit 10\"' to sample any listed data object",
				"Cite the evidence carried by absent capabilities instead of re-probing; unknown capabilities got no verdict and may still exist",
			})
		}
		return printer.Print(inv)
	},
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
		// dt.entity.* views can number in the hundreds; the census above already
		// says which entities exist, so compress them to a count here. The full
		// list stays available via -o json|yaml.
		var core []string
		entityViews := 0
		for _, o := range inv.DataObjects {
			if strings.HasPrefix(o, "dt.entity.") {
				entityViews++
				continue
			}
			core = append(core, o)
		}
		line := strings.Join(core, ", ")
		if entityViews > 0 {
			line += fmt.Sprintf(" (+%d dt.entity.* lookback views)", entityViews)
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
}
