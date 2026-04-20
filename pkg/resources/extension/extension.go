package extension

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

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
	Version       string `json:"version,omitempty" table:"VERSION"`
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

// MarshalYAML implements yaml.Marshaler to properly serialize json.RawMessage Value
// as a structured object instead of a byte array.
func (m MonitoringConfiguration) MarshalYAML() (interface{}, error) {
	var parsedValue interface{}
	if len(m.Value) > 0 {
		if err := json.Unmarshal(m.Value, &parsedValue); err != nil {
			// If we can't parse the JSON, fall back to string representation
			parsedValue = string(m.Value)
		}
	}

	return struct {
		Type          string      `yaml:"type,omitempty"`
		ExtensionName string      `yaml:"extensionName,omitempty"`
		ObjectID      string      `yaml:"objectId"`
		Scope         string      `yaml:"scope,omitempty"`
		Value         interface{} `yaml:"value,omitempty"`
	}{
		Type:          m.Type,
		ExtensionName: m.ExtensionName,
		ObjectID:      m.ObjectID,
		Scope:         m.Scope,
		Value:         parsedValue,
	}, nil
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

// ActiveGateEntry represents a single ActiveGate instance within an active gate group
type ActiveGateEntry struct {
	ID     int64           `json:"id"`
	Errors json.RawMessage `json:"errors,omitempty"`
}

// ActiveGateGroupItem represents one active gate group available for an extension version
type ActiveGateGroupItem struct {
	GroupName            string            `json:"groupName" table:"GROUP"`
	AvailableActiveGates int               `json:"availableActiveGates" table:"AVAILABLE"`
	ActiveGates          []ActiveGateEntry `json:"activeGates,omitempty" table:"-"`
}

// ActiveGateGroupList represents the list of active gate groups for an extension version
type ActiveGateGroupList struct {
	Items []ActiveGateGroupItem `json:"items"`
}

// maxPageSize is the maximum page size accepted by the Extensions 2.0 API.
const maxPageSize = 100

// List lists all extensions with automatic pagination
func (h *Handler) List(name string, chunkSize int64) (*ExtensionList, error) {
	var allExtensions []Extension
	var totalCount int
	nextPageKey := ""

	// Cap page size to API maximum
	if chunkSize > maxPageSize {
		chunkSize = maxPageSize
	}

	for {
		var result ExtensionList
		req := h.client.HTTP().R().SetResult(&result)

		client.PaginationParams{
			Style:         client.PaginationDefault,
			PageKeyParam:  "next-page-key",
			PageSizeParam: "page-size",
			NextPageKey:   nextPageKey,
			PageSize:      chunkSize,
			Filters:       map[string]string{"name": name},
		}.Apply(req)

		resp, err := req.Get("/platform/extensions/v2/extensions")

		if err != nil {
			return nil, fmt.Errorf("failed to list extensions: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list extensions: status %d: %s", resp.StatusCode(), resp.Request.URL)
		}

		allExtensions = append(allExtensions, result.Items...)
		totalCount = result.TotalCount

		if chunkSize == 0 || result.NextPageKey == "" {
			break
		}

		nextPageKey = result.NextPageKey
	}

	// Client-side filtering: the API accepts the name parameter but ignores it,
	// so we filter locally using a case-insensitive substring match.
	if name != "" {
		nameLower := strings.ToLower(name)
		filtered := allExtensions[:0]
		for _, ext := range allExtensions {
			if strings.Contains(strings.ToLower(ext.ExtensionName), nameLower) {
				filtered = append(filtered, ext)
			}
		}
		allExtensions = filtered
		totalCount = len(filtered)
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

		client.PaginationParams{
			Style:        client.PaginationDefault,
			PageKeyParam: "next-page-key",
			NextPageKey:  nextPageKey,
		}.Apply(req)

		resp, err := req.Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s", url.PathEscape(extensionName)))
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
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/%s", url.PathEscape(extensionName), url.PathEscape(version)))

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

// GetEnvironmentConfig gets the environment configuration for a specific extension version.
// The version parameter is required by the Dynatrace Extensions 2.0 API.
func (h *Handler) GetEnvironmentConfig(extensionName, version string) (*ExtensionEnvironmentConfig, error) {
	var result ExtensionEnvironmentConfig

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/%s/environmentConfiguration", url.PathEscape(extensionName), url.PathEscape(version)))

	if err != nil {
		return nil, fmt.Errorf("failed to get extension environment config: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("extension %q version %q has no environment configuration", extensionName, version)
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

	// Cap page size to API maximum
	if chunkSize > maxPageSize {
		chunkSize = maxPageSize
	}

	for {
		var result MonitoringConfigurationList
		req := h.client.HTTP().R().SetResult(&result)

		client.PaginationParams{
			Style:         client.PaginationDefault,
			PageKeyParam:  "next-page-key",
			PageSizeParam: "page-size",
			NextPageKey:   nextPageKey,
			PageSize:      chunkSize,
			Filters:       map[string]string{"version": version},
		}.Apply(req)

		resp, err := req.Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations", url.PathEscape(extensionName)))
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

		if chunkSize == 0 || result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	// Client-side filtering: the API accepts the version parameter but ignores it,
	// so we filter locally by extracting the version from the config value JSON.
	if version != "" {
		filtered := allItems[:0]
		for _, item := range allItems {
			if len(item.Value) > 0 {
				var val map[string]interface{}
				if err := json.Unmarshal(item.Value, &val); err == nil {
					if v, ok := val["version"].(string); ok && v == version {
						filtered = append(filtered, item)
					}
				}
			}
		}
		allItems = filtered
		totalCount = len(filtered)
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
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations/%s", url.PathEscape(extensionName), url.PathEscape(configID)))

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
		Post(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations", url.PathEscape(extensionName)))

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
		Put(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations/%s", url.PathEscape(extensionName), url.PathEscape(configID)))
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

// Upload uploads a custom extension zip file to the Dynatrace environment.
// The zipData should contain the raw bytes of the extension zip package.
// The optional fileName is used as the multipart filename; if empty, "extension.zip" is used.
func (h *Handler) Upload(fileName string, zipData []byte) (*ExtensionVersion, error) {
	if fileName == "" {
		fileName = "extension.zip"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart field: %w", err)
	}
	if _, err := part.Write(zipData); err != nil {
		return nil, fmt.Errorf("failed to write extension data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	var result ExtensionVersion

	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetBody(body.Bytes()).
		SetResult(&result).
		Post("/platform/extensions/v2/extensions")

	if err != nil {
		return nil, fmt.Errorf("failed to upload extension: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("invalid extension package: status %d: %s", resp.StatusCode(), resp.String())
		case http.StatusForbidden:
			return nil, fmt.Errorf("access denied: insufficient permissions to upload extensions")
		case http.StatusConflict:
			return nil, fmt.Errorf("extension version already exists: %s", resp.String())
		default:
			return nil, fmt.Errorf("failed to upload extension: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// InstallFromHub installs a Dynatrace Hub extension into the environment using the
// Extensions 2.0 API. extensionName is the hub extension catalog ID (path parameter).
// version is optional — when provided it is sent as a query parameter to select a
// specific release; when empty the API resolves the latest available version.
func (h *Handler) InstallFromHub(extensionName, version string) (*ExtensionVersion, error) {
	var result ExtensionVersion

	req := h.client.HTTP().R().SetResult(&result)
	if version != "" {
		req.SetQueryParam("version", version)
	}

	resp, err := req.Post(fmt.Sprintf("/platform/extensions/v2/extensions/%s", url.PathEscape(extensionName)))
	if err != nil {
		return nil, fmt.Errorf("failed to install Hub extension %q: %w", extensionName, err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case http.StatusNotFound:
			return nil, fmt.Errorf("hub extension %q not found", extensionName)
		case http.StatusForbidden:
			return nil, fmt.Errorf("access denied: insufficient permissions to install extensions")
		case http.StatusConflict:
			if version == "" {
				return nil, fmt.Errorf("hub extension %q (latest version) is already installed", extensionName)
			}
			return nil, fmt.Errorf("hub extension %q version %q is already installed", extensionName, version)
		default:
			return nil, fmt.Errorf("failed to install Hub extension %q: status %d: %s", extensionName, resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// DeleteMonitoringConfiguration deletes a monitoring configuration for an extension
func (h *Handler) DeleteMonitoringConfiguration(extensionName, configID string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/extensions/v2/extensions/%s/monitoring-configurations/%s", url.PathEscape(extensionName), url.PathEscape(configID)))

	if err != nil {
		return fmt.Errorf("failed to delete monitoring configuration: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("monitoring configuration %q not found for extension %q", configID, extensionName)
		case 403:
			return fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return fmt.Errorf("failed to delete monitoring configuration: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetMonitoringConfigurationSchema retrieves the monitoring configuration schema for a specific
// extension version. The schema is an arbitrary JSON Schema document returned verbatim.
func (h *Handler) GetMonitoringConfigurationSchema(extensionName, version string) (json.RawMessage, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/%s/schema", url.PathEscape(extensionName), url.PathEscape(version)))

	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring configuration schema: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case http.StatusNotFound:
			return nil, fmt.Errorf("extension %q version %q not found", extensionName, version)
		case http.StatusForbidden:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to get monitoring configuration schema: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return json.RawMessage(resp.Body()), nil
}

// GetActiveGateGroups retrieves the active gate groups available for a specific extension version.
func (h *Handler) GetActiveGateGroups(extensionName, version string) (*ActiveGateGroupList, error) {
	var result ActiveGateGroupList

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/extensions/v2/extensions/%s/%s/active-gate-groups", url.PathEscape(extensionName), url.PathEscape(version)))

	if err != nil {
		return nil, fmt.Errorf("failed to get active gate groups: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case http.StatusNotFound:
			return nil, fmt.Errorf("extension %q version %q not found", extensionName, version)
		case http.StatusForbidden:
			return nil, fmt.Errorf("access denied to extension %q", extensionName)
		default:
			return nil, fmt.Errorf("failed to get active gate groups: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}
