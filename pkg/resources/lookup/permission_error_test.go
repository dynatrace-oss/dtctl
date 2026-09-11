package lookup

import (
	"errors"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
)

func TestServerErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "resource store error envelope",
			body: `{"error":{"message":"Delete failed: No permission to delete file '/lookups/a/b'.","code":403}}`,
			want: "Delete failed: No permission to delete file '/lookups/a/b'.",
		},
		{
			name: "plain text body falls back to raw",
			body: "access denied",
			want: "access denied",
		},
		{
			name: "envelope without message falls back to raw",
			body: `{"error":{"code":403}}`,
			want: `{"error":{"code":403}}`,
		},
		{
			name: "empty body yields nothing",
			body: "   ",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverErrorMessage(tt.body); got != tt.want {
				t.Errorf("serverErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHandleDeleteErrorForbidden pins the behaviour reported in issue #344: the
// server's explanation must reach the user, and the error must be typed so it
// exits with the permission code instead of the generic one.
func TestHandleDeleteErrorForbidden(t *testing.T) {
	const path = "/lookups/product/prd/lookup_ip"
	body := `{"error":{"message":"Delete failed: No permission to delete file '` + path + `'.","code":403}}`

	err := handleDeleteError(403, body, path)
	if err == nil {
		t.Fatal("handleDeleteError() returned nil for 403")
	}

	var diagErr *diagnostic.Error
	if !errors.As(err, &diagErr) {
		t.Fatalf("handleDeleteError() = %T, want *diagnostic.Error", err)
	}
	if diagErr.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want 403", diagErr.StatusCode)
	}

	msg := err.Error()
	// The server said why; the old implementation dropped this.
	if !strings.Contains(msg, "No permission to delete file") {
		t.Errorf("error message lost the server explanation: %q", msg)
	}
	if !strings.Contains(msg, path) {
		t.Errorf("error message %q does not mention path %q", msg, path)
	}

	joined := strings.Join(diagErr.Suggestions, "\n")
	if !strings.Contains(joined, "--check-scopes") {
		t.Errorf("suggestions do not point at the scope check: %q", joined)
	}
	if !strings.Contains(joined, `ALLOW storage:files:delete WHERE storage:file-path startsWith "/lookups/"`) {
		t.Errorf("suggestions do not include the IAM policy statement: %q", joined)
	}
}

func TestHandleUploadErrorForbidden(t *testing.T) {
	const path = "/lookups/dtctl-test/issue344"

	err := handleUploadError(403, `{"error":{"message":"No permission to write file.","code":403}}`, path)
	if err == nil {
		t.Fatal("handleUploadError() returned nil for 403")
	}

	var diagErr *diagnostic.Error
	if !errors.As(err, &diagErr) {
		t.Fatalf("handleUploadError() = %T, want *diagnostic.Error", err)
	}

	if !strings.Contains(err.Error(), "No permission to write file.") {
		t.Errorf("error message lost the server explanation: %q", err.Error())
	}
	if joined := strings.Join(diagErr.Suggestions, "\n"); !strings.Contains(joined, "storage:files:write") {
		t.Errorf("suggestions do not name the write permission: %q", joined)
	}
}

// A 403 with no body must still produce the scope-vs-policy guidance.
func TestForbiddenErrorWithEmptyBody(t *testing.T) {
	err := handleDeleteError(403, "", "/lookups/a/b")

	msg := err.Error()
	if !strings.Contains(msg, `access denied to delete file "/lookups/a/b"`) {
		t.Errorf("unexpected message: %q", msg)
	}
	if strings.Contains(msg, "file \"/lookups/a/b\": ") {
		t.Errorf("empty body should not leave a dangling separator: %q", msg)
	}
	if !strings.Contains(msg, "separate gates") {
		t.Errorf("guidance missing from message: %q", msg)
	}
}
