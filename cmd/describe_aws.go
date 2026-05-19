package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
)

var describeAWSConnectionCmd = &cobra.Command{
	Use:     "connection <id-or-name>",
	Aliases: []string{"connections", "awsconn"},
	Short:   "Show details of an AWS connection",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		h := awsconnection.NewHandler(c)
		identifier := args[0]
		item, err := h.FindByName(identifier)
		if err != nil {
			item, err = h.Get(identifier)
			if err != nil {
				return fmt.Errorf("aws connection with name or ID %q not found", identifier)
			}
		}

		if outputFormat == "table" {
			const w = 6
			output.DescribeKV("ID:", w, "%s", item.ObjectID)
			output.DescribeKV("Name:", w, "%s", item.Value.Name)
			output.DescribeKV("Type:", w, "%s", item.Value.Type)
			if item.Value.AwsRoleBasedAuthentication != nil {
				output.DescribeSection("Role-Based Auth Config:")
				output.DescribeKV("  Role ARN:", 14, "%s", item.Value.AwsRoleBasedAuthentication.RoleArn)
				output.DescribeKV("  Consumers:", 14, "%v", item.Value.AwsRoleBasedAuthentication.Consumers)
			}
			return nil
		}

		enrichAgent(printer, "describe", "aws-connection")
		return printer.Print(item)
	},
}

var describeAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring <id-or-name>",
	Aliases: []string{"monitoring-config", "monitoring-configs", "awsmon"},
	Short:   "Show details of an AWS monitoring configuration",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		_, c, printer, err := Setup()
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

		if outputFormat == "table" {
			const w = 13
			output.DescribeKV("ID:", w, "%s", item.ObjectID)
			output.DescribeKV("Description:", w, "%s", item.Value.Description)
			output.DescribeKV("Enabled:", w, "%v", item.Value.Enabled)
			output.DescribeKV("Version:", w, "%s", item.Value.Version)
			output.DescribeSection("AWS Config:")
			output.DescribeKV("  Deployment Region:", 25, "%s", item.Value.Aws.DeploymentRegion)
			output.DescribeKV("  Deployment Scope:", 25, "%s", item.Value.Aws.DeploymentScope)
			output.DescribeKV("  Configuration Mode:", 25, "%s", item.Value.Aws.ConfigurationMode)
			output.DescribeKV("  Deployment Mode:", 25, "%s", item.Value.Aws.DeploymentMode)
			output.DescribeKV("  Smartscape Enabled:", 25, "%v", item.Value.Aws.SmartscapeConfiguration.Enabled)
			output.DescribeKV("  Metrics Enabled:", 25, "%v", item.Value.Aws.MetricsConfiguration.Enabled)
			output.DescribeKV("  Metrics Regions:", 25, "%v", item.Value.Aws.MetricsConfiguration.Regions)
			output.DescribeKV("  CW Logs Enabled:", 25, "%v", item.Value.Aws.CloudWatchLogsConfiguration.Enabled)
			output.DescribeKV("  Region Filtering:", 25, "%v", item.Value.Aws.RegionFiltering)

			if len(item.Value.Aws.TagEnrichment) > 0 {
				output.DescribeKV("  Tag Enrichment:", 25, "%v", item.Value.Aws.TagEnrichment)
			}

			if len(item.Value.Aws.Credentials) > 0 {
				output.DescribeSection("  Credentials:")
				for _, cred := range item.Value.Aws.Credentials {
					output.DescribeKV("    - Description:", 21, "%s", cred.Description)
					output.DescribeKV("      Connection ID:", 21, "%s", cred.ConnectionID)
					output.DescribeKV("      Account ID:", 21, "%s", cred.AccountID)
					output.DescribeKV("      Enabled:", 21, "%v", cred.Enabled)
				}
			}

			if len(item.Value.Aws.Namespaces) > 0 {
				output.DescribeSection("  Custom Namespaces:")
				for _, ns := range item.Value.Aws.Namespaces {
					output.DescribeKV("    - Namespace:", 21, "%s", ns.Namespace)
					for _, m := range ns.Metrics {
						output.DescribeKV("      Metric:", 21, "%s (%s) dims=%v agg=%v", m.Name, m.Unit, m.Dimensions, m.Aggregations)
					}
				}
			}

			printAWSMonitoringConfigStatus(c, item.ObjectID)
			return nil
		}

		enrichAgent(printer, "describe", "aws-monitoring")
		return printer.Print(item)
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
	output.DescribeSection("Status:")

	smartscapeResult, err := executor.ExecuteQuery(smartscapeQuery)
	if err != nil {
		fmt.Printf("  Smartscape updates: query failed (%v)\n", err)
	} else {
		records := exec.ExtractQueryRecords(smartscapeResult)
		if latest, ok := exec.ExtractLatestPointFromTimeseries(records, "sum(dt.sfm.da.aws.smartscape.updates.count)"); ok {
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
		records := exec.ExtractQueryRecords(metricsResult)
		if latest, ok := exec.ExtractLatestPointFromTimeseries(records, "sum(dt.sfm.da.aws.metric.data_points.count)"); ok {
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
	describeAWSProviderCmd.AddCommand(describeAWSConnectionCmd)
	describeAWSProviderCmd.AddCommand(describeAWSMonitoringConfigCmd)
}
