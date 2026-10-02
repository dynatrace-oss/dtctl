package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// envShareServer mocks the Document API endpoints share/unshare --environment
// touch, for a launchpad document "lp-1" that starts out public with one
// read environment share. Every mutating request is recorded as
// "METHOD path [detail]" in call order.
type envShareServer struct {
	mu          sync.Mutex
	mutations   []string
	isPrivate   bool
	patchStatus int // non-zero: the isPrivate PATCH fails with this status
	shares      []map[string]any
}

func newEnvShareServer(t *testing.T) (*envShareServer, *httptest.Server) {
	t.Helper()
	s := &envShareServer{
		shares: []map[string]any{{"id": "env-share-1", "documentId": "lp-1", "access": []string{"read"}}},
	}
	record := func(entry string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.mutations = append(s.mutations, entry)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/documents/lp-1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "lp-1", "name": "Team launchpad", "type": "launchpad",
			"owner": "owner-1", "version": 4, "isPrivate": s.isPrivate,
		})
	})
	mux.HandleFunc("/platform/document/v1/documents/lp-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		record("PATCH isPrivate=" + r.FormValue("isPrivate"))
		if s.patchStatus != 0 {
			w.WriteHeader(s.patchStatus)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"environment-shares": s.shares, "totalCount": len(s.shares)})
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		record("POST environment-share " + body["documentId"] + " " + body["access"])
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "env-share-2", "documentId": body["documentId"], "access": []string{"read", "write"}})
	})
	mux.HandleFunc("/platform/document/v1/environment-shares/", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method + " " + r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/platform/document/v1/direct-shares", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"direct-shares": []map[string]any{{"id": "direct-1", "documentId": "lp-1", "access": []string{"read"}}},
				"totalCount":    1,
			})
			return
		}
		record(r.Method + " " + r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/platform/document/v1/direct-shares/", func(w http.ResponseWriter, r *http.Request) {
		record(r.Method + " " + r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return s, srv
}

func (s *envShareServer) calls() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.mutations, "\n")
}

func runShareCmd(t *testing.T, c *cobra.Command, flags map[string]string) error {
	t.Helper()
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	var err error
	captureExtStdout(t, func() {
		captureDocStderr(t, func() {
			err = c.RunE(c, []string{"lp-1"})
		})
	})
	return err
}

// TestShareDocument_Environment: --environment shares a document of any type
// (here a launchpad) with the whole environment at the requested access level
// and marks it public, without touching direct shares.
func TestShareDocument_Environment(t *testing.T) {
	s, srv := newEnvShareServer(t)
	s.isPrivate = true
	s.shares = nil
	setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

	if err := runShareCmd(t, shareDocumentCmd, map[string]string{"environment": "true", "access": "read-write"}); err != nil {
		t.Fatalf("share document --environment: %v", err)
	}
	want := "POST environment-share lp-1 read-write\nPATCH isPrivate=false"
	if got := s.calls(); got != want {
		t.Errorf("mutating calls:\n%s\nwant:\n%s", got, want)
	}
}

// TestShareDocument_EnvironmentReplacesAccess: re-sharing at a different
// access level replaces the existing environment share.
func TestShareDocument_EnvironmentReplacesAccess(t *testing.T) {
	s, srv := newEnvShareServer(t)
	setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

	if err := runShareCmd(t, shareDocumentCmd, map[string]string{"environment": "true", "access": "read-write"}); err != nil {
		t.Fatalf("share document --environment: %v", err)
	}
	want := "DELETE /platform/document/v1/environment-shares/env-share-1\nPOST environment-share lp-1 read-write"
	if got := s.calls(); got != want {
		t.Errorf("mutating calls:\n%s\nwant:\n%s", got, want)
	}
}

func TestShareDocument_EnvironmentFlagConflicts(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{"with --user", map[string]string{"environment": "true", "user": "user-1"}, "cannot be combined"},
		{"with --group", map[string]string{"environment": "true", "group": "group-1"}, "cannot be combined"},
		{"with --no-notify", map[string]string{"environment": "true", "no-notify": "true"}, "--no-notify"},
		{"nothing to share with", map[string]string{}, "--environment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, srv := newEnvShareServer(t)
			setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

			err := runShareCmd(t, shareDocumentCmd, tt.flags)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one mentioning %q", err, tt.want)
			}
			if got := s.calls(); got != "" {
				t.Errorf("mutating calls on a rejected invocation:\n%s", got)
			}
		})
	}
}

// TestShareUnshareDocument_EnvironmentNeedsSafetyLevel: environment sharing
// changes who can read the document, so a readonly context refuses it.
func TestShareUnshareDocument_EnvironmentNeedsSafetyLevel(t *testing.T) {
	for _, c := range []*cobra.Command{shareDocumentCmd, unshareDocumentCmd} {
		t.Run(c.Parent().Name(), func(t *testing.T) {
			s, srv := newEnvShareServer(t)
			setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadOnly)

			err := runShareCmd(t, c, map[string]string{"environment": "true"})
			if err == nil || !strings.Contains(err.Error(), "does not allow") {
				t.Fatalf("error = %v, want the safety check's refusal", err)
			}
			if got := s.calls(); got != "" {
				t.Errorf("mutating calls in a readonly context:\n%s", got)
			}
		})
	}
}

func TestShareUnshareDocument_EnvironmentDryRun(t *testing.T) {
	for _, c := range []*cobra.Command{shareDocumentCmd, unshareDocumentCmd} {
		t.Run(c.Parent().Name(), func(t *testing.T) {
			s, srv := newEnvShareServer(t)
			setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)
			dryRun = true

			if err := c.Flags().Set("environment", "true"); err != nil {
				t.Fatal(err)
			}
			var err error
			out := captureExtStdout(t, func() { err = c.RunE(c, []string{"lp-1"}) })
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if !strings.Contains(out, "Dry run") || !strings.Contains(out, "environment") {
				t.Errorf("output %q does not preview the environment share change", out)
			}
			if got := s.calls(); got != "" {
				t.Errorf("mutating calls in a dry run:\n%s", got)
			}
		})
	}
}

// TestUnshareDocument_Environment: --environment makes the document private
// again and then deletes the environment share; direct shares stay untouched.
func TestUnshareDocument_Environment(t *testing.T) {
	s, srv := newEnvShareServer(t)
	setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

	if err := runShareCmd(t, unshareDocumentCmd, map[string]string{"environment": "true"}); err != nil {
		t.Fatalf("unshare document --environment: %v", err)
	}
	want := "PATCH isPrivate=true\nDELETE /platform/document/v1/environment-shares/env-share-1"
	if got := s.calls(); got != want {
		t.Errorf("mutating calls:\n%s\nwant:\n%s", got, want)
	}
}

// TestUnshareDocument_EnvironmentPrivateFails: when the document cannot be
// made private, no share is deleted and the error says the document is
// unchanged, so it is never left public without its share.
func TestUnshareDocument_EnvironmentPrivateFails(t *testing.T) {
	s, srv := newEnvShareServer(t)
	s.patchStatus = http.StatusForbidden
	setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

	err := runShareCmd(t, unshareDocumentCmd, map[string]string{"environment": "true"})
	if err == nil || !strings.Contains(err.Error(), "the document is unchanged") {
		t.Fatalf("error = %v, want one stating the document is unchanged", err)
	}
	if got, want := s.calls(), "PATCH isPrivate=true"; got != want {
		t.Errorf("mutating calls:\n%s\nwant:\n%s", got, want)
	}
}

// TestUnshareDocument_RejectsUnknownAccess: an unrecognized --access level
// used to be matched as 'read', so a typo deleted read shares.
func TestUnshareDocument_RejectsUnknownAccess(t *testing.T) {
	s, srv := newEnvShareServer(t)
	setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

	err := runShareCmd(t, unshareDocumentCmd, map[string]string{"environment": "true", "access": "rw"})
	if err == nil || !strings.Contains(err.Error(), "invalid access level") {
		t.Fatalf("error = %v, want an invalid access level error", err)
	}
	if got := s.calls(); got != "" {
		t.Errorf("mutating calls on a rejected invocation:\n%s", got)
	}
}

// TestUnshareDocument_AllLeavesEnvironment: --all keeps its meaning (user and
// group shares only); combined with --environment it removes both.
func TestUnshareDocument_AllLeavesEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{"--all", map[string]string{"all": "true"},
			"DELETE /platform/document/v1/direct-shares/direct-1"},
		{"--all --environment", map[string]string{"all": "true", "environment": "true"},
			"PATCH isPrivate=true\nDELETE /platform/document/v1/environment-shares/env-share-1\n" +
				"DELETE /platform/document/v1/direct-shares/direct-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, srv := newEnvShareServer(t)
			setupDocumentCmdTest(t, srv.URL, config.SafetyLevelReadWriteAll)

			if err := runShareCmd(t, unshareDocumentCmd, tt.flags); err != nil {
				t.Fatalf("unshare document: %v", err)
			}
			if got := s.calls(); got != tt.want {
				t.Errorf("mutating calls:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
