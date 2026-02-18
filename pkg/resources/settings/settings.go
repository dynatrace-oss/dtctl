package settings

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// uuidPattern matches UUID format (with or without hyphens)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{12}$`)

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

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
	SchemaID    string         `json:"schemaId" table:"SCHEMA_ID"`
	DisplayName string         `json:"displayName" table:"DISPLAY_NAME"`
	Description string         `json:"description,omitempty" table:"-"`
	Version     string         `json:"version" table:"VERSION"`
	MultiObject bool           `json:"multiObject,omitempty" table:"MULTI,wide"`
	Ordered     bool           `json:"ordered,omitempty" table:"ORDERED,wide"`
	Properties  map[string]any `json:"properties,omitempty" table:"-"`
	Scopes      []string       `json:"scopes,omitempty" table:"-"`
}

// SchemaList represents a list of schemas
type SchemaList struct {
	Items      []Schema `json:"items"`
	TotalCount int      `json:"totalCount"`
}

// SettingsObject represents a settings object
type SettingsObject struct {
	ObjectID         string            `json:"objectId" table:"OBJECT_ID,wide"`
	SchemaID         string            `json:"schemaId" table:"SCHEMA_ID"`
	SchemaVersion    string            `json:"schemaVersion,omitempty" table:"VERSION,wide"`
	Scope            string            `json:"scope" table:"SCOPE,wide"`
	ExternalID       string            `json:"externalId,omitempty" table:"-"`
	Summary          string            `json:"summary,omitempty" table:"SUMMARY"`
	Value            map[string]any    `json:"value,omitempty" table:"-"`
	ModificationInfo *ModificationInfo `json:"modificationInfo,omitempty" table:"-"`

	// Decoded fields (computed from ObjectID, not from API)
	ObjectIDShort string `json:"-" yaml:"-" table:"OBJECT_ID_SHORT"`
	UID           string `json:"-" yaml:"-" table:"UID,wide"`
	ScopeType     string `json:"-" yaml:"-" table:"SCOPE_TYPE,wide"`
	ScopeID       string `json:"-" yaml:"-" table:"SCOPE_ID,wide"`
}

// decodeObjectID decodes the ObjectID and populates UID, ScopeType, ScopeID, and ObjectIDShort fields.
// This is called automatically after unmarshaling from the API.
// Errors are silently ignored to maintain backward compatibility.
func (s *SettingsObject) decodeObjectID() {
	if s.ObjectID == "" {
		return
	}

	// Create truncated version for table display (first 20 chars + "...")
	if len(s.ObjectID) > 23 {
		s.ObjectIDShort = s.ObjectID[:20] + "..."
	} else {
		s.ObjectIDShort = s.ObjectID
	}

	decoded, err := DecodeObjectID(s.ObjectID)
	if err != nil {
		// Silently ignore decode errors - the ObjectID is still usable as-is
		return
	}

	s.UID = decoded.UID
	s.ScopeType = decoded.ScopeType
	s.ScopeID = decoded.ScopeID
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
	SchemaID      string         `json:"schemaId"`
	Scope         string         `json:"scope"`
	Value         map[string]any `json:"value"`
	SchemaVersion string         `json:"schemaVersion,omitempty"`
	ExternalID    string         `json:"externalId,omitempty"`
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

		// When using nextPageKey, ONLY send the nextPageKey parameter
		// The API doesn't allow other parameters with nextPageKey
		if nextPageKey != "" {
			req.SetQueryParam("nextPageKey", nextPageKey)
		} else {
			// First request: set filters and page size
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

		// API bug workaround: The API returns empty schemaId field, so populate it from the query parameter
		if schemaID != "" {
			for i := range result.Items {
				if result.Items[i].SchemaID == "" {
					result.Items[i].SchemaID = schemaID
				}
			}
		}

		// Decode all objectIDs to populate UID and DecodedScope fields
		for i := range result.Items {
			result.Items[i].decodeObjectID()
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

// Get gets a specific settings object by ID or UID.
// If the provided string looks like a UUID, it will attempt to resolve it to an objectID
// by listing all settings objects and finding the one with the matching UID.
// This requires listing all settings objects which may be slow for large schemas.
func (h *Handler) Get(idOrUID string) (*SettingsObject, error) {
	return h.GetWithContext(idOrUID, "", "")
}

// GetWithContext gets a specific settings object by ID or UID with optional schema/scope context.
// If the provided string looks like a UUID, it will attempt to resolve it to an objectID
// by listing settings objects filtered by the provided schema and/or scope.
// Providing schemaID and/or scope can significantly speed up UID resolution and is required by the API.
func (h *Handler) GetWithContext(idOrUID, schemaID, scope string) (*SettingsObject, error) {
	// If it looks like a UID (UUID format), try to resolve it to an objectID
	if isUUID(idOrUID) {
		return h.getByUID(idOrUID, schemaID, scope)
	}

	// Otherwise, treat it as an objectID
	return h.getByObjectID(idOrUID)
}

// getByObjectID gets a settings object by its full objectID (base64-encoded composite key)
func (h *Handler) getByObjectID(objectID string) (*SettingsObject, error) {
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

	// Decode the objectID to populate UID and DecodedScope
	result.decodeObjectID()

	return &result, nil
}

// getByUID resolves a UID to an objectID by listing settings objects and finding a match.
// This is slower than getByObjectID as it requires listing settings.
// The schemaID parameter is required to narrow the search and prevent expensive operations.
func (h *Handler) getByUID(uid, schemaID, scope string) (*SettingsObject, error) {
	// Require schemaID to prevent expensive searches across all settings
	if schemaID == "" {
		return nil, fmt.Errorf("schema ID is required when looking up settings by UID. Use --schema flag to specify the schema (e.g., --schema builtin:openpipeline.logs.pipelines)")
	}

	// If no scope provided, search without scope filter (will search all scopes)
	// This is more expensive but necessary since we don't know which scope the UID is in
	// The schemaID filter still keeps this reasonably efficient
	searchScope := scope

	// Paginate through settings objects, stopping when we find the matching UID
	// This is more efficient than loading all objects at once
	nextPageKey := ""
	pageSize := int64(500)
	totalSearched := 0

	for {
		req := h.client.HTTP().R()

		// When using nextPageKey, ONLY send the nextPageKey parameter
		if nextPageKey != "" {
			req.SetQueryParam("nextPageKey", nextPageKey)
		} else {
			// First request: set filters and page size
			if schemaID != "" {
				req.SetQueryParam("schemaIds", schemaID)
			}
			// Only add scope filter if explicitly provided
			if searchScope != "" {
				req.SetQueryParam("scopes", searchScope)
			}
			req.SetQueryParam("pageSize", fmt.Sprintf("%d", pageSize))
		}

		resp, err := req.Get("/platform/classic/environment-api/v2/settings/objects")
		if err != nil {
			return nil, fmt.Errorf("failed to list settings objects for UID resolution: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list settings objects for UID resolution: status %d: %s", resp.StatusCode(), resp.String())
		}

		var result SettingsObjectsList
		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return nil, fmt.Errorf("failed to parse settings objects response: %w", err)
		}

		// Decode all objectIDs to populate UID fields
		for i := range result.Items {
			result.Items[i].decodeObjectID()
		}

		// Search for matching UID in this page
		for i := range result.Items {
			if result.Items[i].UID == uid {
				// Found it! Return immediately without fetching more pages
				return &result.Items[i], nil
			}
		}

		totalSearched += len(result.Items)

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	// Provide helpful error message
	if searchScope != "" {
		return nil, fmt.Errorf("settings object with UID %q not found in schema %q with scope %q (searched %d objects). Try omitting --scope to search all scopes", uid, schemaID, searchScope, totalSearched)
	}
	return nil, fmt.Errorf("settings object with UID %q not found in schema %q (searched %d objects across all scopes)", uid, schemaID, totalSearched)
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

	var createResp []SettingsObjectResponse
	if err := json.Unmarshal(resp.Body(), &createResp); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	if len(createResp) == 0 {
		return nil, fmt.Errorf("no items returned in create response")
	}

	result := &createResp[0]
	if result.Error != nil {
		return nil, fmt.Errorf("create failed: %s", result.Error.Message)
	}

	return result, nil
}

// ValidateUpdate validates a settings object update without applying it
func (h *Handler) ValidateUpdate(objectID string, value map[string]any) error {
	return h.ValidateUpdateWithContext(objectID, value, "", "")
}

// ValidateUpdateWithContext validates a settings object update without applying it with optional context
func (h *Handler) ValidateUpdateWithContext(objectID string, value map[string]any, schemaID, scope string) error {
	// First get current object to obtain version (and resolve UID to objectID if needed)
	obj, err := h.GetWithContext(objectID, schemaID, scope)
	if err != nil {
		return err
	}

	body := map[string]any{
		"value": value,
	}

	// Use the resolved ObjectID (not the input which might be a UID)
	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", obj.SchemaVersion).
		SetQueryParam("validateOnly", "true").
		Put(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", obj.ObjectID))

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
	return h.UpdateWithContext(objectID, value, "", "")
}

// UpdateWithContext updates an existing settings object with optional context for UID resolution
func (h *Handler) UpdateWithContext(objectID string, value map[string]any, schemaID, scope string) (*SettingsObject, error) {
	// First get current object to obtain version (and resolve UID to objectID if needed)
	obj, err := h.GetWithContext(objectID, schemaID, scope)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"value": value,
	}

	// Use the resolved ObjectID (not the input which might be a UID)
	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", obj.SchemaVersion).
		Put(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", obj.ObjectID))

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

	// Return updated object (use resolved ObjectID)
	return h.Get(obj.ObjectID)
}

// Delete deletes a settings object
func (h *Handler) Delete(objectID string) error {
	return h.DeleteWithContext(objectID, "", "")
}

// DeleteWithContext deletes a settings object with optional context for UID resolution
func (h *Handler) DeleteWithContext(objectID, schemaID, scope string) error {
	// First get current object to obtain version (and resolve UID to objectID if needed)
	obj, err := h.GetWithContext(objectID, schemaID, scope)
	if err != nil {
		return err
	}

	// Use the resolved ObjectID (not the input which might be a UID)
	resp, err := h.client.HTTP().R().
		SetHeader("If-Match", obj.SchemaVersion).
		Delete(fmt.Sprintf("/platform/classic/environment-api/v2/settings/objects/%s", obj.ObjectID))

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
