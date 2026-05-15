package edgeconnect

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles EdgeConnect resources.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new EdgeConnect handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// EdgeConnect represents an EdgeConnect configuration.
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

// ModificationInfo contains modification timestamps.
type ModificationInfo struct {
	CreatedBy        string `json:"createdBy,omitempty"`
	CreatedTime      string `json:"createdTime,omitempty"`
	LastModifiedBy   string `json:"lastModifiedBy,omitempty"`
	LastModifiedTime string `json:"lastModifiedTime,omitempty"`
}

// Metadata contains additional metadata.
type Metadata struct {
	Version string `json:"version,omitempty"`
}

// EdgeConnectList represents a list of EdgeConnects.
type EdgeConnectList struct {
	EdgeConnects []EdgeConnect `json:"edgeConnects"`
	TotalCount   int           `json:"totalCount"`
	PageSize     int           `json:"pageSize"`
}

// List lists all EdgeConnect configurations.
func (h *Handler) List() (*EdgeConnectList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "modificationInfo,metadata").
		Get("/platform/app-engine/edge-connect/v1/edge-connects")
	if err != nil {
		return nil, fmt.Errorf("list edge connects: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list edge connects: %w", err)
	}

	var result EdgeConnectList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list edge connects: parse response: %w", err)
	}

	return &result, nil
}

// Get gets a specific EdgeConnect by ID.
func (h *Handler) Get(edgeConnectID string) (*EdgeConnect, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/app-engine/edge-connect/v1/edge-connects/%s", edgeConnectID))
	if err != nil {
		return nil, fmt.Errorf("get edge connect: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get edge connect %q: %w", edgeConnectID, err)
	}

	var result EdgeConnect
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get edge connect: parse response: %w", err)
	}

	return &result, nil
}

// Create creates a new EdgeConnect.
func (h *Handler) Create(req EdgeConnect) (*EdgeConnect, error) {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/app-engine/edge-connect/v1/edge-connects")
	if err != nil {
		return nil, fmt.Errorf("create edge connect: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create edge connect: %w", err)
	}

	var result EdgeConnect
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("create edge connect: parse response: %w", err)
	}

	return &result, nil
}

// Update updates an existing EdgeConnect.
func (h *Handler) Update(edgeConnectID string, req EdgeConnect) error {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Put(fmt.Sprintf("/platform/app-engine/edge-connect/v1/edge-connects/%s", edgeConnectID))
	if err != nil {
		return fmt.Errorf("update edge connect: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("update edge connect %q: %w", edgeConnectID, err)
	}

	return nil
}

// Delete deletes an EdgeConnect.
func (h *Handler) Delete(edgeConnectID string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/app-engine/edge-connect/v1/edge-connects/%s", edgeConnectID))
	if err != nil {
		return fmt.Errorf("delete edge connect: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete edge connect %q: %w", edgeConnectID, err)
	}

	return nil
}
