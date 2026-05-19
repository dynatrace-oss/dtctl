package credstore

import (
	"os"
	"runtime"
	"testing"
)

func TestFileStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	// Set
	if err := store.Set("test-cred", "secret-value"); err != nil {
		t.Fatal(err)
	}

	// Get
	val, err := store.Get("test-cred")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret-value" {
		t.Errorf("got %q, want secret-value", val)
	}

	// Delete
	if err := store.Delete("test-cred"); err != nil {
		t.Fatal(err)
	}

	// Get after delete
	_, err = store.Get("test-cred")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestFileStore_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent credential")
	}
}

func TestFileStore_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	// Should not error
	if err := store.Delete("nonexistent"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileStore_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions are not enforced on Windows")
	}

	dir := t.TempDir()
	store := NewFileStore(dir)

	if err := store.Set("perm-test", "value"); err != nil {
		t.Fatal(err)
	}

	// Check file permissions
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		info, _ := e.Info()
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permission = %o, want 0600", perm)
		}
	}
}

func TestFileStore_SanitizeName(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	// Name with colons (keyring-style)
	if err := store.Set("oauth:prod:my-token", "value"); err != nil {
		t.Fatal(err)
	}
	val, err := store.Get("oauth:prod:my-token")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value" {
		t.Errorf("got %q", val)
	}
}
