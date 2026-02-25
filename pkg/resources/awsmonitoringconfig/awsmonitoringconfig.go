package awsmonitoringconfig

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

const (
	ExtensionName      = "com.dynatrace.extension.da-aws"
	BaseAPI            = "/platform/extensions/v2/extensions/" + ExtensionName + "/monitoring-configurations"
	ExtensionAPI       = "/platform/extensions/v2/extensions/" + ExtensionName
	ExtensionSchemaAPI = ExtensionAPI + "/%s/schema"
)

type Handler struct {
	client *client.Client
}

func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

type ExtensionResponse struct {
	Items []ExtensionItem `json:"items"`
}

type ExtensionItem struct {
	Version string `json:"version"`
}

type SchemaEnumItem struct {
	Value string `json:"value"`
}

type SchemaEnum struct {
	Items []SchemaEnumItem `json:"items"`
}

type ExtensionSchemaResponse struct {
	Enums map[string]SchemaEnum `json:"enums"`
}

type Region struct {
	Value string `json:"value" table:"REGION"`
}

type FeatureSet struct {
	Value string `json:"value" table:"FEATURE_SET"`
}

type AWSMonitoringConfig struct {
	ObjectID string `json:"objectId,omitempty" table:"ID"`
	Scope    string `json:"scope,omitempty"`
	Value    Value  `json:"value" table:"-"`

	Description string `json:"description" table:"DESCRIPTION"`
	Enabled     bool   `json:"enabled" table:"ENABLED"`
	Version     string `json:"version" table:"VERSION"`
}

type Value struct {
	Enabled           bool      `json:"enabled"`
	Description       string    `json:"description"`
	Version           string    `json:"version"`
	FeatureSets       []string  `json:"featureSets"`
	ActivationContext string    `json:"activationContext,omitempty"`
	AWS               AWSConfig `json:"aws"`
}

type AWSConfig struct {
	DeploymentRegion                   string               `json:"deploymentRegion,omitempty"`
	Credentials                        []Credential         `json:"credentials"`
	RegionFiltering                    []string             `json:"regionFiltering,omitempty"`
	TagFiltering                       []TagFilter          `json:"tagFiltering,omitempty"`
	TagEnrichment                      []string             `json:"tagEnrichment,omitempty"`
	DTLabelsEnrichment                 map[string]LabelRule `json:"dtLabelsEnrichment,omitempty"`
	MetricsConfiguration               FlagConfig           `json:"metricsConfiguration,omitempty"`
	CloudWatchLogsConfiguration        FlagConfig           `json:"cloudWatchLogsConfiguration,omitempty"`
	EventsConfiguration                FlagConfig           `json:"eventsConfiguration,omitempty"`
	Namespaces                         []Namespace          `json:"namespaces,omitempty"`
	ConfigurationMode                  string               `json:"configurationMode,omitempty"`
	DeploymentMode                     string               `json:"deploymentMode,omitempty"`
	DeploymentScope                    string               `json:"deploymentScope,omitempty"`
	IngestPercentileMetrics            bool                 `json:"ingestPercentileMetrics,omitempty"`
	IngestS3StorageLENSMetrics         bool                 `json:"ingestS3StorageLENSMetrics,omitempty"`
	ManualDeploymentStatus             string               `json:"manualDeploymentStatus,omitempty"`
	AutomatedDeploymentTemplateVersion string               `json:"automatedDeploymentTemplateVersion,omitempty"`
}

type Credential struct {
	Description                 string `json:"description"`
	Enabled                     bool   `json:"enabled"`
	ConnectionID                string `json:"connectionId"`
	AccountID                   string `json:"accountId,omitempty"`
	OverrideParentConfiguration bool   `json:"overrideParentConfiguration,omitempty"`
}

type TagFilter struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Condition string `json:"condition"`
}

type LabelRule struct {
	Literal string `json:"literal,omitempty"`
	TagKey  string `json:"tagKey,omitempty"`
}

type FlagConfig struct {
	Enabled bool     `json:"enabled"`
	Regions []string `json:"regions,omitempty"`
}

type Namespace struct {
	Namespace            string   `json:"namespace"`
	AutoDiscoveryEnabled bool     `json:"autoDiscoveryEnabled"`
	Metrics              []Metric `json:"metrics,omitempty"`
}

type Metric struct {
	Name                 string   `json:"name"`
	Unit                 string   `json:"unit,omitempty"`
	Dimensions           []string `json:"dimensions,omitempty"`
	Aggregations         []string `json:"aggregations,omitempty"`
	UseCustomAggregation bool     `json:"useCustomAggregation,omitempty"`
	Type                 string   `json:"type,omitempty"`
}

type ListResponse struct {
	Items []AWSMonitoringConfig `json:"items"`
}

func (h *Handler) GetLatestVersion() (string, error) {
	var result ExtensionResponse
	resp, err := h.client.HTTP().R().SetResult(&result).Get(ExtensionAPI)
	if err != nil {
		return "", fmt.Errorf("failed to fetch extension versions: %w", err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("failed to fetch extension versions: %s", resp.String())
	}

	versions := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if item.Version != "" {
			versions = append(versions, item.Version)
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for extension %s", ExtensionName)
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersion(versions[i], versions[j]) > 0
	})

	return versions[0], nil
}

func compareVersion(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for idx := 0; idx < maxLen; idx++ {
		aVal := 0
		if idx < len(aParts) {
			aVal, _ = strconv.Atoi(aParts[idx])
		}
		bVal := 0
		if idx < len(bParts) {
			bVal, _ = strconv.Atoi(bParts[idx])
		}
		if aVal > bVal {
			return 1
		}
		if aVal < bVal {
			return -1
		}
	}

	return 0
}

func (h *Handler) ListAvailableRegions() ([]Region, error) {
	latestVersion, err := h.GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to determine latest extension version: %w", err)
	}

	var schema ExtensionSchemaResponse
	schemaEndpoint := fmt.Sprintf(ExtensionSchemaAPI, latestVersion)
	resp, err := h.client.HTTP().R().SetResult(&schema).Get(schemaEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch extension schema: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to fetch extension schema: %s", resp.String())
	}

	regionEnum, ok := schema.Enums["dynatrace.datasource.aws:region"]
	if !ok {
		return nil, fmt.Errorf("schema enum %q not found", "dynatrace.datasource.aws:region")
	}

	regions := make([]Region, 0, len(regionEnum.Items))
	for _, item := range regionEnum.Items {
		if item.Value != "" {
			regions = append(regions, Region{Value: item.Value})
		}
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("no regions found in schema enum %q", "dynatrace.datasource.aws:region")
	}

	return regions, nil
}

func (h *Handler) ListAvailableFeatureSets() ([]FeatureSet, error) {
	latestVersion, err := h.GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to determine latest extension version: %w", err)
	}

	var schema ExtensionSchemaResponse
	schemaEndpoint := fmt.Sprintf(ExtensionSchemaAPI, latestVersion)
	resp, err := h.client.HTTP().R().SetResult(&schema).Get(schemaEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch extension schema: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to fetch extension schema: %s", resp.String())
	}

	featureSetEnum, ok := schema.Enums["FeatureSetsType"]
	if !ok {
		return nil, fmt.Errorf("schema enum %q not found", "FeatureSetsType")
	}

	featureSets := make([]FeatureSet, 0, len(featureSetEnum.Items))
	for _, item := range featureSetEnum.Items {
		if item.Value != "" {
			featureSets = append(featureSets, FeatureSet{Value: item.Value})
		}
	}
	if len(featureSets) == 0 {
		return nil, fmt.Errorf("no feature sets found in schema enum %q", "FeatureSetsType")
	}

	sort.Slice(featureSets, func(i, j int) bool {
		return featureSets[i].Value < featureSets[j].Value
	})

	return featureSets, nil
}

func (h *Handler) Get(id string) (*AWSMonitoringConfig, error) {
	var result AWSMonitoringConfig
	resp, err := h.client.HTTP().R().SetResult(&result).Get(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to get aws_monitoring_config: %s", resp.String())
	}

	result.Description = result.Value.Description
	result.Enabled = result.Value.Enabled
	result.Version = result.Value.Version

	return &result, nil
}

func (h *Handler) List() ([]AWSMonitoringConfig, error) {
	var result ListResponse
	resp, err := h.client.HTTP().R().SetResult(&result).Get(BaseAPI)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to list aws_monitoring_configs: %s", resp.String())
	}

	for i := range result.Items {
		result.Items[i].Description = result.Items[i].Value.Description
		result.Items[i].Enabled = result.Items[i].Value.Enabled
		result.Items[i].Version = result.Items[i].Value.Version
	}

	return result.Items, nil
}

func (h *Handler) FindByName(name string) (*AWSMonitoringConfig, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Description == name {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("AWS monitoring config with description %q not found", name)
}

func (h *Handler) Create(data []byte) (*AWSMonitoringConfig, error) {
	var result AWSMonitoringConfig
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Post(BaseAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws_monitoring_config: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to create aws_monitoring_config: %s", resp.String())
	}

	return &result, nil
}

func (h *Handler) Update(id string, data []byte) (*AWSMonitoringConfig, error) {
	var result AWSMonitoringConfig
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Put(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return nil, fmt.Errorf("failed to update aws_monitoring_config: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to update aws_monitoring_config: %s", resp.String())
	}

	return &result, nil
}

func (h *Handler) Delete(id string) error {
	resp, err := h.client.HTTP().R().Delete(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("failed to delete aws_monitoring_config: status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}
