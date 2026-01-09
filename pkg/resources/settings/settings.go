package settings

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles settings resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new settings handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// Schema represents a settings schema
type Schema struct {
	SchemaID            string   `json:"schemaId" table:"SCHEMA_ID"`
	DisplayName         string   `json:"displayName" table:"DISPLAY_NAME"`
	LatestSchemaVersion string   `json:"latestSchemaVersion" table:"VERSION"`
	MultiObject         bool     `json:"multiObject,omitempty" table:"MULTI,wide"`
	Ordered             bool     `json:"ordered,omitempty" table:"ORDERED,wide"`
	SchemaVersions      []string `json:"schemaVersions,omitempty" table:"-"`
}

// SchemaList represents a list of schemas
type SchemaList struct {
	Items      []Schema `json:"items"`
	TotalCount int      `json:"totalCount"`
}

// SettingsObject represents a settings object
type SettingsObject struct {
	ObjectID         string                 `json:"objectId" table:"OBJECT_ID"`
	SchemaID         string                 `json:"schemaId" table:"SCHEMA_ID"`
	SchemaVersion    string                 `json:"schemaVersion,omitempty" table:"VERSION,wide"`
	Scope            string                 `json:"scope" table:"SCOPE"`
	Version          string                 `json:"version,omitempty" table:"-"`
	Summary          string                 `json:"summary,omitempty" table:"SUMMARY"`
	Value            map[string]interface{} `json:"value,omitempty" table:"-"`
	ModificationInfo *ModificationInfo      `json:"modificationInfo,omitempty" table:"-"`
}

// ModificationInfo contains modification timestamps
type ModificationInfo struct {
	CreatedBy        string `json:"createdBy,omitempty"`
	CreatedTime      string `json:"createdTime,omitempty"`
	LastModifiedBy   string `json:"lastModifiedBy,omitempty"`
	LastModifiedTime string `json:"lastModifiedTime,omitempty"`
}

// SettingsObjectsList represents a list of settings objects
type SettingsObjectsList struct {
	Items       []SettingsObject `json:"items"`
	TotalCount  int              `json:"totalCount"`
	NextPageKey string           `json:"nextPageKey,omitempty"`
	Version     string           `json:"version,omitempty"`
}

// SettingsObjectCreate represents the request body for creating a settings object
type SettingsObjectCreate struct {
	SchemaID      string                 `json:"schemaId"`
	Scope         string                 `json:"scope"`
	Value         map[string]interface{} `json:"value"`
	SchemaVersion string                 `json:"schemaVersion,omitempty"`
	ExternalID    string                 `json:"externalId,omitempty"`
}

// SettingsObjectResponse represents the response from creating/updating a settings object
type SettingsObjectResponse struct {
	ObjectID string `json:"objectId"`
	Version  string `json:"version,omitempty"`
}

// ListSchemas lists all available settings schemas
func (h *Handler) ListSchemas() (*SchemaList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "schemaId,displayName,latestSchemaVersion,multiObject,ordered").
		Get("/platform/settings/v0.1/schemas")

	if err != nil {
		return nil, fmt.Errorf("failed to list schemas: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list schemas: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Manually parse JSON since API returns application/octet-stream content-type
	var result SchemaList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse schemas response: %w", err)
	}

	return &result, nil
}

// GetSchema gets a specific schema definition
func (h *Handler) GetSchema(schemaID string) (map[string]interface{}, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/settings/v0.1/schemas/%s", schemaID))

	if err != nil {
		return nil, fmt.Errorf("failed to get schema: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("schema %q not found", schemaID)
		default:
			return nil, fmt.Errorf("failed to get schema: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse schema response: %w", err)
	}

	return result, nil
}

// ListObjects lists settings objects for a schema with automatic pagination
func (h *Handler) ListObjects(schemaID, scope string, chunkSize int64) (*SettingsObjectsList, error) {
	var allItems []SettingsObject
	var totalCount int
	nextPageKey := ""

	for {
		req := h.client.HTTP().R().
			SetQueryParam("add-fields", "objectId,version,summary,scope,schemaId,schemaVersion,value,modificationInfo")

		if schemaID != "" {
			req.SetQueryParam("schema-id", schemaID)
		}
		if scope != "" {
			req.SetQueryParam("scope", scope)
		}

		// Set page size if chunking is enabled (chunkSize > 0)
		if chunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		resp, err := req.Get("/platform/settings/v0.1/objects")
		if err != nil {
			return nil, fmt.Errorf("failed to list settings objects: %w", err)
		}

		if resp.IsError() {
			switch resp.StatusCode() {
			case 404:
				return nil, fmt.Errorf("schema %q not found", schemaID)
			default:
				return nil, fmt.Errorf("failed to list settings objects: status %d: %s", resp.StatusCode(), resp.String())
			}
		}

		var result SettingsObjectsList
		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return nil, fmt.Errorf("failed to parse settings objects response: %w", err)
		}

		allItems = append(allItems, result.Items...)
		totalCount = result.TotalCount

		// If chunking is disabled (chunkSize == 0), return first page only
		if chunkSize == 0 {
			return &result, nil
		}

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &SettingsObjectsList{
		Items:      allItems,
		TotalCount: totalCount,
	}, nil
}

// Get gets a specific settings object by ID
func (h *Handler) Get(objectID string) (*SettingsObject, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/settings/v0.1/objects/%s", objectID))

	if err != nil {
		return nil, fmt.Errorf("failed to get settings object: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("settings object %q not found", objectID)
		case 403:
			return nil, fmt.Errorf("access denied to settings object %q", objectID)
		default:
			return nil, fmt.Errorf("failed to get settings object: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result SettingsObject
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse settings object response: %w", err)
	}

	return &result, nil
}

// ValidateCreate validates a settings object without creating it
func (h *Handler) ValidateCreate(req SettingsObjectCreate) error {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetQueryParam("validate-only", "true").
		Post("/platform/settings/v0.1/objects")

	if err != nil {
		return fmt.Errorf("failed to validate settings object: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("validation failed: %s", resp.String())
		case 403:
			return fmt.Errorf("access denied")
		case 404:
			return fmt.Errorf("schema %q not found", req.SchemaID)
		default:
			return fmt.Errorf("validation failed: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Create creates a new settings object
func (h *Handler) Create(req SettingsObjectCreate) (*SettingsObjectResponse, error) {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/settings/v0.1/objects")

	if err != nil {
		return nil, fmt.Errorf("failed to create settings object: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid settings object: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create settings object")
		case 404:
			return nil, fmt.Errorf("schema %q not found", req.SchemaID)
		case 409:
			return nil, fmt.Errorf("settings object already exists or conflicts with existing object")
		default:
			return nil, fmt.Errorf("failed to create settings object: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result SettingsObjectResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	return &result, nil
}

// ValidateUpdate validates a settings object update without applying it
func (h *Handler) ValidateUpdate(objectID string, version string, value map[string]interface{}) error {
	body := map[string]interface{}{
		"value": value,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetQueryParam("optimistic-locking-version", version).
		SetQueryParam("validate-only", "true").
		Put(fmt.Sprintf("/platform/settings/v0.1/objects/%s", objectID))

	if err != nil {
		return fmt.Errorf("failed to validate settings object: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("validation failed: %s", resp.String())
		case 403:
			return fmt.Errorf("access denied to update settings object %q", objectID)
		case 404:
			return fmt.Errorf("settings object %q not found", objectID)
		case 409:
			return fmt.Errorf("settings object version conflict (object was modified)")
		default:
			return fmt.Errorf("validation failed: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Update updates an existing settings object
func (h *Handler) Update(objectID string, version string, value map[string]interface{}) (*SettingsObjectResponse, error) {
	body := map[string]interface{}{
		"value": value,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetQueryParam("optimistic-locking-version", version).
		Put(fmt.Sprintf("/platform/settings/v0.1/objects/%s", objectID))

	if err != nil {
		return nil, fmt.Errorf("failed to update settings object: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid settings object: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to update settings object %q", objectID)
		case 404:
			return nil, fmt.Errorf("settings object %q not found", objectID)
		case 409:
			return nil, fmt.Errorf("settings object version conflict (object was modified)")
		default:
			return nil, fmt.Errorf("failed to update settings object: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result SettingsObjectResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse update response: %w", err)
	}

	return &result, nil
}

// Delete deletes a settings object
func (h *Handler) Delete(objectID string, version string) error {
	resp, err := h.client.HTTP().R().
		SetQueryParam("optimistic-locking-version", version).
		Delete(fmt.Sprintf("/platform/settings/v0.1/objects/%s", objectID))

	if err != nil {
		return fmt.Errorf("failed to delete settings object: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 403:
			return fmt.Errorf("access denied to delete settings object %q", objectID)
		case 404:
			return fmt.Errorf("settings object %q not found", objectID)
		case 409:
			return fmt.Errorf("settings object version conflict (object was modified)")
		default:
			return fmt.Errorf("failed to delete settings object: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetRaw gets a settings object as raw JSON bytes (for editing)
func (h *Handler) GetRaw(objectID string) ([]byte, error) {
	obj, err := h.Get(objectID)
	if err != nil {
		return nil, err
	}

	// Return the value as JSON
	return json.MarshalIndent(obj.Value, "", "  ")
}
