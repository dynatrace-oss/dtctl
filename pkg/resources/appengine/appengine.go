package appengine

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkae "github.com/dynatrace-oss/dtctl/sdk/api/appengine"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	App              = sdkae.App
	ResourceStatus   = sdkae.ResourceStatus
	SignatureInfo    = sdkae.SignatureInfo
	ModificationInfo = sdkae.ModificationInfo
	AppList          = sdkae.AppList
	AppFunction      = sdkae.AppFunction
)

// Handler handles App Engine resources.
type Handler struct {
	sdk *sdkae.Handler
}

// NewHandler creates a new App Engine handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkae.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// ListApps lists all installed apps
func (h *Handler) ListApps() (*AppList, error) {
	return h.sdk.ListApps()
}

// GetApp gets a specific app by ID
func (h *Handler) GetApp(appID string) (*App, error) {
	return h.sdk.GetApp(appID)
}

// DeleteApp uninstalls an app
func (h *Handler) DeleteApp(appID string) error {
	return h.sdk.DeleteApp(appID)
}

// ListFunctions lists all functions across apps (or filtered by app ID)
func (h *Handler) ListFunctions(appIDFilter string) ([]AppFunction, error) {
	return h.sdk.ListFunctions(appIDFilter)
}

// GetFunction gets details about a specific function
func (h *Handler) GetFunction(fullName string) (*AppFunction, error) {
	return h.sdk.GetFunction(fullName)
}
