package azureconnection

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

const (
	SchemaID    = "builtin:hyperscaler-authentication.connections.azure"
	SettingsAPI = "/platform/classic/environment-api/v2/settings/objects"
)

type Handler struct {
	client *client.Client
}

func NewHandler(c *client.Client) *Handler {
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

	// Flattened fields for table view
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
	ClientSecret  string   `json:"clientSecret,omitempty"` // Often masked in responses
	Consumers     []string `json:"consumers"`
}

type FederatedIdentityCredential struct {
	Consumers []string `json:"consumers"`
}

func (v Value) String() string {
	s := fmt.Sprintf("name=%s type=%s", v.Name, v.Type)

	if v.ClientSecret != nil {
		// Mask the secret to prevent leaking it in terminal/logs
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
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to get azure_connection: %s", resp.String())
	}

	// Populate flattened fields
	result.Name = result.Value.Name
	result.Type = result.Value.Type

	return &result, nil
}

func (h *Handler) List() ([]AzureConnection, error) {
	var allItems []AzureConnection

	// Settings API usually supports pagination, but for simplicity we start with basic fetch used in scripts
	// However, we should filter by schemaId
	req := h.client.HTTP().R().SetQueryParam("schemaIds", SchemaID)

	var result ListResponse
	req.SetResult(&result)

	resp, err := req.Get(SettingsAPI)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to list azure_connections: %s", resp.String())
	}

	for i := range result.Items {
		result.Items[i].Name = result.Items[i].Value.Name
		result.Items[i].Type = result.Items[i].Value.Type
	}
	allItems = append(allItems, result.Items...)

	return allItems, nil
}

// Delete deletes an Azure connection by ID
func (h *Handler) Delete(id string) error {
	resp, err := h.client.HTTP().R().Delete(fmt.Sprintf("%s/%s", SettingsAPI, id))
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("failed to delete azure_connection: status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

// FindByName finds an Azure connection by name
func (h *Handler) FindByName(name string) (*AzureConnection, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Name == name {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("Azure connection with name %q not found", name)
}
