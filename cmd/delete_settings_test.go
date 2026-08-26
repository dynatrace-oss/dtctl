package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/cmd/testutil"
)

func TestDeleteSettingsValidateOnly_Success(t *testing.T) {
	const objectID = "test-obj-delete-validate"

	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		"/platform/classic/environment-api/v2/settings/objects/" + objectID: func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				t.Errorf("unexpected DELETE — validate-only must not delete")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"objectId":"` + objectID + `","schemaVersion":"1.0","summary":"Test"}`))
				return
			}
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()
	cfgFile = configPath
	plainMode = true

	testutil.ResetCommandFlags(deleteSettingsCmd)
	_ = deleteSettingsCmd.Flags().Set("validate-only", "true")

	if err := deleteSettingsCmd.RunE(deleteSettingsCmd, []string{objectID}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestDeleteSettingsValidateOnly_ValidationFailed(t *testing.T) {
	const objectID = "test-obj-delete-validate-fail"

	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		// GET returns 404 — object is not reachable.
		"/platform/classic/environment-api/v2/settings/objects/" + objectID: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()
	cfgFile = configPath
	plainMode = true

	testutil.ResetCommandFlags(deleteSettingsCmd)
	_ = deleteSettingsCmd.Flags().Set("validate-only", "true")

	err := deleteSettingsCmd.RunE(deleteSettingsCmd, []string{objectID})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected error containing 'validation failed', got %q", err.Error())
	}
}

func TestDeleteSettingsValidateOnly_ObjectNotReachable(t *testing.T) {
	const objectID = "test-obj-delete-validate-forbidden"

	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		// GET returns 403 — object exists but is not reachable.
		"/platform/classic/environment-api/v2/settings/objects/" + objectID: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()
	cfgFile = configPath
	plainMode = true

	testutil.ResetCommandFlags(deleteSettingsCmd)
	_ = deleteSettingsCmd.Flags().Set("validate-only", "true")

	err := deleteSettingsCmd.RunE(deleteSettingsCmd, []string{objectID})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected error containing 'validation failed', got %q", err.Error())
	}
}
