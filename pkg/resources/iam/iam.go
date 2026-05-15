package iam

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkiam "github.com/dynatrace-oss/dtctl/sdk/api/iam"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	User              = sdkiam.User
	UserListResponse  = sdkiam.UserListResponse
	Group             = sdkiam.Group
	GroupListResponse = sdkiam.GroupListResponse
)

// Handler handles IAM resources.
// It delegates to the SDK handler.
type Handler struct {
	sdk *sdkiam.Handler
}

// NewHandler creates a new IAM handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkiam.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// ListUsers lists all users in the current environment with automatic pagination.
func (h *Handler) ListUsers(partialString string, uuids []string, chunkSize int64) (*UserListResponse, error) {
	return h.sdk.ListUsers(partialString, uuids, chunkSize)
}

// GetUser gets a specific user by UUID.
func (h *Handler) GetUser(uuid string) (*User, error) {
	return h.sdk.GetUser(uuid)
}

// ListGroups lists all groups in the current account with automatic pagination.
func (h *Handler) ListGroups(partialGroupName string, uuids []string, chunkSize int64) (*GroupListResponse, error) {
	return h.sdk.ListGroups(partialGroupName, uuids, chunkSize)
}
