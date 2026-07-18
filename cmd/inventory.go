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
every absent capability.

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

		// Segments come from the API, not DQL — fetched here, best-effort.
		var segs []inventory.SegmentInfo
		if list, serr := segment.NewHandler(c).List(); serr == nil {
			for _, s := range list.FilterSegments {
				segs = append(segs, inventory.SegmentInfo{UID: s.UID, Name: s.Name, Description: s.Description})
			}
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

		if outputFormat == "table" && !agentMode {
			printInventoryHuman(inv)
			return nil
		}
		printer := NewPrinter()
		if ap := enrichAgent(printer, "inventory", ""); ap != nil {
			ap.SetSuggestions([]string{
				"# query any listed data object: dtctl query 'fetch <object> | limit 10'",
				"# absent capabilities carry the evidence checked — cite it instead of re-probing",
			})
		}
		return printer.Print(inv)
	},
}

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
		ClientContext:          "inventory",
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, context.Canceled
	}
	return &inventory.RunResult{
		Records: resp.GetRecords(),
		Seconds: time.Since(start).Seconds(),
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
		output.DescribeSection("Absent (with the evidence checked)")
		for _, a := range inv.Absent {
			fmt.Printf("  %s\n", a)
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
	if len(inv.Unfetchable) > 0 {
		output.DescribeKV("Query-only:", w, "%s (no fetch — see notes)", strings.Join(inv.Unfetchable, ", "))
	}
	if len(inv.Buckets) > 0 {
		output.DescribeKV("Buckets:", w, "%s", strings.Join(inv.Buckets, ", "))
	}
	if len(inv.Segments) > 0 {
		output.DescribeSection("Segments (apply with -S <name>)")
		for _, s := range inv.Segments {
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
	inventoryCmd.Flags().Int("budget-queries", 0, "Discovery budget: max queries (default 100)")
	inventoryCmd.Flags().Float64("budget-seconds", 0, "Discovery budget: max cumulative query seconds (default 300)")
	inventoryCmd.Flags().Float64("scan-limit-gbytes", 25, "Scan cap applied to every discovery probe")
}
