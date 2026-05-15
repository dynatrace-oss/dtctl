package document

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkdocument "github.com/dynatrace-oss/dtctl/sdk/api/document"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK trash types so existing CLI code continues to compile unchanged.
type (
	TrashedDocument        = sdkdocument.TrashedDocument
	TrashDocumentListEntry = sdkdocument.TrashDocumentListEntry
	DeletionInfo           = sdkdocument.DeletionInfo
	TrashListOptions       = sdkdocument.TrashListOptions
	RestoreOptions         = sdkdocument.RestoreOptions
	TrashList              = sdkdocument.TrashList
)

// TrashHandler handles trash operations for documents.
// It delegates to the SDK trash handler.
type TrashHandler struct {
	sdk *sdkdocument.TrashHandler
}

// NewTrashHandler creates a new trash handler.
func NewTrashHandler(c *client.Client) *TrashHandler {
	return &TrashHandler{
		sdk: sdkdocument.NewTrashHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List retrieves trashed documents matching the provided filters.
func (h *TrashHandler) List(opts TrashListOptions) ([]TrashDocumentListEntry, error) {
	return h.sdk.List(opts)
}

// Get retrieves a specific trashed document by ID.
func (h *TrashHandler) Get(id string) (*TrashedDocument, error) {
	return h.sdk.Get(id)
}

// Restore restores a document from trash.
func (h *TrashHandler) Restore(id string, opts RestoreOptions) error {
	return h.sdk.Restore(id, opts)
}

// Delete permanently deletes a document from trash.
func (h *TrashHandler) Delete(id string) error {
	return h.sdk.Delete(id)
}
