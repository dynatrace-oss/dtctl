package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var (
	updateAWSConnectionName    string
	updateAWSConnectionRoleArn string

	updateAWSMonitoringConfigName        string
	updateAWSMonitoringConfigRegions     string
	updateAWSMonitoringConfigFeatureSets string
)

var updateAWSConnectionCmd = &cobra.Command{
	Use:     "connection [id]",
	Aliases: []string{"connections"},
	Short:   "Update AWS connection from flags",
	Long: `Update AWS connection by ID argument or by --name.

Examples:
  dtctl update aws connection --name "my-aws-connection" --roleArn "arn:aws:iam::123456789012:role/dynatrace-monitoring"
  dtctl update aws connection <id> --roleArn "arn:aws:iam::123456789012:role/dynatrace-monitoring"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateAWSConnectionRoleArn == "" {
			return fmt.Errorf("--roleArn is required")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsconnection.NewHandler(c)

		var existing *awsconnection.AWSConnection
		if len(args) > 0 {
			existing, err = handler.Get(args[0])
			if err != nil {
				return err
			}
		} else {
			if updateAWSConnectionName == "" {
				return fmt.Errorf("provide connection ID argument or --name")
			}
			existing, err = handler.FindByName(updateAWSConnectionName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		if value.Type == "" {
			value.Type = "awsRoleBasedAuthentication"
		}
		if value.AWSRoleBasedAuthentication == nil {
			value.AWSRoleBasedAuthentication = &awsconnection.AWSRoleBasedAuthentication{
				Consumers: []string{"SVC:com.dynatrace.da"},
			}
		}
		if len(value.AWSRoleBasedAuthentication.Consumers) == 0 {
			value.AWSRoleBasedAuthentication.Consumers = []string{"SVC:com.dynatrace.da"}
		}
		value.AWSRoleBasedAuthentication.RoleArn = updateAWSConnectionRoleArn

		updated, err := handler.Update(existing.ObjectID, value)
		if err != nil {
			return err
		}

		fmt.Printf("AWS connection updated: %s\n", updated.ObjectID)
		return nil
	},
}

var updateAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Update AWS monitoring config from flags",
	Long: `Update AWS monitoring configuration by ID argument or by --name.

Examples:
  dtctl update aws monitoring --name "my-monitoring" --regionFiltering "eu-west-1,us-east-1"
  dtctl update aws monitoring --name "my-monitoring" --featureSets "EC2_essential,S3_essential"
  dtctl update aws monitoring <id> --regionFiltering "eu-west-1,us-east-1"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(updateAWSMonitoringConfigRegions) == "" && strings.TrimSpace(updateAWSMonitoringConfigFeatureSets) == "" {
			return fmt.Errorf("at least one of --regionFiltering or --featureSets is required")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsmonitoringconfig.NewHandler(c)

		var existing *awsmonitoringconfig.AWSMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]
			existing, err = handler.FindByName(identifier)
			if err != nil {
				existing, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
			}
		} else {
			if updateAWSMonitoringConfigName == "" {
				return fmt.Errorf("provide config ID argument or --name")
			}
			existing, err = handler.FindByName(updateAWSMonitoringConfigName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		if strings.TrimSpace(updateAWSMonitoringConfigRegions) != "" {
			regions := awsmonitoringconfig.SplitCSV(updateAWSMonitoringConfigRegions)
			if len(regions) == 0 {
				return fmt.Errorf("--regionFiltering must contain at least one region")
			}
			value.AWS.RegionFiltering = regions
			value.AWS.DeploymentRegion = regions[0]
			if value.AWS.MetricsConfiguration.Enabled {
				value.AWS.MetricsConfiguration.Regions = regions
			}
			if value.AWS.CloudWatchLogsConfiguration.Enabled {
				value.AWS.CloudWatchLogsConfiguration.Regions = regions
			}
			if value.AWS.EventsConfiguration.Enabled {
				value.AWS.EventsConfiguration.Regions = regions
			}
		}
		if strings.TrimSpace(updateAWSMonitoringConfigFeatureSets) != "" {
			featureSets := awsmonitoringconfig.SplitCSV(updateAWSMonitoringConfigFeatureSets)
			if len(featureSets) == 0 {
				return fmt.Errorf("--featureSets must contain at least one feature set")
			}
			value.FeatureSets = featureSets
		}

		payload := awsmonitoringconfig.AWSMonitoringConfig{Scope: existing.Scope, Value: value}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := handler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		fmt.Printf("AWS monitoring config updated: %s\n", updated.ObjectID)
		return nil
	},
}

func init() {
	updateAWSProviderCmd.AddCommand(updateAWSConnectionCmd)
	updateAWSProviderCmd.AddCommand(updateAWSMonitoringConfigCmd)

	updateAWSConnectionCmd.Flags().StringVar(&updateAWSConnectionName, "name", "", "AWS connection name (used when ID argument is not provided)")
	updateAWSConnectionCmd.Flags().StringVar(&updateAWSConnectionRoleArn, "roleArn", "", "AWS IAM role ARN to set")
	updateAWSConnectionCmd.Flags().StringVar(&updateAWSConnectionRoleArn, "rolearn", "", "Alias for --roleArn")

	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigRegions, "regionFiltering", "", "Comma-separated AWS regions")
	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets")
	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
}
