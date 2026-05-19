package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

var (
	createAWSConnectionName    string
	createAWSConnectionRoleArn string

	createAWSMonitoringConfigName        string
	createAWSMonitoringConfigCredentials string
	createAWSMonitoringConfigRegions     string
	createAWSMonitoringConfigFeatureSets string
)

var createAWSConnectionCmd = &cobra.Command{
	Use:     "connection",
	Aliases: []string{"connections"},
	Short:   "Create AWS connection from flags",
	Long: `Create an AWS connection (role-based authentication) for the Dynatrace AWS data
acquisition extension. The role ARN can be omitted at creation time and patched
later (after the IAM role is created with the trust policy that uses the new
connection's objectId as sts:ExternalId).

Examples:
  dtctl create aws connection --name "my-aws"
  dtctl create aws connection --name "my-aws" --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createAWSConnectionName == "" {
			return fmt.Errorf("--name is required")
		}
		if createAWSConnectionRoleArn != "" {
			if err := awsconnection.ValidateRoleArn(createAWSConnectionRoleArn); err != nil {
				return err
			}
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		handler := awsconnection.NewHandler(c)

		value := awsconnection.Value{
			Name: createAWSConnectionName,
			Type: awsconnection.TypeRoleBased,
			AwsRoleBasedAuthentication: &awsconnection.AwsRoleBasedAuthenticationConfig{
				RoleArn:   createAWSConnectionRoleArn,
				Consumers: []string{awsconnection.DefaultConsumer},
			},
		}

		created, err := handler.Create(awsconnection.AWSConnectionCreate{Value: value})
		if err != nil {
			return err
		}

		output.PrintSuccess("AWS connection created: %s", created.ObjectID)
		printAWSConnectionInstructions(c.BaseURL(), created.ObjectID, createAWSConnectionName)
		return nil
	},
}

var createAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring",
	Aliases: []string{"monitoring-config"},
	Short:   "Create AWS monitoring config from flags",
	Long: `Create an AWS monitoring configuration in disabled state.

Use 'dtctl enable aws monitoring' to enable it once the underlying IAM role
is in place. The --regions flag is required (comma-separated AWS regions).

Examples:
  dtctl create aws monitoring --name "my-aws" --credentials "my-aws" --regions us-east-1,eu-central-1
  dtctl create aws monitoring --name "my-aws" --credentials "my-aws" --regions us-east-1 --featureSets EC2_essential,RDS_essential`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createAWSMonitoringConfigName == "" {
			return fmt.Errorf("--name is required")
		}
		if createAWSMonitoringConfigCredentials == "" {
			return fmt.Errorf("--credentials is required")
		}

		_, c, err := SetupWithSafety(safety.OperationCreate)
		if err != nil {
			return err
		}

		connectionHandler := awsconnection.NewHandler(c)
		monitoringHandler := awsmonitoringconfig.NewHandler(c)

		credential, err := awsmonitoringconfig.ResolveCredential(createAWSMonitoringConfigCredentials, connectionHandler)
		if err != nil {
			return err
		}
		credential.Enabled = false // created in disabled state

		regions, err := awsmonitoringconfig.ParseRequiredRegions(createAWSMonitoringConfigRegions)
		if err != nil {
			return err
		}

		featureSets, err := awsmonitoringconfig.ParseOrDefaultFeatureSets(createAWSMonitoringConfigFeatureSets, monitoringHandler)
		if err != nil {
			return err
		}

		version, err := monitoringHandler.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to determine extension version: %w", err)
		}

		deploymentRegion := regions[0]

		payload := awsmonitoringconfig.AWSMonitoringConfig{
			Scope: awsmonitoringconfig.DefaultScope,
			Value: awsmonitoringconfig.Value{
				Enabled:           false,
				Description:       createAWSMonitoringConfigName,
				Version:           version,
				ActivationContext: awsmonitoringconfig.DefaultActivationContext,
				FeatureSets:       featureSets,
				Aws: awsmonitoringconfig.AWSConfig{
					DeploymentRegion:        deploymentRegion,
					Credentials:             []awsmonitoringconfig.Credential{credential},
					RegionFiltering:         regions,
					TagFiltering:            []awsmonitoringconfig.TagFilter{},
					TagEnrichment:           []string{},
					Namespaces:              []awsmonitoringconfig.CustomNamespace{},
					ConfigurationMode:       "QUICK_START",
					DeploymentMode:          "AUTOMATED",
					DeploymentScope:         "SINGLE_ACCOUNT",
					SmartscapeConfiguration: awsmonitoringconfig.FlagConfig{Enabled: true},
					MetricsConfiguration:    awsmonitoringconfig.RegionalFlagConfig{Enabled: true, Regions: regions},
				},
			},
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		created, err := monitoringHandler.Create(body)
		if err != nil {
			return err
		}

		output.PrintSuccess("AWS monitoring config created (disabled): %s", created.ObjectID)
		output.PrintInfo("Run 'dtctl enable aws monitoring --name %q' to enable it", createAWSMonitoringConfigName)
		return nil
	},
}

// printAWSConnectionInstructions prints a copy-paste 'aws cloudformation deploy'
// one-liner that creates the Dynatrace monitoring IAM role with the least-
// privilege managed policies maintained by Dynatrace. The upstream template
// itself handles trust policy (Principal + sts:ExternalId via parameters) so
// dtctl does not need to generate trust-policy.json.
func printAWSConnectionInstructions(tenantURL, objectID, connectionName string) {
	const templateURL = "https://dynatrace-data-acquisition.s3.amazonaws.com/aws/deployment/cfn/latest/da-aws-nested-monitoring-role.yaml"
	stackName := "dynatrace-monitoring-" + sanitizeRoleName(connectionName)

	fmt.Println()
	fmt.Println("Create the Dynatrace monitoring IAM role with the least-privilege policy")
	fmt.Println("maintained by Dynatrace. Run in AWS CloudShell (aws CLI + curl pre-installed):")
	fmt.Println()
	if runtime.GOOS == "windows" {
		fmt.Printf("   $STACK = \"%s\"\n", stackName)
		fmt.Printf("   curl.exe -fsSLo da-role.yaml %s\n", templateURL)
		fmt.Printf("   aws cloudformation deploy `\n     --stack-name $STACK `\n     --template-file da-role.yaml `\n     --parameter-overrides pDynatraceUrl=%s pRoleExternalId=%s `\n     --capabilities CAPABILITY_NAMED_IAM\n",
			tenantURL, objectID)
		fmt.Println("   $ROLE_ARN = aws cloudformation describe-stacks --stack-name $STACK `")
		fmt.Println("     --query \"Stacks[0].Outputs[?OutputKey=='DynatraceMonitoringRoleArn'].OutputValue\" --output text")
		fmt.Printf("   dtctl update aws connection --name %q --roleArn $ROLE_ARN\n", connectionName)
	} else {
		fmt.Printf("   STACK=%q\n", stackName)
		fmt.Printf("   curl -fsSLo da-role.yaml %s\n", templateURL)
		fmt.Printf("   aws cloudformation deploy \\\n     --stack-name \"$STACK\" \\\n     --template-file da-role.yaml \\\n     --parameter-overrides pDynatraceUrl=%s pRoleExternalId=%s \\\n     --capabilities CAPABILITY_NAMED_IAM\n",
			tenantURL, objectID)
		fmt.Println("   ROLE_ARN=$(aws cloudformation describe-stacks --stack-name \"$STACK\" \\")
		fmt.Println("     --query \"Stacks[0].Outputs[?OutputKey=='DynatraceMonitoringRoleArn'].OutputValue\" --output text)")
		fmt.Printf("   dtctl update aws connection --name %q --roleArn \"$ROLE_ARN\"\n", connectionName)
	}
	fmt.Println()
	fmt.Println("The template is Dynatrace's source of truth for the required AWS read/describe")
	fmt.Println("actions; refresh later by re-running the same command (CloudFormation does an")
	fmt.Println("in-place update after curl re-downloads the latest template).")
}

// sanitizeRoleName produces a string usable inside an IAM role name.
func sanitizeRoleName(s string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, s)
	if out == "" {
		out = "connection"
	}
	return out
}

func init() {
	createAWSProviderCmd.AddCommand(createAWSConnectionCmd)
	createAWSProviderCmd.AddCommand(createAWSMonitoringConfigCmd)

	createAWSConnectionCmd.Flags().StringVar(&createAWSConnectionName, "name", "", "AWS connection name (required)")
	createAWSConnectionCmd.Flags().StringVar(&createAWSConnectionRoleArn, "roleArn", "", "AWS IAM role ARN (optional; can be patched later)")

	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigName, "name", "", "Monitoring config name/description (required)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigCredentials, "credentials", "", "AWS connection name or ID (required)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigRegions, "regions", "", "Comma-separated AWS regions (required, first is the deployment region)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets (default: all *_essential)")
}
