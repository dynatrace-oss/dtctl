package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles document resources (dashboards, notebooks, etc.)
type Handler struct {
	client *client.Client
}

// NewHandler creates a new document handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// Document represents a document resource
type Document struct {
	ID          string    `json:"id" table:"ID"`
	Name        string    `json:"name" table:"NAME"`
	Type        string    `json:"type" table:"TYPE"`
	Owner       string    `json:"owner" table:"OWNER"`
	IsPrivate   bool      `json:"isPrivate" table:"PRIVATE"`
	Created     time.Time `json:"-" table:"CREATED"`
	Description string    `json:"description,omitempty" table:"DESCRIPTION,wide"`
	Version     int       `json:"version" table:"VERSION,wide"`
	Modified    time.Time `json:"-" table:"MODIFIED,wide"`
	Content     []byte    `json:"-" table:"-"`
}

// DocumentMetadata represents detailed document metadata
type DocumentMetadata struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Type             string           `json:"type"`
	Description      string           `json:"description,omitempty"`
	Version          int              `json:"version"`
	Owner            string           `json:"owner"`
	IsPrivate        bool             `json:"isPrivate"`
	ModificationInfo ModificationInfo `json:"modificationInfo"`
	Access           []string         `json:"access,omitempty"`
}

// ModificationInfo contains creation and modification timestamps
type ModificationInfo struct {
	CreatedBy        string    `json:"createdBy"`
	CreatedTime      time.Time `json:"createdTime"`
	LastModifiedBy   string    `json:"lastModifiedBy"`
	LastModifiedTime time.Time `json:"lastModifiedTime"`
}

// DocumentList represents a list of documents
type DocumentList struct {
	Documents   []DocumentMetadata `json:"documents"`
	TotalCount  int                `json:"totalCount"`
	NextPageKey string             `json:"nextPageKey,omitempty"`
}

// DocumentFilters contains filter options for listing documents
type DocumentFilters struct {
	Type      string // e.g., "dashboard", "notebook"
	Name      string // Filter by name
	Owner     string // Filter by owner ID
	Filter    string // Raw filter string for complex queries
	ChunkSize int64  // Page size for pagination (0 = no chunking, use API default)
}

// List retrieves documents matching the provided filters with automatic pagination
func (h *Handler) List(filters DocumentFilters) (*DocumentList, error) {
	var allDocuments []DocumentMetadata
	var totalCount int
	nextPageKey := ""

	// Build filter query parameter
	var filterStr string
	if filters.Filter != "" {
		filterStr = filters.Filter
	} else {
		var conditions []string
		if filters.Type != "" {
			conditions = append(conditions, fmt.Sprintf("type=='%s'", filters.Type))
		}
		if filters.Name != "" {
			conditions = append(conditions, fmt.Sprintf("name contains '%s'", filters.Name))
		}
		if filters.Owner != "" {
			conditions = append(conditions, fmt.Sprintf("owner=='%s'", filters.Owner))
		}
		if len(conditions) > 0 {
			filterStr = strings.Join(conditions, " and ")
		}
	}

	for {
		var result DocumentList
		req := h.client.HTTP().R().SetResult(&result)

		if filterStr != "" {
			req.SetQueryParam("filter", filterStr)
		}

		// Set page size if chunking is enabled (ChunkSize > 0)
		if filters.ChunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", filters.ChunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		resp, err := req.Get("/platform/document/v1/documents")
		if err != nil {
			return nil, fmt.Errorf("failed to list documents: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list documents: status %d: %s", resp.StatusCode(), resp.String())
		}

		allDocuments = append(allDocuments, result.Documents...)
		totalCount = result.TotalCount

		// If chunking is disabled (ChunkSize == 0), return first page only
		if filters.ChunkSize == 0 {
			return &result, nil
		}

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &DocumentList{
		Documents:  allDocuments,
		TotalCount: totalCount,
	}, nil
}

// Get retrieves a specific document by ID
func (h *Handler) Get(id string) (*Document, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/document/v1/documents/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("document %q not found", id)
		case 403:
			return nil, fmt.Errorf("access denied to document %q", id)
		default:
			return nil, fmt.Errorf("failed to get document: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	// Parse multipart response
	doc, err := ParseMultipartDocument(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse document response: %w", err)
	}

	return doc, nil
}

// GetMetadata retrieves only the metadata for a document
func (h *Handler) GetMetadata(id string) (*DocumentMetadata, error) {
	var result DocumentMetadata

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/document/v1/documents/%s/metadata", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get document metadata: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("document %q not found", id)
		case 403:
			return nil, fmt.Errorf("access denied to document %q", id)
		default:
			return nil, fmt.Errorf("failed to get document metadata: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// GetRaw retrieves a document's content as raw bytes
func (h *Handler) GetRaw(id string) ([]byte, error) {
	doc, err := h.Get(id)
	if err != nil {
		return nil, err
	}
	return doc.Content, nil
}

// Delete deletes a document
func (h *Handler) Delete(id string, version int) error {
	resp, err := h.client.HTTP().R().
		SetQueryParam("optimistic-locking-version", fmt.Sprintf("%d", version)).
		Delete(fmt.Sprintf("/platform/document/v1/documents/%s", id))

	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("document %q not found", id)
		case 403:
			return fmt.Errorf("access denied to document %q", id)
		case 409:
			return fmt.Errorf("document version conflict (document was modified)")
		default:
			return fmt.Errorf("failed to delete document: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// CreateRequest contains the data needed to create a new document
type CreateRequest struct {
	ID          string // Optional - if not provided, system generates one
	Name        string // Required
	Type        string // Required - e.g., "dashboard", "notebook"
	Description string // Optional
	Content     []byte // Required - the document content
}

// Create creates a new document
func (h *Handler) Create(req CreateRequest) (*Document, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("document name is required")
	}
	if req.Type == "" {
		return nil, fmt.Errorf("document type is required")
	}
	if len(req.Content) == 0 {
		return nil, fmt.Errorf("document content is required")
	}

	// Build multipart form request
	// The API expects multipart/form-data with content as a file part
	r := h.client.HTTP().R().
		SetMultipartFormData(map[string]string{
			"name": req.Name,
			"type": req.Type,
		}).
		SetMultipartField("content", "content.json", "application/json", bytes.NewReader(req.Content))

	if req.ID != "" {
		r.SetMultipartFormData(map[string]string{"id": req.ID})
	}
	if req.Description != "" {
		r.SetMultipartFormData(map[string]string{"description": req.Description})
	}

	resp, err := r.Post("/platform/document/v1/documents")

	if err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid document data: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create document")
		case 409:
			return nil, fmt.Errorf("document with ID %q already exists", req.ID)
		case 413:
			return nil, fmt.Errorf("document content too large (max 50MB)")
		default:
			return nil, fmt.Errorf("failed to create document: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	// Parse the response - API may return JSON or multipart
	respContentType := resp.Header().Get("Content-Type")
	var doc *Document

	if strings.HasPrefix(respContentType, "multipart/") {
		doc, err = ParseMultipartDocument(resp)
		if err != nil {
			// Operation succeeded (2xx) but response parsing failed
			// Try to extract ID from response body as fallback
			doc = &Document{
				Name: req.Name,
				Type: req.Type,
			}
			if req.ID != "" {
				doc.ID = req.ID
			} else {
				// Try to extract ID from any JSON in the response
				if id := extractIDFromResponse(resp.Body()); id != "" {
					doc.ID = id
				}
			}
		}
	} else {
		// JSON response - documentMetadata wrapper
		var createResp struct {
			DocumentMetadata DocumentMetadata `json:"documentMetadata"`
		}
		if err := json.Unmarshal(resp.Body(), &createResp); err != nil {
			// Operation succeeded (2xx) but response parsing failed
			// Try to extract ID from response body as fallback
			doc = &Document{
				Name: req.Name,
				Type: req.Type,
			}
			if req.ID != "" {
				doc.ID = req.ID
			} else {
				// Try to extract ID from any JSON in the response
				if id := extractIDFromResponse(resp.Body()); id != "" {
					doc.ID = id
				}
			}
		} else {
			metadata := createResp.DocumentMetadata
			doc = &Document{
				ID:          metadata.ID,
				Name:        metadata.Name,
				Type:        metadata.Type,
				Description: metadata.Description,
				Version:     metadata.Version,
				Owner:       metadata.Owner,
				IsPrivate:   metadata.IsPrivate,
				Created:     metadata.ModificationInfo.CreatedTime,
				Modified:    metadata.ModificationInfo.LastModifiedTime,
			}
		}
	}

	return doc, nil
}

// Update updates a document's content
func (h *Handler) Update(id string, version int, content []byte, contentType string) (*Document, error) {
	if contentType == "" {
		contentType = "application/json"
	}

	resp, err := h.client.HTTP().R().
		SetQueryParam("optimistic-locking-version", fmt.Sprintf("%d", version)).
		SetMultipartField("content", "content", contentType, bytes.NewReader(content)).
		Patch(fmt.Sprintf("/platform/document/v1/documents/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("document %q not found", id)
		case 403:
			return nil, fmt.Errorf("access denied to document %q", id)
		case 409:
			return nil, fmt.Errorf("document version conflict (document was modified)")
		default:
			return nil, fmt.Errorf("failed to update document: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	// Parse the response - API may return JSON or multipart
	respContentType := resp.Header().Get("Content-Type")
	var doc *Document

	if strings.HasPrefix(respContentType, "multipart/") {
		doc, err = ParseMultipartDocument(resp)
		if err != nil {
			// Operation succeeded (2xx) but response parsing failed
			// Return a minimal document with what we know rather than failing
			doc = &Document{
				ID:      id,
				Version: version + 1, // Assume version incremented
			}
			// Try to extract name from response
			if name := extractNameFromResponse(resp.Body()); name != "" {
				doc.Name = name
			}
		}
	} else {
		// JSON response - UpdateDocumentMetadata wraps documentMetadata
		var updateResp struct {
			DocumentMetadata DocumentMetadata `json:"documentMetadata"`
		}
		if err := json.Unmarshal(resp.Body(), &updateResp); err != nil {
			// Operation succeeded (2xx) but response parsing failed
			// Return a minimal document with what we know rather than failing
			doc = &Document{
				ID:      id,
				Version: version + 1, // Assume version incremented
			}
			// Try to extract name from response
			if name := extractNameFromResponse(resp.Body()); name != "" {
				doc.Name = name
			}
		} else {
			metadata := updateResp.DocumentMetadata
			doc = &Document{
				ID:        metadata.ID,
				Name:      metadata.Name,
				Type:      metadata.Type,
				Version:   metadata.Version,
				Owner:     metadata.Owner,
				IsPrivate: metadata.IsPrivate,
			}
		}
	}

	return doc, nil
}

// extractIDFromResponse attempts to extract an ID from a response body
// This is a fallback for when normal response parsing fails
func extractIDFromResponse(body []byte) string {
	// Try to find an ID in various JSON structures
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}

	// Check common ID field locations
	if id, ok := raw["id"].(string); ok && id != "" {
		return id
	}
	if metadata, ok := raw["documentMetadata"].(map[string]interface{}); ok {
		if id, ok := metadata["id"].(string); ok && id != "" {
			return id
		}
	}
	return ""
}

// extractNameFromResponse attempts to extract a name from a response body
func extractNameFromResponse(body []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}

	if name, ok := raw["name"].(string); ok && name != "" {
		return name
	}
	if metadata, ok := raw["documentMetadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok && name != "" {
			return name
		}
	}
	return ""
}

// documentListItemToDocument converts a DocumentMetadata to a Document for table display
func documentListItemToDocument(metadata DocumentMetadata) Document {
	return Document{
		ID:          metadata.ID,
		Name:        metadata.Name,
		Type:        metadata.Type,
		Description: metadata.Description,
		Version:     metadata.Version,
		Owner:       metadata.Owner,
		IsPrivate:   metadata.IsPrivate,
		Created:     metadata.ModificationInfo.CreatedTime,
		Modified:    metadata.ModificationInfo.LastModifiedTime,
	}
}

// ConvertToDocuments converts a list of DocumentMetadata to a list of Documents for table output
func ConvertToDocuments(list *DocumentList) []Document {
	docs := make([]Document, len(list.Documents))
	for i, meta := range list.Documents {
		docs[i] = documentListItemToDocument(meta)
	}
	return docs
}

// DirectShare represents a direct share for a document
type DirectShare struct {
	ID         string `json:"id" table:"ID"`
	DocumentID string `json:"documentId" table:"DOCUMENT_ID"`
	Access     string `json:"access" table:"ACCESS"`
}

// DirectShareList represents a list of direct shares
type DirectShareList struct {
	Shares      []DirectShare `json:"directShares"`
	TotalCount  int           `json:"totalCount"`
	NextPageKey string        `json:"nextPageKey,omitempty"`
}

// SsoEntity represents an SSO user or group
type SsoEntity struct {
	ID   string `json:"id" table:"ID"`
	Type string `json:"type" table:"TYPE"` // "user" or "group"
}

// CreateDirectShareRequest contains the data needed to create a direct share
type CreateDirectShareRequest struct {
	DocumentID string      `json:"documentId"`
	Access     string      `json:"access"` // "read" or "read-write"
	Recipients []SsoEntity `json:"recipients"`
}

// CreateDirectShare creates a direct share for a document
func (h *Handler) CreateDirectShare(req CreateDirectShareRequest) (*DirectShare, error) {
	var result DirectShare

	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetResult(&result).
		Post("/platform/document/v1/direct-shares")

	if err != nil {
		return nil, fmt.Errorf("failed to create direct share: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("document %q not found", req.DocumentID)
		case 403:
			return nil, fmt.Errorf("access denied to share document %q", req.DocumentID)
		case 409:
			return nil, fmt.Errorf("a share with access %q already exists for document %q", req.Access, req.DocumentID)
		default:
			return nil, fmt.Errorf("failed to create direct share: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// ListDirectShares lists direct shares for a document
func (h *Handler) ListDirectShares(documentID string) (*DirectShareList, error) {
	var result DirectShareList

	req := h.client.HTTP().R().SetResult(&result)

	if documentID != "" {
		req.SetQueryParam("filter", fmt.Sprintf("documentId=='%s'", documentID))
	}

	resp, err := req.Get("/platform/document/v1/direct-shares")

	if err != nil {
		return nil, fmt.Errorf("failed to list direct shares: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list direct shares: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// DeleteDirectShare deletes a direct share
func (h *Handler) DeleteDirectShare(shareID string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/document/v1/direct-shares/%s", shareID))

	if err != nil {
		return fmt.Errorf("failed to delete direct share: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("share %q not found", shareID)
		case 403:
			return fmt.Errorf("access denied to delete share %q", shareID)
		default:
			return fmt.Errorf("failed to delete direct share: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// AddDirectShareRecipients adds recipients to a direct share
func (h *Handler) AddDirectShareRecipients(shareID string, recipients []SsoEntity) error {
	body := map[string]interface{}{
		"recipients": recipients,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		Post(fmt.Sprintf("/platform/document/v1/direct-shares/%s/recipients/add", shareID))

	if err != nil {
		return fmt.Errorf("failed to add recipients: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("share %q not found", shareID)
		case 403:
			return fmt.Errorf("access denied to modify share %q", shareID)
		default:
			return fmt.Errorf("failed to add recipients: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// RemoveDirectShareRecipients removes recipients from a direct share
func (h *Handler) RemoveDirectShareRecipients(shareID string, recipientIDs []string) error {
	body := map[string]interface{}{
		"ids": recipientIDs,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		Post(fmt.Sprintf("/platform/document/v1/direct-shares/%s/recipients/remove", shareID))

	if err != nil {
		return fmt.Errorf("failed to remove recipients: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("share %q not found", shareID)
		case 403:
			return fmt.Errorf("access denied to modify share %q", shareID)
		default:
			return fmt.Errorf("failed to remove recipients: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// MarshalJSON custom marshaler for Document to include content when present
func (d Document) MarshalJSON() ([]byte, error) {
	type Alias Document

	// If content is present, try to parse it as JSON for cleaner output
	var contentJSON json.RawMessage
	if len(d.Content) > 0 {
		// Check if content is valid JSON
		if json.Valid(d.Content) {
			contentJSON = d.Content
		} else {
			// If not valid JSON, encode as base64 string
			contentJSON, _ = json.Marshal(string(d.Content))
		}
	}

	// Only include modificationInfo if timestamps are set
	var modInfo *ModificationInfo
	if !d.Created.IsZero() || !d.Modified.IsZero() {
		modInfo = &ModificationInfo{
			CreatedTime:      d.Created,
			LastModifiedTime: d.Modified,
		}
	}

	return json.Marshal(&struct {
		*Alias
		ModificationInfo *ModificationInfo `json:"modificationInfo,omitempty"`
		Content          json.RawMessage   `json:"content,omitempty"`
	}{
		Alias:            (*Alias)(&d),
		ModificationInfo: modInfo,
		Content:          contentJSON,
	})
}

// UnmarshalJSON custom unmarshaler for Document to handle nested modificationInfo
func (d *Document) UnmarshalJSON(data []byte) error {
	type Alias Document
	aux := &struct {
		ModificationInfo *ModificationInfo `json:"modificationInfo"`
		Content          json.RawMessage   `json:"content"`
		*Alias
	}{
		Alias: (*Alias)(d),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.ModificationInfo != nil {
		d.Created = aux.ModificationInfo.CreatedTime
		d.Modified = aux.ModificationInfo.LastModifiedTime
	}

	// Handle content field - it could be a JSON object or a string
	if len(aux.Content) > 0 {
		// If it's a JSON object/array, store as-is
		if aux.Content[0] == '{' || aux.Content[0] == '[' {
			d.Content = aux.Content
		} else {
			// It's a quoted string, unmarshal it
			var contentStr string
			if err := json.Unmarshal(aux.Content, &contentStr); err == nil {
				d.Content = []byte(contentStr)
			}
		}
	}

	return nil
}

// Snapshot represents a document snapshot (version)
type Snapshot struct {
	SnapshotVersion  int             `json:"snapshotVersion" table:"VERSION"`
	DocumentVersion  int             `json:"documentVersion" table:"DOC_VERSION,wide"`
	Description      string          `json:"description,omitempty" table:"DESCRIPTION"`
	ModificationInfo SnapshotModInfo `json:"modificationInfo" table:"-"`
	CreatedBy        string          `json:"-" table:"CREATED_BY"`
	CreatedTime      time.Time       `json:"-" table:"CREATED"`
}

// SnapshotModInfo contains creation info for a snapshot
type SnapshotModInfo struct {
	CreatedBy   string    `json:"createdBy"`
	CreatedTime time.Time `json:"createdTime"`
}

// UnmarshalJSON custom unmarshaler for Snapshot to flatten modificationInfo
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	type Alias Snapshot
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	// Flatten modificationInfo fields for table display
	s.CreatedBy = s.ModificationInfo.CreatedBy
	s.CreatedTime = s.ModificationInfo.CreatedTime
	return nil
}

// SnapshotList represents a list of snapshots
type SnapshotList struct {
	Snapshots   []Snapshot `json:"snapshots"`
	TotalCount  int        `json:"totalCount"`
	NextPageKey string     `json:"nextPageKey,omitempty"`
}

// ListSnapshots retrieves all snapshots for a document
func (h *Handler) ListSnapshots(documentID string) (*SnapshotList, error) {
	var allSnapshots []Snapshot
	var totalCount int
	nextPageKey := ""

	for {
		var result SnapshotList
		req := h.client.HTTP().R().SetResult(&result)

		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		resp, err := req.Get(fmt.Sprintf("/platform/document/v1/documents/%s/snapshots", documentID))
		if err != nil {
			return nil, fmt.Errorf("failed to list snapshots: %w", err)
		}

		if resp.IsError() {
			switch resp.StatusCode() {
			case 404:
				return nil, fmt.Errorf("document %q not found", documentID)
			case 403:
				return nil, fmt.Errorf("access denied to document %q", documentID)
			default:
				return nil, fmt.Errorf("failed to list snapshots: status %d: %s", resp.StatusCode(), resp.String())
			}
		}

		allSnapshots = append(allSnapshots, result.Snapshots...)
		totalCount = result.TotalCount

		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &SnapshotList{
		Snapshots:  allSnapshots,
		TotalCount: totalCount,
	}, nil
}

// GetSnapshot retrieves metadata for a specific snapshot
func (h *Handler) GetSnapshot(documentID string, version int) (*Snapshot, error) {
	var result Snapshot

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/document/v1/documents/%s/snapshots/%d", documentID, version))

	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("snapshot version %d not found for document %q", version, documentID)
		case 403:
			return nil, fmt.Errorf("access denied to document %q", documentID)
		default:
			return nil, fmt.Errorf("failed to get snapshot: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// RestoreSnapshot restores a document to a specific snapshot version
func (h *Handler) RestoreSnapshot(documentID string, version int) (*DocumentMetadata, error) {
	var result struct {
		DocumentMetadata DocumentMetadata `json:"documentMetadata"`
	}

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Post(fmt.Sprintf("/platform/document/v1/documents/%s/snapshots/%d:restore", documentID, version))

	if err != nil {
		return nil, fmt.Errorf("failed to restore snapshot: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("snapshot version %d not found for document %q", version, documentID)
		case 403:
			return nil, fmt.Errorf("access denied to restore document %q (only owner can restore)", documentID)
		case 409:
			return nil, fmt.Errorf("conflict restoring snapshot (document may have been modified)")
		default:
			return nil, fmt.Errorf("failed to restore snapshot: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result.DocumentMetadata, nil
}

// DeleteSnapshot deletes a specific snapshot
func (h *Handler) DeleteSnapshot(documentID string, version int) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/document/v1/documents/%s/snapshots/%d", documentID, version))

	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("snapshot version %d not found for document %q", version, documentID)
		case 403:
			return fmt.Errorf("access denied to delete snapshot (only owner can delete)")
		default:
			return fmt.Errorf("failed to delete snapshot: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetAtVersion retrieves a document's content at a specific snapshot version
func (h *Handler) GetAtVersion(id string, version int) (*Document, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("snapshot-version", fmt.Sprintf("%d", version)).
		Get(fmt.Sprintf("/platform/document/v1/documents/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get document at version: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("document %q or snapshot version %d not found", id, version)
		case 403:
			return nil, fmt.Errorf("access denied to document %q", id)
		default:
			return nil, fmt.Errorf("failed to get document at version: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	doc, err := ParseMultipartDocument(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse document response: %w", err)
	}

	return doc, nil
}

// MarshalYAML custom marshaler for Document to include content for YAML output
func (d Document) MarshalYAML() (interface{}, error) {
	// Parse content as structured data if it's valid JSON
	var contentData interface{}
	if len(d.Content) > 0 && json.Valid(d.Content) {
		if err := json.Unmarshal(d.Content, &contentData); err != nil {
			// If unmarshal fails, use raw string
			contentData = string(d.Content)
		}
	} else if len(d.Content) > 0 {
		contentData = string(d.Content)
	}

	// Build the output map
	output := map[string]interface{}{
		"id":        d.ID,
		"name":      d.Name,
		"type":      d.Type,
		"version":   d.Version,
		"owner":     d.Owner,
		"isPrivate": d.IsPrivate,
	}

	if d.Description != "" {
		output["description"] = d.Description
	}

	if contentData != nil {
		output["content"] = contentData
	}

	if !d.Created.IsZero() || !d.Modified.IsZero() {
		output["modificationInfo"] = map[string]interface{}{
			"createdTime":      d.Created,
			"lastModifiedTime": d.Modified,
		}
	}

	return output, nil
}
