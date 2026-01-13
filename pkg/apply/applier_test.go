package apply

import (
	"encoding/json"
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
			name: "settings object",
			input: `{
				"schemaId": "builtin:test",
				"scope": "environment",
				"value": {"enabled": true}
			}`,
			expected: ResourceSettings,
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
