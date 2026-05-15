package credstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileMode = 0600
	dirMode  = 0700
)

// FileStore stores credentials as individual files in a directory.
// Files are created with 0600 permissions (owner-only read/write).
type FileStore struct {
	dir string
}

// NewFileStore creates a file-based credential store at the given directory.
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

// Get reads a credential from a file.
func (s *FileStore) Get(name string) (string, error) {
	path := filepath.Join(s.dir, sanitizeName(name))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("credential %q not found in file store", name)
		}
		return "", fmt.Errorf("failed to read credential file: %w", err)
	}
	return string(data), nil
}

// Set writes a credential to a file.
func (s *FileStore) Set(name, value string) error {
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return fmt.Errorf("failed to create credential directory: %w", err)
	}
	path := filepath.Join(s.dir, sanitizeName(name))
	if err := os.WriteFile(path, []byte(value), fileMode); err != nil {
		return fmt.Errorf("failed to write credential file: %w", err)
	}
	return nil
}

// Delete removes a credential file.
func (s *FileStore) Delete(name string) error {
	path := filepath.Join(s.dir, sanitizeName(name))
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete credential file: %w", err)
	}
	return nil
}

// sanitizeName converts a credential name to a safe filename.
func sanitizeName(name string) string {
	safe := strings.ReplaceAll(name, ":", "__")
	return filepath.Base(safe) + ".json"
}
