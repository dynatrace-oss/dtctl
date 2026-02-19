package prompt

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// simulateInput simulates user input by replacing os.Stdin
func simulateInput(input string) func() {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r

	// Write input in a goroutine to avoid blocking
	go func() {
		defer w.Close()
		io.WriteString(w, input)
	}()

	// Return cleanup function
	return func() {
		os.Stdin = oldStdin
	}
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "confirm with y",
			input:    "y\n",
			expected: true,
		},
		{
			name:     "confirm with yes",
			input:    "yes\n",
			expected: true,
		},
		{
			name:     "confirm with Y uppercase",
			input:    "Y\n",
			expected: true,
		},
		{
			name:     "confirm with YES uppercase",
			input:    "YES\n",
			expected: true,
		},
		{
			name:     "deny with n",
			input:    "n\n",
			expected: false,
		},
		{
			name:     "deny with no",
			input:    "no\n",
			expected: false,
		},
		{
			name:     "deny with empty input",
			input:    "\n",
			expected: false,
		},
		{
			name:     "deny with invalid input",
			input:    "maybe\n",
			expected: false,
		},
		{
			name:     "deny with whitespace",
			input:    "  \n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := simulateInput(tt.input)
			defer cleanup()

			// Capture stdout to avoid cluttering test output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			result := Confirm("Test message")

			w.Close()
			os.Stdout = oldStdout
			// Discard captured output
			io.Copy(io.Discard, r)

			if result != tt.expected {
				t.Errorf("Confirm() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConfirmDeletion(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		resourceName string
		resourceID   string
		input        string
		expected     bool
	}{
		{
			name:         "confirm deletion with yes",
			resourceType: "workflow",
			resourceName: "test-workflow",
			resourceID:   "wf-123",
			input:        "yes\n",
			expected:     true,
		},
		{
			name:         "deny deletion with no",
			resourceType: "dashboard",
			resourceName: "test-dashboard",
			resourceID:   "db-456",
			input:        "n\n",
			expected:     false,
		},
		{
			name:         "deny deletion with empty",
			resourceType: "slo",
			resourceName: "test-slo",
			resourceID:   "slo-789",
			input:        "\n",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := simulateInput(tt.input)
			defer cleanup()

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			result := ConfirmDeletion(tt.resourceType, tt.resourceName, tt.resourceID)

			w.Close()
			os.Stdout = oldStdout

			// Read and verify output contains expected information
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if result != tt.expected {
				t.Errorf("ConfirmDeletion() = %v, expected %v", result, tt.expected)
			}

			// Verify output contains resource details
			if !bytes.Contains(buf.Bytes(), []byte(tt.resourceType)) {
				t.Errorf("Output missing resource type: %s", output)
			}
			if !bytes.Contains(buf.Bytes(), []byte(tt.resourceName)) {
				t.Errorf("Output missing resource name: %s", output)
			}
			if !bytes.Contains(buf.Bytes(), []byte(tt.resourceID)) {
				t.Errorf("Output missing resource ID: %s", output)
			}
		})
	}
}

func TestConfirmDataDeletion(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		resourceName string
		input        string
		expected     bool
	}{
		{
			name:         "confirm data deletion with exact name",
			resourceType: "bucket",
			resourceName: "my-bucket",
			input:        "my-bucket\n",
			expected:     true,
		},
		{
			name:         "deny with incorrect name",
			resourceType: "bucket",
			resourceName: "my-bucket",
			input:        "wrong-name\n",
			expected:     false,
		},
		{
			name:         "deny with partial name",
			resourceType: "bucket",
			resourceName: "my-bucket",
			input:        "my-\n",
			expected:     false,
		},
		{
			name:         "deny with empty input",
			resourceType: "bucket",
			resourceName: "test-bucket",
			input:        "\n",
			expected:     false,
		},
		{
			name:         "deny with case mismatch",
			resourceType: "bucket",
			resourceName: "MyBucket",
			input:        "mybucket\n",
			expected:     false,
		},
		{
			name:         "confirm with special characters",
			resourceType: "bucket",
			resourceName: "my-bucket_v2",
			input:        "my-bucket_v2\n",
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := simulateInput(tt.input)
			defer cleanup()

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			result := ConfirmDataDeletion(tt.resourceType, tt.resourceName)

			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if result != tt.expected {
				t.Errorf("ConfirmDataDeletion() = %v, expected %v", result, tt.expected)
			}

			// Verify output contains warning and resource details
			if !bytes.Contains(buf.Bytes(), []byte("WARNING")) {
				t.Errorf("Output missing WARNING: %s", output)
			}
			if !bytes.Contains(buf.Bytes(), []byte(tt.resourceType)) {
				t.Errorf("Output missing resource type: %s", output)
			}
			if !bytes.Contains(buf.Bytes(), []byte(tt.resourceName)) {
				t.Errorf("Output missing resource name: %s", output)
			}
		})
	}
}

func TestValidateConfirmFlag(t *testing.T) {
	tests := []struct {
		name         string
		confirmValue string
		resourceName string
		expected     bool
	}{
		{
			name:         "matching names",
			confirmValue: "my-bucket",
			resourceName: "my-bucket",
			expected:     true,
		},
		{
			name:         "non-matching names",
			confirmValue: "my-bucket",
			resourceName: "your-bucket",
			expected:     false,
		},
		{
			name:         "empty confirm value",
			confirmValue: "",
			resourceName: "my-bucket",
			expected:     false,
		},
		{
			name:         "empty resource name",
			confirmValue: "my-bucket",
			resourceName: "",
			expected:     false,
		},
		{
			name:         "both empty",
			confirmValue: "",
			resourceName: "",
			expected:     true, // Empty strings match
		},
		{
			name:         "case sensitive",
			confirmValue: "MyBucket",
			resourceName: "mybucket",
			expected:     false,
		},
		{
			name:         "with special characters",
			confirmValue: "my-bucket_v2.prod",
			resourceName: "my-bucket_v2.prod",
			expected:     true,
		},
		{
			name:         "whitespace differences",
			confirmValue: "my-bucket ",
			resourceName: "my-bucket",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateConfirmFlag(tt.confirmValue, tt.resourceName)
			if result != tt.expected {
				t.Errorf("ValidateConfirmFlag(%q, %q) = %v, expected %v",
					tt.confirmValue, tt.resourceName, result, tt.expected)
			}
		})
	}
}
