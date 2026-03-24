package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
)

var describeGCPConnectionCmd = &cobra.Command{
	Use:     "connection <id>",
	Aliases: []string{"connections", "gcpconn"},
	Short:   "Show details of a GCP connection",
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

		h := gcpconnection.NewHandler(c)
		item, err := h.Get(args[0])
		if err != nil {
			return err
		}

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 6
			output.DescribeKV("ID:", w, "%s", item.ObjectID)
			output.DescribeKV("Name:", w, "%s", item.Value.Name)
			output.DescribeKV("Type:", w, "%s", item.Value.Type)
			if item.Value.ServiceAccountImpersonation != nil {
				output.DescribeSection("Service Account Impersonation:")
				output.DescribeKV("  Service Account ID:", 23, "%s", item.Value.ServiceAccountImpersonation.ServiceAccountID)
				output.DescribeKV("  Consumers:", 23, "%v", item.Value.ServiceAccountImpersonation.Consumers)
			}

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "gcp-connection")
		return printer.Print(item)
	},
}

var describeGCPMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring <id-or-name>",
	Aliases: []string{"monitoring-config", "monitoring-configs", "gcpmon"},
	Short:   "Show details of a GCP monitoring configuration",
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

		h := gcpmonitoringconfig.NewHandler(c)
		connHandler := gcpconnection.NewHandler(c)

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

		// For table output, show detailed human-readable information
		if outputFormat == "table" {
			const w = 13
			output.DescribeKV("ID:", w, "%s", item.ObjectID)
			output.DescribeKV("Description:", w, "%s", item.Value.Description)
			output.DescribeKV("Enabled:", w, "%v", item.Value.Enabled)
			output.DescribeKV("Version:", w, "%s", item.Value.Version)
			output.DescribeSection("Google Cloud Config:")
			output.DescribeKV("  Location Filtering:", 23, "%v", item.Value.GoogleCloud.LocationFiltering)
			output.DescribeKV("  Project Filtering:", 23, "%v", item.Value.GoogleCloud.ProjectFiltering)
			output.DescribeKV("  Folder Filtering:", 23, "%v", item.Value.GoogleCloud.FolderFiltering)
			output.DescribeKV("  Feature Sets:", 23, "%v", item.Value.FeatureSets)

			if len(item.Value.GoogleCloud.Credentials) > 0 {
				output.DescribeSection("  Credentials:")
				for _, cred := range item.Value.GoogleCloud.Credentials {
					output.DescribeKV("    - Description:", 23, "%s", cred.Description)
					output.DescribeKV("      Connection ID:", 23, "%s", cred.ConnectionID)
					output.DescribeKV("      Service Account:", 23, "%s", cred.ServiceAccount)
				}
			}

			if principal, principalErr := connHandler.GetDynatracePrincipal(); principalErr == nil {
				output.DescribeSection("Dynatrace:")
				output.DescribeKV("  Principal ID:", 17, "%s", principal.ObjectID)
				if principal.Principal != "" {
					output.DescribeKV("  Principal:", 17, "%s", principal.Principal)
				}
			}

			printGCPMonitoringConfigStatus(c, item.ObjectID)

			return nil
		}

		// For other formats, use standard printer
		printer := NewPrinter()
		enrichAgent(printer, "describe", "gcp-monitoring")
		return printer.Print(item)
	},
}

func printGCPMonitoringConfigStatus(c *client.Client, configID string) {
	executor := exec.NewDQLExecutor(c)

	smartscapeQuery := fmt.Sprintf(`timeseries sum(dt.sfm.da.gcp.smartscape.updates.count), interval:1h, by:{dt.config.id}
| filter dt.config.id == %q`, configID)
	metricsQuery := fmt.Sprintf(`timeseries sum(dt.sfm.da.gcp.metric.data_points.count), interval:1h, by:{dt.config.id}
| filter dt.config.id == %q`, configID)
	eventsQuery := fmt.Sprintf(`fetch dt.system.events
| filter event.kind == "DATA_ACQUISITION_EVENT"
| filter da.clouds.configurationId == %q
| sort timestamp desc
| limit 100`, configID)

	fmt.Println()
	output.DescribeSection("Status:")

	smartscapeResult, err := executor.ExecuteQuery(smartscapeQuery)
	if err != nil {
		fmt.Printf("  Smartscape updates: query failed (%v)\n", err)
	} else {
		smartscapeRecords := exec.ExtractQueryRecords(smartscapeResult)
		smartscapeMetricConfigID := configID
		for _, rec := range smartscapeRecords {
			candidate := stringFromRecord(rec, "dt.config.id")
			if candidate != "" {
				smartscapeMetricConfigID = candidate
				break
			}
		}
		fmt.Printf("  Smartscape metric config ID: %s\n", smartscapeMetricConfigID)
		if latest, ok := exec.ExtractLatestPointFromTimeseries(smartscapeRecords, "sum(dt.sfm.da.gcp.smartscape.updates.count)"); ok {
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
		if latest, ok := exec.ExtractLatestPointFromTimeseries(metricsRecords, "sum(dt.sfm.da.gcp.metric.data_points.count)"); ok {
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
	output.DescribeSection("Recent events:")
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

func init() {
	describeGCPProviderCmd.AddCommand(describeGCPConnectionCmd)
	describeGCPProviderCmd.AddCommand(describeGCPMonitoringConfigCmd)
}
