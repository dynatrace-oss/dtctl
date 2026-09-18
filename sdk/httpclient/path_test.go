package httpclient

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"
)

func TestCheckRequestPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "a plain collection path", path: "/platform/slo/v1/slos"},
		{name: "a plain resource path", path: "/platform/slo/v1/slos/slo-1"},
		{name: "an id with dots", path: "/platform/davis/analyzers/v1/analyzers/dt.statistics.Clustering"},
		{name: "an action suffix keeps its colon", path: "/platform/storage/management/v1/bucket-definitions/b:truncate"},
		{name: "an escaped separator stays one segment", path: "/platform/slo/v1/slos/" + url.PathEscape("a/b")},
		{name: "an escaped hash stays one segment", path: "/platform/slo/v1/slos/" + url.PathEscape("slo-1#x")},
		{name: "a query string is the caller's business", path: "/platform/example/v1/things?page=2"},
		{name: "an empty path resolves to the base URL", path: ""},
		{name: "an absolute URL is vetted on its path only", path: "http://127.0.0.1:8080/platform/dob/graphql"},
		{name: "an absolute URL with no path", path: "https://example.dynatrace.com"},
		{
			name:    "an absolute URL is still vetted",
			path:    "http://127.0.0.1:8080/platform/slo/v1/slos/..",
			wantErr: `normalizes to "/platform/slo/v1"`,
		},

		{
			name:    "an unescaped hash would retarget the request",
			path:    "/platform/slo/v1/slos/slo-1#x",
			wantErr: `would target "/platform/slo/v1/slos/slo-1"`,
		},
		{
			name:    "a hash before a query is still a fragment",
			path:    "/platform/slo/v1/slos/slo-1#x?page=2",
			wantErr: "'#' starts a URL fragment",
		},
		{
			name:    "an empty id addresses the collection",
			path:    "/platform/slo/v1/slos/",
			wantErr: `normalizes to "/platform/slo/v1/slos"`,
		},
		{
			name:    "an empty id in the middle drops a segment",
			path:    "/platform/document/v1/documents//metadata",
			wantErr: `normalizes to "/platform/document/v1/documents/metadata"`,
		},
		{
			name:    "a dot id addresses the collection",
			path:    "/platform/slo/v1/slos/.",
			wantErr: `normalizes to "/platform/slo/v1/slos"`,
		},
		{
			name:    "a dot-dot id climbs out of the collection",
			path:    "/platform/slo/v1/slos/..",
			wantErr: `normalizes to "/platform/slo/v1"`,
		},
		{
			name:    "a dot-dot id is caught behind a query too",
			path:    "/platform/slo/v1/slos/..?page=2",
			wantErr: `normalizes to "/platform/slo/v1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckRequestPath(tt.path)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
			require.Contains(t, err.Error(), "invalid request path",
				"the message must classify as a validation error, not a generic one")
		})
	}
}

// TestGuardRequestPathsRefusesBeforeSending is the property that matters: the
// request must not reach the server at all. A guard that only reported the
// problem afterwards would already have deleted the wrong object.
func TestGuardRequestPathsRefusesBeforeSending(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rc := GuardRequestPaths(resty.New()).SetBaseURL(srv.URL)

	_, err := rc.R().Delete("/platform/slo/v1/slos/slo-1#x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "URL fragment")
	require.Empty(t, got, "the request must never have been sent")

	// The same id, escaped the way a handler escapes it, does go out -- as one
	// segment, so the API decides whether it exists rather than dtctl silently
	// acting on a different object.
	_, err = rc.R().Delete("/platform/slo/v1/slos/" + url.PathEscape("slo-1#x"))
	require.NoError(t, err)
	require.Equal(t, []string{"/platform/slo/v1/slos/slo-1%23x"}, got)
}

// TestGuardRequestPathsIsInstalledByNew pins the wiring: the guard is worthless
// if a constructor forgets it.
func TestGuardRequestPathsIsInstalledByNew(t *testing.T) {
	c, err := New("https://example.apps.dynatrace.com", WithToken("dt0c01.TEST"))
	require.NoError(t, err)

	_, err = c.HTTP().R().Get("/platform/slo/v1/slos/slo-1#x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid request path")
}
