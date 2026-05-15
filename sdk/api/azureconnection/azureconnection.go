package azureconnection

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const (
	SchemaID    = "builtin:hyperscaler-authentication.connections.azure"
	SettingsAPI = "/platform/classic/environment-api/v2/settings/objects"
)

type Handler struct {
	client *httpclient.Client
}

func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

type AzureConnection struct {
	ObjectID      string `json:"objectId" table:"ID"`
	SchemaID      string `json:"schemaId,omitempty" table:"SCHEMA,wide"`
	SchemaVersion string `json:"schemaVersion,omitempty" table:"VERSION,wide"`
	Scope         string `json:"scope,omitempty" table:"-"`
	Author        string `json:"author,omitempty" table:"AUTHOR,wide"`
	Created       int64  `json:"created,omitempty" table:"-"`
	Modified      int64  `json:"modified,omitempty" table:"-"`
	Summary       string `json:"summary,omitempty" table:"SUMMARY,wide"`
	Value         Value  `json:"value" table:"-"`

	Name string `json:"name,omitempty" table:"NAME"`
	Type string `json:"type,omitempty" table:"TYPE"`
}

type Value struct {
	Name                        string                       `json:"name"`
	Type                        string                       `json:"type"`
	ClientSecret                *ClientSecretCredential      `json:"clientSecret,omitempty"`
	FederatedIdentityCredential *FederatedIdentityCredential `json:"federatedIdentityCredential,omitempty"`
}

type ClientSecretCredential struct {
	ApplicationID string   `json:"applicationId"`
	DirectoryID   string   `json:"directoryId"`
	ClientSecret  string   `json:"clientSecret,omitempty"`
	Consumers     []string `json:"consumers"`
}

type FederatedIdentityCredential struct {
	DirectoryID   string   `json:"directoryId,omitempty"`
	ApplicationID string   `json:"applicationId,omitempty"`
	Consumers     []string `json:"consumers"`
}

func (v Value) String() string {
	s := fmt.Sprintf("name=%s type=%s", v.Name, v.Type)

	if v.ClientSecret != nil {
		secret := ""
		if v.ClientSecret.ClientSecret != "" {
			secret = "[REDACTED]"
		}
		s += fmt.Sprintf(" dirId=%s appId=%s secret=%s consumers=%v",
			v.ClientSecret.DirectoryID,
			v.ClientSecret.ApplicationID,
			secret,
			v.ClientSecret.Consumers)
	}

	if v.FederatedIdentityCredential != nil {
		s += fmt.Sprintf(" consumers=%v", v.FederatedIdentityCredential.Consumers)
	}

	return s
}

type ListResponse struct {
	Items      []AzureConnection `json:"items"`
	TotalCount int               `json:"totalCount"`
}

func (h *Handler) Get(id string) (*AzureConnection, error) {
	var result AzureConnection
	req := h.client.HTTP().R().SetResult(&result)
	resp, err := req.Get(fmt.Sprintf("%s/%s", SettingsAPI, id))
	if err != nil {
		return nil, fmt.Errorf("get azure connection %q: %w", id, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get azure connection %q: %w", id, err)
	}

	result.Name = result.Value.Name
	result.Type = result.Value.Type

	return &result, nil
}

func (h *Handler) List() ([]AzureConnection, error) {
	req := h.client.HTTP().R().SetQueryParam("schemaIds", SchemaID)

	var result ListResponse
	req.SetResult(&result)

	resp, err := req.Get(SettingsAPI)
	if err != nil {
		return nil, fmt.Errorf("list azure connections: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list azure connections: %w", err)
	}

	for i := range result.Items {
		result.Items[i].Name = result.Items[i].Value.Name
		result.Items[i].Type = result.Items[i].Value.Type
	}

	return result.Items, nil
}

// Delete deletes an Azure connection by ID
func (h *Handler) Delete(id string) error {
	resp, err := h.client.HTTP().R().Delete(fmt.Sprintf("%s/%s", SettingsAPI, id))
	if err != nil {
		return fmt.Errorf("delete azure connection %q: %w", id, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete azure connection %q: %w", id, err)
	}
	return nil
}

// FindByName finds an Azure connection by name
func (h *Handler) FindByName(name string) (*AzureConnection, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Name == name {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("azure connection with name %q not found", name)
}

// FindByNameAndType finds an Azure connection by name and type
func (h *Handler) FindByNameAndType(name, typeVal string) (*AzureConnection, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Name == name && items[i].Type == typeVal {
			return &items[i], nil
		}
	}
	return nil, nil
}

// AzureConnectionCreate represents the request body for creating an Azure connection
type AzureConnectionCreate struct {
	SchemaID      string `json:"schemaId"`
	Scope         string `json:"scope"`
	Value         Value  `json:"value"`
	SchemaVersion string `json:"schemaVersion,omitempty"`
	ExternalID    string `json:"externalId,omitempty"`
}

// CreateResponse represents the response from creating an Azure connection
type CreateResponse struct {
	ObjectID string `json:"objectId"`
	Code     int    `json:"code,omitempty"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Create creates a new Azure connection
func (h *Handler) Create(req AzureConnectionCreate) (*AzureConnection, error) {
	if req.SchemaID == "" {
		req.SchemaID = SchemaID
	}
	if req.Scope == "" {
		req.Scope = "environment"
	}

	body := []AzureConnectionCreate{req}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		Post(SettingsAPI)

	if err != nil {
		return nil, fmt.Errorf("create azure connection: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create azure connection: %w", err)
	}

	var createResp []CreateResponse
	if err := json.Unmarshal(resp.Body(), &createResp); err != nil {
		return nil, fmt.Errorf("parse create response: %w", err)
	}

	if len(createResp) == 0 {
		return nil, fmt.Errorf("no items returned in create response")
	}

	result := &createResp[0]
	if result.Error != nil {
		return nil, fmt.Errorf("create failed: %s", result.Error.Message)
	}

	return h.Get(result.ObjectID)
}

// Update updates an existing Azure connection
func (h *Handler) Update(objectID string, value Value) (*AzureConnection, error) {
	obj, err := h.Get(objectID)
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"value": value,
	}

	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", obj.SchemaVersion).
		Put(fmt.Sprintf("%s/%s", SettingsAPI, objectID))

	if err != nil {
		return nil, fmt.Errorf("update azure connection %q: %w", objectID, err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("update azure connection %q: %w", objectID, err)
	}

	return h.Get(objectID)
}
