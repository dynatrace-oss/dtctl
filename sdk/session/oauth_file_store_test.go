package session

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOAuthFileStore_SetGetDeleteToken(t *testing.T) {
	dir := t.TempDir()
	fs := NewOAuthFileStoreWithDir(dir)

	const name = "oauth:prod:my-token"
	const tokenData = `{"access_token":"abc","refresh_token":"xyz","name":"my-token"}`

	// Set
	if err := fs.SetToken(name, tokenData); err != nil {
		t.Fatalf("SetToken() error: %v", err)
	}

	// Verify file exists with correct permissions (skip on Windows — no Unix perms)
	path := filepath.Join(dir, "oauth__prod__my-token.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("token file not found: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}

	// Get
	got, err := fs.GetToken(name)
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if got != tokenData {
		t.Errorf("GetToken() = %q, want %q", got, tokenData)
	}

	// Delete
	if err := fs.DeleteToken(name); err != nil {
		t.Fatalf("DeleteToken() error: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, got err: %v", err)
	}

	// Get after delete should fail
	_, err = fs.GetToken(name)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestOAuthFileStore_GetToken_NotFound(t *testing.T) {
	dir := t.TempDir()
	fs := NewOAuthFileStoreWithDir(dir)

	_, err := fs.GetToken("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent token, got nil")
	}
	if !containsString(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestOAuthFileStore_DeleteToken_NotFound(t *testing.T) {
	dir := t.TempDir()
	fs := NewOAuthFileStoreWithDir(dir)

	// Deleting a nonexistent token should succeed (no-op)
	if err := fs.DeleteToken("nonexistent"); err != nil {
		t.Errorf("DeleteToken() for nonexistent token should succeed, got: %v", err)
	}
}

func TestOAuthFileStore_Overwrite(t *testing.T) {
	dir := t.TempDir()
	fs := NewOAuthFileStoreWithDir(dir)

	const name = "oauth:prod:overwrite-test"

	// Write initial value
	if err := fs.SetToken(name, `{"version":1}`); err != nil {
		t.Fatalf("SetToken(1) error: %v", err)
	}

	// Overwrite
	if err := fs.SetToken(name, `{"version":2}`); err != nil {
		t.Fatalf("SetToken(2) error: %v", err)
	}

	got, err := fs.GetToken(name)
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if got != `{"version":2}` {
		t.Errorf("GetToken() = %q, want version 2", got)
	}
}

func TestOAuthFileStore_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	fs := NewOAuthFileStoreWithDir(nested)

	if err := fs.SetToken("test-token", `{"data":"test"}`); err != nil {
		t.Fatalf("SetToken() should create nested dirs, got: %v", err)
	}

	got, err := fs.GetToken("test-token")
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if got != `{"data":"test"}` {
		t.Errorf("GetToken() = %q, want test data", got)
	}
}

func TestSanitizeTokenName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"oauth:prod:my-token", "oauth__prod__my-token"},
		{"simple-name", "simple-name"},
		{"oauth:dev:test", "oauth__dev__test"},
		{"no-colons", "no-colons"},
		{"a:b:c:d", "a__b__c__d"},
		{"../../etc/passwd", "passwd"},
		{"foo/../../bar", "bar"},
		{"../escape", "escape"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeTokenName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTokenName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsFileTokenStorage(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   bool
	}{
		{"not set", "", false},
		{"set to file", "file", true},
		{"set to FILE (case insensitive)", "FILE", true},
		{"set to File", "File", true},
		{"set to keyring", "keyring", false},
		{"set to other", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal == "" {
				t.Setenv(EnvTokenStorage, "")
				os.Unsetenv(EnvTokenStorage)
			} else {
				t.Setenv(EnvTokenStorage, tt.envVal)
			}

			got := IsFileTokenStorage()
			if got != tt.want {
				t.Errorf("IsFileTokenStorage() = %v, want %v (env=%q)", got, tt.want, tt.envVal)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
