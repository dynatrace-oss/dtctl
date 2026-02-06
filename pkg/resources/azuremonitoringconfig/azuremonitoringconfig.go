package azuremonitoringconfig

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

const (
	ExtensionName = "com.dynatrace.extension.da-azure"
	BaseAPI       = "/platform/extensions/v2/extensions/" + ExtensionName + "/monitoring-configurations"
)

type Handler struct {
	client *client.Client
}

func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

type AzureMonitoringConfig struct {
	ObjectID string `json:"objectId,omitempty" table:"ID"`
	Scope    string `json:"scope,omitempty"`
	Value    Value  `json:"value" table:"-"`

	// Flattened fields for table view
	Description string `json:"description" table:"DESCRIPTION"`
	Enabled     bool   `json:"enabled" table:"ENABLED"`
	Version     string `json:"version" table:"VERSION"`
}

type Value struct {
	Enabled     bool        `json:"enabled"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	Azure       AzureConfig `json:"azure"`
	FeatureSets []string    `json:"featureSets"`
}

type AzureConfig struct {
	DeploymentScope           string            `json:"deploymentScope,omitempty"`
	SubscriptionFilteringMode string            `json:"subscriptionFilteringMode,omitempty"`
	Credentials               []Credential      `json:"credentials"`
	LocationFiltering         []string          `json:"locationFiltering,omitempty"`
	ConfigurationMode         string            `json:"configurationMode,omitempty"`
	DeploymentMode            string            `json:"deploymentMode,omitempty"`
	TagFiltering              []TagFilter       `json:"tagFiltering,omitempty"`
	TagEnrichment             []string          `json:"tagEnrichment,omitempty"`
	DtLabelsEnrichment        map[string]Labels `json:"dtLabelsEnrichment,omitempty"`
}

type TagFilter struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Condition string `json:"condition"`
}

type Labels struct {
	Literal string `json:"literal,omitempty"`
	TagKey  string `json:"tagKey,omitempty"`
}

type Credential struct {
	Enabled            bool   `json:"enabled"`
	Description        string `json:"description"`
	ConnectionId       string `json:"connectionId"`
	ServicePrincipalId string `json:"servicePrincipalId"`
	Type               string `json:"type"`
}

type ListResponse struct {
	Items []AzureMonitoringConfig `json:"items"`
}

func (h *Handler) Get(id string) (*AzureMonitoringConfig, error) {
	var result AzureMonitoringConfig
	req := h.client.HTTP().R().SetResult(&result)
	resp, err := req.Get(fmt.Sprintf("%s/%s", BaseAPI, id))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to get azure_monitoring_config: %s", resp.String())
	}

	result.Description = result.Value.Description
	result.Enabled = result.Value.Enabled
	result.Version = result.Value.Version

	return &result, nil
}

func (h *Handler) List() ([]AzureMonitoringConfig, error) {
	var result ListResponse
	req := h.client.HTTP().R().SetResult(&result)
	
	resp, err := req.Get(BaseAPI)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to list azure_monitoring_configs: %s", resp.String())
	}

	for i := range result.Items {
		result.Items[i].Description = result.Items[i].Value.Description
		result.Items[i].Enabled = result.Items[i].Value.Enabled
		result.Items[i].Version = result.Items[i].Value.Version
	}

	return result.Items, nil
}

// FindByName finds an Azure monitoring config by description (name)
func (h *Handler) FindByName(name string) (*AzureMonitoringConfig, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		// Matching by Description as it serves as the name
		if item.Description == name {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("Azure monitoring config with description %q not found", name)
}

// Create creates a new Azure monitoring config
func (h *Handler) Create(data []byte) (*AzureMonitoringConfig, error) {
	var result AzureMonitoringConfig
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Post(BaseAPI)

	if err != nil {
		return nil, fmt.Errorf("failed to create azure_monitoring_config: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to create azure_monitoring_config: %s", resp.String())
	}

	return &result, nil
}

// Update updates an existing Azure monitoring config
func (h *Handler) Update(id string, data []byte) (*AzureMonitoringConfig, error) {
	var result AzureMonitoringConfig
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		SetResult(&result).
		Put(fmt.Sprintf("%s/%s", BaseAPI, id))

	if err != nil {
		return nil, fmt.Errorf("failed to update azure_monitoring_config: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to update azure_monitoring_config: %s", resp.String())
	}

	return &result, nil
}
