package appengine

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles App Engine resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new App Engine handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// App represents an installed app
type App struct {
	ID               string                 `json:"id" table:"ID"`
	Name             string                 `json:"name" table:"NAME"`
	Version          string                 `json:"version" table:"VERSION"`
	Description      string                 `json:"description" table:"DESCRIPTION,wide"`
	IsBuiltin        bool                   `json:"isBuiltin,omitempty" table:"BUILTIN,wide"`
	ResourceStatus   *ResourceStatus        `json:"resourceStatus,omitempty" table:"-"`
	SignatureInfo    *SignatureInfo         `json:"signatureInfo,omitempty" table:"-"`
	Manifest         map[string]interface{} `json:"manifest,omitempty" table:"-"`
	ModificationInfo *ModificationInfo      `json:"modificationInfo,omitempty" table:"-"`
}

// ResourceStatus represents the status of an app's resources
type ResourceStatus struct {
	Status              string   `json:"status"`
	SubResourceTypes    []string `json:"subResourceTypes,omitempty"`
	SubResourceStatuses []string `json:"subResourceStatuses,omitempty"`
}

// SignatureInfo represents signature information for an app
type SignatureInfo struct {
	Signature string `json:"signature,omitempty"`
}

// ModificationInfo contains modification timestamps
type ModificationInfo struct {
	CreatedBy        string `json:"createdBy,omitempty"`
	CreatedTime      string `json:"createdTime,omitempty"`
	LastModifiedBy   string `json:"lastModifiedBy,omitempty"`
	LastModifiedTime string `json:"lastModifiedTime,omitempty"`
}

// AppList represents a list of apps
type AppList struct {
	Apps []App `json:"apps"`
}

// ListApps lists all installed apps
func (h *Handler) ListApps() (*AppList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "isBuiltin,manifest").
		Get("/platform/app-engine/registry/v1/apps")

	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list apps: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result AppList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse apps response: %w", err)
	}

	return &result, nil
}

// GetApp gets a specific app by ID
func (h *Handler) GetApp(appID string) (*App, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "isBuiltin,manifest,resourceStatus.subResourceTypes").
		Get(fmt.Sprintf("/platform/app-engine/registry/v1/apps/%s", appID))

	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("app %q not found", appID)
		default:
			return nil, fmt.Errorf("failed to get app: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result App
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse app response: %w", err)
	}

	return &result, nil
}

// DeleteApp uninstalls an app
func (h *Handler) DeleteApp(appID string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/app-engine/registry/v1/apps/%s", appID))

	if err != nil {
		return fmt.Errorf("failed to uninstall app: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("app %q not found", appID)
		case 403:
			return fmt.Errorf("access denied to uninstall app %q", appID)
		default:
			return fmt.Errorf("failed to uninstall app: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}
