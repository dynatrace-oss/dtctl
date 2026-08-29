package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/spf13/cobra"
)

func TestCloudMonitoringCentralEnrichmentFlagsAreHiddenAndDisabledByDefault(t *testing.T) {
	commands := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "aws", cmd: createAWSMonitoringConfigCmd},
		{name: "azure", cmd: createAzureMonitoringConfigCmd},
		{name: "gcp", cmd: createGCPMonitoringConfigCmd},
	}

	for _, test := range commands {
		t.Run(test.name, func(t *testing.T) {
			flag := test.cmd.Flags().Lookup("central-enrichment")
			if flag == nil {
				t.Fatal("central-enrichment flag is not registered")
			}
			if !flag.Hidden {
				t.Error("central-enrichment flag must remain hidden before customer release")
			}
			if flag.DefValue != "false" {
				t.Errorf("central-enrichment default = %q, want false", flag.DefValue)
			}
		})
	}
}

func TestCentralEnrichmentIntent(t *testing.T) {
	if centralEnrichmentIntent(false) != nil {
		t.Error("disabled central enrichment must be omitted from the request")
	}
	intent := centralEnrichmentIntent(true)
	if intent == nil || !*intent {
		t.Error("enabled central enrichment must be sent as true")
	}
}

func TestCloudMonitoringCreatePayloadCentralIntent(t *testing.T) {
	for _, test := range []struct {
		name    string
		central bool
		payload any
	}{
		{
			name: "aws legacy default",
			payload: buildAWSMonitoringConfig("aws", "1", awsmonitoringconfig.Credential{},
				[]string{"eu-central-1"}, nil, false),
		},
		{
			name:    "aws central",
			central: true,
			payload: buildAWSMonitoringConfig("aws", "1", awsmonitoringconfig.Credential{},
				[]string{"eu-central-1"}, nil, true),
		},
		{
			name:    "azure legacy default",
			payload: buildAzureMonitoringConfig("azure", "1", azuremonitoringconfig.Credential{}, nil, nil, false),
		},
		{
			name:    "azure central",
			central: true,
			payload: buildAzureMonitoringConfig("azure", "1", azuremonitoringconfig.Credential{}, nil, nil, true),
		},
		{
			name:    "gcp legacy default",
			payload: buildGCPMonitoringConfig("gcp", "1", gcpmonitoringconfig.Credential{}, nil, nil, false),
		},
		{
			name:    "gcp central",
			central: true,
			payload: buildGCPMonitoringConfig("gcp", "1", gcpmonitoringconfig.Credential{}, nil, nil, true),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.payload)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			text := string(encoded)
			present := strings.Contains(text, `"useIngestEnrichmentConfig":true`)
			if present != test.central {
				t.Errorf("central intent presence = %v, want %v: %s", present, test.central, text)
			}
			if strings.Contains(text, "ingestEnrichmentMigrationProcessed") {
				t.Errorf("payload contains backend-owned processed marker: %s", text)
			}
		})
	}
}
