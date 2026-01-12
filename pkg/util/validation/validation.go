package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateEditorPath validates that an editor path is safe to execute.
// It checks that:
// - The path is not empty
// - The path does not contain shell metacharacters
// - The executable exists and is executable
// - The path is not a directory
func ValidateEditorPath(editor string) error {
	if editor == "" {
		return fmt.Errorf("editor path is empty")
	}

	// Check for shell metacharacters that could enable command injection
	dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "{", "}", "<", ">", "\n", "\r"}
	for _, char := range dangerousChars {
		if strings.Contains(editor, char) {
			return fmt.Errorf("editor path contains potentially dangerous character %q", char)
		}
	}

	// Split editor into command and arguments (e.g., "code --wait")
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("editor path is empty")
	}

	execPath := parts[0]

	// If it's a bare command name (no path separator), look it up in PATH
	if !strings.Contains(execPath, string(os.PathSeparator)) {
		path, err := lookupExecutable(execPath)
		if err != nil {
			return fmt.Errorf("editor %q not found in PATH: %w", execPath, err)
		}
		execPath = path
	}

	// Validate the executable path
	info, err := os.Stat(execPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("editor %q does not exist", execPath)
		}
		return fmt.Errorf("cannot access editor %q: %w", execPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("editor path %q is a directory, not an executable", execPath)
	}

	// Check if executable (Unix-like systems)
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("editor %q is not executable", execPath)
	}

	return nil
}

// lookupExecutable searches for an executable in PATH
func lookupExecutable(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return "", fmt.Errorf("PATH environment variable is empty")
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return path, nil
		}
	}

	return "", fmt.Errorf("executable not found")
}

// ValidateFilePath validates a file path for safety.
// It checks for:
// - Directory traversal attempts (../)
// - Null bytes
// - Absolute paths when not expected
func ValidateFilePath(path string, allowAbsolute bool) error {
	if path == "" {
		return fmt.Errorf("file path is empty")
	}

	// Check for null bytes (can cause truncation in C-based systems)
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("file path contains null byte")
	}

	// Clean the path to resolve . and ..
	cleanPath := filepath.Clean(path)

	// Check for directory traversal
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("file path contains directory traversal")
	}

	// Check absolute path restriction
	if !allowAbsolute && filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths are not allowed")
	}

	return nil
}

// SanitizeFilename removes or replaces characters that are unsafe in filenames
func SanitizeFilename(name string) string {
	// Characters that are problematic in filenames across platforms
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\x00"}

	result := name
	for _, char := range unsafe {
		result = strings.ReplaceAll(result, char, "_")
	}

	// Trim spaces and dots from beginning and end
	result = strings.Trim(result, " .")

	// Limit length (255 is common max filename length)
	if len(result) > 255 {
		result = result[:255]
	}

	return result
}

// ValidateDQLQuery performs basic validation on a DQL query string.
// This is a lightweight check to catch obvious issues.
func ValidateDQLQuery(query string) error {
	if query == "" {
		return fmt.Errorf("DQL query is empty")
	}

	// Check for null bytes
	if strings.Contains(query, "\x00") {
		return fmt.Errorf("DQL query contains null byte")
	}

	// Basic length check (very long queries are suspicious)
	if len(query) > 100000 {
		return fmt.Errorf("DQL query exceeds maximum length")
	}

	return nil
}
