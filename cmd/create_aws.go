package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
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
	Use:     "connection [name]",
	Aliases: []string{"connections"},
	Short:   "Create AWS connection from flags",
	Long: `Create AWS connection using command flags.

Examples:
  dtctl create aws connection --name "my-aws-connection"
  dtctl create aws connection my-aws-connection
  dtctl create aws connection --name "my-aws-connection" --roleArn "arn:aws:iam::123456789012:role/dynatrace-monitoring"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if createAWSConnectionName == "" && len(args) > 0 {
			createAWSConnectionName = args[0]
		}
		if createAWSConnectionName == "" {
			return fmt.Errorf("connection name is required (use positional argument or --name)")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsconnection.NewHandler(c)
		value := awsconnection.Value{
			Name: createAWSConnectionName,
			Type: "awsRoleBasedAuthentication",
			AWSRoleBasedAuthentication: &awsconnection.AWSRoleBasedAuthentication{
				RoleArn:   "",
				Consumers: []string{"SVC:com.dynatrace.da"},
			},
		}

		created, err := handler.Create(awsconnection.AWSConnectionCreate{Value: value})
		if err != nil {
			return err
		}

		externalID := created.ExternalID
		if externalID == "" {
			externalID = created.ObjectID
		}

		fmt.Printf("AWS connection created: %s\n", created.ObjectID)
		fmt.Printf("External ID: %s\n", externalID)
		printAWSRoleSetupInstructions(c.BaseURL(), externalID, createAWSConnectionName)
		if createAWSConnectionRoleArn != "" {
			_, err = handler.Update(created.ObjectID, awsconnection.Value{
				Name: createAWSConnectionName,
				Type: "awsRoleBasedAuthentication",
				AWSRoleBasedAuthentication: &awsconnection.AWSRoleBasedAuthentication{
					RoleArn:   createAWSConnectionRoleArn,
					Consumers: []string{"SVC:com.dynatrace.da"},
				},
			})
			if err != nil {
				return fmt.Errorf("connection created, but failed to set role ARN in second call: %w", err)
			}
			fmt.Println()
			fmt.Printf("Connection updated with role ARN: %s\n", createAWSConnectionRoleArn)
		}
		return nil
	},
}

func printAWSRoleSetupInstructions(baseURL, externalID, connectionName string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		fmt.Printf("Warning: Could not parse base URL for instructions: %v\n", err)
		return
	}
	host := u.Host

	dynatraceAWSAccountID := "476114158034"
	if strings.Contains(host, ".live.dynatrace.com") || strings.Contains(host, ".apps.dynatrace.com") {
		dynatraceAWSAccountID = "314146291599"
	}

	fmt.Println("\nAWS CLI setup (copy/paste):")
	if runtime.GOOS == "windows" {
		fmt.Println("$ROLE_NAME = \"dynatrace-monitoring\"")
		fmt.Printf("$TRUST = '{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"AWS\":\"%s\"},\"Action\":\"sts:AssumeRole\",\"Condition\":{\"StringEquals\":{\"sts:ExternalId\":\"%s\"}}}]}'\n", dynatraceAWSAccountID, externalID)
		fmt.Println("$env:AWS_PAGER = \"\"")
		fmt.Println("aws iam create-role --role-name $ROLE_NAME --assume-role-policy-document $TRUST --no-cli-pager --query \"Role.{RoleName:RoleName,Arn:Arn,CreateDate:CreateDate}\" --output table")
		fmt.Println("aws iam attach-role-policy --role-name $ROLE_NAME --policy-arn arn:aws:iam::aws:policy/ReadOnlyAccess --no-cli-pager")
		fmt.Println("Write-Host \"Attached policy: arn:aws:iam::aws:policy/ReadOnlyAccess\"")
		fmt.Println("$ROLE_ARN = aws iam get-role --role-name $ROLE_NAME --query Role.Arn --output text")
		fmt.Printf("dtctl update aws connection --name %q --roleArn $ROLE_ARN\n", connectionName)
	} else {
		fmt.Println("ROLE_NAME=\"dynatrace-monitoring\"")
		fmt.Println("# BEGIN TRUST (copy until '# END TRUST')")
		fmt.Println("TRUST=$(cat <<'JSON'")
		fmt.Println("{")
		fmt.Println("  \"Version\": \"2012-10-17\",")
		fmt.Println("  \"Statement\": [")
		fmt.Println("    {")
		fmt.Println("      \"Effect\": \"Allow\",")
		fmt.Printf("      \"Principal\": {\"AWS\": \"%s\"},\n", dynatraceAWSAccountID)
		fmt.Println("      \"Action\": \"sts:AssumeRole\",")
		fmt.Println("      \"Condition\": {")
		fmt.Println("        \"StringEquals\": {")
		fmt.Printf("          \"sts:ExternalId\": \"%s\"\n", externalID)
		fmt.Println("        }")
		fmt.Println("      }")
		fmt.Println("    }")
		fmt.Println("  ]")
		fmt.Println("}")
		fmt.Println("JSON")
		fmt.Println(")")
		fmt.Println("# END TRUST")
		fmt.Println("aws iam create-role --role-name \"$ROLE_NAME\" --assume-role-policy-document \"$TRUST\" --no-cli-pager --query 'Role.{RoleName:RoleName,Arn:Arn,CreateDate:CreateDate}' --output table")
		fmt.Println("aws iam attach-role-policy --role-name \"$ROLE_NAME\" --policy-arn arn:aws:iam::aws:policy/ReadOnlyAccess --no-cli-pager")
		fmt.Println("ROLE_ARN=$(aws iam get-role --role-name \"$ROLE_NAME\" --no-cli-pager --query 'Role.Arn' --output text)")
		fmt.Printf("dtctl update aws connection --name %q --roleArn \"$ROLE_ARN\"\n", connectionName)
	}
}

var createAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring",
	Aliases: []string{"monitoring-config"},
	Short:   "Create AWS monitoring config from flags",
	Long: `Create AWS monitoring configuration using command flags.

Examples:
  dtctl create aws monitoring --name "my-aws-monitoring" --credentials "my-aws-connection"
  dtctl create aws monitoring --name "my-aws-monitoring" --credentials "my-aws-connection" --regionFiltering "eu-west-1,us-east-1"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createAWSMonitoringConfigName == "" {
			return fmt.Errorf("--name is required")
		}
		if createAWSMonitoringConfigCredentials == "" {
			return fmt.Errorf("--credentials is required")
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		connectionHandler := awsconnection.NewHandler(c)
		monitoringHandler := awsmonitoringconfig.NewHandler(c)

		credential, err := awsmonitoringconfig.ResolveCredential(createAWSMonitoringConfigCredentials, connectionHandler)
		if err != nil {
			return err
		}
		if credential.AccountID == "" {
			return fmt.Errorf("could not infer AWS account ID from role ARN; update AWS connection with --roleArn first")
		}

		regions, err := awsmonitoringconfig.ParseOrDefaultRegions(createAWSMonitoringConfigRegions, monitoringHandler)
		if err != nil {
			return err
		}
		if len(regions) == 0 {
			return fmt.Errorf("at least one AWS region is required")
		}

		featureSets, err := awsmonitoringconfig.ParseOrDefaultFeatureSets(createAWSMonitoringConfigFeatureSets, monitoringHandler)
		if err != nil {
			return err
		}

		version, err := monitoringHandler.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to determine extension version: %w", err)
		}

		payload := awsmonitoringconfig.AWSMonitoringConfig{
			Scope: "integration-aws",
			Value: awsmonitoringconfig.Value{
				Enabled:           true,
				Description:       createAWSMonitoringConfigName,
				Version:           version,
				FeatureSets:       featureSets,
				ActivationContext: "DATA_ACQUISITION",
				AWS: awsmonitoringconfig.AWSConfig{
					DeploymentRegion:            regions[0],
					Credentials:                 []awsmonitoringconfig.Credential{credential},
					RegionFiltering:             regions,
					TagFiltering:                []awsmonitoringconfig.TagFilter{},
					TagEnrichment:               []string{},
					DTLabelsEnrichment:          map[string]awsmonitoringconfig.LabelRule{},
					MetricsConfiguration:        awsmonitoringconfig.FlagConfig{Enabled: true, Regions: regions},
					CloudWatchLogsConfiguration: awsmonitoringconfig.FlagConfig{Enabled: true, Regions: regions},
					EventsConfiguration:         awsmonitoringconfig.FlagConfig{Enabled: true, Regions: regions},
					Namespaces:                  []awsmonitoringconfig.Namespace{},
					ConfigurationMode:           "QUICK_START",
					DeploymentMode:              "AUTOMATED",
					DeploymentScope:             "SINGLE_ACCOUNT",
					ManualDeploymentStatus:      "NA",
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

		fmt.Printf("AWS monitoring config created: %s\n", created.ObjectID)
		return nil
	},
}

func init() {
	createAWSProviderCmd.AddCommand(createAWSConnectionCmd)
	createAWSProviderCmd.AddCommand(createAWSMonitoringConfigCmd)

	createAWSConnectionCmd.Flags().StringVar(&createAWSConnectionName, "name", "", "AWS connection name (required)")
	createAWSConnectionCmd.Flags().StringVar(&createAWSConnectionRoleArn, "roleArn", "", "AWS IAM role ARN for monitoring (optional at create, can be set later with update)")
	createAWSConnectionCmd.Flags().StringVar(&createAWSConnectionRoleArn, "rolearn", "", "Alias for --roleArn")

	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigName, "name", "", "Monitoring config name/description (required)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigCredentials, "credentials", "", "AWS connection name or ID (required)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigRegions, "regionFiltering", "", "Comma-separated AWS regions (default: all from schema)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets (default: all *_essential from schema)")
	createAWSMonitoringConfigCmd.Flags().StringVar(&createAWSMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
	_ = createAWSMonitoringConfigCmd.MarkFlagRequired("name")
	_ = createAWSMonitoringConfigCmd.MarkFlagRequired("credentials")
}
