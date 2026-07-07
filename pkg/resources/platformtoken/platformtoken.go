package platformtoken

import (
	"context"
	"strings"

	sdkpt "github.com/dynatrace-oss/dtctl/sdk/api/platformtoken"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK create type for use in cmd layer.
type PlatformTokenCreate = sdkpt.PlatformTokenCreate

// PlatformToken is the CLI output struct with table tags.
type PlatformToken struct {
	Name           string `json:"name"            table:"NAME"`
	TokenID        string `json:"tokenId"         table:"TOKEN-ID"`
	Status         string `json:"status"          table:"STATUS"`
	ExpirationDate string `json:"expirationDate"  table:"EXPIRES"`
	Scope          string `json:"scope"           table:"SCOPE,wide"`
	Token          string `json:"token,omitempty" table:"-"` // never shown in table
}

// Handler wraps the SDK handler with CLI-specific output conversion.
type Handler struct {
	sdk *sdkpt.Handler
}

// NewHandler creates a CLI platform token handler.
func NewHandler(accountClient *httpclient.Client, accountUUID string) *Handler {
	return &Handler{sdk: sdkpt.NewHandler(accountClient, accountUUID)}
}

func fromSDK(s *sdkpt.PlatformToken) PlatformToken {
	return PlatformToken{
		Name:           s.Name,
		TokenID:        s.TokenID,
		Status:         s.Status,
		ExpirationDate: s.ExpirationDate,
		Scope:          strings.Join(s.Scope, " "),
		Token:          s.Token,
	}
}

// List returns all platform tokens.
func (h *Handler) List() ([]PlatformToken, error) {
	res, err := h.sdk.List(context.Background())
	if err != nil {
		return nil, err
	}
	tokens := make([]PlatformToken, len(res.PlatformTokens))
	for i := range res.PlatformTokens {
		tokens[i] = fromSDK(&res.PlatformTokens[i])
	}
	return tokens, nil
}

// Create creates a new platform token.
func (h *Handler) Create(req PlatformTokenCreate) (*PlatformToken, error) {
	res, err := h.sdk.Create(context.Background(), req)
	if err != nil {
		return nil, err
	}
	t := fromSDK(res)
	return &t, nil
}

// Revoke revokes (deletes) a platform token by ID.
func (h *Handler) Revoke(tokenID string) error {
	return h.sdk.Revoke(context.Background(), tokenID)
}
