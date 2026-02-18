package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/spf13/cobra"
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

		fmt.Printf("ID:   %s\n", item.ObjectID)
		fmt.Printf("Name: %s\n", item.Value.Name)
		fmt.Printf("Type: %s\n", item.Value.Type)
		if item.Value.ServiceAccountImpersonation != nil {
			fmt.Println("Service Account Impersonation:")
			fmt.Printf("  Service Account ID: %s\n", item.Value.ServiceAccountImpersonation.ServiceAccountID)
			fmt.Printf("  Consumers:          %v\n", item.Value.ServiceAccountImpersonation.Consumers)
		}

		return nil
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

		fmt.Printf("ID:          %s\n", item.ObjectID)
		fmt.Printf("Description: %s\n", item.Value.Description)
		fmt.Printf("Enabled:     %v\n", item.Value.Enabled)
		fmt.Printf("Version:     %s\n", item.Value.Version)
		fmt.Println("Google Cloud Config:")
		fmt.Printf("  Location Filtering: %v\n", item.Value.GoogleCloud.LocationFiltering)
		fmt.Printf("  Project Filtering:  %v\n", item.Value.GoogleCloud.ProjectFiltering)
		fmt.Printf("  Folder Filtering:   %v\n", item.Value.GoogleCloud.FolderFiltering)
		fmt.Printf("  Feature Sets:       %v\n", item.Value.FeatureSets)

		if len(item.Value.GoogleCloud.Credentials) > 0 {
			fmt.Println("  Credentials:")
			for _, cred := range item.Value.GoogleCloud.Credentials {
				fmt.Printf("    - Description:     %s\n", cred.Description)
				fmt.Printf("      Connection ID:   %s\n", cred.ConnectionID)
				fmt.Printf("      Service Account: %s\n", cred.ServiceAccount)
			}
		}

		if principal, principalErr := connHandler.GetDynatracePrincipal(); principalErr == nil {
			fmt.Println("Dynatrace:")
			fmt.Printf("  Principal ID: %s\n", principal.ObjectID)
			if principal.Principal != "" {
				fmt.Printf("  Principal:    %s\n", principal.Principal)
			}
		}

		printGCPMonitoringConfigStatus(c, item.ObjectID)

		return nil
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
	fmt.Println("Status:")

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

func init() {
	describeGCPProviderCmd.AddCommand(describeGCPConnectionCmd)
	describeGCPProviderCmd.AddCommand(describeGCPMonitoringConfigCmd)
}
