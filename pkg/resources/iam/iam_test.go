package iam

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func TestNewHandler(t *testing.T) {
	c, err := client.New("https://test.dynatrace.com", "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	h := NewHandler(c)

	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.client == nil {
		t.Error("Handler.client is nil")
	}
}

func TestExtractEnvironmentID(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		want        string
		expectError bool
	}{
		{
			name:        "live dynatrace URL",
			baseURL:     "https://abc12345.live.dynatrace.com",
			want:        "abc12345",
			expectError: false,
		},
		{
			name:        "apps dynatrace URL",
			baseURL:     "https://env123.apps.dynatrace.com",
			want:        "env123",
			expectError: false,
		},
		{
			name:        "sprint environment",
			baseURL:     "https://xyz789.sprint.dynatracelabs.com",
			want:        "xyz789",
			expectError: false,
		},
		{
			name:        "URL with port",
			baseURL:     "https://test123.live.dynatrace.com:443",
			want:        "test123",
			expectError: false,
		},
		{
			name:        "URL with path",
			baseURL:     "https://env456.live.dynatrace.com/e/env456",
			want:        "env456",
			expectError: false,
		},
		{
			name:        "invalid URL",
			baseURL:     "://invalid",
			want:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractEnvironmentID(tt.baseURL)

			if (err != nil) != tt.expectError {
				t.Errorf("extractEnvironmentID() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if got != tt.want {
				t.Errorf("extractEnvironmentID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListUsers(t *testing.T) {
	tests := []struct {
		name          string
		partialString string
		uuids         []string
		chunkSize     int64
		pages         []UserListResponse
		expectError   bool
		errorContains string
		validate      func(*testing.T, *UserListResponse)
	}{
		{
			name:          "successful list single page",
			partialString: "",
			uuids:         nil,
			chunkSize:     0,
			pages: []UserListResponse{
				{
					TotalCount: 2,
					Results: []User{
						{
							UID:     "user-1",
							Email:   "user1@example.com",
							Name:    "John",
							Surname: "Doe",
						},
						{
							UID:     "user-2",
							Email:   "user2@example.com",
							Name:    "Jane",
							Surname: "Smith",
						},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *UserListResponse) {
				if len(result.Results) != 2 {
					t.Errorf("expected 2 users, got %d", len(result.Results))
				}
				if result.TotalCount != 2 {
					t.Errorf("expected TotalCount 2, got %d", result.TotalCount)
				}
			},
		},
		{
			name:          "paginated list with chunking",
			partialString: "",
			uuids:         nil,
			chunkSize:     10,
			pages: []UserListResponse{
				{
					TotalCount:  3,
					NextPageKey: "page2",
					Results: []User{
						{UID: "user-1", Email: "user1@example.com"},
						{UID: "user-2", Email: "user2@example.com"},
					},
				},
				{
					TotalCount:  3,
					NextPageKey: "",
					Results: []User{
						{UID: "user-3", Email: "user3@example.com"},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *UserListResponse) {
				if len(result.Results) != 3 {
					t.Errorf("expected 3 users across pages, got %d", len(result.Results))
				}
			},
		},
		{
			name:          "with partial string filter",
			partialString: "john",
			uuids:         nil,
			chunkSize:     0,
			pages: []UserListResponse{
				{
					TotalCount: 1,
					Results: []User{
						{UID: "user-1", Email: "john@example.com", Name: "John"},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *UserListResponse) {
				if len(result.Results) != 1 {
					t.Errorf("expected 1 filtered user, got %d", len(result.Results))
				}
			},
		},
		{
			name:          "with UUID filter",
			partialString: "",
			uuids:         []string{"uuid-1", "uuid-2"},
			chunkSize:     0,
			pages: []UserListResponse{
				{
					TotalCount: 2,
					Results: []User{
						{UID: "uuid-1", Email: "user1@example.com"},
						{UID: "uuid-2", Email: "user2@example.com"},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *UserListResponse) {
				if len(result.Results) != 2 {
					t.Errorf("expected 2 users by UUID, got %d", len(result.Results))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageIndex := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify path format
				if !strings.HasPrefix(r.URL.Path, "/platform/iam/v1/organizational-levels/environment/") {
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/users") {
					t.Errorf("expected path to end with /users, got: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// Verify query parameters
				if tt.partialString != "" {
					partial := r.URL.Query().Get("partialString")
					if partial != tt.partialString {
						t.Errorf("expected partialString %q, got %q", tt.partialString, partial)
					}
				}

				if len(tt.uuids) > 0 {
					uuidParam := r.URL.Query().Get("uuid")
					expectedUUIDs := strings.Join(tt.uuids, ",")
					if uuidParam != expectedUUIDs {
						t.Errorf("expected uuid %q, got %q", expectedUUIDs, uuidParam)
					}
				}

				if tt.chunkSize > 0 {
					pageSize := r.URL.Query().Get("page-size")
					if pageSize == "" {
						t.Error("expected page-size parameter when chunking enabled")
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(tt.pages[pageIndex])

				if pageIndex < len(tt.pages)-1 {
					pageIndex++
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			handler := NewHandler(c)

			result, err := handler.ListUsers(tt.partialString, tt.uuids, tt.chunkSize)

			if (err != nil) != tt.expectError {
				t.Errorf("ListUsers() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if !tt.expectError && tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	tests := []struct {
		name          string
		uuid          string
		statusCode    int
		responseBody  interface{}
		expectError   bool
		errorContains string
	}{
		{
			name:       "successful get",
			uuid:       "user-123",
			statusCode: 200,
			responseBody: User{
				UID:         "user-123",
				Email:       "john@example.com",
				Name:        "John",
				Surname:     "Doe",
				Description: "Test user",
			},
			expectError: false,
		},
		{
			name:          "user not found",
			uuid:          "nonexistent",
			statusCode:    404,
			responseBody:  map[string]string{"error": "Not found"},
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "server error",
			uuid:          "user-error",
			statusCode:    500,
			responseBody:  map[string]string{"error": "Internal error"},
			expectError:   true,
			errorContains: "status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify path contains UUID
				if !strings.Contains(r.URL.Path, tt.uuid) {
					t.Errorf("expected path to contain UUID %q, got: %s", tt.uuid, r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			handler := NewHandler(c)

			result, err := handler.GetUser(tt.uuid)

			if (err != nil) != tt.expectError {
				t.Errorf("GetUser() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if tt.expectError {
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if result == nil {
					t.Fatal("GetUser() returned nil result without error")
				}
				if result.UID != tt.uuid {
					t.Errorf("expected UID %q, got %q", tt.uuid, result.UID)
				}
			}
		})
	}
}

func TestListGroups(t *testing.T) {
	tests := []struct {
		name             string
		partialGroupName string
		uuids            []string
		chunkSize        int64
		pages            []GroupListResponse
		expectError      bool
		validate         func(*testing.T, *GroupListResponse)
	}{
		{
			name:             "successful list",
			partialGroupName: "",
			uuids:            nil,
			chunkSize:        0,
			pages: []GroupListResponse{
				{
					TotalCount: 2,
					Results: []Group{
						{
							UUID:      "group-1",
							GroupName: "Admins",
							Type:      "user",
						},
						{
							UUID:      "group-2",
							GroupName: "Developers",
							Type:      "user",
						},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *GroupListResponse) {
				if len(result.Results) != 2 {
					t.Errorf("expected 2 groups, got %d", len(result.Results))
				}
			},
		},
		{
			name:             "paginated list",
			partialGroupName: "",
			uuids:            nil,
			chunkSize:        10,
			pages: []GroupListResponse{
				{
					TotalCount:  3,
					NextPageKey: "page2",
					Results: []Group{
						{UUID: "group-1", GroupName: "Group 1"},
						{UUID: "group-2", GroupName: "Group 2"},
					},
				},
				{
					TotalCount:  3,
					NextPageKey: "",
					Results: []Group{
						{UUID: "group-3", GroupName: "Group 3"},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *GroupListResponse) {
				if len(result.Results) != 3 {
					t.Errorf("expected 3 groups across pages, got %d", len(result.Results))
				}
			},
		},
		{
			name:             "with partial name filter",
			partialGroupName: "admin",
			uuids:            nil,
			chunkSize:        0,
			pages: []GroupListResponse{
				{
					TotalCount: 1,
					Results: []Group{
						{UUID: "group-1", GroupName: "Admins"},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *GroupListResponse) {
				if len(result.Results) != 1 {
					t.Errorf("expected 1 filtered group, got %d", len(result.Results))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageIndex := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify path format
				if !strings.HasSuffix(r.URL.Path, "/groups") {
					t.Errorf("expected path to end with /groups, got: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// Verify query parameters
				if tt.partialGroupName != "" {
					partial := r.URL.Query().Get("partialGroupName")
					if partial != tt.partialGroupName {
						t.Errorf("expected partialGroupName %q, got %q", tt.partialGroupName, partial)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(tt.pages[pageIndex])

				if pageIndex < len(tt.pages)-1 {
					pageIndex++
				}
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			handler := NewHandler(c)

			result, err := handler.ListGroups(tt.partialGroupName, tt.uuids, tt.chunkSize)

			if (err != nil) != tt.expectError {
				t.Errorf("ListGroups() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if !tt.expectError && tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestListServiceUsers(t *testing.T) {
	tests := []struct {
		name          string
		partialString string
		uuids         []string
		statusCode    int
		responseBody  ServiceUserListResponse
		expectError   bool
		validate      func(*testing.T, *ServiceUserListResponse)
	}{
		{
			name:          "successful list",
			partialString: "",
			uuids:         nil,
			statusCode:    200,
			responseBody: ServiceUserListResponse{
				TotalCount: 2,
				Results: []ServiceUser{
					{
						UID:         "service-1",
						Email:       "service1@example.com",
						Name:        "Service",
						Surname:     "User 1",
						Description: "API service user",
						CreatedAt:   "2024-01-01T00:00:00Z",
					},
					{
						UID:         "service-2",
						Email:       "service2@example.com",
						Name:        "Service",
						Surname:     "User 2",
						Description: "Integration service user",
						CreatedAt:   "2024-01-02T00:00:00Z",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *ServiceUserListResponse) {
				if len(result.Results) != 2 {
					t.Errorf("expected 2 service users, got %d", len(result.Results))
				}
				if result.TotalCount != 2 {
					t.Errorf("expected TotalCount 2, got %d", result.TotalCount)
				}
			},
		},
		{
			name:          "with partial string filter",
			partialString: "service1",
			uuids:         nil,
			statusCode:    200,
			responseBody: ServiceUserListResponse{
				TotalCount: 1,
				Results: []ServiceUser{
					{UID: "service-1", Email: "service1@example.com"},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *ServiceUserListResponse) {
				if len(result.Results) != 1 {
					t.Errorf("expected 1 filtered service user, got %d", len(result.Results))
				}
			},
		},
		{
			name:          "with UUID filter",
			partialString: "",
			uuids:         []string{"uuid-1"},
			statusCode:    200,
			responseBody: ServiceUserListResponse{
				TotalCount: 1,
				Results: []ServiceUser{
					{UID: "uuid-1", Email: "service@example.com"},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *ServiceUserListResponse) {
				if len(result.Results) != 1 {
					t.Errorf("expected 1 service user by UUID, got %d", len(result.Results))
				}
			},
		},
		{
			name:          "empty list",
			partialString: "",
			uuids:         nil,
			statusCode:    200,
			responseBody: ServiceUserListResponse{
				TotalCount: 0,
				Results:    []ServiceUser{},
			},
			expectError: false,
			validate: func(t *testing.T, result *ServiceUserListResponse) {
				if len(result.Results) != 0 {
					t.Errorf("expected 0 service users, got %d", len(result.Results))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify path format
				if !strings.HasSuffix(r.URL.Path, "/service-users") {
					t.Errorf("expected path to end with /service-users, got: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// Verify query parameters
				if tt.partialString != "" {
					partial := r.URL.Query().Get("partialString")
					if partial != tt.partialString {
						t.Errorf("expected partialString %q, got %q", tt.partialString, partial)
					}
				}

				if len(tt.uuids) > 0 {
					uuidParam := r.URL.Query().Get("uuid")
					expectedUUIDs := strings.Join(tt.uuids, ",")
					if uuidParam != expectedUUIDs {
						t.Errorf("expected uuid %q, got %q", expectedUUIDs, uuidParam)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			handler := NewHandler(c)

			result, err := handler.ListServiceUsers(tt.partialString, tt.uuids)

			if (err != nil) != tt.expectError {
				t.Errorf("ListServiceUsers() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if !tt.expectError && tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
