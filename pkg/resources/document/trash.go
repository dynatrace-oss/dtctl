package document

import (
	"fmt"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// TrashedDocument represents a document in the trash (from GET /trash/documents/{id})
type TrashedDocument struct {
	ID               string           `json:"id" yaml:"id" table:"ID"`
	Name             string           `json:"name" yaml:"name" table:"NAME"`
	Type             string           `json:"type" yaml:"type" table:"TYPE"`
	Version          int              `json:"version" yaml:"version" table:"VERSION,wide"`
	Owner            string           `json:"owner" yaml:"owner" table:"OWNER,wide"`
	DeletionInfo     DeletionInfo     `json:"deletionInfo" yaml:"deletionInfo" table:"-"`
	ModificationInfo ModificationInfo `json:"modificationInfo,omitempty" yaml:"modificationInfo,omitempty" table:"-"`
	// Computed fields for display
	DeletedBy string    `json:"-" yaml:"-" table:"DELETED BY"`
	DeletedAt time.Time `json:"-" yaml:"-" table:"DELETED AT"`
}

// TrashDocumentListEntry represents a document in the trash list (from GET /trash/documents)
type TrashDocumentListEntry struct {
	ID           string       `json:"id" yaml:"id" table:"ID"`
	Name         string       `json:"name" yaml:"name" table:"NAME"`
	Type         string       `json:"type" yaml:"type" table:"TYPE"`
	DeletionInfo DeletionInfo `json:"deletionInfo" yaml:"deletionInfo" table:"-"`
	// Computed fields for display
	DeletedBy string    `json:"-" yaml:"-" table:"DELETED BY"`
	DeletedAt time.Time `json:"-" yaml:"-" table:"DELETED AT"`
}

// DeletionInfo contains deletion metadata
type DeletionInfo struct {
	DeletedBy   string    `json:"deletedBy" yaml:"deletedBy"`
	DeletedTime time.Time `json:"deletedTime" yaml:"deletedTime"`
}

// TrashListOptions contains filter options for listing trashed documents
type TrashListOptions struct {
	Type          string    // Filter by type: "dashboard", "notebook"
	DeletedBy     string    // Filter by who deleted it
	DeletedAfter  time.Time // Show documents deleted after date
	DeletedBefore time.Time // Show documents deleted before date
	ChunkSize     int64     // Page size for pagination (0 = no chunking)
}

// RestoreOptions contains options for restoring a document
type RestoreOptions struct {
	Force   bool   // Restore even if name conflicts exist (not supported by API yet)
	NewName string // Restore with a new name (not supported by API yet)
}

// TrashList represents a list of trashed documents
type TrashList struct {
	Documents   []TrashDocumentListEntry `json:"documents"`
	TotalCount  int                      `json:"totalCount"`
	NextPageKey string                   `json:"nextPageKey,omitempty"`
}

// TrashHandler handles trash operations for documents
type TrashHandler struct {
	client *client.Client
}

// NewTrashHandler creates a new trash handler
func NewTrashHandler(c *client.Client) *TrashHandler {
	return &TrashHandler{client: c}
}

// List retrieves trashed documents matching the provided filters
func (h *TrashHandler) List(opts TrashListOptions) ([]TrashDocumentListEntry, error) {
	var allDocuments []TrashDocumentListEntry
	nextPageKey := ""

	// Build filter query parameter
	var filterStr string
	var conditions []string

	if opts.Type != "" {
		conditions = append(conditions, fmt.Sprintf("type=='%s'", opts.Type))
	}
	if opts.DeletedBy != "" {
		conditions = append(conditions, fmt.Sprintf("deletionInfo.deletedBy=='%s'", opts.DeletedBy))
	}
	if !opts.DeletedAfter.IsZero() {
		conditions = append(conditions, fmt.Sprintf("deletionInfo.deletedTime>='%s'", opts.DeletedAfter.Format(time.RFC3339)))
	}
	if !opts.DeletedBefore.IsZero() {
		conditions = append(conditions, fmt.Sprintf("deletionInfo.deletedTime<='%s'", opts.DeletedBefore.Format(time.RFC3339)))
	}

	if len(conditions) > 0 {
		filterStr = strings.Join(conditions, " and ")
	}

	for {
		var result TrashList
		req := h.client.HTTP().R().SetResult(&result)

		if filterStr != "" {
			req.SetQueryParam("filter", filterStr)
		}

		// Set page size if chunking is enabled
		if opts.ChunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", opts.ChunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		resp, err := req.Get("/platform/document/v1/trash/documents")
		if err != nil {
			return nil, fmt.Errorf("failed to list trash: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list trash: status %d: %s", resp.StatusCode(), resp.String())
		}

		// Populate computed fields
		for i := range result.Documents {
			doc := &result.Documents[i]
			doc.DeletedBy = doc.DeletionInfo.DeletedBy
			doc.DeletedAt = doc.DeletionInfo.DeletedTime
		}

		allDocuments = append(allDocuments, result.Documents...)

		// If chunking is disabled, return first page only
		if opts.ChunkSize == 0 {
			return result.Documents, nil
		}

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return allDocuments, nil
}

// Get retrieves a specific trashed document by ID
func (h *TrashHandler) Get(id string) (*TrashedDocument, error) {
	var doc TrashedDocument

	resp, err := h.client.HTTP().R().
		SetResult(&doc).
		Get(fmt.Sprintf("/platform/document/v1/trash/documents/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get trashed document: %w", err)
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("document not found in trash: %s", id)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to get trashed document: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Populate computed fields
	doc.DeletedBy = doc.DeletionInfo.DeletedBy
	doc.DeletedAt = doc.DeletionInfo.DeletedTime

	return &doc, nil
}

// Restore restores a document from trash
func (h *TrashHandler) Restore(id string, opts RestoreOptions) error {
	req := h.client.HTTP().R()

	// Note: The API doesn't support newName or force options in the spec
	// These are left here for potential future support
	if opts.NewName != "" {
		req.SetBody(map[string]interface{}{
			"name": opts.NewName,
		})
	}

	if opts.Force {
		req.SetQueryParam("force", "true")
	}

	resp, err := req.Post(fmt.Sprintf("/platform/document/v1/trash/documents/%s/restore", id))
	if err != nil {
		return fmt.Errorf("failed to restore document: %w", err)
	}

	if resp.StatusCode() == 404 {
		return fmt.Errorf("document not found in trash: %s", id)
	}

	if resp.StatusCode() == 409 {
		return fmt.Errorf("name conflict: document with same name exists")
	}

	if resp.IsError() {
		return fmt.Errorf("failed to restore document: status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

// Delete permanently deletes a document from trash
func (h *TrashHandler) Delete(id string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/document/v1/trash/documents/%s", id))

	if err != nil {
		return fmt.Errorf("failed to permanently delete document: %w", err)
	}

	if resp.StatusCode() == 404 {
		return fmt.Errorf("document not found in trash: %s", id)
	}

	if resp.IsError() {
		return fmt.Errorf("failed to permanently delete document: status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

