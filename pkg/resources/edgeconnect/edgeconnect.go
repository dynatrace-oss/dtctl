package edgeconnect

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles EdgeConnect resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new EdgeConnect handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// EdgeConnect represents an EdgeConnect configuration
type EdgeConnect struct {
	ID                         string            `json:"id,omitempty" table:"ID"`
	Name                       string            `json:"name" table:"NAME"`
	HostPatterns               []string          `json:"hostPatterns,omitempty" table:"-"`
	OAuthClientID              string            `json:"oauthClientId,omitempty" table:"-"`
	OAuthClientSecret          string            `json:"oauthClientSecret,omitempty" table:"-"`
	OAuthClientResource        string            `json:"oauthClientResource,omitempty" table:"-"`
	ModificationInfo           *ModificationInfo `json:"modificationInfo,omitempty" table:"-"`
	ManagedByDynatraceOperator bool              `json:"managedByDynatraceOperator,omitempty" table:"MANAGED,wide"`
	Metadata                   *Metadata         `json:"metadata,omitempty" table:"-"`
}

// ModificationInfo contains modification timestamps
type ModificationInfo struct {
	CreatedBy        string `json:"createdBy,omitempty"`
	CreatedTime      string `json:"createdTime,omitempty"`
	LastModifiedBy   string `json:"lastModifiedBy,omitempty"`
	LastModifiedTime string `json:"lastModifiedTime,omitempty"`
}

// Metadata contains additional metadata
type Metadata struct {
	Version string `json:"version,omitempty"`
}

// EdgeConnectList represents a list of EdgeConnects
type EdgeConnectList struct {
	EdgeConnects []EdgeConnect `json:"edgeConnects"`
	TotalCount   int           `json:"totalCount"`
	PageSize     int           `json:"pageSize"`
}

// EdgeConnectCreate represents the request body for creating an EdgeConnect
type EdgeConnectCreate struct {
	Name          string   `json:"name"`
	HostPatterns  []string `json:"hostPatterns,omitempty"`
	OAuthClientID string   `json:"oauthClientId,omitempty"`
}

// List lists all EdgeConnect configurations
func (h *Handler) List() (*EdgeConnectList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "modificationInfo,metadata").
		Get("/platform/app-engine/edge-connect/v1/edge-connects")

	if err != nil {
		return nil, fmt.Errorf("failed to list EdgeConnects: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list EdgeConnects: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result EdgeConnectList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse EdgeConnects response: %w", err)
	}

	return &result, nil
}

// Get gets a specific EdgeConnect by ID
func (h *Handler) Get(edgeConnectID string) (*EdgeConnect, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/app-engine/edge-connect/v1/edge-connects/%s", edgeConnectID))

	if err != nil {
		return nil, fmt.Errorf("failed to get EdgeConnect: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("EdgeConnect %q not found", edgeConnectID)
		default:
			return nil, fmt.Errorf("failed to get EdgeConnect: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result EdgeConnect
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse EdgeConnect response: %w", err)
	}

	return &result, nil
}

// Create creates a new EdgeConnect
func (h *Handler) Create(req EdgeConnectCreate) (*EdgeConnect, error) {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/app-engine/edge-connect/v1/edge-connects")

	if err != nil {
		return nil, fmt.Errorf("failed to create EdgeConnect: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid EdgeConnect configuration: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create EdgeConnect")
		default:
			return nil, fmt.Errorf("failed to create EdgeConnect: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result EdgeConnect
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	return &result, nil
}

// Update updates an existing EdgeConnect
func (h *Handler) Update(edgeConnectID string, req EdgeConnect) error {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Put(fmt.Sprintf("/platform/app-engine/edge-connect/v1/edge-connects/%s", edgeConnectID))

	if err != nil {
		return fmt.Errorf("failed to update EdgeConnect: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("invalid EdgeConnect configuration: %s", resp.String())
		case 404:
			return fmt.Errorf("EdgeConnect %q not found", edgeConnectID)
		default:
			return fmt.Errorf("failed to update EdgeConnect: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Delete deletes an EdgeConnect
func (h *Handler) Delete(edgeConnectID string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/app-engine/edge-connect/v1/edge-connects/%s", edgeConnectID))

	if err != nil {
		return fmt.Errorf("failed to delete EdgeConnect: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("EdgeConnect %q not found", edgeConnectID)
		case 403:
			return fmt.Errorf("access denied to delete EdgeConnect %q", edgeConnectID)
		default:
			return fmt.Errorf("failed to delete EdgeConnect: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetRaw gets an EdgeConnect as raw JSON bytes (for editing)
func (h *Handler) GetRaw(edgeConnectID string) ([]byte, error) {
	ec, err := h.Get(edgeConnectID)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(ec, "", "  ")
}
