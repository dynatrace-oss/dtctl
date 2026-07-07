package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/cmd/testutil"
)

func TestCreateSettingsValidateOnly_Success(t *testing.T) {
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		"/platform/classic/environment-api/v2/settings/objects": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Query().Get("validateOnly") != "true" {
				t.Errorf("expected validateOnly=true, got %q", r.URL.Query().Get("validateOnly"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	settingsFile := testutil.CreateTempFile(t, `{"key": "value"}`, "settings-*.json")

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()
	cfgFile = configPath
	plainMode = true

	// Note: avoid ResetCommandFlags here — it calls Set("[]") on the StringArray --set flag,
	// which adds the literal string "[]" as a value and breaks template parsing.
	_ = createSettingsCmd.Flags().Set("file", settingsFile)
	_ = createSettingsCmd.Flags().Set("schema", "builtin:alerting.profile")
	_ = createSettingsCmd.Flags().Set("scope", "environment")
	_ = createSettingsCmd.Flags().Set("validate-only", "true")

	if err := createSettingsCmd.RunE(createSettingsCmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func TestCreateSettingsValidateOnly_ValidationFailed(t *testing.T) {
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		"/platform/classic/environment-api/v2/settings/objects": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "invalid value"}}`))
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	settingsFile := testutil.CreateTempFile(t, `{"key": "value"}`, "settings-*.json")

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()
	cfgFile = configPath
	plainMode = true

	// Note: avoid ResetCommandFlags here — see TestCreateSettingsValidateOnly_Success.
	_ = createSettingsCmd.Flags().Set("file", settingsFile)
	_ = createSettingsCmd.Flags().Set("schema", "builtin:alerting.profile")
	_ = createSettingsCmd.Flags().Set("scope", "environment")
	_ = createSettingsCmd.Flags().Set("validate-only", "true")

	err := createSettingsCmd.RunE(createSettingsCmd, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected error containing 'validation failed', got %q", err.Error())
	}
}

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

func TestUpdateSettingsRedirectsToApply(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"settings with file flag", []string{"update", "settings", "some-id", "-f", "config.yaml"}},
		{"setting alias", []string{"update", "setting", "some-id", "-f", "config.yaml"}},
		{"settings without flags", []string{"update", "settings"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected error with redirect hint")
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "dtctl apply -f") {
				t.Errorf("expected hint to use 'dtctl apply -f', got: %s", errMsg)
			}
			if !strings.Contains(errMsg, "objectId") {
				t.Errorf("expected hint to mention objectId, got: %s", errMsg)
			}
		})
	}
}
