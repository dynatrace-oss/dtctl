package iam

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles IAM resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new IAM handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// User represents a Dynatrace user
type User struct {
	UID         string `json:"uid" table:"UID"`
	Email       string `json:"email" table:"EMAIL"`
	Name        string `json:"name,omitempty" table:"NAME"`
	Surname     string `json:"surname,omitempty" table:"SURNAME"`
	Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
}

// UserListResponse represents a list of users
type UserListResponse struct {
	Results     []User `json:"results"`
	NextPageKey string `json:"nextPageKey,omitempty"`
	TotalCount  int64  `json:"totalCount"`
}

// Group represents a Dynatrace group
type Group struct {
	UUID      string `json:"uuid" table:"UUID"`
	GroupName string `json:"groupName" table:"NAME"`
	Type      string `json:"type" table:"TYPE"`
}

// GroupListResponse represents a list of groups
type GroupListResponse struct {
	Results     []Group `json:"results"`
	NextPageKey string  `json:"nextPageKey,omitempty"`
	TotalCount  int64   `json:"totalCount"`
}

// ServiceUser represents a Dynatrace service user
type ServiceUser struct {
	UID         string `json:"uid" table:"UID"`
	Email       string `json:"email" table:"EMAIL"`
	Name        string `json:"name,omitempty" table:"NAME"`
	Surname     string `json:"surname,omitempty" table:"SURNAME"`
	Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
	CreatedAt   string `json:"createdAt,omitempty" table:"CREATED_AT,wide"`
}

// ServiceUserListResponse represents a list of service users
type ServiceUserListResponse struct {
	Results     []ServiceUser `json:"results"`
	NextPageKey string        `json:"nextPageKey,omitempty"`
	TotalCount  int64         `json:"totalCount"`
}

// extractEnvironmentID extracts the environment ID from the base URL
func extractEnvironmentID(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	// Extract the subdomain as the environment ID
	// E.g., abc12345.live.dynatrace.com -> abc12345
	// Or abc12345.apps.dynatrace.com -> abc12345
	hostname := u.Hostname()
	parts := strings.Split(hostname, ".")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid hostname format: %s", hostname)
	}

	return parts[0], nil
}

// ListUsers lists all users in the current environment with automatic pagination
func (h *Handler) ListUsers(partialString string, uuids []string, chunkSize int64) (*UserListResponse, error) {
	envID, err := extractEnvironmentID(h.client.HTTP().BaseURL)
	if err != nil {
		return nil, err
	}

	var allUsers []User
	var totalCount int64
	nextPageKey := ""

	for {
		var result UserListResponse
		req := h.client.HTTP().R().SetResult(&result)

		// Add query parameters
		if partialString != "" {
			req.SetQueryParam("partialString", partialString)
		}
		if len(uuids) > 0 {
			req.SetQueryParam("uuid", strings.Join(uuids, ","))
		}

		// Set page size if chunking is enabled (chunkSize > 0)
		if chunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		resp, err := req.Get(fmt.Sprintf("/platform/iam/v1/organizational-levels/environment/%s/users", envID))
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list users: status %d: %s", resp.StatusCode(), resp.String())
		}

		allUsers = append(allUsers, result.Results...)
		totalCount = result.TotalCount

		// If chunking is disabled (chunkSize == 0), return first page only
		if chunkSize == 0 {
			return &result, nil
		}

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &UserListResponse{
		Results:    allUsers,
		TotalCount: totalCount,
	}, nil
}

// GetUser gets a specific user by UUID
func (h *Handler) GetUser(uuid string) (*User, error) {
	envID, err := extractEnvironmentID(h.client.HTTP().BaseURL)
	if err != nil {
		return nil, err
	}

	var result User

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/iam/v1/organizational-levels/environment/%s/users/%s", envID, uuid))

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("user %q not found", uuid)
		default:
			return nil, fmt.Errorf("failed to get user: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// ListGroups lists all groups in the current account with automatic pagination
func (h *Handler) ListGroups(partialGroupName string, uuids []string, chunkSize int64) (*GroupListResponse, error) {
	envID, err := extractEnvironmentID(h.client.HTTP().BaseURL)
	if err != nil {
		return nil, err
	}

	var allGroups []Group
	var totalCount int64
	nextPageKey := ""

	for {
		var result GroupListResponse
		req := h.client.HTTP().R().SetResult(&result)

		// Add query parameters
		if partialGroupName != "" {
			req.SetQueryParam("partialGroupName", partialGroupName)
		}
		if len(uuids) > 0 {
			req.SetQueryParam("uuid", strings.Join(uuids, ","))
		}

		// Set page size if chunking is enabled (chunkSize > 0)
		if chunkSize > 0 {
			req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
		}

		// Set page key for subsequent requests
		if nextPageKey != "" {
			req.SetQueryParam("page-key", nextPageKey)
		}

		// Note: Groups are at the account level, but we use environment for now
		// This might need adjustment based on the actual API requirements
		resp, err := req.Get(fmt.Sprintf("/platform/iam/v1/organizational-levels/environment/%s/groups", envID))
		if err != nil {
			return nil, fmt.Errorf("failed to list groups: %w", err)
		}

		if resp.IsError() {
			return nil, fmt.Errorf("failed to list groups: status %d: %s", resp.StatusCode(), resp.String())
		}

		allGroups = append(allGroups, result.Results...)
		totalCount = result.TotalCount

		// If chunking is disabled (chunkSize == 0), return first page only
		if chunkSize == 0 {
			return &result, nil
		}

		// Check if there are more pages
		if result.NextPageKey == "" {
			break
		}
		nextPageKey = result.NextPageKey
	}

	return &GroupListResponse{
		Results:    allGroups,
		TotalCount: totalCount,
	}, nil
}

// ListServiceUsers lists all service users in the current environment
func (h *Handler) ListServiceUsers(partialString string, uuids []string) (*ServiceUserListResponse, error) {
	envID, err := extractEnvironmentID(h.client.HTTP().BaseURL)
	if err != nil {
		return nil, err
	}

	var result ServiceUserListResponse

	req := h.client.HTTP().R().SetResult(&result)

	// Add query parameters
	if partialString != "" {
		req.SetQueryParam("partialString", partialString)
	}
	if len(uuids) > 0 {
		req.SetQueryParam("uuid", strings.Join(uuids, ","))
	}

	resp, err := req.Get(fmt.Sprintf("/platform/iam/v1/organizational-levels/environment/%s/service-users", envID))

	if err != nil {
		return nil, fmt.Errorf("failed to list service users: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list service users: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}
