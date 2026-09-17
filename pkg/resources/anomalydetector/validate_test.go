package anomalydetector

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// minimalFlattened is the smallest definition a user would hand-write: no
// description, no executionSettings, no source. The Settings API rejects every
// one of those omissions with "Must not be null" (issue #369).
const minimalFlattened = `{
	"title": "Minimal Detector",
	"analyzer": {
		"name": "dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer",
		"input": {"threshold": "90", "alertCondition": "ABOVE"}
	},
	"eventTemplate": {"event.type": "CUSTOM_ALERT", "event.name": "Minimal"}
}`

func TestPrepareCreateBody_FillsNonNullableSchemaDefaults(t *testing.T) {
	h := NewHandler(nil)

	body, err := h.PrepareCreateBody([]byte(minimalFlattened))
	if err != nil {
		t.Fatalf("PrepareCreateBody() error = %v", err)
	}

	value, ok := body["value"].(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any", body["value"])
	}
	if got, ok := value["description"].(string); !ok || got != "" {
		t.Errorf("description = %#v, want empty string", value["description"])
	}
	if got := value["enabled"]; got != true {
		t.Errorf("enabled = %#v, want true", got)
	}
	if got := value["source"]; got != defaultSource {
		t.Errorf("source = %#v, want %q", got, defaultSource)
	}
	if _, ok := value["executionSettings"].(map[string]any); !ok {
		t.Errorf("executionSettings = %#v, want an object", value["executionSettings"])
	}
}

func TestPrepareCreateBody_RawFormatKeepsEnvelopeAndFillsDefaults(t *testing.T) {
	h := NewHandler(nil)

	// The exact shape from issue #369: raw Settings envelope, no description,
	// empty executionSettings.
	data := []byte(fmt.Sprintf(`{
		"schemaId": %q,
		"scope": %q,
		"objectId": "obj-1",
		"value": {
			"title": "Raw Detector",
			"enabled": true,
			"source": "Custom Alerts",
			"analyzer": {"name": "dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer", "input": []},
			"eventTemplate": {"properties": [{"key": "event.type", "value": "CUSTOM_ALERT"}]},
			"executionSettings": {}
		}
	}`, SchemaID, Scope))

	body, err := h.PrepareCreateBody(data)
	if err != nil {
		t.Fatalf("PrepareCreateBody() error = %v", err)
	}
	if body["objectId"] != "obj-1" {
		t.Errorf("objectId = %#v, want the envelope to be preserved", body["objectId"])
	}
	value := body["value"].(map[string]any)
	if got, ok := value["description"].(string); !ok || got != "" {
		t.Errorf("description = %#v, want empty string", value["description"])
	}
	if got := value["source"]; got != "Custom Alerts" {
		t.Errorf("source = %#v, want the author's value to survive", got)
	}
}

func TestPrepareCreateBody_NullExecutionSettings(t *testing.T) {
	// "executionSettings:" with no body in YAML becomes a present-but-null key.
	data := []byte(`{
		"title": "Null Exec Detector",
		"analyzer": {"name": "dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer"},
		"eventTemplate": {"event.type": "CUSTOM_ALERT"},
		"executionSettings": null
	}`)

	body, err := NewHandler(nil).PrepareCreateBody(data)
	if err != nil {
		t.Fatalf("PrepareCreateBody() error = %v", err)
	}
	value := body["value"].(map[string]any)
	if _, ok := value["executionSettings"].(map[string]any); !ok {
		t.Errorf("executionSettings = %#v, want an object", value["executionSettings"])
	}
}

func TestPrepareCreateBody_FillsDefaultActor(t *testing.T) {
	const actor = "e1a88cd0-bfe5-4fa1-b36f-9f85c9fac22e"
	h := NewHandler(nil).WithDefaultActor(actor)

	body, err := h.PrepareCreateBody([]byte(minimalFlattened))
	if err != nil {
		t.Fatalf("PrepareCreateBody() error = %v", err)
	}

	value := body["value"].(map[string]any)
	es := value["executionSettings"].(map[string]any)
	if es["actor"] != actor {
		t.Errorf("executionSettings.actor = %#v, want %q", es["actor"], actor)
	}
}

func TestPrepareCreateBody_KeepsAuthoredActor(t *testing.T) {
	const authored = "11111111-2222-3333-4444-555555555555"
	data := []byte(fmt.Sprintf(`{
		"title": "Authored Actor",
		"analyzer": {"name": "dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer"},
		"eventTemplate": {"event.type": "CUSTOM_ALERT"},
		"executionSettings": {"actor": %q}
	}`, authored))

	body, err := NewHandler(nil).WithDefaultActor("99999999-9999-9999-9999-999999999999").PrepareCreateBody(data)
	if err != nil {
		t.Fatalf("PrepareCreateBody() error = %v", err)
	}
	es := body["value"].(map[string]any)["executionSettings"].(map[string]any)
	if es["actor"] != authored {
		t.Errorf("executionSettings.actor = %#v, want the authored value %q", es["actor"], authored)
	}
}

func TestPrepareCreateBody_LocalValidation(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name:    "title too short",
			data:    `{"title":"x","analyzer":{"name":"analyzer.Name"},"eventTemplate":{}}`,
			wantErr: "title must be 2 to 500 characters long",
		},
		{
			name:    "analyzer name missing (flattened)",
			data:    `{"title":"No Analyzer Name","analyzer":{"input":{}},"eventTemplate":{}}`,
			wantErr: "'analyzer.name' is required",
		},
		{
			name: "analyzer name missing (raw settings)",
			data: fmt.Sprintf(`{"schemaId":%q,"scope":%q,"value":{"title":"No Analyzer Name","analyzer":{},"eventTemplate":{"properties":[]}}}`,
				SchemaID, Scope),
			wantErr: "analyzer.name is required",
		},
		{
			name:    "actor not a uuid",
			data:    `{"title":"Bad Actor","analyzer":{"name":"analyzer.Name"},"eventTemplate":{},"executionSettings":{"actor":"service-user-1"}}`,
			wantErr: `executionSettings.actor must be a UUID (got "service-user-1")`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHandler(nil).PrepareCreateBody([]byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("PrepareCreateBody() error = %v, want to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateCreate_UsesValidateOnly(t *testing.T) {
	var gotQuery, gotMethod string
	h, server := newTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("validateOnly")
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"code":200}]`))
	})
	defer server.Close()

	if err := h.ValidateCreate([]byte(minimalFlattened)); err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotQuery != "true" {
		t.Errorf("validateOnly = %q, want %q", gotQuery, "true")
	}
}

func TestValidateCreate_ReportsConstraintViolations(t *testing.T) {
	h, server := newTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `[{"code":400,"error":{"code":400,"message":"Validation failed for 1 Validators.",
			"constraintViolations":[{"path":%q,"message":"Must not be null"}]},"invalidValue":{"title":"x"}}]`,
			SchemaID+"/0/executionSettings.actor")
	})
	defer server.Close()

	err := h.ValidateCreate([]byte(minimalFlattened))
	if err == nil {
		t.Fatal("ValidateCreate() error = nil, want a validation error")
	}
	if !strings.Contains(err.Error(), "executionSettings.actor: Must not be null") {
		t.Errorf("error = %v, want the offending field and message", err)
	}
	if strings.Contains(err.Error(), "invalidValue") {
		t.Errorf("error = %v, want the echoed payload to be dropped", err)
	}
	var unavailable *ValidationUnavailableError
	if errors.As(err, &unavailable) {
		t.Errorf("error = %v, want a rejection rather than an unavailable verdict", err)
	}
}

func TestValidateCreate_ServerErrorIsUnavailable(t *testing.T) {
	h, server := newTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":403,"message":"no scope"}}`))
	})
	defer server.Close()

	err := h.ValidateCreate([]byte(minimalFlattened))
	var unavailable *ValidationUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("ValidateCreate() error = %v, want *ValidationUnavailableError", err)
	}
}

func TestValidateUpdate_UsesValidateOnly(t *testing.T) {
	var gotQuery, gotIfMatch string
	h, server := newTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(sampleItem("obj-1", "Existing", true))
		case http.MethodPut:
			gotQuery = r.URL.Query().Get("validateOnly")
			gotIfMatch = r.Header.Get("If-Match")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"code":200}`))
		}
	})
	defer server.Close()

	if err := h.ValidateUpdate("obj-1", []byte(minimalFlattened)); err != nil {
		t.Fatalf("ValidateUpdate() error = %v", err)
	}
	if gotQuery != "true" {
		t.Errorf("validateOnly = %q, want %q", gotQuery, "true")
	}
	if gotIfMatch != "1.0.15" {
		t.Errorf("If-Match = %q, want the existing schema version", gotIfMatch)
	}
}

func TestUpdate_PreservesExistingActor(t *testing.T) {
	const existingActor = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	var sentActor any
	h, server := newTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			item := sampleItem("obj-1", "Existing", true)
			item.Value["executionSettings"] = map[string]any{"actor": existingActor}
			json.NewEncoder(w).Encode(item)
		case http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			value := body["value"].(map[string]any)
			es := value["executionSettings"].(map[string]any)
			sentActor = es["actor"]
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}
	})
	defer server.Close()

	// The definition being applied omits executionSettings entirely — a PUT
	// replaces the whole value, so the actor the detector already runs as must
	// not be dropped.
	if _, err := h.WithDefaultActor("99999999-9999-9999-9999-999999999999").Update("obj-1", []byte(minimalFlattened)); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if sentActor != existingActor {
		t.Errorf("executionSettings.actor = %#v, want the existing %q", sentActor, existingActor)
	}
}

func TestCreate_400ErrorNamesTheField(t *testing.T) {
	h, server := newTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `[{"code":400,"error":{"code":400,"message":"Validation failed for 2 Validators.",
			"constraintViolations":[{"path":%q,"message":"Must not be null"},{"path":%q,"message":"Must not be null"}]},
			"invalidValue":{"title":"Minimal Detector"}}]`,
			SchemaID+"/0/description", SchemaID+"/0/executionSettings")
	})
	defer server.Close()

	_, err := h.Create([]byte(minimalFlattened))
	if err == nil {
		t.Fatal("Create() error = nil, want a validation error")
	}
	for _, want := range []string{"description: Must not be null", "executionSettings: Must not be null"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want to contain %q", err, want)
		}
	}
}

func TestDescribeSettingsError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "post array with violations",
			body: `[{"code":400,"error":{"code":400,"message":"Validation failed.","constraintViolations":[{"path":"builtin:davis.anomaly-detectors/0/title","message":"Size must be between 2 and 500"}]}}]`,
			want: "title: Size must be between 2 and 500",
		},
		{
			name: "put object with violations",
			body: `{"code":400,"error":{"code":400,"message":"Validation failed.","constraintViolations":[{"path":"builtin:davis.anomaly-detectors/0/source","message":"Must not be blank"}]}}`,
			want: "source: Must not be blank",
		},
		{
			name: "message only",
			body: `{"error":{"code":400,"message":"Constraints violated."}}`,
			want: "invalid anomaly detector: Constraints violated.",
		},
		{
			name: "unparseable",
			body: `boom`,
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := describeSettingsError([]byte(tc.body))
			if tc.want == "" {
				if got != "" {
					t.Fatalf("describeSettingsError() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("describeSettingsError() = %q, want to contain %q", got, tc.want)
			}
		})
	}
}

func TestTrimViolationPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: SchemaID + "/0/description", want: "description"},
		{path: SchemaID + "/0/executionSettings.actor", want: "executionSettings.actor"},
		{path: SchemaID + "/0", want: "0"},
		{path: "some/other/path", want: "some/other/path"},
	}
	for _, tc := range tests {
		if got := trimViolationPath(tc.path); got != tc.want {
			t.Errorf("trimViolationPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
