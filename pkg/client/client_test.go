package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/version"
	"github.com/sirupsen/logrus"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		token       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid config",
			baseURL: "https://example.dynatrace.com",
			token:   "dt0c01.token",
			wantErr: false,
		},
		{
			name:        "empty base URL",
			baseURL:     "",
			token:       "dt0c01.token",
			wantErr:     true,
			errContains: "base URL is required",
		},
		{
			name:        "empty token",
			baseURL:     "https://example.dynatrace.com",
			token:       "",
			wantErr:     true,
			errContains: "token is required",
		},
		{
			name:        "both empty",
			baseURL:     "",
			token:       "",
			wantErr:     true,
			errContains: "base URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Enable parallel execution
			client, err := New(tt.baseURL, tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("New() error = %v, want error containing %q", err, tt.errContains)
				}
			}
			if !tt.wantErr && client == nil {
				t.Error("New() returned nil client without error")
			}
			if !tt.wantErr {
				if client.BaseURL() != tt.baseURL {
					t.Errorf("BaseURL() = %v, want %v", client.BaseURL(), tt.baseURL)
				}
				// Verify client is properly configured
				if client.HTTP() == nil {
					t.Error("HTTP client not initialized")
				}
				if client.Logger() == nil {
					t.Error("Logger not initialized")
				}
			}
		})
	}
}

// contains checks if a string contains a substring (helper for error checking)
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestClient_HTTP(t *testing.T) {
	client, err := New("https://example.dynatrace.com", "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	httpClient := client.HTTP()
	if httpClient == nil {
		t.Error("HTTP() returned nil")
	}
}

func TestClient_SetVerbosity(t *testing.T) {
	client, err := New("https://example.dynatrace.com", "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test various verbosity levels - should not panic
	client.SetVerbosity(0)
	client.SetVerbosity(1)
	client.SetVerbosity(2)
}

func TestClient_Logger(t *testing.T) {
	client, err := New("https://example.dynatrace.com", "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger := client.Logger()
	if logger == nil {
		t.Error("Logger() returned nil")
	}
}

func TestIsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Make a request to get a response object for testing
	resp, err := client.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Test with successful response - should not retry
	if isRetryable(resp, nil) {
		t.Error("isRetryable() should return false for 200 response")
	}

	// Test with error - should retry
	if !isRetryable(nil, http.ErrServerClosed) {
		t.Error("isRetryable() should return true for error")
	}
}

func TestClient_CurrentUser(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		wantErr    bool
	}{
		{
			name:       "successful response",
			statusCode: http.StatusOK,
			response: UserInfo{
				UserName:     "test.user",
				UserID:       "user-123",
				EmailAddress: "test@example.com",
			},
			wantErr: false,
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			response:   map[string]string{"error": "unauthorized"},
			wantErr:    true,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			response:   map[string]string{"error": "internal error"},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/platform/metadata/v1/user" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			client, err := New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			// Disable retries for faster tests
			client.HTTP().SetRetryCount(0)

			userInfo, err := client.CurrentUser()
			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if userInfo.UserID != "user-123" {
					t.Errorf("CurrentUser() UserID = %v, want user-123", userInfo.UserID)
				}
			}
		})
	}
}

func TestExtractUserIDFromToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid JWT with sub claim",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTEyMyIsIm5hbWUiOiJUZXN0IFVzZXIifQ.signature",
			want:    "user-123",
			wantErr: false,
		},
		{
			name:    "invalid JWT format - too few parts",
			token:   "invalid.token",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid JWT format - not base64",
			token:   "header.!!!invalid!!!.signature",
			want:    "",
			wantErr: true,
		},
		{
			name:    "JWT without sub claim",
			token:   "eyJhbGciOiJIUzI1NiJ9.eyJuYW1lIjoiVGVzdCJ9.signature",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractUserIDFromToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractUserIDFromToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractUserIDFromToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_RetryBehavior(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// Configure faster retries for testing
	client.HTTP().SetRetryWaitTime(10 * time.Millisecond)
	client.HTTP().SetRetryMaxWaitTime(50 * time.Millisecond)

	resp, err := client.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200 after retries, got %d", resp.StatusCode())
	}

	if requestCount < 3 {
		t.Errorf("Expected at least 3 requests (with retries), got %d", requestCount)
	}
}

func TestClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// Set very short timeout
	client.HTTP().SetTimeout(10 * time.Millisecond)
	client.HTTP().SetRetryCount(0)

	_, err = client.HTTP().R().Get("/test")
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestClient_AuthHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	token := "my-secret-token"
	client, err := New(server.URL, token)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	expectedAuth := "Bearer " + token
	if receivedAuth != expectedAuth {
		t.Errorf("Authorization header = %v, want %v", receivedAuth, expectedAuth)
	}
}

func TestClient_UserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	expectedUA := fmt.Sprintf("dtctl/%s", version.Version)
	if receivedUA != expectedUA {
		t.Errorf("User-Agent = %v, want %v", receivedUA, expectedUA)
	}
}

func TestClient_SetLogger(t *testing.T) {
	t.Parallel()

	client, err := New("https://example.dynatrace.com", "test-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	customLogger := &logrus.Logger{}
	client.SetLogger(customLogger)

	if client.Logger() != customLogger {
		t.Error("SetLogger() did not set the custom logger")
	}
}

func TestClient_BaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{
			name:    "standard dynatrace URL",
			baseURL: "https://example.dynatrace.com",
		},
		{
			name:    "managed URL",
			baseURL: "https://managed.example.com/e/environment-id",
		},
		{
			name:    "localhost",
			baseURL: "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, err := New(tt.baseURL, "test-token")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if client.BaseURL() != tt.baseURL {
				t.Errorf("BaseURL() = %v, want %v", client.BaseURL(), tt.baseURL)
			}
		})
	}
}

func TestIsSensitiveHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{
			name:   "authorization header",
			header: "Authorization",
			want:   true,
		},
		{
			name:   "authorization lowercase",
			header: "authorization",
			want:   true,
		},
		{
			name:   "x-api-key",
			header: "X-API-Key",
			want:   true,
		},
		{
			name:   "cookie",
			header: "Cookie",
			want:   true,
		},
		{
			name:   "set-cookie",
			header: "Set-Cookie",
			want:   true,
		},
		{
			name:   "content-type not sensitive",
			header: "Content-Type",
			want:   false,
		},
		{
			name:   "user-agent not sensitive",
			header: "User-Agent",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isSensitiveHeader(tt.header)
			if got != tt.want {
				t.Errorf("isSensitiveHeader(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestClient_SetVerbosityLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level int
	}{
		{
			name:  "level 0 - no debug",
			level: 0,
		},
		{
			name:  "level 1 - summary",
			level: 1,
		},
		{
			name:  "level 2 - full details",
			level: 2,
		},
		{
			name:  "level 3 - verbose",
			level: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, err := New("https://example.dynatrace.com", "test-token")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			// Should not panic
			client.SetVerbosity(tt.level)
		})
	}
}

func TestClient_CurrentUserID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apiSuccess  bool
		apiResponse UserInfo
		apiStatus   int
		token       string
		wantID      string
		wantErr     bool
		errContains string
	}{
		{
			name:       "successful API call",
			apiSuccess: true,
			apiResponse: UserInfo{
				UserID:       "api-user-123",
				UserName:     "test.user",
				EmailAddress: "test@example.com",
			},
			apiStatus: http.StatusOK,
			token:     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0b2tlbi11c2VyLTQ1NiJ9.signature",
			wantID:    "api-user-123",
			wantErr:   false,
		},
		{
			name:       "API fails, fallback to JWT",
			apiSuccess: false,
			apiStatus:  http.StatusUnauthorized,
			token:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0b2tlbi11c2VyLTQ1NiJ9.signature",
			wantID:     "token-user-456",
			wantErr:    false,
		},
		{
			name:        "API fails and invalid JWT",
			apiSuccess:  false,
			apiStatus:   http.StatusUnauthorized,
			token:       "invalid-token",
			wantErr:     true,
			errContains: "invalid JWT token format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.apiStatus)
				if tt.apiSuccess {
					_ = json.NewEncoder(w).Encode(tt.apiResponse)
				} else {
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				}
			}))
			defer server.Close()

			client, err := New(server.URL, tt.token)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			client.HTTP().SetRetryCount(0)

			userID, err := client.CurrentUserID()
			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentUserID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("CurrentUserID() error = %v, want error containing %q", err, tt.errContains)
				}
			}
			if !tt.wantErr && userID != tt.wantID {
				t.Errorf("CurrentUserID() = %v, want %v", userID, tt.wantID)
			}
		})
	}
}

func TestIsRetryable_ContextDeadline(t *testing.T) {
	t.Parallel()

	// Test that context deadline errors are NOT retried
	err := context.DeadlineExceeded
	if isRetryable(nil, err) {
		t.Error("isRetryable() should return false for context.DeadlineExceeded")
	}
}

func TestIsRetryable_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{
			name:       "200 OK - no retry",
			statusCode: http.StatusOK,
			want:       false,
		},
		{
			name:       "400 Bad Request - no retry",
			statusCode: http.StatusBadRequest,
			want:       false,
		},
		{
			name:       "404 Not Found - no retry",
			statusCode: http.StatusNotFound,
			want:       false,
		},
		{
			name:       "429 Rate Limit - retry",
			statusCode: http.StatusTooManyRequests,
			want:       true,
		},
		{
			name:       "500 Internal Server Error - retry",
			statusCode: http.StatusInternalServerError,
			want:       true,
		},
		{
			name:       "502 Bad Gateway - retry",
			statusCode: http.StatusBadGateway,
			want:       true,
		},
		{
			name:       "503 Service Unavailable - retry",
			statusCode: http.StatusServiceUnavailable,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client, err := New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			client.HTTP().SetRetryCount(0)

			resp, _ := client.HTTP().R().Get("/test")
			got := isRetryable(resp, nil)
			if got != tt.want {
				t.Errorf("isRetryable() with status %d = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}
