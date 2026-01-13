package validation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateEditorPath(t *testing.T) {
	tests := []struct {
		name    string
		editor  string
		wantErr bool
	}{
		{
			name:    "empty path",
			editor:  "",
			wantErr: true,
		},
		{
			name:    "shell injection semicolon",
			editor:  "vim; rm -rf /",
			wantErr: true,
		},
		{
			name:    "shell injection pipe",
			editor:  "vim | cat",
			wantErr: true,
		},
		{
			name:    "shell injection ampersand",
			editor:  "vim & malicious",
			wantErr: true,
		},
		{
			name:    "shell injection backtick",
			editor:  "vim `whoami`",
			wantErr: true,
		},
		{
			name:    "shell injection dollar",
			editor:  "vim $(whoami)",
			wantErr: true,
		},
		{
			name:    "nonexistent command",
			editor:  "nonexistent-editor-12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEditorPath(tt.editor)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEditorPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEditorPath_ValidEditor(t *testing.T) {
	// Skip on Windows as executable bit checking doesn't work the same way
	if runtime.GOOS == "windows" {
		t.Skip("Skipping executable permission test on Windows")
	}

	// Create a temporary executable for testing
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	execPath := filepath.Join(tmpDir, "test-editor")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\nexit 0"), 0755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	err = ValidateEditorPath(execPath)
	if err != nil {
		t.Errorf("ValidateEditorPath() for valid executable error = %v", err)
	}
}

func TestValidateEditorPath_Directory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = ValidateEditorPath(tmpDir)
	if err == nil {
		t.Error("ValidateEditorPath() should error for directory")
	}
}

func TestValidateEditorPath_NotExecutable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "not-executable")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = ValidateEditorPath(filePath)
	if err == nil {
		t.Error("ValidateEditorPath() should error for non-executable file")
	}
}

func TestValidateFilePath(t *testing.T) {
	// Get a platform-appropriate absolute path for testing
	var absPath string
	if runtime.GOOS == "windows" {
		absPath = "C:\\tmp\\file.txt"
	} else {
		absPath = "/tmp/file.txt"
	}

	tests := []struct {
		name          string
		path          string
		allowAbsolute bool
		wantErr       bool
	}{
		{
			name:          "empty path",
			path:          "",
			allowAbsolute: true,
			wantErr:       true,
		},
		{
			name:          "null byte",
			path:          "file\x00name",
			allowAbsolute: true,
			wantErr:       true,
		},
		{
			name:          "directory traversal",
			path:          "../../../etc/passwd",
			allowAbsolute: true,
			wantErr:       true,
		},
		{
			name:          "absolute path allowed",
			path:          absPath,
			allowAbsolute: true,
			wantErr:       false,
		},
		{
			name:          "absolute path not allowed",
			path:          absPath,
			allowAbsolute: false,
			wantErr:       true,
		},
		{
			name:          "relative path",
			path:          "subdir/file.txt",
			allowAbsolute: false,
			wantErr:       false,
		},
		{
			name:          "simple filename",
			path:          "file.txt",
			allowAbsolute: false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path, tt.allowAbsolute)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean filename",
			input: "document.txt",
			want:  "document.txt",
		},
		{
			name:  "with slashes",
			input: "path/to/file",
			want:  "path_to_file",
		},
		{
			name:  "with special chars",
			input: "file:name*test?.txt",
			want:  "file_name_test_.txt",
		},
		{
			name:  "with quotes",
			input: `file"name`,
			want:  "file_name",
		},
		{
			name:  "leading dots",
			input: "...hidden",
			want:  "hidden",
		},
		{
			name:  "trailing spaces",
			input: "file.txt   ",
			want:  "file.txt",
		},
		{
			name:  "null byte",
			input: "file\x00name",
			want:  "file_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeFilename(tt.input); got != tt.want {
				t.Errorf("SanitizeFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename_LongName(t *testing.T) {
	longName := ""
	for i := 0; i < 300; i++ {
		longName += "a"
	}

	result := SanitizeFilename(longName)
	if len(result) > 255 {
		t.Errorf("SanitizeFilename() length = %d, want <= 255", len(result))
	}
}

func TestValidateDQLQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "valid query",
			query:   "fetch logs | limit 100",
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
		},
		{
			name:    "null byte",
			query:   "fetch logs\x00| limit 100",
			wantErr: true,
		},
		{
			name:    "complex valid query",
			query:   `fetch logs | filter status == "error" | sort timestamp desc | limit 50`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDQLQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDQLQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDQLQuery_TooLong(t *testing.T) {
	longQuery := "fetch logs | filter message == \""
	for i := 0; i < 100001; i++ {
		longQuery += "a"
	}
	longQuery += "\""

	err := ValidateDQLQuery(longQuery)
	if err == nil {
		t.Error("ValidateDQLQuery() should error for very long query")
	}
}

func TestLookupExecutable(t *testing.T) {
	// Test with a command that should exist on most systems
	_, err := lookupExecutable("sh")
	if err != nil {
		// sh might not be in PATH in some environments, skip
		t.Skip("sh not found in PATH")
	}

	// Test with nonexistent command
	_, err = lookupExecutable("nonexistent-command-12345")
	if err == nil {
		t.Error("lookupExecutable() should error for nonexistent command")
	}
}
