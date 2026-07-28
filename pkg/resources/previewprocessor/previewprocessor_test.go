package previewprocessor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		configID   string
		wantConfig string // expected value of configId ("" = key must be absent)
		wantErr    bool
	}{
		{
			name:       "wraps processor body without configId when flag empty",
			body:       `{"type":"dql","dqlScript":"fieldsAdd foo = 1"}`,
			configID:   "",
			wantConfig: "",
		},
		{
			name:       "wraps processor body and adds configId from flag",
			body:       `{"type":"dql","dqlScript":"fieldsAdd foo = 1"}`,
			configID:   "logs",
			wantConfig: "logs",
		},
		{
			name:     "non-object processor body is an error",
			body:     `["not","an","object"]`,
			configID: "logs",
			wantErr:  true,
		},
		{
			name:     "invalid JSON body is an error",
			body:     `{not json`,
			configID: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := buildEnvelope(json.RawMessage(tt.body), tt.configID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var env map[string]json.RawMessage
			if err := json.Unmarshal(out, &env); err != nil {
				t.Fatalf("envelope is not valid JSON: %v", err)
			}

			// The processor body must be embedded verbatim under "processor".
			gotProcessor, ok := env["processor"]
			if !ok {
				t.Fatalf("envelope missing \"processor\" key")
			}
			if !jsonEqual(t, gotProcessor, json.RawMessage(tt.body)) {
				t.Errorf("processor body altered:\n got: %s\nwant: %s", gotProcessor, tt.body)
			}

			// configId present iff the flag was set.
			rawID, hasID := env["configId"]
			if tt.wantConfig == "" {
				if hasID {
					t.Errorf("configId present but flag was empty: %s", rawID)
				}
				return
			}
			var id string
			if err := json.Unmarshal(rawID, &id); err != nil {
				t.Fatalf("configId is not a JSON string: %v", err)
			}
			if id != tt.wantConfig {
				t.Errorf("configId = %q, want %q", id, tt.wantConfig)
			}
		})
	}
}

func TestReadFileOrStdin(t *testing.T) {
	t.Run("reads a valid JSON file", func(t *testing.T) {
		body := `{"type":"dql","dqlScript":"fieldsAdd foo = 1"}`
		path := filepath.Join(t.TempDir(), "processor.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		got, err := readFileOrStdin(path)
		if err != nil {
			t.Fatalf("readFileOrStdin() error = %v", err)
		}
		if !jsonEqual(t, got, json.RawMessage(body)) {
			t.Errorf("content = %s, want %s", got, body)
		}
	})

	t.Run("missing file is an error", func(t *testing.T) {
		if _, err := readFileOrStdin(filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("invalid JSON is an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		if _, err := readFileOrStdin(path); err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})
}

// jsonEqual reports whether two raw JSON documents are semantically equal.
func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return string(ab) == string(bb)
}
