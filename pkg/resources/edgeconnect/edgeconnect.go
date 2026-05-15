package edgeconnect

import (
	"encoding/json"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkedgeconnect "github.com/dynatrace-oss/dtctl/sdk/api/edgeconnect"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	EdgeConnect      = sdkedgeconnect.EdgeConnect
	EdgeConnectList  = sdkedgeconnect.EdgeConnectList
	ModificationInfo = sdkedgeconnect.ModificationInfo
	Metadata         = sdkedgeconnect.Metadata
)

// EdgeConnectCreate represents the request body for creating an EdgeConnect.
// CLI-specific type: the SDK Create method accepts an EdgeConnect directly.
type EdgeConnectCreate struct {
	Name          string   `json:"name"`
	HostPatterns  []string `json:"hostPatterns,omitempty"`
	OAuthClientID string   `json:"oauthClientId,omitempty"`
}

// Handler handles EdgeConnect resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	sdk *sdkedgeconnect.Handler
}

// NewHandler creates a new EdgeConnect handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkedgeconnect.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List lists all EdgeConnect configurations.
func (h *Handler) List() (*EdgeConnectList, error) { return h.sdk.List() }

// Get gets a specific EdgeConnect by ID.
func (h *Handler) Get(edgeConnectID string) (*EdgeConnect, error) {
	return h.sdk.Get(edgeConnectID)
}

// Create creates a new EdgeConnect.
func (h *Handler) Create(req EdgeConnectCreate) (*EdgeConnect, error) {
	return h.sdk.Create(EdgeConnect{
		Name:          req.Name,
		HostPatterns:  req.HostPatterns,
		OAuthClientID: req.OAuthClientID,
	})
}

// Update updates an existing EdgeConnect.
func (h *Handler) Update(edgeConnectID string, req EdgeConnect) error {
	return h.sdk.Update(edgeConnectID, req)
}

// Delete deletes an EdgeConnect.
func (h *Handler) Delete(edgeConnectID string) error { return h.sdk.Delete(edgeConnectID) }

// GetRaw gets an EdgeConnect as raw JSON bytes (for editing).
func (h *Handler) GetRaw(edgeConnectID string) ([]byte, error) {
	ec, err := h.sdk.Get(edgeConnectID)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(ec, "", "  ")
}
