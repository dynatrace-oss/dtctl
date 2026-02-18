package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var (
	updateGCPConnectionName             string
	updateGCPConnectionServiceAccountID string

	updateGCPMonitoringConfigName              string
	updateGCPMonitoringConfigLocationFiltering string
	updateGCPMonitoringConfigFeatureSets       string
)

var updateGCPConnectionCmd = &cobra.Command{
	Use:     "connection [id]",
	Aliases: []string{"connections"},
	Short:   "Update GCP connection from flags",
	Long: `Update GCP connection by ID argument or by --name.

Examples:
  dtctl update gcp connection --name "my-gcp-connection" --serviceAccountId "my-reader@project.iam.gserviceaccount.com"
  dtctl update gcp connection <id> --serviceAccountId "my-reader@project.iam.gserviceaccount.com"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateGCPConnectionServiceAccountID == "" {
			return fmt.Errorf("--serviceAccountId is required")
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

		handler := gcpconnection.NewHandler(c)

		var existing *gcpconnection.GCPConnection
		if len(args) > 0 {
			existing, err = handler.Get(args[0])
			if err != nil {
				return err
			}
		} else {
			if updateGCPConnectionName == "" {
				return fmt.Errorf("provide connection ID argument or --name")
			}
			existing, err = handler.FindByName(updateGCPConnectionName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		if value.Type == "" {
			value.Type = "serviceAccountImpersonation"
		}
		if value.ServiceAccountImpersonation == nil {
			value.ServiceAccountImpersonation = &gcpconnection.ServiceAccountImpersonation{
				Consumers: []string{"SVC:com.dynatrace.da"},
			}
		}
		if len(value.ServiceAccountImpersonation.Consumers) == 0 {
			value.ServiceAccountImpersonation.Consumers = []string{"SVC:com.dynatrace.da"}
		}
		value.ServiceAccountImpersonation.ServiceAccountID = updateGCPConnectionServiceAccountID

		updated, err := handler.Update(existing.ObjectID, value)
		if err != nil {
			if strings.Contains(err.Error(), "GCP authentication failed") {
				return fmt.Errorf("%w\nIAM Policy update can take couple of minutes before become active, please retry in a moment.", err)
			}
			return err
		}

		fmt.Printf("GCP connection updated: %s\n", updated.ObjectID)
		return nil
	},
}

var updateGCPMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config"},
	Short:   "Update GCP monitoring config from flags",
	Long: `Update GCP monitoring configuration by ID argument or by --name.

Examples:
  dtctl update gcp monitoring --name "my-monitoring" --locationFiltering "us-central1,europe-west1"
  dtctl update gcp monitoring --name "my-monitoring" --featureSets "compute_engine_essential,cloud_run_essential"
  dtctl update gcp monitoring <id> --locationFiltering "us-central1,europe-west1"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(updateGCPMonitoringConfigLocationFiltering) == "" &&
			strings.TrimSpace(updateGCPMonitoringConfigFeatureSets) == "" {
			return fmt.Errorf("at least one of --locationFiltering or --featureSets is required")
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

		handler := gcpmonitoringconfig.NewHandler(c)

		var existing *gcpmonitoringconfig.GCPMonitoringConfig
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
			if updateGCPMonitoringConfigName == "" {
				return fmt.Errorf("provide config ID argument or --name")
			}
			existing, err = handler.FindByName(updateGCPMonitoringConfigName)
			if err != nil {
				return err
			}
		}

		value := existing.Value
		if strings.TrimSpace(updateGCPMonitoringConfigLocationFiltering) != "" {
			locations := gcpmonitoringconfig.SplitCSV(updateGCPMonitoringConfigLocationFiltering)
			if len(locations) == 0 {
				return fmt.Errorf("--locationFiltering must contain at least one location")
			}
			value.GoogleCloud.LocationFiltering = locations
		}
		if strings.TrimSpace(updateGCPMonitoringConfigFeatureSets) != "" {
			featureSets := gcpmonitoringconfig.SplitCSV(updateGCPMonitoringConfigFeatureSets)
			if len(featureSets) == 0 {
				return fmt.Errorf("--featureSets must contain at least one feature set")
			}
			value.FeatureSets = featureSets
		}

		payload := gcpmonitoringconfig.GCPMonitoringConfig{Scope: existing.Scope, Value: value}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to prepare request payload: %w", err)
		}

		updated, err := handler.Update(existing.ObjectID, body)
		if err != nil {
			return err
		}

		fmt.Printf("GCP monitoring config updated: %s\n", updated.ObjectID)
		return nil
	},
}

func init() {
	updateGCPProviderCmd.AddCommand(updateGCPConnectionCmd)
	updateGCPProviderCmd.AddCommand(updateGCPMonitoringConfigCmd)

	updateGCPConnectionCmd.Flags().StringVar(&updateGCPConnectionName, "name", "", "GCP connection name (used when ID argument is not provided)")
	updateGCPConnectionCmd.Flags().StringVar(&updateGCPConnectionServiceAccountID, "serviceAccountId", "", "Service account email to set")
	updateGCPConnectionCmd.Flags().StringVar(&updateGCPConnectionServiceAccountID, "serviceaccountid", "", "Alias for --serviceAccountId")

	updateGCPMonitoringConfigCmd.Flags().StringVar(&updateGCPMonitoringConfigName, "name", "", "Monitoring config name/description (used when ID argument is not provided)")
	updateGCPMonitoringConfigCmd.Flags().StringVar(&updateGCPMonitoringConfigLocationFiltering, "locationFiltering", "", "Comma-separated locations")
	updateGCPMonitoringConfigCmd.Flags().StringVar(&updateGCPMonitoringConfigFeatureSets, "featureSets", "", "Comma-separated feature sets")
	updateGCPMonitoringConfigCmd.Flags().StringVar(&updateGCPMonitoringConfigFeatureSets, "featuresets", "", "Alias for --featureSets")
}
