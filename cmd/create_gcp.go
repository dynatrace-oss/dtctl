package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var (
	createGCPConnectionName             string
	createGCPConnectionServiceAccountID string

	createGCPMonitoringConfigName              string
	createGCPMonitoringConfigCredentials       string
	createGCPMonitoringConfigLocationFiltering string
	createGCPMonitoringConfigFeatureSets       string
)

var createGCPConnectionCmd = &cobra.Command{
	Use:     "connection [name]",
	Aliases: []string{"connections"},
	Short:   "Create GCP connection from flags",
	Long: `Create GCP connection using command flags.

Examples:
	  dtctl create gcp connection --name "my-gcp-connection"
	  dtctl create gcp connection my-gcp-connection
	  dtctl create gcp connection --name "my-gcp-connection" --serviceAccountId "my-reader@project.iam.gserviceaccount.com"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if createGCPConnectionName == "" && len(args) > 0 {
			createGCPConnectionName = args[0]
		}

		if createGCPConnectionName == "" {
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

		handler := gcpconnection.NewHandler(c)
		value := gcpconnection.Value{
			Name: createGCPConnectionName,
			Type: "serviceAccountImpersonation",
			ServiceAccountImpersonation: &gcpconnection.ServiceAccountImpersonation{
				ServiceAccountID: createGCPConnectionServiceAccountID,
				Consumers:        []string{"SVC:com.dynatrace.da"},
			},
		}

		created, err := handler.Create(gcpconnection.GCPConnectionCreate{Value: value})
		if err != nil {
			printGCPPrincipalHint(handler, createGCPConnectionServiceAccountID)
			return err
		}

		fmt.Printf("GCP connection created: %s\n", created.ObjectID)
		printGCPPrincipalHint(handler, createGCPConnectionServiceAccountID)
		return nil
	},
}

func printGCPPrincipalHint(handler *gcpconnection.Handler, serviceAccountID string) {
	principal, err := handler.GetDynatracePrincipal()
	if err != nil {
		return
	}

	fmt.Println("Dynatrace GCP principal:")
	fmt.Printf("  Principal ID: %s\n", principal.ObjectID)
	if principal.Principal != "" {
		fmt.Printf("  Principal:    %s\n", principal.Principal)
	}

	if serviceAccountID != "" && principal.Principal != "" {
		fmt.Println("Grant Token Creator role (copy/paste):")
		fmt.Printf("gcloud iam service-accounts add-iam-policy-binding %q --member=\"serviceAccount:%s\" --role=\"roles/iam.serviceAccountTokenCreator\"\n", serviceAccountID, principal.Principal)
	}

	dynatracePrincipal := principal.Principal
	if dynatracePrincipal == "" {
		dynatracePrincipal = "dynatrace-<tenant-id>@dtp-prod-gcp-auth.iam.gserviceaccount.com"
	}

	customerServiceAccount := serviceAccountID
	if customerServiceAccount == "" {
		customerServiceAccount = "${CUSTOMER_SA_EMAIL}"
	}

	fmt.Println("GCP quickstart snippets:")
	fmt.Println("1) Define variables:")
	fmt.Println("PROJECT_ID=\"my-project-id\"")
	fmt.Printf("DT_GCP_PRINCIPAL=%q\n", dynatracePrincipal)
	fmt.Println("CUSTOMER_SA_NAME=\"dynatrace-integration\"")
	fmt.Println("CUSTOMER_SA_EMAIL=\"${CUSTOMER_SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com\"")
	fmt.Println()
	fmt.Println("2) Create customer service account:")
	fmt.Println("gcloud iam service-accounts create \"${CUSTOMER_SA_NAME}\" \\")
	fmt.Println("  --project \"${PROJECT_ID}\" \\")
	fmt.Println("  --display-name \"Dynatrace Integration\"")
	fmt.Println()
	fmt.Println("3) Grant required viewer roles:")
	fmt.Println("for ROLE in roles/browser roles/monitoring.viewer roles/compute.viewer roles/cloudasset.viewer; do")
	fmt.Println("  gcloud projects add-iam-policy-binding \"${PROJECT_ID}\" \\")
	fmt.Println("    --member \"serviceAccount:${CUSTOMER_SA_EMAIL}\" \\")
	fmt.Println("    --role \"${ROLE}\"")
	fmt.Println("done")
	fmt.Println()
	fmt.Println("4) Grant Token Creator role to Dynatrace principal:")
	fmt.Printf("gcloud iam service-accounts add-iam-policy-binding %q \\\n", customerServiceAccount)
	fmt.Printf("  --member=\"serviceAccount:%s\" \\\n", dynatracePrincipal)
	fmt.Println("  --role=\"roles/iam.serviceAccountTokenCreator\"")
	fmt.Println("   If you get PERMISSION_DENIED (iam.serviceAccounts.getIamPolicy), ask your GCP admin for rights to read/update IAM policy on this service account.")
	fmt.Println()
	fmt.Println("5) Update connection in Dynatrace:")
	fmt.Printf("dtctl update gcp connection --name %q --serviceAccountId \"${CUSTOMER_SA_EMAIL}\"\n", createGCPConnectionName)
	fmt.Println()
	fmt.Println("Optional: check Domain Restricted Sharing policy allows Dynatrace customer:")
	fmt.Println("gcloud resource-manager org-policies describe constraints/iam.allowedPolicyMemberDomains \\")
	fmt.Println("  --project=\"${PROJECT_ID}\" \\")
	fmt.Println("  --format=\"value(spec.rules.values.allowedValues)\" | tr ';' '\\n' | grep -Fx 'customers/C03cngnp6'")
	fmt.Println()
}

var createGCPMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring",
	Aliases: []string{"monitoring-config"},
	Short:   "Create GCP monitoring config from flags",
	Long: `Create GCP monitoring configuration using command flags.

Examples:
  dtctl create gcp monitoring --name "my-gcp-monitoring" --credentials "my-gcp-connection"
  dtctl create gcp monitoring --name "my-gcp-monitoring" --credentials "<connection-id>" --locationFiltering "us-central1,europe-west1"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if createGCPMonitoringConfigName == "" {
			return fmt.Errorf("--name is required")
		}
		if createGCPMonitoringConfigCredentials == "" {
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

		connectionHandler := gcpconnection.NewHandler(c)
		monitoringHandler := gcpmonitoringconfig.NewHandler(c)

		credential, err := gcpmonitoringconfig.ResolveCredential(createGCPMonitoringConfigCredentials, connectionHandler)
		if err != nil {
			return err
		}

		locations, err := gcpmonitoringconfig.ParseOrDefaultLocations(createGCPMonitoringConfigLocationFiltering, monitoringHandler)
		if err != nil {
			return err
		}

		featureSets, err := gcpmonitoringconfig.ParseOrDefaultFeatureSets(createGCPMonitoringConfigFeatureSets, monitoringHandler)
		if err != nil {
			return err
		}

		version, err := monitoringHandler.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to determine extension version: %w", err)
		}

		payload := gcpmonitoringconfig.GCPMonitoringConfig{
			Scope: "integration-gcp",
			Value: gcpmonitoringconfig.Value{
				Enabled:     true,
				Description: createGCPMonitoringConfigName,
				Version:     version,
				GoogleCloud: gcpmonitoringconfig.GoogleCloudConfig{
					Credentials:                []gcpmonitoringconfig.Credential{credential},
					LocationFiltering:          locations,
					ProjectFiltering:           []string{},
					FolderFiltering:            []string{},
					TagFiltering:               []gcpmonitoringconfig.TagFilter{},
					LabelFiltering:             []gcpmonitoringconfig.TagFilter{},
					TagEnrichment:              []string{},
					LabelEnrichment:            []string{},
					ObservabilityScopesEnabled: false,
					SmartscapeConfiguration:    gcpmonitoringconfig.FlagConfig{Enabled: true},
					Resources:                  []gcpmonitoringconfig.MetricSource{},
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

		fmt.Printf("GCP monitoring config created: %s\n", created.ObjectID)
		return nil
	},
}

func init() {
	createGCPProviderCmd.AddCommand(createGCPConnectionCmd)
	createGCPProviderCmd.AddCommand(createGCPMonitoringConfigCmd)

	createGCPConnectionCmd.Flags().StringVar(&createGCPConnectionName, "name", "", "GCP connection name (required)")
	createGCPConnectionCmd.Flags().StringVar(&createGCPConnectionServiceAccountID, "serviceAccountId", "", "Customer service account email (optional; can be set later with update)")
	createGCPConnectionCmd.Flags().StringVar(&createGCPConnectionServiceAccountID, "serviceaccountid", "", "Alias for --serviceAccountId")

	createGCPMonitoringConfigCmd.Flags().StringVar(&createGCPMonitoringConfigName, "name", "", "Monitoring config name/description (required)")
	createGCPMonitoringConfigCmd.Flags().StringVar(&createGCPMonitoringConfigCredentials, "credentials", "", "GCP connection name or ID (required)")
	createGCPMonitoringConfigCmd.Flags().StringVar(&createGCPMonitoringConfigLocationFiltering, "locationFiltering", "", "Comma-separated locations (default: all from schema)")
	createGCPMonitoringConfigCmd.Flags().StringVar(&createGCPMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets (default: all *_essential from schema)")
	createGCPMonitoringConfigCmd.Flags().StringVar(&createGCPMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
	_ = createGCPMonitoringConfigCmd.MarkFlagRequired("name")
	_ = createGCPMonitoringConfigCmd.MarkFlagRequired("credentials")
}
