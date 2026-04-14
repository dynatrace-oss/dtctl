package hub

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles Dynatrace Hub catalog resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new Hub handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// ---------------------------------------------------------------------------
// Hub Extensions
// ---------------------------------------------------------------------------

// HubExtension represents a Dynatrace Hub catalog extension
type HubExtension struct {
	ID          string `json:"id" table:"ID"`
	Name        string `json:"name" table:"NAME"`
	Type        string `json:"type,omitempty" table:"-"`
	Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
}

// HubExtensionList represents a paginated list of Hub extensions
type HubExtensionList struct {
	Items       []HubExtension `json:"items"`
	TotalCount  int            `json:"totalCount"`
	NextPageKey string         `json:"nextPageKey,omitempty"`
}

// HubExtensionRelease represents a release of a Hub extension
type HubExtensionRelease struct {
	Version     string `json:"version" yaml:"version" table:"VERSION"`
	ReleaseDate string `json:"releaseDate,omitempty" yaml:"releaseDate,omitempty" table:"RELEASE_DATE,wide"`
	Notes       string `json:"notes,omitempty" yaml:"notes,omitempty" table:"-"`
}

// HubExtensionReleaseList represents a list of Hub extension releases
type HubExtensionReleaseList struct {
	Items       []HubExtensionRelease `json:"items"`
	TotalCount  int                   `json:"totalCount"`
	NextPageKey string                `json:"nextPageKey,omitempty"`
}

// ListExtensions lists all Hub catalog extensions with automatic pagination.
// filter is a case-insensitive substring matched against id, name, and description.
func (h *Handler) ListExtensions(filter string, chunkSize int64) (*HubExtensionList, error) {
	var allItems []HubExtension
	var totalCount int
	nextPageKey := ""

	for {
		var result HubExtensionList
		req := h.client.HTTP().R().SetResult(&result)

		client.PaginationParams{
			Style:         client.PaginationDefault,
			PageKeyParam:  "page-key",
			PageSizeParam: "page-size",
			NextPageKey:   nextPageKey,
			PageSize:      chunkSize,
		}.Apply(req)

		resp, err := req.Get("/platform/hub/v1/catalog/extensions")
		if err != nil {
			return nil, fmt.Errorf("failed to list Hub extensions: %w", err)
		}
		if resp.IsError() {
			return nil, fmt.Errorf("failed to list Hub extensions: status %d: %s", resp.StatusCode(), resp.String())
		}

		allItems = append(allItems, result.Items...)
		totalCount = result.TotalCount

		if chunkSize == 0 || result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	// Client-side filtering: the API does not support server-side filtering,
	// so we match case-insensitively against id, name, and description.
	if filter != "" {
		q := strings.ToLower(filter)
		filtered := allItems[:0]
		for _, ext := range allItems {
			if strings.Contains(strings.ToLower(ext.ID), q) ||
				strings.Contains(strings.ToLower(ext.Name), q) ||
				strings.Contains(strings.ToLower(ext.Description), q) {
				filtered = append(filtered, ext)
			}
		}
		allItems = filtered
		totalCount = len(filtered)
	}

	return &HubExtensionList{Items: allItems, TotalCount: totalCount}, nil
}

// GetExtension gets a specific Hub extension by ID
func (h *Handler) GetExtension(id string) (*HubExtension, error) {
	var result HubExtension

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/hub/v1/catalog/extensions/%s", url.PathEscape(id)))

	if err != nil {
		return nil, fmt.Errorf("failed to get Hub extension: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to get Hub extension %q: status %d: %s", id, resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// ListExtensionReleases lists all releases for a Hub extension
func (h *Handler) ListExtensionReleases(id string, chunkSize int64) (*HubExtensionReleaseList, error) {
	var allItems []HubExtensionRelease
	var totalCount int
	nextPageKey := ""

	for {
		var result HubExtensionReleaseList
		req := h.client.HTTP().R().SetResult(&result)

		client.PaginationParams{
			Style:         client.PaginationDefault,
			PageKeyParam:  "page-key",
			PageSizeParam: "page-size",
			NextPageKey:   nextPageKey,
			PageSize:      chunkSize,
		}.Apply(req)

		resp, err := req.Get(fmt.Sprintf("/platform/hub/v1/catalog/extensions/%s/releases", url.PathEscape(id)))
		if err != nil {
			return nil, fmt.Errorf("failed to list releases for Hub extension %q: %w", id, err)
		}
		if resp.IsError() {
			return nil, fmt.Errorf("failed to list releases for Hub extension %q: status %d: %s", id, resp.StatusCode(), resp.String())
		}

		allItems = append(allItems, result.Items...)
		totalCount = result.TotalCount

		if chunkSize == 0 || result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &HubExtensionReleaseList{Items: allItems, TotalCount: totalCount}, nil
}
