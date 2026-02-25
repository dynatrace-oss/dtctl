package awsconnection

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

const (
	SchemaID    = "builtin:hyperscaler-authentication.connections.aws"
	SettingsAPI = "/platform/classic/environment-api/v2/settings/objects"
)

type Handler struct {
	client *client.Client
}

func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

type AWSConnection struct {
	ObjectID      string `json:"objectId" table:"ID"`
	SchemaID      string `json:"schemaId,omitempty" table:"SCHEMA,wide"`
	SchemaVersion string `json:"schemaVersion,omitempty" table:"VERSION,wide"`
	Scope         string `json:"scope,omitempty" table:"-"`
	Author        string `json:"author,omitempty" table:"AUTHOR,wide"`
	Created       int64  `json:"created,omitempty" table:"-"`
	Modified      int64  `json:"modified,omitempty" table:"-"`
	Summary       string `json:"summary,omitempty" table:"SUMMARY,wide"`
	Value         Value  `json:"value" table:"-"`

	Name       string `json:"name,omitempty" table:"NAME"`
	Type       string `json:"type,omitempty" table:"TYPE"`
	RoleArn    string `json:"roleArn,omitempty" table:"ROLE_ARN"`
	ExternalID string `json:"externalId,omitempty" table:"EXTERNAL_ID"`
}

type Value struct {
	Name                       string                      `json:"name"`
	Type                       string                      `json:"type"`
	AWSRoleBasedAuthentication *AWSRoleBasedAuthentication `json:"awsRoleBasedAuthentication,omitempty"`
}

type AWSRoleBasedAuthentication struct {
	RoleArn    string   `json:"roleArn"`
	ExternalID string   `json:"externalId,omitempty"`
	Consumers  []string `json:"consumers"`
}

type ListResponse struct {
	Items      []AWSConnection `json:"items"`
	TotalCount int             `json:"totalCount"`
}

type AWSConnectionCreate struct {
	SchemaID      string `json:"schemaId"`
	Scope         string `json:"scope"`
	Value         Value  `json:"value"`
	SchemaVersion string `json:"schemaVersion,omitempty"`
	ExternalID    string `json:"externalId,omitempty"`
}

type CreateResponse struct {
	ObjectID string `json:"objectId"`
	Code     int    `json:"code,omitempty"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func flattenConnection(item *AWSConnection) {
	item.Name = item.Value.Name
	item.Type = item.Value.Type
	if item.Value.AWSRoleBasedAuthentication != nil {
		item.RoleArn = item.Value.AWSRoleBasedAuthentication.RoleArn
		item.ExternalID = item.Value.AWSRoleBasedAuthentication.ExternalID
	}
}

func (h *Handler) Get(id string) (*AWSConnection, error) {
	var result AWSConnection
	req := h.client.HTTP().R().SetResult(&result)
	resp, err := req.Get(fmt.Sprintf("%s/%s", SettingsAPI, id))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to get aws_connection: %s", resp.String())
	}

	flattenConnection(&result)

	return &result, nil
}

func (h *Handler) List() ([]AWSConnection, error) {
	var result ListResponse
	req := h.client.HTTP().R().SetQueryParam("schemaIds", SchemaID)
	req.SetResult(&result)

	resp, err := req.Get(SettingsAPI)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("failed to list aws_connections: %s", resp.String())
	}

	for i := range result.Items {
		flattenConnection(&result.Items[i])
	}

	return result.Items, nil
}

func (h *Handler) Delete(id string) error {
	resp, err := h.client.HTTP().R().Delete(fmt.Sprintf("%s/%s", SettingsAPI, id))
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("failed to delete aws_connection: status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

func (h *Handler) FindByName(name string) (*AWSConnection, error) {
	items, err := h.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Name == name {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("AWS connection with name %q not found", name)
}

func (h *Handler) FindByNameAndType(name, typeVal string) (*AWSConnection, error) {
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

func (h *Handler) Create(req AWSConnectionCreate) (*AWSConnection, error) {
	if req.SchemaID == "" {
		req.SchemaID = SchemaID
	}
	if req.Scope == "" {
		req.Scope = "environment"
	}
	if req.Value.Type == "" {
		req.Value.Type = "awsRoleBasedAuthentication"
	}

	body := []AWSConnectionCreate{req}
	resp, err := h.client.HTTP().R().SetBody(body).Post(SettingsAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws_connection: %w", err)
	}
	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid aws_connection: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create aws_connection")
		case 404:
			return nil, fmt.Errorf("schema %q not found", req.SchemaID)
		case 409:
			return nil, fmt.Errorf("aws_connection already exists or conflicts with existing connection")
		default:
			return nil, fmt.Errorf("failed to create aws_connection: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var createResp []CreateResponse
	if err := json.Unmarshal(resp.Body(), &createResp); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}
	if len(createResp) == 0 {
		return nil, fmt.Errorf("no items returned in create response")
	}
	if createResp[0].Error != nil {
		return nil, fmt.Errorf("create failed: %s", createResp[0].Error.Message)
	}

	return h.Get(createResp[0].ObjectID)
}

func (h *Handler) Update(objectID string, value Value) (*AWSConnection, error) {
	obj, err := h.Get(objectID)
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{"value": value}
	resp, err := h.client.HTTP().R().
		SetBody(body).
		SetHeader("If-Match", obj.SchemaVersion).
		Put(fmt.Sprintf("%s/%s", SettingsAPI, objectID))
	if err != nil {
		return nil, fmt.Errorf("failed to update aws_connection: %w", err)
	}
	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid aws_connection: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to update aws_connection %q", objectID)
		case 404:
			return nil, fmt.Errorf("aws_connection %q not found", objectID)
		case 409, 412:
			return nil, fmt.Errorf("aws_connection version conflict (connection was modified)")
		default:
			return nil, fmt.Errorf("failed to update aws_connection: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return h.Get(objectID)
}
