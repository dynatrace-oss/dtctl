package apply

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDetectResourceType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ResourceType
		wantErr  bool
	}{
		{
			name: "dashboard with tiles at root",
			input: `{
				"tiles": [{"name": "test", "tileType": "MARKDOWN"}],
				"version": "1"
			}`,
			expected: ResourceDashboard,
			wantErr:  false,
		},
		{
			name: "dashboard with content wrapper",
			input: `{
				"name": "My Dashboard",
				"content": {
					"tiles": [{"name": "test", "tileType": "MARKDOWN"}],
					"version": "1"
				}
			}`,
			expected: ResourceDashboard,
			wantErr:  false,
		},
		{
			name: "dashboard with type field",
			input: `{
				"type": "dashboard",
				"content": {"version": "1"}
			}`,
			expected: ResourceDashboard,
			wantErr:  false,
		},
		{
			name: "dashboard with metadata",
			input: `{
				"metadata": {"name": "test"},
				"type": "dashboard"
			}`,
			expected: ResourceDashboard,
			wantErr:  false,
		},
		{
			name: "notebook with sections at root",
			input: `{
				"sections": [{"title": "test"}]
			}`,
			expected: ResourceNotebook,
			wantErr:  false,
		},
		{
			name: "notebook with content wrapper",
			input: `{
				"name": "My Notebook",
				"content": {
					"sections": [{"title": "test"}]
				}
			}`,
			expected: ResourceNotebook,
			wantErr:  false,
		},
		{
			name: "workflow",
			input: `{
				"tasks": [{"name": "test"}],
				"trigger": {"type": "event"}
			}`,
			expected: ResourceWorkflow,
			wantErr:  false,
		},
		{
			name: "SLO",
			input: `{
				"name": "Test SLO",
				"criteria": {"threshold": 95},
				"customSli": {"enabled": true}
			}`,
			expected: ResourceSLO,
			wantErr:  false,
		},
		{
			name: "bucket",
			input: `{
				"bucketName": "my-bucket",
				"table": "logs"
			}`,
			expected: ResourceBucket,
			wantErr:  false,
		},
		{
			name: "unknown resource",
			input: `{
				"random": "field"
			}`,
			expected: ResourceUnknown,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate JSON
			var testJSON map[string]interface{}
			if err := json.Unmarshal([]byte(tt.input), &testJSON); err != nil {
				t.Fatalf("test input is not valid JSON: %v", err)
			}

			result, err := detectResourceType([]byte(tt.input))

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid UUID lowercase",
			input:    "550e8400-e29b-41d4-a716-446655440000",
			expected: true,
		},
		{
			name:     "valid UUID uppercase",
			input:    "550E8400-E29B-41D4-A716-446655440000",
			expected: true,
		},
		{
			name:     "valid UUID mixed case",
			input:    "550e8400-E29B-41d4-A716-446655440000",
			expected: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "too short",
			input:    "550e8400-e29b-41d4",
			expected: false,
		},
		{
			name:     "no dashes",
			input:    "550e8400e29b41d4a716446655440000",
			expected: false,
		},
		{
			name:     "wrong dash positions",
			input:    "550e-8400-e29b-41d4-a716-446655440000",
			expected: false,
		},
		{
			name:     "contains invalid characters",
			input:    "550e8400-e29b-41d4-a716-44665544000g",
			expected: false,
		},
		{
			name:     "simple string",
			input:    "my-dashboard-id",
			expected: false,
		},
		{
			name:     "document ID format (not UUID)",
			input:    "abc123def456",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUUID(tt.input)
			if result != tt.expected {
				t.Errorf("isUUID(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractDocumentContent(t *testing.T) {
	tests := []struct {
		name            string
		doc             map[string]interface{}
		docType         string
		wantName        string
		wantDescription string
		wantWarnings    int
		wantTiles       bool // check if content has tiles
		wantSections    bool // check if content has sections
	}{
		{
			name: "dashboard with content wrapper",
			doc: map[string]interface{}{
				"name":        "My Dashboard",
				"description": "A test dashboard",
				"content": map[string]interface{}{
					"tiles":   []interface{}{map[string]interface{}{"name": "tile1"}},
					"version": "1",
				},
			},
			docType:         "dashboard",
			wantName:        "My Dashboard",
			wantDescription: "A test dashboard",
			wantWarnings:    0,
			wantTiles:       true,
		},
		{
			name: "dashboard with direct tiles",
			doc: map[string]interface{}{
				"name":  "Direct Dashboard",
				"tiles": []interface{}{map[string]interface{}{"name": "tile1"}},
			},
			docType:      "dashboard",
			wantName:     "Direct Dashboard",
			wantWarnings: 0,
			wantTiles:    true,
		},
		{
			name: "dashboard missing tiles warning",
			doc: map[string]interface{}{
				"name": "Empty Dashboard",
				"content": map[string]interface{}{
					"version": "1",
				},
			},
			docType:      "dashboard",
			wantName:     "Empty Dashboard",
			wantWarnings: 1, // missing tiles warning
		},
		{
			name: "dashboard missing version warning",
			doc: map[string]interface{}{
				"name": "Dashboard",
				"content": map[string]interface{}{
					"tiles": []interface{}{},
				},
			},
			docType:      "dashboard",
			wantName:     "Dashboard",
			wantWarnings: 1, // missing version warning
		},
		{
			name: "dashboard with double-nested content",
			doc: map[string]interface{}{
				"name": "Double Nested",
				"content": map[string]interface{}{
					"content": map[string]interface{}{
						"tiles":   []interface{}{},
						"version": "1",
					},
				},
			},
			docType:      "dashboard",
			wantName:     "Double Nested",
			wantWarnings: 1, // double-nested warning
		},
		{
			name: "notebook with sections",
			doc: map[string]interface{}{
				"name": "My Notebook",
				"content": map[string]interface{}{
					"sections": []interface{}{map[string]interface{}{"title": "section1"}},
				},
			},
			docType:      "notebook",
			wantName:     "My Notebook",
			wantWarnings: 0,
			wantSections: true,
		},
		{
			name: "notebook missing sections warning",
			doc: map[string]interface{}{
				"name": "Empty Notebook",
				"content": map[string]interface{}{
					"version": "1",
				},
			},
			docType:      "notebook",
			wantName:     "Empty Notebook",
			wantWarnings: 1, // missing sections warning
		},
		{
			name: "notebook with direct sections",
			doc: map[string]interface{}{
				"name":     "Direct Notebook",
				"sections": []interface{}{map[string]interface{}{"title": "section1"}},
			},
			docType:      "notebook",
			wantName:     "Direct Notebook",
			wantWarnings: 0,
			wantSections: true,
		},
		{
			name: "dashboard with no content or tiles",
			doc: map[string]interface{}{
				"name": "Broken Dashboard",
			},
			docType:      "dashboard",
			wantName:     "Broken Dashboard",
			wantWarnings: 1, // structure warning
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentData, name, description, warnings := extractDocumentContent(tt.doc, tt.docType)

			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}

			if description != tt.wantDescription {
				t.Errorf("description = %q, want %q", description, tt.wantDescription)
			}

			if len(warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tt.wantWarnings, warnings)
			}

			// Verify content is valid JSON
			var content map[string]interface{}
			if err := json.Unmarshal(contentData, &content); err != nil {
				t.Errorf("contentData is not valid JSON: %v", err)
			}

			// Check for expected content structure
			if tt.wantTiles {
				if _, ok := content["tiles"]; !ok {
					t.Error("expected tiles in content")
				}
			}
			if tt.wantSections {
				if _, ok := content["sections"]; !ok {
					t.Error("expected sections in content")
				}
			}
		})
	}
}

func TestCountDocumentItems(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		docType  string
		expected int
	}{
		{
			name:     "dashboard with 3 tiles",
			content:  `{"tiles": [{"name": "a"}, {"name": "b"}, {"name": "c"}], "version": "1"}`,
			docType:  "dashboard",
			expected: 3,
		},
		{
			name:     "dashboard with no tiles",
			content:  `{"version": "1"}`,
			docType:  "dashboard",
			expected: 0,
		},
		{
			name:     "dashboard with empty tiles",
			content:  `{"tiles": [], "version": "1"}`,
			docType:  "dashboard",
			expected: 0,
		},
		{
			name:     "notebook with 2 sections",
			content:  `{"sections": [{"title": "a"}, {"title": "b"}]}`,
			docType:  "notebook",
			expected: 2,
		},
		{
			name:     "notebook with no sections",
			content:  `{"version": "1"}`,
			docType:  "notebook",
			expected: 0,
		},
		{
			name:     "invalid JSON",
			content:  `{invalid}`,
			docType:  "dashboard",
			expected: 0,
		},
		{
			name:     "tiles is not an array",
			content:  `{"tiles": "not-an-array"}`,
			docType:  "dashboard",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countDocumentItems([]byte(tt.content), tt.docType)
			if result != tt.expected {
				t.Errorf("countDocumentItems() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestItemName(t *testing.T) {
	tests := []struct {
		docType  string
		expected string
	}{
		{"dashboard", "tiles"},
		{"notebook", "sections"},
		{"other", "sections"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.docType, func(t *testing.T) {
			result := itemName(tt.docType)
			if result != tt.expected {
				t.Errorf("itemName(%q) = %q, want %q", tt.docType, result, tt.expected)
			}
		})
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"dashboard", "Dashboard"},
		{"notebook", "Notebook"},
		{"tiles", "Tiles"},
		{"sections", "Sections"},
		{"a", "A"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := capitalize(tt.input)
			if result != tt.expected {
				t.Errorf("capitalize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShowJSONDiff(t *testing.T) {
	tests := []struct {
		name         string
		oldData      string
		newData      string
		resourceType string
		wantContains []string
	}{
		{
			name:         "simple change",
			oldData:      `{"name": "old"}`,
			newData:      `{"name": "new"}`,
			resourceType: "dashboard",
			wantContains: []string{"--- existing dashboard", "+++ new dashboard", "old", "new"},
		},
		{
			name:         "no changes",
			oldData:      `{"name": "same"}`,
			newData:      `{"name": "same"}`,
			resourceType: "notebook",
			wantContains: []string{"(no changes)"},
		},
		{
			name:         "addition",
			oldData:      `{}`,
			newData:      `{"key": "value"}`,
			resourceType: "dashboard",
			wantContains: []string{"+"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			showJSONDiff([]byte(tt.oldData), []byte(tt.newData), tt.resourceType)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q, got: %s", want, output)
				}
			}
		})
	}
}

func TestDocumentURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		docType  string
		id       string
		expected string
	}{
		{
			name:     "dashboard URL",
			baseURL:  "https://abc12345.apps.dynatrace.com",
			docType:  "dashboard",
			id:       "doc-123",
			expected: "https://abc12345.apps.dynatrace.com/ui/document/v0/#/dashboards/doc-123",
		},
		{
			name:     "notebook URL",
			baseURL:  "https://abc12345.apps.dynatrace.com",
			docType:  "notebook",
			id:       "nb-456",
			expected: "https://abc12345.apps.dynatrace.com/ui/document/v0/#/notebooks/nb-456",
		},
		{
			name:     "other document type URL",
			baseURL:  "https://tenant.apps.dynatrace.com",
			docType:  "report",
			id:       "rpt-789",
			expected: "https://tenant.apps.dynatrace.com/ui/document/v0/#/reports/rpt-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create applier with just the baseURL (no real client needed for this test)
			a := &Applier{baseURL: tt.baseURL}

			result := a.documentURL(tt.docType, tt.id)
			if result != tt.expected {
				t.Errorf("documentURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDetectResourceTypeEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ResourceType
		wantErr  bool
	}{
		{
			name:     "invalid JSON",
			input:    `{not valid json}`,
			expected: ResourceUnknown,
			wantErr:  true,
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: ResourceUnknown,
			wantErr:  true,
		},
		{
			name: "notebook with type field",
			input: `{
				"type": "notebook",
				"content": {"sections": []}
			}`,
			expected: ResourceNotebook,
			wantErr:  false,
		},
		{
			name: "SLO with sliReference",
			input: `{
				"name": "Test SLO",
				"criteria": {"threshold": 95},
				"sliReference": {"id": "sli-123"}
			}`,
			expected: ResourceSLO,
			wantErr:  false,
		},
		{
			name: "SLO minimal (criteria and name only)",
			input: `{
				"name": "Minimal SLO",
				"criteria": {"threshold": 99}
			}`,
			expected: ResourceSLO,
			wantErr:  false,
		},
		{
			name: "settings with camelCase schemaId",
			input: `{
				"schemaId": "builtin:alerting.profile",
				"scope": "environment",
				"value": {"enabled": true}
			}`,
			expected: ResourceSettings,
			wantErr:  false,
		},
		{
			name: "settings with lowercase schemaid",
			input: `{
				"schemaid": "builtin:alerting.profile",
				"scope": "environment",
				"value": {"enabled": true}
			}`,
			expected: ResourceSettings,
			wantErr:  false,
		},
		{
			name: "metadata only defaults to dashboard",
			input: `{
				"metadata": {"name": "test"}
			}`,
			expected: ResourceDashboard,
			wantErr:  false,
		},
		{
			name: "metadata with notebook type",
			input: `{
				"metadata": {"name": "test"},
				"type": "notebook"
			}`,
			expected: ResourceNotebook,
			wantErr:  false,
		},
		{
			name: "tasks without trigger is not workflow",
			input: `{
				"tasks": [{"name": "test"}]
			}`,
			expected: ResourceUnknown,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := detectResourceType([]byte(tt.input))

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
