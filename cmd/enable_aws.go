package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var (
	enableAWSMonitoringName    string
	enableAWSMonitoringRoleArn string
)

var enableAWSProviderCmd = &cobra.Command{
	Use:   "aws",
	Short: "Enable AWS resources",
	RunE:  requireSubcommand,
}

var enableAWSMonitoringCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Enable AWS monitoring configuration",
	Long: `Enable an AWS monitoring configuration by optionally patching the linked connection's
roleArn and then enabling the monitoring config in a single step.

Examples:
  dtctl enable aws monitoring --name "my-aws" --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole
  dtctl enable aws monitoring <id> --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole
  dtctl enable aws monitoring --name "my-aws"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && enableAWSMonitoringName == "" {
			return fmt.Errorf("provide monitoring config ID argument or --name")
		}
		if enableAWSMonitoringRoleArn != "" {
			if err := awsconnection.ValidateRoleArn(enableAWSMonitoringRoleArn); err != nil {
				return err
			}
		}

		if dryRun {
			name := enableAWSMonitoringName
			if len(args) > 0 {
				name = args[0]
			}
			output.PrintInfo("Dry run: would resolve AWS monitoring config %q", name)
			if enableAWSMonitoringRoleArn != "" {
				output.PrintInfo("Dry run: would update linked AWS connection roleArn=%q", enableAWSMonitoringRoleArn)
			}
			output.PrintInfo("Dry run: would enable monitoring config and all credentials")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		monitoringHandler := awsmonitoringconfig.NewHandler(c)
		connectionHandler := awsconnection.NewHandler(c)

		var existing *awsmonitoringconfig.AWSMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]
			existing, err = monitoringHandler.FindByName(identifier)
			if err != nil {
				existing, err = monitoringHandler.Get(identifier)
				if err != nil {
					return fmt.Errorf("aws monitoring config %q not found by name or ID", identifier)
				}
			}
		} else {
			existing, err = monitoringHandler.FindByName(enableAWSMonitoringName)
			if err != nil {
				return err
			}
		}

		configName := existing.Value.Description
		if configName == "" {
			configName = existing.ObjectID
		}

		// Step 1: optionally patch the linked connection's roleArn.
		if enableAWSMonitoringRoleArn != "" {
			if len(existing.Value.Aws.Credentials) == 0 {
				return fmt.Errorf("monitoring config %q has no credentials configured", configName)
			}
			if len(existing.Value.Aws.Credentials) > 1 {
				output.PrintWarning("monitoring config %q has %d credentials — only the first connection will be updated; use 'dtctl update aws connection' for the others",
					configName, len(existing.Value.Aws.Credentials))
			}

			connectionID := existing.Value.Aws.Credentials[0].ConnectionID
			output.PrintInfo("Updating AWS connection %q with role ARN...", connectionID)

			conn, err := connectionHandler.Get(connectionID)
			if err != nil {
				return fmt.Errorf("failed to get linked connection %q: %w", connectionID, err)
			}

			value := conn.Value
			if value.Type != awsconnection.TypeRoleBased {
				return fmt.Errorf("unsupported aws connection type %q", value.Type)
			}
			if value.AwsRoleBasedAuthentication == nil {
				value.AwsRoleBasedAuthentication = &awsconnection.AwsRoleBasedAuthenticationConfig{
					Consumers: []string{awsconnection.DefaultConsumer},
				}
			}
			value.AwsRoleBasedAuthentication.RoleArn = enableAWSMonitoringRoleArn
			if len(value.AwsRoleBasedAuthentication.Consumers) == 0 {
				value.AwsRoleBasedAuthentication.Consumers = []string{awsconnection.DefaultConsumer}
			}

			if _, err := connectionHandler.Update(conn.ObjectID, value); err != nil {
				return fmt.Errorf("failed to update connection roleArn: %w", err)
			}
			output.PrintSuccess("AWS connection %q updated", connectionID)

			// Refresh credential AccountID from updated ARN.
			existing.Value.Aws.Credentials[0].AccountID = awsmonitoringconfig.AccountIDFromRoleArn(enableAWSMonitoringRoleArn)
		}

		// Step 2: enable monitoring config and all credentials.
		output.PrintInfo("Enabling AWS monitoring config %q...", configName)
		value := existing.Value
		value.Enabled = true
		for i := range value.Aws.Credentials {
			value.Aws.Credentials[i].Enabled = true
		}

		payload := awsmonitoringconfig.AWSMonitoringConfig{Scope: existing.Scope, Value: value}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := monitoringHandler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		output.PrintSuccess("AWS monitoring config %q enabled (%s)", configName, updated.ObjectID)
		return nil
	},
}

func init() {
	enableCmd.AddCommand(enableAWSProviderCmd)
	enableAWSProviderCmd.AddCommand(enableAWSMonitoringCmd)

	enableAWSMonitoringCmd.Flags().StringVar(&enableAWSMonitoringName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	enableAWSMonitoringCmd.Flags().StringVar(&enableAWSMonitoringRoleArn, "roleArn", "", "AWS IAM role ARN to set on the linked connection (optional)")
}
