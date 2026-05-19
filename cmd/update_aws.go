package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
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
	Long: `Patch an existing AWS connection. Currently supports updating the IAM role ARN.

Examples:
  dtctl update aws connection --name "my-aws" --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole
  dtctl update aws connection <id> --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateAWSConnectionRoleArn == "" {
			return fmt.Errorf("--roleArn is required")
		}
		if err := awsconnection.ValidateRoleArn(updateAWSConnectionRoleArn); err != nil {
			return err
		}

		_, c, err := SetupWithSafety(safety.OperationUpdate)
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
		if value.Type != awsconnection.TypeRoleBased {
			return fmt.Errorf("unsupported aws connection type %q", value.Type)
		}
		if value.AwsRoleBasedAuthentication == nil {
			value.AwsRoleBasedAuthentication = &awsconnection.AwsRoleBasedAuthenticationConfig{
				Consumers: []string{awsconnection.DefaultConsumer},
			}
		}
		value.AwsRoleBasedAuthentication.RoleArn = updateAWSConnectionRoleArn
		if len(value.AwsRoleBasedAuthentication.Consumers) == 0 {
			value.AwsRoleBasedAuthentication.Consumers = []string{awsconnection.DefaultConsumer}
		}

		updated, err := handler.Update(existing.ObjectID, value)
		if err != nil {
			return err
		}

		output.PrintSuccess("AWS connection updated: %s", updated.ObjectID)
		return nil
	},
}

var updateAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Update AWS monitoring config from flags",
	Long: `Update an AWS monitoring configuration by ID argument or by --name.

Examples:
  dtctl update aws monitoring --name "my-aws" --regions us-east-1,eu-central-1
  dtctl update aws monitoring --name "my-aws" --featureSets EC2_essential,RDS_essential`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(updateAWSMonitoringConfigRegions) == "" &&
			strings.TrimSpace(updateAWSMonitoringConfigFeatureSets) == "" {
			return fmt.Errorf("at least one of --regions or --featureSets is required")
		}

		_, c, err := SetupWithSafety(safety.OperationUpdate)
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
			regions, err := awsmonitoringconfig.ParseRequiredRegions(updateAWSMonitoringConfigRegions)
			if err != nil {
				return err
			}
			value.Aws.MetricsConfiguration.Regions = regions
			value.Aws.RegionFiltering = regions
			value.Aws.CloudWatchLogsConfiguration.Regions = regions
			if value.Aws.DeploymentRegion == "" {
				value.Aws.DeploymentRegion = regions[0]
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

		output.PrintSuccess("AWS monitoring config updated: %s", updated.ObjectID)
		return nil
	},
}

func init() {
	updateAWSProviderCmd.AddCommand(updateAWSConnectionCmd)
	updateAWSProviderCmd.AddCommand(updateAWSMonitoringConfigCmd)

	updateAWSConnectionCmd.Flags().StringVar(&updateAWSConnectionName, "name", "", "AWS connection name (used when ID argument is not provided)")
	updateAWSConnectionCmd.Flags().StringVar(&updateAWSConnectionRoleArn, "roleArn", "", "AWS IAM role ARN (required)")

	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigRegions, "regions", "", "Comma-separated AWS regions")
	updateAWSMonitoringConfigCmd.Flags().StringVar(&updateAWSMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets")
}
