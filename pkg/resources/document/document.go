package document

import (
	"errors"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkdocument "github.com/dynatrace-oss/dtctl/sdk/api/document"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	Document                      = sdkdocument.Document
	DocumentMetadata              = sdkdocument.DocumentMetadata
	DocumentList                  = sdkdocument.DocumentList
	DocumentFilters               = sdkdocument.DocumentFilters
	ModificationInfo              = sdkdocument.ModificationInfo
	ShareInfo                     = sdkdocument.ShareInfo
	UserContext                    = sdkdocument.UserContext
	CreateRequest                 = sdkdocument.CreateRequest
	DirectShare                   = sdkdocument.DirectShare
	DirectShareList               = sdkdocument.DirectShareList
	SsoEntity                     = sdkdocument.SsoEntity
	CreateDirectShareRequest      = sdkdocument.CreateDirectShareRequest
	EnvironmentShare              = sdkdocument.EnvironmentShare
	EnvironmentShareList          = sdkdocument.EnvironmentShareList
	CreateEnvironmentShareRequest = sdkdocument.CreateEnvironmentShareRequest
	Snapshot                      = sdkdocument.Snapshot
	SnapshotModInfo               = sdkdocument.SnapshotModInfo
	SnapshotList                  = sdkdocument.SnapshotList
)

// Re-export SDK sentinel errors.
var (
	ErrShareConflict   = sdkdocument.ErrShareConflict
	ErrVersionConflict = sdkdocument.ErrVersionConflict
)

// Re-export SDK functions.
var (
	ConvertToDocuments     = sdkdocument.ConvertToDocuments
	ParseMultipartDocument = sdkdocument.ParseMultipartDocument
	CreateUpdateRequest    = sdkdocument.CreateUpdateRequest
)

// Handler handles document resources (dashboards, notebooks, etc.)
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	client *client.Client
	sdk    *sdkdocument.Handler
}

// NewHandler creates a new document handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		client: c,
		sdk:    sdkdocument.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List retrieves documents matching the provided filters with automatic pagination.
func (h *Handler) List(filters DocumentFilters) (*DocumentList, error) {
	return h.sdk.List(filters)
}

// Get retrieves a specific document by ID.
func (h *Handler) Get(id string) (*Document, error) {
	return h.sdk.Get(id)
}

// GetMetadata retrieves only the metadata for a document.
func (h *Handler) GetMetadata(id string) (*DocumentMetadata, error) {
	return h.sdk.GetMetadata(id)
}

// GetRaw retrieves a document's content as raw bytes.
func (h *Handler) GetRaw(id string) ([]byte, error) {
	doc, err := h.sdk.Get(id)
	if err != nil {
		return nil, err
	}
	return doc.Content, nil
}

// Delete deletes a document.
func (h *Handler) Delete(id string, version int) error {
	return h.sdk.Delete(id, version)
}

// Create creates a new document.
func (h *Handler) Create(req CreateRequest) (*Document, error) {
	return h.sdk.Create(req)
}

// Update updates a document's content.
func (h *Handler) Update(id string, version int, content []byte, contentType string) (*Document, error) {
	return h.sdk.Update(id, version, content, contentType)
}

// UpdateWithMetadata updates a document's content and optionally its metadata (name, description).
func (h *Handler) UpdateWithMetadata(id string, version int, content []byte, contentType string, name string, description string) (*Document, error) {
	return h.sdk.UpdateWithMetadata(id, version, content, contentType, name, description)
}

// CreateDirectShare creates a direct share for a document.
func (h *Handler) CreateDirectShare(req CreateDirectShareRequest) (*DirectShare, error) {
	return h.sdk.CreateDirectShare(req)
}

// ListDirectShares lists direct shares for a document.
func (h *Handler) ListDirectShares(documentID string) (*DirectShareList, error) {
	return h.sdk.ListDirectShares(documentID)
}

// DeleteDirectShare deletes a direct share.
func (h *Handler) DeleteDirectShare(shareID string) error {
	return h.sdk.DeleteDirectShare(shareID)
}

// AddDirectShareRecipients adds recipients to a direct share.
func (h *Handler) AddDirectShareRecipients(shareID string, recipients []SsoEntity) error {
	return h.sdk.AddDirectShareRecipients(shareID, recipients)
}

// RemoveDirectShareRecipients removes recipients from a direct share.
func (h *Handler) RemoveDirectShareRecipients(shareID string, recipientIDs []string) error {
	return h.sdk.RemoveDirectShareRecipients(shareID, recipientIDs)
}

// CreateEnvironmentShare creates an environment-wide share for a document.
func (h *Handler) CreateEnvironmentShare(req CreateEnvironmentShareRequest) (*EnvironmentShare, error) {
	return h.sdk.CreateEnvironmentShare(req)
}

// ListEnvironmentShares lists environment shares for a document (or all if documentID is empty).
func (h *Handler) ListEnvironmentShares(documentID string) (*EnvironmentShareList, error) {
	return h.sdk.ListEnvironmentShares(documentID)
}

// DeleteEnvironmentShare deletes an environment share.
func (h *Handler) DeleteEnvironmentShare(shareID string) error {
	return h.sdk.DeleteEnvironmentShare(shareID)
}

// SetDocumentPublic flips a document's isPrivate flag to false.
func (h *Handler) SetDocumentPublic(id string, version int) error {
	return h.sdk.SetDocumentPublic(id, version)
}

// ListSnapshots retrieves all snapshots for a document.
func (h *Handler) ListSnapshots(documentID string) (*SnapshotList, error) {
	return h.sdk.ListSnapshots(documentID)
}

// GetSnapshot retrieves metadata for a specific snapshot.
func (h *Handler) GetSnapshot(documentID string, version int) (*Snapshot, error) {
	return h.sdk.GetSnapshot(documentID, version)
}

// RestoreSnapshot restores a document to a specific snapshot version.
func (h *Handler) RestoreSnapshot(documentID string, version int) (*DocumentMetadata, error) {
	return h.sdk.RestoreSnapshot(documentID, version)
}

// DeleteSnapshot deletes a specific snapshot.
func (h *Handler) DeleteSnapshot(documentID string, version int) error {
	return h.sdk.DeleteSnapshot(documentID, version)
}

// GetAtVersion retrieves a document's content at a specific snapshot version.
func (h *Handler) GetAtVersion(id string, version int) (*Document, error) {
	return h.sdk.GetAtVersion(id, version)
}

// EnsureEnvironmentShare idempotently ensures the document has an environment share at the given
// access level, AND that the document itself is marked public (isPrivate=false).
//
// This is a CLI-specific composite operation not present in the SDK.
func (h *Handler) EnsureEnvironmentShare(documentID, access string) (*EnvironmentShare, error) {
	share, err := h.ensureShareAtAccess(documentID, access)
	if err != nil {
		return nil, err
	}

	// Flip the document to public. Fetch current version for optimistic locking.
	meta, err := h.sdk.GetMetadata(documentID)
	if err != nil {
		return share, fmt.Errorf("share created but could not read document metadata to flip isPrivate: %w", err)
	}
	if meta.IsPrivate {
		if err := h.sdk.SetDocumentPublic(documentID, meta.Version); err != nil {
			if !errors.Is(err, ErrVersionConflict) {
				return share, err
			}
			// Retry once: re-fetch metadata and try again.
			meta, err = h.sdk.GetMetadata(documentID)
			if err != nil {
				return share, fmt.Errorf("share created but retry metadata fetch failed: %w", err)
			}
			if meta.IsPrivate {
				if err := h.sdk.SetDocumentPublic(documentID, meta.Version); err != nil {
					return share, err
				}
			}
		}
	}
	return share, nil
}

// ensureShareAtAccess handles the share creation/replacement logic, including 409 race recovery.
func (h *Handler) ensureShareAtAccess(documentID, access string) (*EnvironmentShare, error) {
	existing, err := h.sdk.ListEnvironmentShares(documentID)
	if err != nil {
		return nil, err
	}

	share, toDelete := findOrCollectShares(existing.Shares, access)
	if share != nil {
		return share, nil
	}

	// Delete non-matching shares and create a new one at the requested access level.
	for _, id := range toDelete {
		if err := h.sdk.DeleteEnvironmentShare(id); err != nil {
			return nil, fmt.Errorf("failed to replace existing environment share: %w", err)
		}
	}

	created, err := h.sdk.CreateEnvironmentShare(CreateEnvironmentShareRequest{
		DocumentID: documentID,
		Access:     access,
	})
	if err == nil {
		return created, nil
	}

	// Handle race condition: another process may have created the share
	if !errors.Is(err, ErrShareConflict) {
		return nil, err
	}

	reListed, reErr := h.sdk.ListEnvironmentShares(documentID)
	if reErr != nil {
		return nil, fmt.Errorf("create returned conflict and re-list failed: %w", reErr)
	}

	share, toDelete = findOrCollectShares(reListed.Shares, access)
	if share != nil {
		return share, nil
	}

	for _, id := range toDelete {
		if err := h.sdk.DeleteEnvironmentShare(id); err != nil {
			return nil, fmt.Errorf("failed to replace racing environment share: %w", err)
		}
	}
	return h.sdk.CreateEnvironmentShare(CreateEnvironmentShareRequest{
		DocumentID: documentID,
		Access:     access,
	})
}

// findOrCollectShares scans shares for an exact access match. Returns the match (if any)
// and a list of non-matching share IDs suitable for deletion.
func findOrCollectShares(shares []EnvironmentShare, access string) (*EnvironmentShare, []string) {
	var match *EnvironmentShare
	var toDelete []string
	for i := range shares {
		s := shares[i]
		if s.ExactAccess(access) {
			match = &s
		} else {
			toDelete = append(toDelete, s.ID)
		}
	}
	return match, toDelete
}
