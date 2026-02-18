package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var (
	createAzureConnectionName string
	createAzureConnectionType string
	createCloudConnectionProvider string

	createAzureMonitoringConfigName              string
	createAzureMonitoringConfigCredentials       string
	createAzureMonitoringConfigLocationFiltering string
	createAzureMonitoringConfigFeatureSets       string
	createCloudMonitoringConfigProvider          string
)

var createAzureConnectionCmd = &cobra.Command{
	Use:   "cloud_connection",
	Aliases: []string{"azure_connection"},
	Short: "Create Azure connection from flags",
	Long: `Create Azure connection using command flags.

Examples:
  dtctl create cloud_connection --provider azure --name "siwek" --type "federatedIdentityCredential"
  dtctl create cloud_connection --provider azure --name "siwek" --type "clientSecret"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(createCloudConnectionProvider); err != nil {
			return err
		}

		if createAzureConnectionName == "" || createAzureConnectionType == "" {
			missing := make([]string, 0, 2)
			if createAzureConnectionName == "" {
				missing = append(missing, "--name")
			}
			if createAzureConnectionType == "" {
				missing = append(missing, "--type")
			}

			return fmt.Errorf(
				"required flag(s) %s not set\nAvailable --type values: federatedIdentityCredential, clientSecret\nExample: dtctl create cloud_connection --provider azure --name \"my-conn\" --type federatedIdentityCredential",
				strings.Join(missing, ", "),
			)
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

		handler := azureconnection.NewHandler(c)

		value := azureconnection.Value{
			Name: createAzureConnectionName,
			Type: createAzureConnectionType,
		}

		switch createAzureConnectionType {
		case "federatedIdentityCredential":
			value.FederatedIdentityCredential = &azureconnection.FederatedIdentityCredential{Consumers: []string{"SVC:com.dynatrace.da"}}
		case "clientSecret":
			value.ClientSecret = &azureconnection.ClientSecretCredential{Consumers: []string{"SVC:com.dynatrace.da"}}
		default:
			return fmt.Errorf("unsupported --type %q (supported: federatedIdentityCredential, clientSecret)", createAzureConnectionType)
		}

		created, err := handler.Create(azureconnection.AzureConnectionCreate{Value: value})
		if err != nil {
			return err
		}

		fmt.Printf("Azure connection created: %s\n", created.ObjectID)
		if createAzureConnectionType == "federatedIdentityCredential" {
			printFederatedCreateInstructions(c.BaseURL(), created.ObjectID, createAzureConnectionName)
		}
		return nil
	},
}

var createAzureMonitoringConfigCmd = &cobra.Command{
	Use:   "cloud_monitoring_config",
	Aliases: []string{"azure_monitoring_config"},
	Short: "Create Azure monitoring config from flags",
	Long: `Create Azure monitoring configuration using command flags.

Examples:
  dtctl create cloud_monitoring_config --provider azure --name "siwek" --credentials "siwek" --locationFiltering "eastus,northcentralus" --featureSets "microsoft_apimanagement.service_essential,microsoft_cache.redis_essential"
  dtctl create cloud_monitoring_config --provider azure --name "siwek" --credentials "<connection-id>"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAzureProvider(createCloudMonitoringConfigProvider); err != nil {
			return err
		}

		if createAzureMonitoringConfigName == "" {
			return fmt.Errorf("--name is required")
		}
		if createAzureMonitoringConfigCredentials == "" {
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

		connectionHandler := azureconnection.NewHandler(c)
		monitoringHandler := azuremonitoringconfig.NewHandler(c)

		credential, err := azuremonitoringconfig.ResolveCredential(createAzureMonitoringConfigCredentials, connectionHandler)
		if err != nil {
			return err
		}

		locations, err := azuremonitoringconfig.ParseOrDefaultLocations(createAzureMonitoringConfigLocationFiltering, monitoringHandler)
		if err != nil {
			return err
		}

		featureSets, err := azuremonitoringconfig.ParseOrDefaultFeatureSets(createAzureMonitoringConfigFeatureSets, monitoringHandler)
		if err != nil {
			return err
		}

		version, err := monitoringHandler.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to determine extension version: %w", err)
		}

		payload := azuremonitoringconfig.AzureMonitoringConfig{
			Scope: "integration-azure",
			Value: azuremonitoringconfig.Value{
				Enabled:     true,
				Description: createAzureMonitoringConfigName,
				Version:     version,
				Azure: azuremonitoringconfig.AzureConfig{
					DeploymentScope:           "SUBSCRIPTION",
					ConfigurationMode:         "ADVANCED",
					DeploymentMode:            "AUTOMATED",
					SubscriptionFilteringMode: "INCLUDE",
					Credentials:               []azuremonitoringconfig.Credential{credential},
					LocationFiltering:         locations,
				},
				FeatureSets: featureSets,
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

		fmt.Printf("Azure monitoring config created: %s\n", created.ObjectID)
		return nil
	},
}

func printFederatedCreateInstructions(baseURL, objectID, connectionName string) {
	u, err := url.Parse(baseURL)
	if err != nil {
		fmt.Printf("Warning: Could not parse base URL for instructions: %v\n", err)
		return
	}
	host := u.Host

	issuer := "https://token.dynatrace.com"
	if strings.Contains(host, "dev.apps.dynatracelabs.com") || strings.Contains(host, "dev.dynatracelabs.com") {
		issuer = "https://dev.token.dynatracelabs.com"
	}

	fmt.Println("\nTo complete the configuration, additional setup is required in the Azure Portal (Federated Credentials).")
	fmt.Println("Details for Azure configuration:")
	fmt.Printf("  Issuer:    %s\n", issuer)
	fmt.Printf("  Subject:   dt:connection-id/%s\n", objectID)
	fmt.Printf("  Audiences: %s/svc-id/com.dynatrace.da\n", host)
	fmt.Println()
	fmt.Println("Azure CLI commands:")
	fmt.Println("1. Create Service Principal and capture IDs:")
	if runtime.GOOS == "windows" {
		fmt.Printf("   $CLIENT_ID = az ad sp create-for-rbac --name %q --create-password false --query appId -o tsv\n", connectionName)
		fmt.Println("   $TENANT_ID = az account show --query tenantId -o tsv")
		fmt.Println()
		fmt.Println("2. Assign Reader role on subscription scope:")
		fmt.Println("   $IAM_SCOPE = \"/subscriptions/00000000-0000-0000-0000-000000000000\"")
		fmt.Println("   az role assignment create --assignee \"$CLIENT_ID\" --role Reader --scope \"$IAM_SCOPE\"")
		fmt.Println()
		fmt.Println("3. Create Federated Credential:")
		fmt.Printf("   az ad app federated-credential create --id \"$CLIENT_ID\" --parameters \"{'name': 'fd-Federated-Credential', 'issuer': '%s', 'subject': 'dt:connection-id/%s', 'audiences': ['%s/svc-id/com.dynatrace.da']}\"\n", issuer, objectID, host)
		fmt.Println()
		fmt.Println("4. Finalize connection in Dynatrace (set directoryId + applicationId):")
		fmt.Printf("   dtctl update cloud_connection --provider azure --name %q --directoryId \"$TENANT_ID\" --applicationId \"$CLIENT_ID\"\n", connectionName)
	} else {
		fmt.Printf("   CLIENT_ID=$(az ad sp create-for-rbac --name %q --create-password false --query appId -o tsv)\n", connectionName)
		fmt.Println("   TENANT_ID=$(az account show --query tenantId -o tsv)")
		fmt.Println()
		fmt.Println("2. Assign Reader role on subscription scope:")
		fmt.Println("   IAM_SCOPE=\"/subscriptions/00000000-0000-0000-0000-000000000000\"")
		fmt.Println("   az role assignment create --assignee \"$CLIENT_ID\" --role Reader --scope \"$IAM_SCOPE\"")
		fmt.Println()
		fmt.Println("3. Create Federated Credential:")
		fmt.Printf("   az ad app federated-credential create --id \"$CLIENT_ID\" --parameters \"{'name': 'fd-Federated-Credential', 'issuer': '%s', 'subject': 'dt:connection-id/%s', 'audiences': ['%s/svc-id/com.dynatrace.da']}\"\n", issuer, objectID, host)
		fmt.Println()
		fmt.Println("4. Finalize connection in Dynatrace (set directoryId + applicationId):")
		fmt.Printf("   dtctl update cloud_connection --provider azure --name %q --directoryId \"$TENANT_ID\" --applicationId \"$CLIENT_ID\"\n", connectionName)
	}
	fmt.Println()
}

func init() {
	createCmd.AddCommand(createAzureConnectionCmd)
	createCmd.AddCommand(createAzureMonitoringConfigCmd)

	addRequiredProviderFlagVar(createAzureConnectionCmd, &createCloudConnectionProvider)
	createAzureConnectionCmd.Flags().StringVar(&createAzureConnectionName, "name", "", "Azure connection name (required)")
	createAzureConnectionCmd.Flags().StringVar(&createAzureConnectionType, "type", "", "Azure connection type: federatedIdentityCredential or clientSecret (required)")
	_ = createAzureConnectionCmd.RegisterFlagCompletionFunc("type", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"federatedIdentityCredential\tUse workload identity federation (recommended)",
			"clientSecret\tUse service principal client secret",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	addRequiredProviderFlagVar(createAzureMonitoringConfigCmd, &createCloudMonitoringConfigProvider)
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigName, "name", "", "Monitoring config name/description (required)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigCredentials, "credentials", "", "Azure connection name or ID (required)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigLocationFiltering, "locationFiltering", "", "Comma-separated locations (default: all from schema)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets (default: all *_essential from schema)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
	_ = createAzureMonitoringConfigCmd.MarkFlagRequired("name")
	_ = createAzureMonitoringConfigCmd.MarkFlagRequired("credentials")
}
