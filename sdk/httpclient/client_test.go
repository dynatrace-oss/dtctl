package httpclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_RequiresBaseURL(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Error("expected error for empty base URL")
	}
}

func TestNew_WithToken(t *testing.T) {
	c, err := New("https://example.com", WithToken("dt0c01.test"))
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL() != "https://example.com" {
		t.Errorf("base URL = %q", c.BaseURL())
	}
}

func TestClient_SetToken(t *testing.T) {
	c, err := New("https://example.com", WithToken("initial"))
	if err != nil {
		t.Fatal(err)
	}
	c.SetToken("dt0c01.new-token")
	// Verify the client still works
	if c.BaseURL() != "https://example.com" {
		t.Error("base URL changed unexpectedly")
	}
}

func TestClient_HTTP(t *testing.T) {
	c, err := New("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTP() == nil {
		t.Error("HTTP() returned nil")
	}
}

func TestClient_Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithToken("test-token"), WithRetry(3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.HTTP().R().Get("/test")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode())
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClient_RetryResendsMultipartBody(t *testing.T) {
	const content = `{"tiles":{"0":{"type":"markdown"}}}`

	attempts := 0
	received := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		file, _, err := r.FormFile("content")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("ReadAll: %v", err)
		}
		received = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithToken("test-token"), WithRetry(3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.HTTP().R().
		SetMultipartField("content", "content.json", "application/json", bytes.NewReader([]byte(content))).
		Patch("/test")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode())
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if received != content {
		t.Errorf("content = %q, want %q", received, content)
	}
}
