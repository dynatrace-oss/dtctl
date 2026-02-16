package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show details of a specific resource",
	Long:  `Show detailed information about a specific resource.`,
	RunE:  requireSubcommand,
}

// formatDuration formats seconds into a human-readable duration
func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		m := seconds / 60
		s := seconds % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// formatBytes formats bytes into a human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// printTriggerInfo prints trigger configuration
func printTriggerInfo(trigger map[string]interface{}) {
	if triggerType, ok := trigger["type"].(string); ok {
		fmt.Printf("  Type: %s\n", triggerType)
	}

	// Handle schedule trigger
	if schedule, ok := trigger["schedule"].(map[string]interface{}); ok {
		if rule, exists := schedule["rule"]; exists {
			fmt.Printf("  Schedule: %v\n", rule)
		}
		if tz, exists := schedule["timezone"]; exists {
			fmt.Printf("  Timezone: %v\n", tz)
		}
	}

	// Handle event trigger
	if eventTrigger, ok := trigger["eventTrigger"].(map[string]interface{}); ok {
		if triggerConfig, exists := eventTrigger["triggerConfiguration"].(map[string]interface{}); exists {
			if eventType, exists := triggerConfig["type"]; exists {
				fmt.Printf("  Event Type: %v\n", eventType)
			}
		}
	}
}

func init() {
	rootCmd.AddCommand(describeCmd)
	describeCmd.AddCommand(describeWorkflowCmd)
	describeCmd.AddCommand(describeWorkflowExecutionCmd)
	describeCmd.AddCommand(describeDashboardCmd)
	describeCmd.AddCommand(describeNotebookCmd)
	describeCmd.AddCommand(describeTrashCmd)
	describeCmd.AddCommand(describeBucketCmd)
	describeCmd.AddCommand(describeLookupCmd)
	describeCmd.AddCommand(describeAppCmd)
	describeCmd.AddCommand(describeFunctionCmd)
	describeCmd.AddCommand(describeIntentCmd)
	describeCmd.AddCommand(describeEdgeConnectCmd)
	describeCmd.AddCommand(describeUserCmd)
	describeCmd.AddCommand(describeGroupCmd)
	describeCmd.AddCommand(describeSettingsCmd)
	describeCmd.AddCommand(describeSettingsSchemaCmd)
	describeCmd.AddCommand(describeSLOCmd)
}
