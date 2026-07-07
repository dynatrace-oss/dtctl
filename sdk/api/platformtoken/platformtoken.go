package platformtoken

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles platform token resources via the Account Management API.
type Handler struct {
	client      *httpclient.Client
	accountUUID string
}

// NewHandler creates a new platform token handler.
func NewHandler(c *httpclient.Client, accountUUID string) *Handler {
	return &Handler{client: c, accountUUID: accountUUID}
}

// PlatformToken represents a Dynatrace platform token.
type PlatformToken struct {
	Name           string   `json:"name"`
	TokenID        string   `json:"tokenId"`
	Token          string   `json:"token,omitempty"` // secret, returned only on create
	AccountUUID    string   `json:"accountUuid,omitempty"`
	Scope          []string `json:"scope"`
	Resource       []string `json:"resource,omitempty"`
	Status         string   `json:"status,omitempty"`
	ExpirationDate string   `json:"expirationDate,omitempty"`
	CreatedAt      string   `json:"createdAt,omitempty"`
	Owner          string   `json:"owner,omitempty"`
}

// PlatformTokenListResponse wraps the list API response.
type PlatformTokenListResponse struct {
	PlatformTokens []PlatformToken `json:"platformTokens"`
	NextPageKey    string          `json:"nextPageKey,omitempty"`
}

// PlatformTokenCreate is the request body for creating a platform token.
type PlatformTokenCreate struct {
	Name           string   `json:"name"`
	UserUUID       string   `json:"userUuid"`
	Scope          []string `json:"scope"`
	Resource       []string `json:"resource"`
	Tags           []string `json:"tags"`
	ExpirationDate string   `json:"expirationDate"`
}

func (h *Handler) basePath() string {
	return fmt.Sprintf("/iam/v1/accounts/%s/platform-tokens", h.accountUUID)
}

// List returns all platform tokens for the account.
func (h *Handler) List(ctx context.Context) (*PlatformTokenListResponse, error) {
	var all []PlatformToken
	var nextPageKey string
	for {
		req := h.client.HTTP().R().SetContext(ctx)
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}
		resp, err := req.Get(h.basePath())
		if err != nil {
			return nil, fmt.Errorf("list platform tokens: %w", err)
		}
		if err := httpclient.CheckResponse(resp); err != nil {
			return nil, fmt.Errorf("list platform tokens: %w", err)
		}
		var page PlatformTokenListResponse
		if err := json.Unmarshal(resp.Body(), &page); err != nil {
			return nil, fmt.Errorf("list platform tokens: parse response: %w", err)
		}
		all = append(all, page.PlatformTokens...)
		if page.NextPageKey == "" {
			break
		}
		nextPageKey = page.NextPageKey
	}
	return &PlatformTokenListResponse{PlatformTokens: all}, nil
}

// Create creates a new platform token.
func (h *Handler) Create(ctx context.Context, req PlatformTokenCreate) (*PlatformToken, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		SetBody(req).
		Post(h.basePath())
	if err != nil {
		return nil, fmt.Errorf("create platform token: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create platform token: %w", err)
	}
	var result PlatformToken
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("create platform token: parse response: %w", err)
	}
	return &result, nil
}

// Revoke deletes (revokes) a platform token by ID.
func (h *Handler) Revoke(ctx context.Context, tokenID string) error {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		Delete(fmt.Sprintf("%s/%s", h.basePath(), tokenID))
	if err != nil {
		return fmt.Errorf("revoke platform token: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("revoke platform token %q: %w", tokenID, err)
	}
	return nil
}
