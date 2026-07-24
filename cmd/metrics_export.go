package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/metrics"
)

// metricsCmd groups runtime metrics helpers.
var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Export dtctl runtime metrics",
	Long: `Export the in-memory dtctl metrics collected during the current process.

Metrics collection is disabled by default. Enable it with DTCTL_ENABLE_METRICS=1.`,
	RunE: requireSubcommand,
}

// metricsExportCmd prints the current metrics snapshot.
var metricsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the current metrics snapshot",
	Long: `Print the current dtctl metrics snapshot.

The command emits a JSON document when -o json is selected, otherwise it prints
a human-readable report.`,
	RunE: runMetricsExport,
}

func runMetricsExport(cmd *cobra.Command, args []string) error {
	snapshot := metrics.Default().Snapshot()
	format := outputFormat
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(snapshot)
	}

	fmt.Printf("Metrics enabled: %v\n", snapshot.Enabled)
	fmt.Printf("Total commands: %d\n", snapshot.TotalCommands)
	fmt.Printf("Total command errors: %d\n", snapshot.TotalCommandErrors)
	fmt.Printf("Total API errors: %d\n", snapshot.TotalAPIErrors)

	if len(snapshot.Commands) > 0 {
		fmt.Println()
		fmt.Println("Command metrics:")
		for _, item := range snapshot.Commands {
			fmt.Printf("- %s: count=%d total=%s avg=%s last=%s success=%v\n",
				item.Command, item.Count, item.Total, item.Average, item.Last, item.LastSuccess)
		}
	}

	if len(snapshot.APIErrors) > 0 {
		fmt.Println()
		fmt.Println("API error metrics:")
		for _, item := range snapshot.APIErrors {
			fmt.Printf("- %s: count=%d last_status=%d last_message=%q\n",
				item.Operation, item.Count, item.LastStatus, item.LastMessage)
		}
	}

	if !snapshot.Enabled {
		fmt.Println()
		fmt.Println("Metrics collection is disabled. Set DTCTL_ENABLE_METRICS=1 to record future runs.")
	}

	return nil
}

func init() {
	metricsCmd.AddCommand(metricsExportCmd)
	rootCmd.AddCommand(metricsCmd)
}
