package extension

import (
	"context"
	"encoding/json"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkext "github.com/dynatrace-oss/dtctl/sdk/api/extension"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Extension is the CLI read model for an extension.
type Extension struct {
	ExtensionName string `json:"extensionName" table:"NAME"`
	Version       string `json:"version,omitempty" table:"VERSION"`
}

// fromSDKExtension converts an SDK Extension to the CLI Extension.
func fromSDKExtension(e *sdkext.Extension) Extension {
	return Extension{
		ExtensionName: e.ExtensionName,
		Version:       e.Version,
	}
}

// ExtensionList represents a paginated list of extensions.
type ExtensionList struct {
	Items       []Extension `json:"items"`
	TotalCount  int         `json:"totalCount"`
	NextPageKey string      `json:"nextPageKey,omitempty"`
}

// fromSDKExtensionList converts an SDK ExtensionList to the CLI ExtensionList.
func fromSDKExtensionList(l *sdkext.ExtensionList) *ExtensionList {
	items := make([]Extension, len(l.Items))
	for i := range l.Items {
		items[i] = fromSDKExtension(&l.Items[i])
	}
	return &ExtensionList{
		Items:       items,
		TotalCount:  l.TotalCount,
		NextPageKey: l.NextPageKey,
	}
}

// ExtensionVersion is the CLI read model for a specific version of an extension.
type ExtensionVersion struct {
	Version       string `json:"version" table:"VERSION"`
	ExtensionName string `json:"extensionName" table:"NAME"`
	Active        bool   `json:"active,omitempty" table:"ACTIVE"`
}

// fromSDKExtensionVersion converts an SDK ExtensionVersion to the CLI ExtensionVersion.
func fromSDKExtensionVersion(v *sdkext.ExtensionVersion) *ExtensionVersion {
	return &ExtensionVersion{
		Version:       v.Version,
		ExtensionName: v.ExtensionName,
		Active:        v.Active,
	}
}

// ExtensionVersionList represents a list of extension versions.
type ExtensionVersionList struct {
	Items       []ExtensionVersion `json:"items"`
	TotalCount  int                `json:"totalCount"`
	NextPageKey string             `json:"nextPageKey,omitempty"`
}

// fromSDKExtensionVersionList converts an SDK ExtensionVersionList to the CLI ExtensionVersionList.
func fromSDKExtensionVersionList(l *sdkext.ExtensionVersionList) *ExtensionVersionList {
	items := make([]ExtensionVersion, len(l.Items))
	for i := range l.Items {
		items[i] = *fromSDKExtensionVersion(&l.Items[i])
	}
	return &ExtensionVersionList{
		Items:       items,
		TotalCount:  l.TotalCount,
		NextPageKey: l.NextPageKey,
	}
}

// MonitoringConfiguration is the CLI read model for a monitoring configuration.
type MonitoringConfiguration struct {
	Type          string          `json:"type,omitempty" yaml:"type,omitempty" table:"-"`
	ExtensionName string          `json:"extensionName,omitempty" table:"EXTENSION"`
	ObjectID      string          `json:"objectId" table:"ID"`
	Scope         string          `json:"scope,omitempty" table:"SCOPE"`
	Value         json.RawMessage `json:"value,omitempty" table:"-"`
}

// fromSDKMonitoringConfiguration converts an SDK MonitoringConfiguration to the CLI type.
func fromSDKMonitoringConfiguration(m *sdkext.MonitoringConfiguration) *MonitoringConfiguration {
	return &MonitoringConfiguration{
		Type:          m.Type,
		ExtensionName: m.ExtensionName,
		ObjectID:      m.ObjectID,
		Scope:         m.Scope,
		Value:         m.Value,
	}
}

// MarshalYAML implements yaml.Marshaler to properly serialize json.RawMessage Value
// as a structured object instead of a byte array.
func (m MonitoringConfiguration) MarshalYAML() (interface{}, error) {
	var parsedValue interface{}
	if len(m.Value) > 0 {
		if err := json.Unmarshal(m.Value, &parsedValue); err != nil {
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

// MonitoringConfigurationList represents a list of monitoring configuration instances.
type MonitoringConfigurationList struct {
	Items       []MonitoringConfiguration `json:"items"`
	TotalCount  int                       `json:"totalCount"`
	NextPageKey string                    `json:"nextPageKey,omitempty"`
}

// fromSDKMonitoringConfigurationList converts an SDK MonitoringConfigurationList to the CLI type.
func fromSDKMonitoringConfigurationList(l *sdkext.MonitoringConfigurationList) *MonitoringConfigurationList {
	items := make([]MonitoringConfiguration, len(l.Items))
	for i := range l.Items {
		items[i] = *fromSDKMonitoringConfiguration(&l.Items[i])
	}
	return &MonitoringConfigurationList{
		Items:       items,
		TotalCount:  l.TotalCount,
		NextPageKey: l.NextPageKey,
	}
}

// ActiveGateGroupItem is the CLI read model for an active gate group.
type ActiveGateGroupItem struct {
	GroupName            string                  `json:"groupName" table:"GROUP"`
	AvailableActiveGates int                     `json:"availableActiveGates" table:"AVAILABLE"`
	ActiveGates          []sdkext.ActiveGateEntry `json:"activeGates,omitempty" table:"-"`
}

// fromSDKActiveGateGroupItem converts an SDK ActiveGateGroupItem to the CLI type.
func fromSDKActiveGateGroupItem(g *sdkext.ActiveGateGroupItem) ActiveGateGroupItem {
	return ActiveGateGroupItem{
		GroupName:            g.GroupName,
		AvailableActiveGates: g.AvailableActiveGates,
		ActiveGates:          g.ActiveGates,
	}
}

// ActiveGateGroupList represents the list of active gate groups for an extension version.
type ActiveGateGroupList struct {
	Items []ActiveGateGroupItem `json:"items"`
}

// fromSDKActiveGateGroupList converts an SDK ActiveGateGroupList to the CLI type.
func fromSDKActiveGateGroupList(l *sdkext.ActiveGateGroupList) *ActiveGateGroupList {
	items := make([]ActiveGateGroupItem, len(l.Items))
	for i := range l.Items {
		items[i] = fromSDKActiveGateGroupItem(&l.Items[i])
	}
	return &ActiveGateGroupList{Items: items}
}

// Re-export SDK types that don't have table tags.
type (
	ExtensionDetails              = sdkext.ExtensionDetails
	ExtensionAuthor               = sdkext.ExtensionAuthor
	FeatureSetDetail              = sdkext.FeatureSetDetail
	FeatureSetMetric              = sdkext.FeatureSetMetric
	ExtensionVariable             = sdkext.ExtensionVariable
	MonitoringConfigurationCreate = sdkext.MonitoringConfigurationCreate
	ExtensionEnvironmentConfig    = sdkext.ExtensionEnvironmentConfig
	ExtensionStatus               = sdkext.ExtensionStatus
	ActiveGateEntry               = sdkext.ActiveGateEntry
)

// Handler handles Extensions 2.0 resources.
// It delegates to the SDK handler.
type Handler struct {
	sdk *sdkext.Handler
}

// NewHandler creates a new Extension handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkext.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List lists all extensions with automatic pagination
func (h *Handler) List(name string, chunkSize int64) (*ExtensionList, error) {
	l, err := h.sdk.List(context.Background(), name, chunkSize)
	if err != nil {
		return nil, err
	}
	return fromSDKExtensionList(l), nil
}

// Get gets a specific extension by name (returns all versions)
func (h *Handler) Get(extensionName string) (*ExtensionVersionList, error) {
	l, err := h.sdk.Get(context.Background(), extensionName)
	if err != nil {
		return nil, err
	}
	return fromSDKExtensionVersionList(l), nil
}

// GetVersion gets details for a specific extension version
func (h *Handler) GetVersion(extensionName, version string) (*ExtensionDetails, error) {
	return h.sdk.GetVersion(context.Background(), extensionName, version)
}

// GetEnvironmentConfig gets the environment configuration for a specific extension version.
func (h *Handler) GetEnvironmentConfig(extensionName, version string) (*ExtensionEnvironmentConfig, error) {
	return h.sdk.GetEnvironmentConfig(context.Background(), extensionName, version)
}

// ListMonitoringConfigurations lists monitoring configurations for an extension
func (h *Handler) ListMonitoringConfigurations(extensionName, version string, chunkSize int64) (*MonitoringConfigurationList, error) {
	l, err := h.sdk.ListMonitoringConfigurations(context.Background(), extensionName, version, chunkSize)
	if err != nil {
		return nil, err
	}
	return fromSDKMonitoringConfigurationList(l), nil
}

// GetMonitoringConfiguration gets a specific monitoring configuration
func (h *Handler) GetMonitoringConfiguration(extensionName, configID string) (*MonitoringConfiguration, error) {
	m, err := h.sdk.GetMonitoringConfiguration(context.Background(), extensionName, configID)
	if err != nil {
		return nil, err
	}
	return fromSDKMonitoringConfiguration(m), nil
}

// CreateMonitoringConfiguration creates a new monitoring configuration for an extension
func (h *Handler) CreateMonitoringConfiguration(extensionName string, body MonitoringConfigurationCreate) (*MonitoringConfiguration, error) {
	m, err := h.sdk.CreateMonitoringConfiguration(context.Background(), extensionName, body)
	if err != nil {
		return nil, err
	}
	return fromSDKMonitoringConfiguration(m), nil
}

// UpdateMonitoringConfiguration updates an existing monitoring configuration for an extension
func (h *Handler) UpdateMonitoringConfiguration(extensionName, configID string, body MonitoringConfigurationCreate) (*MonitoringConfiguration, error) {
	m, err := h.sdk.UpdateMonitoringConfiguration(context.Background(), extensionName, configID, body)
	if err != nil {
		return nil, err
	}
	return fromSDKMonitoringConfiguration(m), nil
}

// Upload uploads a custom extension zip file to the Dynatrace environment.
func (h *Handler) Upload(fileName string, zipData []byte) (*ExtensionVersion, error) {
	v, err := h.sdk.Upload(context.Background(), fileName, zipData)
	if err != nil {
		return nil, err
	}
	return fromSDKExtensionVersion(v), nil
}

// InstallFromHub installs a Dynatrace Hub extension into the environment.
func (h *Handler) InstallFromHub(extensionName, version string) (*ExtensionVersion, error) {
	v, err := h.sdk.InstallFromHub(context.Background(), extensionName, version)
	if err != nil {
		return nil, err
	}
	return fromSDKExtensionVersion(v), nil
}

// DeleteMonitoringConfiguration deletes a monitoring configuration for an extension
func (h *Handler) DeleteMonitoringConfiguration(extensionName, configID string) error {
	return h.sdk.DeleteMonitoringConfiguration(context.Background(), extensionName, configID)
}

// GetMonitoringConfigurationSchema retrieves the monitoring configuration schema for a specific
// extension version.
func (h *Handler) GetMonitoringConfigurationSchema(extensionName, version string) (json.RawMessage, error) {
	return h.sdk.GetMonitoringConfigurationSchema(context.Background(), extensionName, version)
}

// GetActiveGateGroups retrieves the active gate groups available for a specific extension version.
func (h *Handler) GetActiveGateGroups(extensionName, version string) (*ActiveGateGroupList, error) {
	l, err := h.sdk.GetActiveGateGroups(context.Background(), extensionName, version)
	if err != nil {
		return nil, err
	}
	return fromSDKActiveGateGroupList(l), nil
}
