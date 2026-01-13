package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
		wantErr bool
	}{
		{
			name:    "valid config",
			baseURL: "https://example.dynatrace.com",
			token:   "dt0c01.token",
			wantErr: false,
		},
		{
			name:    "empty base URL",
			baseURL: "",
			token:   "dt0c01.token",
			wantErr: true,
		},
		{
			name:    "empty token",
			baseURL: "https://example.dynatrace.com",
			token:   "",
			wantErr: true,
		},
		{
			name:    "both empty",
			baseURL: "",
			token:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(tt.baseURL, tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("New() returned nil client without error")
			}
			if !tt.wantErr {
				if client.BaseURL() != tt.baseURL {
					t.Errorf("BaseURL() = %v, want %v", client.BaseURL(), tt.baseURL)
				}
			}
		})
	}
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

	if receivedUA != "dtctl/dev" {
		t.Errorf("User-Agent = %v, want dtctl/dev", receivedUA)
	}
}
