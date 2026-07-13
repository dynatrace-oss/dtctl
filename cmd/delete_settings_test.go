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
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"objectId":"` + objectID + `","schemaVersion":"1.0","summary":"Test"}`))
			case http.MethodDelete:
				if r.URL.Query().Get("validateOnly") != "true" {
					t.Errorf("expected validateOnly=true, got %q", r.URL.Query().Get("validateOnly"))
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Errorf("unexpected method %s", r.Method)
			}
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
		"/platform/classic/environment-api/v2/settings/objects/" + objectID: func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"objectId":"` + objectID + `","schemaVersion":"1.0"}`))
			case http.MethodDelete:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "deletion not allowed"}}`))
			}
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
