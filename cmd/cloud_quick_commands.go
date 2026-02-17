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

	updateAzureConnectionName          string
	updateAzureConnectionDirectoryID   string
	updateAzureConnectionApplicationID string

	createAzureMonitoringConfigName              string
	createAzureMonitoringConfigCredentials       string
	createAzureMonitoringConfigLocationFiltering string
	createAzureMonitoringConfigTagFiltering      string
	createAzureMonitoringConfigFeatureSets       string
	createAzureMonitoringConfigScope             string

	updateAzureMonitoringConfigName              string
	updateAzureMonitoringConfigLocationFiltering string
	updateAzureMonitoringConfigTagFiltering      string
	updateAzureMonitoringConfigFeatureSets       string
)

var createAzureConnectionCmd = &cobra.Command{
	Use:   "azure_connection",
	Short: "Create Azure connection from flags",
	Long: `Create Azure connection using command flags.

Examples:
  dtctl create azure_connection --name "siwek" --type "federatedIdentityCredential"
  dtctl create azure_connection --name "siwek" --type "clientSecret"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createAzureConnectionName == "" || createAzureConnectionType == "" {
			missing := make([]string, 0, 2)
			if createAzureConnectionName == "" {
				missing = append(missing, "--name")
			}
			if createAzureConnectionType == "" {
				missing = append(missing, "--type")
			}

			return fmt.Errorf(
				"required flag(s) %s not set\nAvailable --type values: federatedIdentityCredential, clientSecret\nExample: dtctl create azure_connection --name \"my-conn\" --type federatedIdentityCredential",
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
			value.FederatedIdentityCredential = &azureconnection.FederatedIdentityCredential{
				Consumers: []string{"SVC:com.dynatrace.da"},
			}
		case "clientSecret":
			value.ClientSecret = &azureconnection.ClientSecretCredential{
				Consumers: []string{"SVC:com.dynatrace.da"},
			}
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

var updateAzureConnectionCmd = &cobra.Command{
	Use:   "azure_connection [id]",
	Short: "Update Azure connection from flags",
	Long: `Update Azure connection by ID argument or by --name.

Examples:
  dtctl update azure_connection --name "siwek" --directoryId "XYZ" --applicationId "ZUZ"
  dtctl update azure_connection <id> --directoryId "XYZ" --applicationId "ZUZ"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateAzureConnectionDirectoryID == "" && updateAzureConnectionApplicationID == "" {
			return fmt.Errorf("at least one of --directoryId or --applicationId is required")
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

		handler := azureconnection.NewHandler(c)

		var existing *azureconnection.AzureConnection
		if len(args) > 0 {
			existing, err = handler.Get(args[0])
			if err != nil {
				return err
			}
		} else {
			if updateAzureConnectionName == "" {
				return fmt.Errorf("provide connection ID argument or --name")
			}
			existing, err = handler.FindByName(updateAzureConnectionName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		switch value.Type {
		case "federatedIdentityCredential":
			if value.FederatedIdentityCredential == nil {
				value.FederatedIdentityCredential = &azureconnection.FederatedIdentityCredential{}
			}
			if updateAzureConnectionDirectoryID != "" {
				value.FederatedIdentityCredential.DirectoryID = updateAzureConnectionDirectoryID
			}
			if updateAzureConnectionApplicationID != "" {
				value.FederatedIdentityCredential.ApplicationID = updateAzureConnectionApplicationID
			}
		case "clientSecret":
			if value.ClientSecret == nil {
				value.ClientSecret = &azureconnection.ClientSecretCredential{}
			}
			if updateAzureConnectionDirectoryID != "" {
				value.ClientSecret.DirectoryID = updateAzureConnectionDirectoryID
			}
			if updateAzureConnectionApplicationID != "" {
				value.ClientSecret.ApplicationID = updateAzureConnectionApplicationID
			}
		default:
			return fmt.Errorf("unsupported azure connection type %q", value.Type)
		}

		updated, err := handler.Update(existing.ObjectID, value)
		if err != nil {
			return err
		}

		fmt.Printf("Azure connection updated: %s\n", updated.ObjectID)
		return nil
	},
}

var createAzureMonitoringConfigCmd = &cobra.Command{
	Use:   "azure_monitoring_config",
	Short: "Create Azure monitoring config from flags",
	Long: `Create Azure monitoring configuration using command flags.

Examples:
  dtctl create azure_monitoring_config --name "siwek" --credentials "siwek" --locationFiltering "eastus,northcentralus" --featuresets "microsoft_apimanagement.service_essential,microsoft_cache.redis_essential"
  dtctl create azure_monitoring_config --name "siwek" --credentials "<connection-id>"`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		tagFilters, err := azuremonitoringconfig.ParseTagFiltering(createAzureMonitoringConfigTagFiltering)
		if err != nil {
			return err
		}

		version, err := monitoringHandler.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to determine extension version: %w", err)
		}

		payload := azuremonitoringconfig.AzureMonitoringConfig{
			Scope: createAzureMonitoringConfigScope,
			Value: azuremonitoringconfig.Value{
				Enabled:     true,
				Description: createAzureMonitoringConfigName,
				Version:     version,
				Azure: azuremonitoringconfig.AzureConfig{
					DeploymentScope:           "SUBSCRIPTION",
					ConfigurationMode:         "ADVANCED",
					DeploymentMode:            "AUTOMATED",
					SubscriptionFilteringMode: "INCLUDE",
					Credentials: []azuremonitoringconfig.Credential{
						credential,
					},
					LocationFiltering: locations,
					TagFiltering:      tagFilters,
				},
				FeatureSets: featureSets,
			},
		}

		if payload.Scope == "" {
			payload.Scope = "integration-azure"
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

var updateAzureMonitoringConfigCmd = &cobra.Command{
	Use:   "azure_monitoring_config [id]",
	Short: "Update Azure monitoring config from flags",
	Long: `Update Azure monitoring configuration by ID argument or by --name.

Examples:
  dtctl update azure_monitoring_config --name "siwek" --location-filtering "eastus,westeurope"
  dtctl update azure_monitoring_config --name "siwek" --featuresets "microsoft_compute.virtualmachines_essential,microsoft_web.sites_functionapp_essential"
  dtctl update azure_monitoring_config --name "siwek" --tagfiltering "include:dt_owner=xyz@example.com"
  dtctl update azure_monitoring_config <id> --location-filtering "eastus,westeurope"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(updateAzureMonitoringConfigLocationFiltering) == "" &&
			strings.TrimSpace(updateAzureMonitoringConfigFeatureSets) == "" &&
			strings.TrimSpace(updateAzureMonitoringConfigTagFiltering) == "" {
			return fmt.Errorf("at least one of --location-filtering, --featuresets, or --tagfiltering is required")
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

		handler := azuremonitoringconfig.NewHandler(c)

		var existing *azuremonitoringconfig.AzureMonitoringConfig
		if len(args) > 0 {
			identifier := args[0]

			// Try to resolve by description/name first
			existing, err = handler.FindByName(identifier)
			if err != nil {
				// If not found by name, fallback to ID
				existing, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
			}
		} else {
			if updateAzureMonitoringConfigName == "" {
				return fmt.Errorf("provide config ID argument or --name")
			}
			existing, err = handler.FindByName(updateAzureMonitoringConfigName)
			if err != nil {
				return err
			}
		}

		value := existing.Value

		if strings.TrimSpace(updateAzureMonitoringConfigLocationFiltering) != "" {
			locations := azuremonitoringconfig.SplitCSV(updateAzureMonitoringConfigLocationFiltering)
			if len(locations) == 0 {
				return fmt.Errorf("--location-filtering must contain at least one location")
			}
			value.Azure.LocationFiltering = locations
		}

		if strings.TrimSpace(updateAzureMonitoringConfigFeatureSets) != "" {
			featureSets := azuremonitoringconfig.SplitCSV(updateAzureMonitoringConfigFeatureSets)
			if len(featureSets) == 0 {
				return fmt.Errorf("--featuresets must contain at least one feature set")
			}
			value.FeatureSets = featureSets
		}

		if strings.TrimSpace(updateAzureMonitoringConfigTagFiltering) != "" {
			tagFilters, err := azuremonitoringconfig.ParseTagFiltering(updateAzureMonitoringConfigTagFiltering)
			if err != nil {
				return err
			}
			value.Azure.TagFiltering = tagFilters
		}

		payload := azuremonitoringconfig.AzureMonitoringConfig{
			Scope: existing.Scope,
			Value: value,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := handler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		fmt.Printf("Azure monitoring config updated: %s\n", updated.ObjectID)
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
		fmt.Printf("   dtctl update azure_connection --name %q --directoryId \"$TENANT_ID\" --applicationId \"$CLIENT_ID\"\n", connectionName)
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
		fmt.Printf("   dtctl update azure_connection --name %q --directoryId \"$TENANT_ID\" --applicationId \"$CLIENT_ID\"\n", connectionName)
	}
	fmt.Println()
}

func init() {
	createCmd.AddCommand(createAzureConnectionCmd)
	createCmd.AddCommand(createAzureMonitoringConfigCmd)
	updateCmd.AddCommand(updateAzureConnectionCmd)
	updateCmd.AddCommand(updateAzureMonitoringConfigCmd)

	createAzureConnectionCmd.Flags().StringVar(&createAzureConnectionName, "name", "", "Azure connection name (required)")
	createAzureConnectionCmd.Flags().StringVar(&createAzureConnectionType, "type", "", "Azure connection type: federatedIdentityCredential or clientSecret (required)")
	_ = createAzureConnectionCmd.RegisterFlagCompletionFunc("type", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"federatedIdentityCredential\tUse workload identity federation (recommended)",
			"clientSecret\tUse service principal client secret",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionName, "name", "", "Azure connection name (used when ID argument is not provided)")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionDirectoryID, "directoryId", "", "Directory ID to set")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionDirectoryID, "directoryID", "", "Alias for --directoryId")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionApplicationID, "applicationId", "", "Application ID to set")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionApplicationID, "applicationID", "", "Alias for --applicationId")
	updateAzureConnectionCmd.Flags().StringVar(&updateAzureConnectionApplicationID, "aplicationID", "", "Compatibility alias for typo --aplicationID")

	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigName, "name", "", "Monitoring config name/description (required)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigCredentials, "credentials", "", "Azure connection name or ID (required)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigLocationFiltering, "locationFiltering", "", "Comma-separated locations (default: all from schema)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigLocationFiltering, "location-filtering", "", "Alias for --locationFiltering")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigTagFiltering, "tagFiltering", "", "Tag filtering rules, e.g. include:environment=prod,tier=db;exclude:owner=pawel")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigTagFiltering, "tagfiltering", "", "Alias for --tagFiltering")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets (default: all *_essential from schema)")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigFeatureSets, "featureset", "", "Alias for --featuresets")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigFeatureSets, "eatureset", "", "Compatibility alias for typo in examples")
	createAzureMonitoringConfigCmd.Flags().StringVar(&createAzureMonitoringConfigScope, "scope", "integration-azure", "Monitoring config scope")
	_ = createAzureMonitoringConfigCmd.MarkFlagRequired("name")
	_ = createAzureMonitoringConfigCmd.MarkFlagRequired("credentials")

	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigLocationFiltering, "locationFiltering", "", "Comma-separated locations")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigLocationFiltering, "location-filtering", "", "Alias for --locationFiltering")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigTagFiltering, "tagFiltering", "", "Tag filtering rules, e.g. include:environment=prod,tier=db;exclude:owner=pawel")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigTagFiltering, "tagfiltering", "", "Alias for --tagFiltering")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
	updateAzureMonitoringConfigCmd.Flags().StringVar(&updateAzureMonitoringConfigFeatureSets, "featureset", "", "Alias for --featuresets")
}
