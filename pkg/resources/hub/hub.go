package hub

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkhub "github.com/dynatrace-oss/dtctl/sdk/api/hub"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	HubExtension            = sdkhub.HubExtension
	HubExtensionList        = sdkhub.HubExtensionList
	HubExtensionRelease     = sdkhub.HubExtensionRelease
	HubExtensionReleaseList = sdkhub.HubExtensionReleaseList
)

// Handler handles Dynatrace Hub catalog resources.
// It delegates to the SDK handler.
type Handler struct {
	sdk *sdkhub.Handler
}

// NewHandler creates a new Hub handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkhub.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// ListExtensions lists all Hub catalog extensions with automatic pagination.
// filter is a case-insensitive substring matched against id, name, and description.
func (h *Handler) ListExtensions(filter string, chunkSize int64) (*HubExtensionList, error) {
	return h.sdk.ListExtensions(filter, chunkSize)
}

// GetExtension gets a specific Hub extension by ID.
func (h *Handler) GetExtension(id string) (*HubExtension, error) {
	return h.sdk.GetExtension(id)
}

// ListExtensionReleases lists all releases for a Hub extension.
func (h *Handler) ListExtensionReleases(id string, chunkSize int64) (*HubExtensionReleaseList, error) {
	return h.sdk.ListExtensionReleases(id, chunkSize)
}
