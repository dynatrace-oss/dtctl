package gcpmonitoringconfig

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const (
	ExtensionName      = "com.dynatrace.extension.da-gcp"
	BaseAPI            = "/platform/extensions/v2/extensions/" + ExtensionName + "/monitoring-configurations"
	ExtensionAPI       = "/platform/extensions/v2/extensions/" + ExtensionName
	ExtensionSchemaAPI = ExtensionAPI + "/%s/schema"
)

type Handler struct {
	client *httpclient.Client
}

func NewHandler(c *httpclient.Client) *Handler {
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

type Location struct {
	Value string `json:"value" table:"LOCATION"`
}

type FeatureSet struct {
	Value string `json:"value" table:"FEATURE_SET"`
}

type GCPMonitoringConfig struct {
	ObjectID string `json:"objectId,omitempty" table:"ID"`
	Scope    string `json:"scope,omitempty"`
	Value    Value  `json:"value" table:"-"`

	Description string `json:"description" table:"DESCRIPTION"`
	Enabled     bool   `json:"enabled" table:"ENABLED"`
	Version     string `json:"version" table:"VERSION"`
}

type Value struct {
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	GoogleCloud GoogleCloudConfig `json:"googleCloud"`
	FeatureSets []string          `json:"featureSets"`
}

type GoogleCloudConfig struct {
	Credentials                []Credential   `json:"credentials"`
	LocationFiltering          []string       `json:"locationFiltering,omitempty"`
	ProjectFiltering           []string       `json:"projectFiltering,omitempty"`
	FolderFiltering            []string       `json:"folderFiltering,omitempty"`
	TagFiltering               []TagFilter    `json:"tagFiltering,omitempty"`
	LabelFiltering             []TagFilter    `json:"labelFiltering,omitempty"`
	TagEnrichment              []string       `json:"tagEnrichment,omitempty"`
	LabelEnrichment            []string       `json:"labelEnrichment,omitempty"`
	ObservabilityScopesEnabled bool           `json:"observabilityScopesEnabled,omitempty"`
	SmartscapeConfiguration    FlagConfig     `json:"smartscapeConfiguration,omitempty"`
	Resources                  []MetricSource `json:"resources,omitempty"`
}

type TagFilter struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Condition string `json:"condition"`
}

type FlagConfig struct {
	Enabled bool `json:"enabled"`
}

type MetricSource struct {
	ResourceType                   string   `json:"resourceType"`
	AutoDiscoveryEnabled           bool     `json:"autoDiscoveryEnabled"`
	AutodiscoveryExcludeMetricType []string `json:"autodiscoveryExcludeMetricType,omitempty"`
}

type Credential struct {
	Description    string `json:"description"`
	Enabled        bool   `json:"enabled"`
	ConnectionID   string `json:"connectionId"`
	ServiceAccount string `json:"serviceAccount"`
}

type ListResponse struct {
	Items []GCPMonitoringConfig `json:"items"`
}

func (h *Handler) GetLatestVersion() (string, error) {
	var result ExtensionResponse
	resp, err := h.client.HTTP().R().SetResult(&result).Get(ExtensionAPI)
	if err != nil {
		return "", fmt.Errorf("fetch extension versions: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return "", fmt.Errorf("fetch extension versions: %w", err)
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

func (h *Handler) ListAvailableLocations() ([]Location, error) {
	latestVersion, err := h.GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("determine latest extension version: %w", err)
	}

	var schema ExtensionSchemaResponse
	schemaEndpoint := fmt.Sprintf(ExtensionSchemaAPI, latestVersion)
	resp, err := h.client.HTTP().R().SetResult(&schema).Get(schemaEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fetch extension schema: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("fetch extension schema: %w", err)
	}

	locationEnum, ok := schema.Enums["dynatrace.datasource.gcp:location"]
	if !ok {
		return nil, fmt.Errorf("schema enum %q not found", "dynatrace.datasource.gcp:location")
	}

	locations := make([]Location, 0, len(locationEnum.Items))
	for _, item := range locationEnum.Items {
		if item.Value != "" {
			locations = append(locations, Location{Value: item.Value})
		}
	}
	if len(locations) == 0 {
		return nil, fmt.Errorf("no locations found in schema enum %q", "dynatrace.datasource.gcp:location")
	}

	return locations, nil
}

func (h *Handler) ListAvailableFeatureSets() ([]FeatureSet, error) {
	latestVersion, err := h.GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("determine latest extension version: %w", err)
	}

	var schema ExtensionSchemaResponse
	schemaEndpoint := fmt.Sprintf(ExtensionSchemaAPI, latestVersion)
	resp, err := h.client.HTTP().R().SetResult(&schema).Get(schemaEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fetch extension schema: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("fetch extension schema: %w", err)
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

func (h *Handler) Get(id string) (*GCPMonitoringConfig, error) {
	var result GCPMonitoringConfig
	resp, err := h.client.HTTP().R().SetResult(&result).Get(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return nil, fmt.Errorf("get gcp monitoring config %q: %w", id, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get gcp monitoring config %q: %w", id, err)
	}

	result.Description = result.Value.Description
	result.Enabled = result.Value.Enabled
	result.Version = result.Value.Version

	return &result, nil
}

func (h *Handler) List() ([]GCPMonitoringConfig, error) {
	var result ListResponse
	resp, err := h.client.HTTP().R().SetResult(&result).Get(BaseAPI)
	if err != nil {
		return nil, fmt.Errorf("list gcp monitoring configs: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list gcp monitoring configs: %w", err)
	}

	for i := range result.Items {
		result.Items[i].Description = result.Items[i].Value.Description
		result.Items[i].Enabled = result.Items[i].Value.Enabled
		result.Items[i].Version = result.Items[i].Value.Version
	}

	return result.Items, nil
}

func (h *Handler) FindByName(name string) (*GCPMonitoringConfig, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Description == name {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("GCP monitoring config with description %q not found", name)
}

func (h *Handler) Create(data []byte) (*GCPMonitoringConfig, error) {
	var result GCPMonitoringConfig
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Post(BaseAPI)
	if err != nil {
		return nil, fmt.Errorf("create gcp monitoring config: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create gcp monitoring config: %w", err)
	}

	return &result, nil
}

func (h *Handler) Update(id string, data []byte) (*GCPMonitoringConfig, error) {
	var result GCPMonitoringConfig
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Put(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return nil, fmt.Errorf("update gcp monitoring config %q: %w", id, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("update gcp monitoring config %q: %w", id, err)
	}

	return &result, nil
}

func (h *Handler) Delete(id string) error {
	resp, err := h.client.HTTP().R().Delete(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return fmt.Errorf("delete gcp monitoring config %q: %w", id, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete gcp monitoring config %q: %w", id, err)
	}
	return nil
}
