package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show details of a specific resource",
	Long:  `Show detailed information about a specific resource.`,
	RunE:  requireSubcommand,
}

var describeAzureProviderCmd = &cobra.Command{
	Use:   "azure",
	Short: "Describe Azure resources",
	RunE:  requireSubcommand,
}

var describeAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Describe AWS resources",
	RunE:  requireSubcommand,
}

var describeGCPProviderCmd = &cobra.Command{
	Use:   "gcp",
	Short: "Describe GCP resources",
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

	if schedule, ok := trigger["schedule"].(map[string]interface{}); ok {
		if rule, exists := schedule["rule"]; exists {
			fmt.Printf("  Schedule: %v\n", rule)
		}
		if tz, exists := schedule["timezone"]; exists {
			fmt.Printf("  Timezone: %v\n", tz)
		}
	}

	if eventTrigger, ok := trigger["eventTrigger"].(map[string]interface{}); ok {
		if triggerConfig, exists := eventTrigger["triggerConfiguration"].(map[string]interface{}); exists {
			if eventType, exists := triggerConfig["type"]; exists {
				fmt.Printf("  Event Type: %v\n", eventType)
			}
		}
	}
}

// describeAzureConnectionCmd shows details of an Azure connection (credential)
var describeAzureConnectionCmd = &cobra.Command{
	Use:     "connection <id>",
	Aliases: []string{"connections", "azconn"},
	Short:   "Show details of an Azure connection (credential)",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		h := azureconnection.NewHandler(c)
		item, err := h.Get(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("ID:   %s\n", item.ObjectID)
		fmt.Printf("Name: %s\n", item.Value.Name)
		fmt.Printf("Type: %s\n", item.Value.Type)

		if item.Value.ClientSecret != nil {
			fmt.Println("Client Secret Config:")
			fmt.Printf("  Application ID: %s\n", item.Value.ClientSecret.ApplicationID)
			fmt.Printf("  Directory ID:   %s\n", item.Value.ClientSecret.DirectoryID)
			fmt.Printf("  Consumers:      %v\n", item.Value.ClientSecret.Consumers)
		}

		if item.Value.FederatedIdentityCredential != nil {
			fmt.Println("Federated Identity Config:")
			fmt.Printf("  Consumers: %v\n", item.Value.FederatedIdentityCredential.Consumers)
		}

		return nil
	},
}

// describeAzureMonitoringConfigCmd shows details of an Azure monitoring configuration
var describeAzureMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring <id-or-name>",
	Aliases: []string{"monitoring-config", "monitoring-configs", "azmon"},
	Short:   "Show details of an Azure monitoring configuration",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		h := azuremonitoringconfig.NewHandler(c)

		item, err := h.FindByName(identifier)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				item, err = h.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
			} else {
				return err
			}
		}

		fmt.Printf("ID:          %s\n", item.ObjectID)
		fmt.Printf("Description: %s\n", item.Value.Description)
		fmt.Printf("Enabled:     %v\n", item.Value.Enabled)
		fmt.Printf("Version:     %s\n", item.Value.Version)
		fmt.Println("Azure Config:")
		fmt.Printf("  Deployment Scope:            %s\n", item.Value.Azure.DeploymentScope)
		fmt.Printf("  Subscription Filtering Mode: %s\n", item.Value.Azure.SubscriptionFilteringMode)
		fmt.Printf("  Configuration Mode:          %s\n", item.Value.Azure.ConfigurationMode)
		fmt.Printf("  Deployment Mode:             %s\n", item.Value.Azure.DeploymentMode)

		if len(item.Value.Azure.Credentials) > 0 {
			fmt.Println("  Credentials:")
			for _, cred := range item.Value.Azure.Credentials {
				fmt.Printf("    - Description:   %s\n", cred.Description)
				fmt.Printf("      Connection ID: %s\n", cred.ConnectionId)
				fmt.Printf("      Type:          %s\n", cred.Type)
			}
		}

		printAzureMonitoringConfigStatus(c, item.ObjectID)

		return nil
	},
}

func printAzureMonitoringConfigStatus(c *client.Client, configID string) {
	executor := exec.NewDQLExecutor(c)

	smartscapeQuery := fmt.Sprintf(`timeseries sum(dt.sfm.da.azure.smartscape.updates.count), interval:1h, by:{dt.config.id}
| filter dt.config.id == %q`, configID)
	metricsQuery := fmt.Sprintf(`timeseries sum(dt.sfm.da.azure.metric.data_points.count), interval:1h, by:{dt.config.id}
| filter dt.config.id == %q`, configID)
	eventsQuery := fmt.Sprintf(`fetch dt.system.events
| filter event.kind == "DATA_ACQUISITION_EVENT"
| filter da.clouds.configurationId == %q
| sort timestamp desc
| limit 100`, configID)

	fmt.Println()
	fmt.Println("Status:")

	smartscapeResult, err := executor.ExecuteQuery(smartscapeQuery)
	if err != nil {
		fmt.Printf("  Smartscape updates: query failed (%v)\n", err)
	} else {
		smartscapeRecords := exec.ExtractQueryRecords(smartscapeResult)
		if latest, ok := exec.ExtractLatestPointFromTimeseries(smartscapeRecords, "sum(dt.sfm.da.azure.smartscape.updates.count)"); ok {
			if !latest.Timestamp.IsZero() {
				fmt.Printf("  Smartscape updates (latest sum, 1h): %.2f at %s\n", latest.Value, latest.Timestamp.Format(time.RFC3339))
			} else {
				fmt.Printf("  Smartscape updates (latest sum, 1h): %.2f\n", latest.Value)
			}
		} else {
			fmt.Println("  Smartscape updates: no data")
		}
	}

	metricsResult, err := executor.ExecuteQuery(metricsQuery)
	if err != nil {
		fmt.Printf("  Metrics ingest: query failed (%v)\n", err)
	} else {
		metricsRecords := exec.ExtractQueryRecords(metricsResult)
		if latest, ok := exec.ExtractLatestPointFromTimeseries(metricsRecords, "sum(dt.sfm.da.azure.metric.data_points.count)"); ok {
			if !latest.Timestamp.IsZero() {
				fmt.Printf("  Metrics ingest (latest sum, 1h): %.2f at %s\n", latest.Value, latest.Timestamp.Format(time.RFC3339))
			} else {
				fmt.Printf("  Metrics ingest (latest sum, 1h): %.2f\n", latest.Value)
			}
		} else {
			fmt.Println("  Metrics ingest: no data")
		}
	}

	eventsResult, err := executor.ExecuteQuery(eventsQuery)
	if err != nil {
		fmt.Printf("  Events: query failed (%v)\n", err)
		return
	}

	eventRecords := exec.ExtractQueryRecords(eventsResult)
	if len(eventRecords) == 0 {
		fmt.Println("  Events: no recent data acquisition events")
		return
	}

	latestStatus := stringFromRecord(eventRecords[0], "da.clouds.status")
	if latestStatus == "" {
		latestStatus = "UNKNOWN"
	}
	fmt.Printf("  Latest event status: %s\n", latestStatus)

	fmt.Println()
	fmt.Println("Recent events:")
	fmt.Printf("%-35s  %s\n", "TIMESTAMP", "DA.CLOUDS.CONTENT")
	for _, rec := range eventRecords {
		timestamp := stringFromRecord(rec, "timestamp")
		content := stringFromRecord(rec, "da.clouds.content")
		if content == "" {
			content = "-"
		}
		fmt.Printf("%-35s  %s\n", timestamp, content)
	}
}

func stringFromRecord(record map[string]interface{}, key string) string {
	if record == nil {
		return ""
	}
	value, ok := record[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

func init() {
	describeCmd.AddCommand(describeAzureProviderCmd)
	describeCmd.AddCommand(describeAWSProviderCmd)
	describeCmd.AddCommand(describeGCPProviderCmd)
	describeAzureProviderCmd.AddCommand(describeAzureConnectionCmd)
	describeAzureProviderCmd.AddCommand(describeAzureMonitoringConfigCmd)
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
