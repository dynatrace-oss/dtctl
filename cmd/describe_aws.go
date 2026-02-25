package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/spf13/cobra"
)

var describeAWSConnectionCmd = &cobra.Command{
	Use:     "connection <id>",
	Aliases: []string{"connections", "awsconn"},
	Short:   "Show details of an AWS connection",
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

		h := awsconnection.NewHandler(c)
		item, err := h.Get(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("ID:   %s\n", item.ObjectID)
		fmt.Printf("Name: %s\n", item.Value.Name)
		fmt.Printf("Type: %s\n", item.Value.Type)
		if item.Value.AWSRoleBasedAuthentication != nil {
			fmt.Println("AWS Role Based Authentication:")
			fmt.Printf("  Role ARN:    %s\n", item.Value.AWSRoleBasedAuthentication.RoleArn)
			fmt.Printf("  External ID: %s\n", item.Value.AWSRoleBasedAuthentication.ExternalID)
			fmt.Printf("  Consumers:   %v\n", item.Value.AWSRoleBasedAuthentication.Consumers)
		}

		return nil
	},
}

var describeAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring <id-or-name>",
	Aliases: []string{"monitoring-config", "monitoring-configs", "awsmon"},
	Short:   "Show details of an AWS monitoring configuration",
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

		h := awsmonitoringconfig.NewHandler(c)

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
		fmt.Println("AWS Config:")
		fmt.Printf("  Deployment Region: %s\n", item.Value.AWS.DeploymentRegion)
		fmt.Printf("  Region Filtering:  %v\n", item.Value.AWS.RegionFiltering)
		fmt.Printf("  Deployment Scope:  %s\n", item.Value.AWS.DeploymentScope)
		fmt.Printf("  Deployment Mode:   %s\n", item.Value.AWS.DeploymentMode)
		fmt.Printf("  Config Mode:       %s\n", item.Value.AWS.ConfigurationMode)
		fmt.Printf("  Feature Sets:      %v\n", item.Value.FeatureSets)

		if len(item.Value.AWS.Credentials) > 0 {
			fmt.Println("  Credentials:")
			for _, cred := range item.Value.AWS.Credentials {
				fmt.Printf("    - Description:   %s\n", cred.Description)
				fmt.Printf("      Connection ID: %s\n", cred.ConnectionID)
				fmt.Printf("      Account ID:    %s\n", cred.AccountID)
			}
		}

		printAWSMonitoringConfigStatus(c, item.ObjectID)

		return nil
	},
}

func printAWSMonitoringConfigStatus(c *client.Client, configID string) {
	executor := exec.NewDQLExecutor(c)

	smartscapeQuery := fmt.Sprintf(`timeseries sum(dt.sfm.da.aws.smartscape.updates.count), interval:1h, by:{dt.config.id}
| filter dt.config.id == %q`, configID)
	metricsQuery := fmt.Sprintf(`timeseries sum(dt.sfm.da.aws.metric.data_points.count), interval:1h, by:{dt.config.id}
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
		if latest, ok := exec.ExtractLatestPointFromTimeseries(smartscapeRecords, "sum(dt.sfm.da.aws.smartscape.updates.count)"); ok {
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
		if latest, ok := exec.ExtractLatestPointFromTimeseries(metricsRecords, "sum(dt.sfm.da.aws.metric.data_points.count)"); ok {
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
	describeAWSProviderCmd.AddCommand(describeAWSConnectionCmd)
	describeAWSProviderCmd.AddCommand(describeAWSMonitoringConfigCmd)
}
