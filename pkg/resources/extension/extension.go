package extension

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles Extensions 2.0 resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new Extension handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// Extension represents an Extensions 2.0 extension
type Extension struct {
	ExtensionName string `json:"extensionName" table:"NAME"`
	ActiveVersion string `json:"activeVersion,omitempty" table:"ACTIVE VERSION"`
}

// ExtensionList represents a paginated list of extensions
type ExtensionList struct {
	Items       []Extension `json:"items"`
	TotalCount  int         `json:"totalCount"`
	NextPageKey string      `json:"nextPageKey,omitempty"`
}

// ExtensionVersion represents a specific version of an extension
type ExtensionVersion struct {
	Version       string `json:"version" table:"VERSION"`
	ExtensionName string `json:"extensionName" table:"NAME"`
	Active        bool   `json:"active,omitempty" table:"ACTIVE"`
}

// ExtensionVersionList represents a list of extension versions
type ExtensionVersionList struct {
	Items       []ExtensionVersion `json:"items"`
	TotalCount  int                `json:"totalCount"`
	NextPageKey string             `json:"nextPageKey,omitempty"`
}

// ExtensionDetails represents detailed information about an extension version
type ExtensionDetails struct {
	ExtensionName       string                      `json:"extensionName"`
	Version             string                      `json:"version"`
	Author              ExtensionAuthor             `json:"author,omitempty"`
	DataSources         []string                    `json:"dataSources,omitempty"`
	FeatureSets         []string                    `json:"featureSets,omitempty"`
	FeatureSetDetails   map[string]FeatureSetDetail `json:"featureSetDetails,omitempty"`
	FileHash            string                      `json:"fileHash,omitempty"`
	MinDynatraceVersion string                      `json:"minDynatraceVersion,omitempty"`
	MinEECVersion       string                      `json:"minEECVersion,omitempty"`
	Variables           []ExtensionVariable         `json:"vars,omitempty"`
}

// ExtensionAuthor represents the author of an extension
type ExtensionAuthor struct {
	Name string `json:"name"`
}

// FeatureSetDetail represents a feature set of an extension
type FeatureSetDetail struct {
	Metrics []FeatureSetMetric `json:"metrics,omitempty"`
}

// FeatureSetMetric represents a metric within a feature set
type FeatureSetMetric struct {
	Key string `json:"key"`
}

// ExtensionVariable represents a variable defined in an extension
type ExtensionVariable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DisplayName string `json:"displayName,omitempty"`
}

// MonitoringConfiguration represents an extension monitoring configuration instance
type MonitoringConfiguration struct {
	Type          string          `json:"type,omitempty" yaml:"type,omitempty" table:"-"`
	ExtensionName string          `json:"extensionName,omitempty" table:"EXTENSION"`
	ObjectID      string          `json:"objectId" table:"ID"`
	Scope         string          `json:"scope,omitempty" table:"SCOPE"`
	Value         json.RawMessage `json:"value,omitempty" table:"-"`
}

// MonitoringConfigurationList represents a list of monitoring configuration instances
type MonitoringConfigurationList struct {
	Items       []MonitoringConfiguration `json:"items"`
	TotalCount  int                       `json:"totalCount"`
	NextPageKey string                    `json:"nextPageKey,omitempty"`
}

// ExtensionEnvironmentConfig represents the environment-wide configuration for an extension
type ExtensionEnvironmentConfig struct {
	Version string `json:"version"`
}

// ExtensionStatus represents the monitoring status of a specific extension version
type ExtensionStatus struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp,omitempty"`
}

// List lists all extensions with automatic pagination
func (h *Handler) List(name string, chunkSize int64) (*ExtensionList, error) {
	var allExtensions []Extension
	var totalCount int
	nextPageKey := ""

	for {
		var result ExtensionList
		req := h.client.HTTP().R().SetResult(&result)

		if name != "" {
			req.SetQueryParam("name", name)
		}

		if chunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
		}

		if nextPageKey != "" {
			req.SetQueryParam("next-page-key", nextPageKey)
		}

		resp, err := req.Get("/platform/extensions/v2/extensions")

		if err != nil {
			return nil, fmt.Errorf("failed to list extensions: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list extensions: status %d: %s", resp.StatusCode(), resp.Request.URL)
		}

		allExtensions = append(allExtensions, result.Items...)
		totalCount = result.TotalCount

		if chunkSize == 0 {
			return &result, nil
		}

		if result.NextPageKey == "" {
			break
		}

		nextPageKey = result.NextPageKey
	}

	return &ExtensionList{
		Items:      allExtensions,
		TotalCount: totalCount,
	}, nil
}

// Get gets a specific extension by name (returns all versions)
func (h *Handler) Get(extensionName string) (*ExtensionVersionList, error) {
	var allVersions []ExtensionVersion
	var totalCount int
	nextPageKey := ""

	for {
		var result ExtensionVersionList
		req := h.client.HTTP().R().SetResult(&result)

		if nextPageKey != "" {
			req.SetQueryParam("next-page-key", nextPageKey)
		}

		resp, err := req.Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s", extensionName))
		if err != nil {
			return nil, fmt.Errorf("failed to get extension: %w", err)
		}

		if resp.IsError() {
			switch resp.StatusCode() {
			case 404:
				return nil, fmt.Errorf("extension %q not found", extensionName)
			case 403:
				return nil, fmt.Errorf("access denied to extension %q", extensionName)
			default:
				return nil, fmt.Errorf("failed to get extension: status %d: %s", resp.StatusCode(), resp.String())
			}
		}

		allVersions = append(allVersions, result.Items...)
		totalCount = result.TotalCount

		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &ExtensionVersionList{
		Items:      allVersions,
		TotalCount: totalCount,
	}, nil
}

// GetVersion gets details for a specific extension version
func (h *Handler) GetVersion(extensionName, version string) (*ExtensionDetails, error) {
	var result ExtensionDetails
	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/%s", extensionName, version))

	if err != nil {
		return nil, fmt.Errorf("failed to get extension version: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("extension %q version %q not found", extensionName, version)
		case 403:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to get extension version: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// GetEnvironmentConfig gets the active environment configuration for an extension
func (h *Handler) GetEnvironmentConfig(extensionName string) (*ExtensionEnvironmentConfig, error) {
	var result ExtensionEnvironmentConfig

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/extensions/v2/%s/environment-configuration", extensionName))

	if err != nil {
		return nil, fmt.Errorf("failed to get extension environment config: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("extension %q has no active environment configuration", extensionName)
		case 403:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to get extension environment config: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// ListMonitoringConfigurations lists monitoring configurations for an extension
func (h *Handler) ListMonitoringConfigurations(extensionName, version string, chunkSize int64) (*MonitoringConfigurationList, error) {
	var allItems []MonitoringConfiguration
	var totalCount int
	nextPageKey := ""

	for {
		var result MonitoringConfigurationList
		req := h.client.HTTP().R().SetResult(&result)

		if version != "" {
			req.SetQueryParam("version", version)
		}

		if chunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
		}

		if nextPageKey != "" {
			req.SetQueryParam("next-page-key", nextPageKey)
		}

		resp, err := req.Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations", extensionName))
		if err != nil {
			return nil, fmt.Errorf("failed to list monitoring configurations: %w", err)
		}

		if resp.IsError() {
			switch resp.StatusCode() {
			case 404:
				return nil, fmt.Errorf("extension %q not found", extensionName)
			case 403:
				return nil, fmt.Errorf("access denied to extension %q", extensionName)
			default:
				return nil, fmt.Errorf("failed to list monitoring configurations: status %d: %s", resp.StatusCode(), resp.String())
			}
		}

		for i := range result.Items {
			result.Items[i].Type = "extension_monitoring_config"
			result.Items[i].ExtensionName = extensionName
		}
		allItems = append(allItems, result.Items...)
		totalCount = result.TotalCount

		if chunkSize == 0 {
			return &result, nil
		}

		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &MonitoringConfigurationList{
		Items:      allItems,
		TotalCount: totalCount,
	}, nil
}

// GetMonitoringConfiguration gets a specific monitoring configuration
func (h *Handler) GetMonitoringConfiguration(extensionName, configID string) (*MonitoringConfiguration, error) {
	var result MonitoringConfiguration

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations/%s", extensionName, configID))

	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring configuration: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("monitoring configuration %q not found for extension %q", configID, extensionName)
		case 403:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to get monitoring configuration: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	result.Type = "extension_monitoring_config"
	result.ExtensionName = extensionName
	return &result, nil
}

// MonitoringConfigurationCreate represents the body for creating/updating a monitoring configuration
type MonitoringConfigurationCreate struct {
	Scope string         `json:"scope,omitempty"`
	Value map[string]any `json:"value"`
}

// CreateMonitoringConfiguration creates a new monitoring configuration for an extension
func (h *Handler) CreateMonitoringConfiguration(extensionName string, body MonitoringConfigurationCreate) (*MonitoringConfiguration, error) {
	var result MonitoringConfiguration

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetResult(&result).
		Post(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations", extensionName))

	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring configuration: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("extension %q not found", extensionName)
		case 403:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to create monitoring configuration: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// UpdateMonitoringConfiguration updates an existing monitoring configuration for an extension
func (h *Handler) UpdateMonitoringConfiguration(extensionName, configID string, body MonitoringConfigurationCreate) (*MonitoringConfiguration, error) {
	var result MonitoringConfiguration

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetResult(&result).
		Put(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations/%s", extensionName, configID))
	if err != nil {
		return nil, fmt.Errorf("failed to update monitoring configuration: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("monitoring configuration %q not found for extension %q", configID, extensionName)
		case 403:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to update monitoring configuration: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}
