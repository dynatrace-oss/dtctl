package extension

import (
	"encoding/json"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkext "github.com/dynatrace-oss/dtctl/sdk/api/extension"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	Extension                    = sdkext.Extension
	ExtensionList                = sdkext.ExtensionList
	ExtensionVersion             = sdkext.ExtensionVersion
	ExtensionVersionList         = sdkext.ExtensionVersionList
	ExtensionDetails             = sdkext.ExtensionDetails
	ExtensionAuthor              = sdkext.ExtensionAuthor
	FeatureSetDetail             = sdkext.FeatureSetDetail
	FeatureSetMetric             = sdkext.FeatureSetMetric
	ExtensionVariable            = sdkext.ExtensionVariable
	MonitoringConfiguration      = sdkext.MonitoringConfiguration
	MonitoringConfigurationList  = sdkext.MonitoringConfigurationList
	MonitoringConfigurationCreate = sdkext.MonitoringConfigurationCreate
	ExtensionEnvironmentConfig   = sdkext.ExtensionEnvironmentConfig
	ExtensionStatus              = sdkext.ExtensionStatus
	ActiveGateEntry              = sdkext.ActiveGateEntry
	ActiveGateGroupItem          = sdkext.ActiveGateGroupItem
	ActiveGateGroupList          = sdkext.ActiveGateGroupList
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
	return h.sdk.List(name, chunkSize)
}

// Get gets a specific extension by name (returns all versions)
func (h *Handler) Get(extensionName string) (*ExtensionVersionList, error) {
	return h.sdk.Get(extensionName)
}

// GetVersion gets details for a specific extension version
func (h *Handler) GetVersion(extensionName, version string) (*ExtensionDetails, error) {
	return h.sdk.GetVersion(extensionName, version)
}

// GetEnvironmentConfig gets the environment configuration for a specific extension version.
func (h *Handler) GetEnvironmentConfig(extensionName, version string) (*ExtensionEnvironmentConfig, error) {
	return h.sdk.GetEnvironmentConfig(extensionName, version)
}

// ListMonitoringConfigurations lists monitoring configurations for an extension
func (h *Handler) ListMonitoringConfigurations(extensionName, version string, chunkSize int64) (*MonitoringConfigurationList, error) {
	return h.sdk.ListMonitoringConfigurations(extensionName, version, chunkSize)
}

// GetMonitoringConfiguration gets a specific monitoring configuration
func (h *Handler) GetMonitoringConfiguration(extensionName, configID string) (*MonitoringConfiguration, error) {
	return h.sdk.GetMonitoringConfiguration(extensionName, configID)
}

// CreateMonitoringConfiguration creates a new monitoring configuration for an extension
func (h *Handler) CreateMonitoringConfiguration(extensionName string, body MonitoringConfigurationCreate) (*MonitoringConfiguration, error) {
	return h.sdk.CreateMonitoringConfiguration(extensionName, body)
}

// UpdateMonitoringConfiguration updates an existing monitoring configuration for an extension
func (h *Handler) UpdateMonitoringConfiguration(extensionName, configID string, body MonitoringConfigurationCreate) (*MonitoringConfiguration, error) {
	return h.sdk.UpdateMonitoringConfiguration(extensionName, configID, body)
}

// Upload uploads a custom extension zip file to the Dynatrace environment.
func (h *Handler) Upload(fileName string, zipData []byte) (*ExtensionVersion, error) {
	return h.sdk.Upload(fileName, zipData)
}

// InstallFromHub installs a Dynatrace Hub extension into the environment.
func (h *Handler) InstallFromHub(extensionName, version string) (*ExtensionVersion, error) {
	return h.sdk.InstallFromHub(extensionName, version)
}

// DeleteMonitoringConfiguration deletes a monitoring configuration for an extension
func (h *Handler) DeleteMonitoringConfiguration(extensionName, configID string) error {
	return h.sdk.DeleteMonitoringConfiguration(extensionName, configID)
}

// GetMonitoringConfigurationSchema retrieves the monitoring configuration schema for a specific
// extension version.
func (h *Handler) GetMonitoringConfigurationSchema(extensionName, version string) (json.RawMessage, error) {
	return h.sdk.GetMonitoringConfigurationSchema(extensionName, version)
}

// GetActiveGateGroups retrieves the active gate groups available for a specific extension version.
func (h *Handler) GetActiveGateGroups(extensionName, version string) (*ActiveGateGroupList, error) {
	return h.sdk.GetActiveGateGroups(extensionName, version)
}
