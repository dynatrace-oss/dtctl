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

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/segment"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
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

These verdicts are retention-scoped: a stream that received data once last week
reads as present. To ask whether data is arriving for one source right now — after
an instrumentation change or an ingest — use 'dtctl inventory arrivals'.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		defs, err := inventoryDefinitions(cmd)
		if err != nil {
			return err
		}
		runner := newInventoryRunner(cmd, cfg, c)

		// Segments come from the API, not DQL — fetched here, best-effort. A
		// failure must stay distinguishable from "no segments exist".
		var segs []inventory.SegmentInfo
		var segNote string
		if list, serr := segment.NewHandler(c).List(); serr == nil {
			for _, sg := range list.FilterSegments {
				segs = append(segs, inventory.SegmentInfo{UID: sg.UID, Name: sg.Name, Description: sg.Description})
			}
		} else {
			segNote = fmt.Sprintf("segment discovery failed: %v — the segment list is unknown, not empty", serr)
		}

		ctx, cancel := inventoryCancelContext()
		defer cancel()

		budgetQueries, budgetSeconds := inventoryBudget(cmd)
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
			ap.SetSuggestions(inventorySuggestions(inv))
		}
		return printer.Print(inv)
	},
}

// loadDefinitionsFile reads one capability-definitions file. File I/O stays in
// the CLI layer — the SDK parses bytes (ParseDefinitions) and never sees paths.
func loadDefinitionsFile(path string) (*inventory.Definitions, error) {
	data, err := vfs.ReadFile(path)
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
	var cause inventory.TruncationCause
	for _, n := range resp.GetNotifications() {
		if c := truncationCause(exec.PartialCause(n)); c != "" {
			cause = c
			break
		}
	}
	return &inventory.RunResult{
		Records:         resp.GetRecords(),
		Seconds:         time.Since(start).Seconds(),
		Truncated:       cause != "",
		TruncationCause: cause,
		ColumnTypes:     flattenColumnTypes(resp.GetTypes()),
	}, nil
}

// truncationCause maps a dtctl notification class onto the SDK's cause. The
// two enums happen to share their string values, but they belong to separate
// modules and are mapped explicitly so that a rename on either side is a
// compile error rather than a silently generic evidence line.
func truncationCause(c string) inventory.TruncationCause {
	switch c {
	case exec.PartialScanLimit:
		return inventory.TruncationScanLimit
	case exec.PartialResultLimit:
		return inventory.TruncationResultLimit
	case exec.PartialTimeout:
		return inventory.TruncationTimeout
	case exec.PartialConsumption:
		return inventory.TruncationConsumption
	}
	return ""
}

// flattenColumnTypes merges the API's per-index-range type blocks into one
// column→type map. Arrival probes use it to tell a field the data does not
// have ("undefined") from one that exists and is empty, so a range that does
// know a column's type must win over one that does not.
func flattenColumnTypes(blocks []exec.ColumnTypes) map[string]string {
	if len(blocks) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, b := range blocks {
		for col, t := range b.Mappings {
			if prev, ok := out[col]; ok && prev != inventory.TypeUndefined {
				continue
			}
			out[col] = t.Type
		}
	}
	return out
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

// addInventoryDiscoveryFlags registers the flags every discovery run takes,
// whichever question it is answering.
func addInventoryDiscoveryFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("definitions", nil, "Capability-definitions file merged over the built-in set (repeatable, later files win)")
	cmd.Flags().Bool("no-builtin-definitions", false, "Start from an empty capability set instead of the built-in one")
	cmd.Flags().Int("budget-queries", 100, "Discovery budget: max queries")
	cmd.Flags().Float64("budget-seconds", 300, "Discovery budget: max cumulative query seconds")
	cmd.Flags().Float64("scan-limit-gbytes", 25, "Scan cap applied to every discovery probe")
}

// inventoryDefinitions builds the capability set a run evaluates.
func inventoryDefinitions(cmd *cobra.Command) (map[string]*inventory.CapabilityDef, error) {
	noBuiltin, _ := cmd.Flags().GetBool("no-builtin-definitions")
	defFiles, _ := cmd.Flags().GetStringArray("definitions")
	base := inventory.BuiltinDefinitions()
	if noBuiltin {
		base = map[string]*inventory.CapabilityDef{}
	}
	overlays := make([]*inventory.Definitions, 0, len(defFiles))
	for _, f := range defFiles {
		d, err := loadDefinitionsFile(f)
		if err != nil {
			return nil, err
		}
		overlays = append(overlays, d)
	}
	return inventory.MergeDefinitions(base, overlays...), nil
}

func inventoryBudget(cmd *cobra.Command) (int, float64) {
	queries, _ := cmd.Flags().GetInt("budget-queries")
	seconds, _ := cmd.Flags().GetFloat64("budget-seconds")
	return queries, seconds
}

func newInventoryRunner(cmd *cobra.Command, cfg *config.Config, c *client.Client) *inventoryRunner {
	scanLimitGB, _ := cmd.Flags().GetFloat64("scan-limit-gbytes")
	return &inventoryRunner{
		executor:    NewDQLExecutorFromConfig(cfg, c),
		scanLimitGB: scanLimitGB,
	}
}

// inventoryCancelContext cancels discovery cleanly on Ctrl+C, so a run aborts
// rather than half-reporting.
func inventoryCancelContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	return ctx, func() {
		signal.Stop(sigCh)
		cancel()
	}
}

func init() {
	rootCmd.AddCommand(inventoryCmd)
	addInventoryDiscoveryFlags(inventoryCmd)
	inventoryCmd.AddCommand(inventoryArrivalsCmd)
}
