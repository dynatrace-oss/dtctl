package openpipeline

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles OpenPipeline resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new OpenPipeline handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// ConfigurationListItem represents a configuration in the list
type ConfigurationListItem struct {
	ID         string                 `json:"id" table:"ID"`
	Editable   bool                   `json:"editable" table:"EDITABLE"`
	Definition map[string]interface{} `json:"definition,omitempty" table:"-"`
}

// Configuration represents a full OpenPipeline configuration
type Configuration struct {
	ID             string                   `json:"id" table:"ID"`
	Editable       bool                     `json:"editable" table:"EDITABLE"`
	Version        string                   `json:"version,omitempty" table:"VERSION,wide"`
	UpdateToken    string                   `json:"updateToken,omitempty" table:"-"`
	CustomBasePath string                   `json:"customBasePath,omitempty" table:"BASE_PATH,wide"`
	Endpoints      []map[string]interface{} `json:"endpoints,omitempty" table:"-"`
	Pipelines      []Pipeline               `json:"pipelines,omitempty" table:"-"`
	Routing        map[string]interface{}   `json:"routing,omitempty" table:"-"`
}

// Pipeline represents a pipeline within a configuration
type Pipeline struct {
	ID               string                 `json:"id" table:"ID"`
	Type             string                 `json:"type" table:"TYPE"`
	DisplayName      string                 `json:"displayName" table:"NAME"`
	Enabled          bool                   `json:"enabled" table:"ENABLED"`
	Editable         bool                   `json:"editable" table:"EDITABLE"`
	Builtin          bool                   `json:"builtin" table:"BUILTIN"`
	Storage          map[string]interface{} `json:"storage,omitempty" table:"-"`
	SecurityContext  map[string]interface{} `json:"securityContext,omitempty" table:"-"`
	MetricExtraction map[string]interface{} `json:"metricExtraction,omitempty" table:"-"`
	DataExtraction   map[string]interface{} `json:"dataExtraction,omitempty" table:"-"`
	Processing       map[string]interface{} `json:"processing,omitempty" table:"-"`
}

// List lists all OpenPipeline configurations
func (h *Handler) List() ([]ConfigurationListItem, error) {
	resp, err := h.client.HTTP().R().
		Get("/platform/openpipeline/v1/configurations")

	if err != nil {
		return nil, fmt.Errorf("failed to list OpenPipeline configurations: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list OpenPipeline configurations: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result []ConfigurationListItem
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse OpenPipeline configurations response: %w", err)
	}

	return result, nil
}

// Get gets a specific OpenPipeline configuration by ID
func (h *Handler) Get(configID string) (*Configuration, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/openpipeline/v1/configurations/%s", configID))

	if err != nil {
		return nil, fmt.Errorf("failed to get OpenPipeline configuration: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("OpenPipeline configuration %q not found", configID)
		default:
			return nil, fmt.Errorf("failed to get OpenPipeline configuration: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result Configuration
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse OpenPipeline configuration response: %w", err)
	}

	return &result, nil
}

// Update updates an OpenPipeline configuration
func (h *Handler) Update(configID string, config *Configuration) error {
	resp, err := h.client.HTTP().R().
		SetBody(config).
		Put(fmt.Sprintf("/platform/openpipeline/v1/configurations/%s", configID))

	if err != nil {
		return fmt.Errorf("failed to update OpenPipeline configuration: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("invalid OpenPipeline configuration: %s", resp.String())
		case 403:
			return fmt.Errorf("access denied to update OpenPipeline configuration %q", configID)
		case 404:
			return fmt.Errorf("OpenPipeline configuration %q not found", configID)
		case 409:
			return fmt.Errorf("OpenPipeline configuration version conflict")
		default:
			return fmt.Errorf("failed to update OpenPipeline configuration: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetRaw gets an OpenPipeline configuration as raw JSON bytes (for editing)
func (h *Handler) GetRaw(configID string) ([]byte, error) {
	config, err := h.Get(configID)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(config, "", "  ")
}
