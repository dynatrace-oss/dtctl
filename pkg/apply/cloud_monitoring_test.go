package apply

import (
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
)

func TestParseCloudMonitoringDocumentPreservesFields(t *testing.T) {
	document, err := parseCloudMonitoringDocument([]byte(`{
		"scope":"integration-aws",
		"Value":{"AWS":{"useIngestEnrichmentConfig":true,"futureLegacyEnrichment":["keep"],"futureNumber":9007199254740993}}
	}`), "aws")
	if err != nil {
		t.Fatalf("parseCloudMonitoringDocument() error = %v", err)
	}
	central := true
	config := awsmonitoringconfig.AWSMonitoringConfig{
		ObjectID: "config-1",
		Value: awsmonitoringconfig.Value{
			Version: "1.2.3",
			Aws: awsmonitoringconfig.AWSConfig{
				UseIngestEnrichmentConfig: &central,
			},
		},
	}
	encoded, err := document.marshal(config, config.Value.Aws)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(encoded)
	for _, want := range []string{
		`"useIngestEnrichmentConfig":true`,
		`"futureLegacyEnrichment":["keep"]`,
		`"futureNumber":9007199254740993`,
		`"version":"1.2.3"`,
		`"objectId":"config-1"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("encoded document %s does not contain %s", text, want)
		}
	}
	if strings.Contains(text, `"AWS"`) || strings.Contains(text, `"Value"`) {
		t.Errorf("known field names were not canonicalized: %s", text)
	}
}

func TestParseCloudMonitoringDocumentRejectsProcessedMarker(t *testing.T) {
	for _, cloud := range []string{"aws", "azure", "googleCloud"} {
		t.Run(cloud, func(t *testing.T) {
			data := []byte(`{"value":{"` + cloud + `":{"ingestEnrichmentMigrationProcessed":false}}}`)
			_, err := parseCloudMonitoringDocument(data, cloud)
			if err == nil || !strings.Contains(err.Error(), "backend-owned") {
				t.Fatalf("parseCloudMonitoringDocument() error = %v, want backend-owned rejection", err)
			}
		})
	}
}

func TestApplyDryRunRejectsProcessedMarker(t *testing.T) {
	for name, data := range map[string]string{
		"single": `{"scope":"integration-aws","value":{"aws":{"ingestEnrichmentMigrationProcessed":null}}}`,
		"array":  `[{"scope":"integration-aws","value":{"aws":{"ingestEnrichmentMigrationProcessed":true}}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&Applier{}).Apply([]byte(data), ApplyOptions{DryRun: true})
			if err == nil || !strings.Contains(err.Error(), "backend-owned") {
				t.Fatalf("Apply() error = %v, want backend-owned rejection", err)
			}
		})
	}
}
