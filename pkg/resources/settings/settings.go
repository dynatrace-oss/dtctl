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
	SchemaID     string                 `json:"schemaId" table:"SCHEMA_ID"`
	DisplayName  string                 `json:"displayName" table:"DISPLAY_NAME"`
	Description  string                 `json:"description,omitempty" table:"-"`
	Version      string                 `json:"version" table:"VERSION"`
	MultiObject  bool                   `json:"multiObject,omitempty" table:"MULTI,wide"`
	Ordered      bool                   `json:"ordered,omitempty" table:"ORDERED,wide"`
	Properties   map[string]any `json:"properties,omitempty" table:"-"`
	Scopes       []string               `json:"scopes,omitempty" table:"-"`
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
	ExternalID       string                 `json:"externalId,omitempty" table:"-"`
	Summary          string                 `json:"summary,omitempty" table:"SUMMARY"`
	Value            map[string]any `json:"value,omitempty" table:"-"`
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
}

// SettingsObjectCreate represents the request body for creating a settings object
type SettingsObjectCreate struct {
	SchemaID      string                 `json:"schemaId"`
	Scope         string                 `json:"scope"`
	Value         map[string]any `json:"value"`
	SchemaVersion string                 `json:"schemaVersion,omitempty"`
	ExternalID    string                 `json:"externalId,omitempty"`
}

// SettingsObjectResponse represents the response from creating/updating a settings object
type SettingsObjectResponse struct {
	ObjectID string `json:"objectId"`
	Code     int    `json:"code,omitempty"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// CreateResponse represents the response from batch create
type CreateResponse struct {
	Items []SettingsObjectResponse `json:"items"`
}

// ListSchemas lists all available settings schemas
func (h *Handler) ListSchemas() (*SchemaList, error) {
	resp, err := h.client.HTTP().R().
		Get("/platform/classic/environment-api/v2/settings/schemas")

	if err != nil {
		return nil, fmt.Errorf("failed to list schemas: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list schemas: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result SchemaList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse schemas response: %w", err)
	}

	return &result, nil
}

// GetSchema gets a specific schema definition
func (h *Handler) GetSchema(schemaID string) (map[string]any, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/classic/environment-api/v2/settings/schemas/%s", schemaID))

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

	var result map[string]any
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
		req := h.client.HTTP().R()

		if schemaID != "" {
			req.SetQueryParam("schemaIds", schemaID)
		}
		if scope != "" {
			req.SetQueryParam("scopes", scope)
		}

		// Set page size if chunking is enabled (chunkSize > 0)
		if chunkSize > 0 {
			req.SetQueryParam("pageSize", fmt.Sprintf("%d", chunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("nextPageKey", nextPageKey)
		}

		resp, err := req.Get("/platform/classic/environment-api/v2/settings/objects")
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
		Get(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", objectID))

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
	// Wrap in array for v2 API
	body := []SettingsObjectCreate{req}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetQueryParam("validateOnly", "true").
		Post("/platform/classic/environment-api/v2/settings/objects")

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
	// Wrap in array for v2 API
	body := []SettingsObjectCreate{req}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		Post("/platform/classic/environment-api/v2/settings/objects")

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

	var createResp CreateResponse
	if err := json.Unmarshal(resp.Body(), &createResp); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	if len(createResp.Items) == 0 {
		return nil, fmt.Errorf("no items returned in create response")
	}

	result := &createResp.Items[0]
	if result.Error != nil {
		return nil, fmt.Errorf("create failed: %s", result.Error.Message)
	}

	return result, nil
}

// ValidateUpdate validates a settings object update without applying it
func (h *Handler) ValidateUpdate(objectID string, value map[string]any) error {
	// First get current object to obtain version
	obj, err := h.Get(objectID)
	if err != nil {
		return err
	}

	body := map[string]any{
		"value": value,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", obj.SchemaVersion).
		SetQueryParam("validateOnly", "true").
		Put(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", objectID))

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
		case 412:
			return fmt.Errorf("settings object version conflict (object was modified)")
		default:
			return fmt.Errorf("validation failed: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Update updates an existing settings object
func (h *Handler) Update(objectID string, value map[string]any) (*SettingsObject, error) {
	// First get current object to obtain version
	obj, err := h.Get(objectID)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"value": value,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", obj.SchemaVersion).
		Put(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", objectID))

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
		case 412:
			return nil, fmt.Errorf("settings object version conflict (object was modified)")
		default:
			return nil, fmt.Errorf("failed to update settings object: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	// Return updated object
	return h.Get(objectID)
}

// Delete deletes a settings object
func (h *Handler) Delete(objectID string) error {
	// First get current object to obtain version
	obj, err := h.Get(objectID)
	if err != nil {
		return err
	}

	resp, err := h.client.HTTP().R().
		SetHeader("If-Match", obj.SchemaVersion).
		Delete(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", objectID))

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
		case 412:
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
