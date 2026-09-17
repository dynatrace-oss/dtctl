package diagnostic

import (
	"strings"
	"testing"
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
			// details.missingScopes is the one field that settles the
			// scope-vs-policy question, so it must not be dropped.
			name: "missing scopes from details are appended",
			body: `{"error":{"code":403,"message":"Insufficient permissions.","details":{"missingScopes":["storage:files:delete","storage:files:read"]}}}`,
			want: "Insufficient permissions. (missing scopes: storage:files:delete, storage:files:read)",
		},
		{
			name: "details as a plain string is appended",
			body: `{"error":{"code":403,"message":"Forbidden.","details":"policy denies /lookups/prod/"}}`,
			want: "Forbidden. (policy denies /lookups/prod/)",
		},
		{
			name: "top-level message envelope",
			body: `{"message":"No permission to delete file '/lookups/a/b'."}`,
			want: "No permission to delete file '/lookups/a/b'.",
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
			if got := ServerErrorMessage(tt.body); got != tt.want {
				t.Errorf("ServerErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A proxy or WAF answering a 403 with a large HTML page must not become the
// whole error message — in agent mode it lands in the envelope verbatim.
func TestServerErrorMessageTruncatesRawBody(t *testing.T) {
	body := "<html>" + strings.Repeat("x", 4096) + "</html>"

	got := ServerErrorMessage(body)
	if len(got) > maxBodyExcerpt+len("... (truncated)") {
		t.Errorf("raw body not truncated: got %d bytes", len(got))
	}
	if !strings.HasSuffix(got, "... (truncated)") {
		t.Errorf("truncation not marked: %q", got[max(0, len(got)-40):])
	}
}
