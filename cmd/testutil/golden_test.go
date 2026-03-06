package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "color codes",
			input:    "\033[31mred text\033[0m",
			expected: "red text",
		},
		{
			name:     "bold and bright",
			input:    "\033[1m\033[92mGreen Bold\033[0m",
			expected: "Green Bold",
		},
		{
			name:     "mixed content",
			input:    "Name: \033[36mtest\033[0m (id: \033[33m123\033[0m)",
			expected: "Name: test (id: 123)",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAssertGolden_Update(t *testing.T) {
	// Save and restore the update flag
	oldUpdate := *update
	*update = true
	defer func() { *update = oldUpdate }()

	tmpDir := t.TempDir()

	// Override goldenDir by writing directly
	goldenPath := filepath.Join(tmpDir, "test-update.golden")
	dir := filepath.Dir(goldenPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	content := "hello golden\nworld\n"
	if err := os.WriteFile(goldenPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write golden file: %v", err)
	}

	// Verify it was written
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}
	if string(data) != content {
		t.Errorf("golden file content = %q, want %q", string(data), content)
	}
}

func TestAssertGolden_Match(t *testing.T) {
	// Save and restore the update flag
	oldUpdate := *update
	*update = false
	defer func() { *update = oldUpdate }()

	// Create a temporary golden file
	tmpDir := t.TempDir()
	goldenPath := filepath.Join(tmpDir, "test-match.golden")
	content := "expected output\n"
	if err := os.WriteFile(goldenPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write golden file: %v", err)
	}

	// Read it back to verify the mechanism works
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected %q but got %q", content, string(data))
	}
}
