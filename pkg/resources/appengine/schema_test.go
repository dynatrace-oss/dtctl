package appengine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSchemaFromError(t *testing.T) {
	tests := []struct {
		name       string
		errorMsg   string
		wantFields int
	}{
		{
			name:       "empty action input message",
			errorMsg:   "Action input must not be empty",
			wantFields: 0,
		},
		{
			name:       "zod-style JSON validation error",
			errorMsg:   `Invalid input: { "query": ["Required"], "timeframe": ["Required"] }`,
			wantFields: 2,
		},
		{
			name:       "no schema information",
			errorMsg:   "Internal server error occurred",
			wantFields: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := parseSchemaFromError(tt.errorMsg)

			if len(fields) != tt.wantFields {
				t.Errorf("parseSchemaFromError() got %d fields, want %d", len(fields), tt.wantFields)
			}
		})
	}
}

func TestFunctionSchema_FormatSchema(t *testing.T) {
	tests := []struct {
		name   string
		schema *FunctionSchema
		checks []string
	}{
		{
			name: "schema with fields",
			schema: &FunctionSchema{
				FunctionName: "execute-dql-query",
				AppID:        "dynatrace.automations",
				Fields: []SchemaField{
					{Name: "query", Type: "string", Required: true, Hint: "DQL query to execute"},
					{Name: "timeout", Type: "number", Required: true, Hint: "Query timeout in seconds"},
				},
			},
			checks: []string{
				"execute-dql-query",
				"dynatrace.automations",
				"query",
				"string",
				"timeout",
				"number",
			},
		},
		{
			name: "function accepts empty payload",
			schema: &FunctionSchema{
				FunctionName: "helloWorld",
				AppID:        "my.xv.wow1",
				Fields:       []SchemaField{},
				ErrorMessage: "Function accepts empty payload (no required fields)",
			},
			checks: []string{
				"helloWorld",
				"my.xv.wow1",
				"accepts an empty payload",
				"no required fields",
				"Example usage",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.schema.FormatSchema()

			for _, check := range tt.checks {
				if !strings.Contains(strings.ToLower(output), strings.ToLower(check)) {
					t.Errorf("FormatSchema() output should contain %q, got:\n%s", check, output)
				}
			}
		})
	}
}

func TestFunctionSchema_GenerateExamplePayload(t *testing.T) {
	tests := []struct {
		name     string
		schema   *FunctionSchema
		wantJSON bool
	}{
		{
			name: "schema with various field types",
			schema: &FunctionSchema{
				FunctionName: "test-function",
				AppID:        "test.app",
				Fields: []SchemaField{
					{Name: "query", Type: "string", Required: true},
					{Name: "count", Type: "number", Required: false},
				},
			},
			wantJSON: true,
		},
		{
			name: "empty schema",
			schema: &FunctionSchema{
				FunctionName: "empty-function",
				AppID:        "test.app",
				Fields:       []SchemaField{},
			},
			wantJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.schema.GenerateExamplePayload()

			if tt.wantJSON {
				if !strings.HasPrefix(strings.TrimSpace(payload), "{") {
					t.Errorf("GenerateExamplePayload() should start with '{'")
				}
			}
		})
	}
}

func TestParseConstraintViolations(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		wantFieldCount int
		wantFieldNames []string
	}{
		{
			name: "nested constraintViolations with specific fields",
			responseBody: `{
				"body": {
					"error": {
						"code": 400,
						"message": "Invalid request structure",
						"details": {
							"constraintViolations": [
								{
									"path": "method",
									"message": "Required"
								},
								{
									"path": "path",
									"message": "Required"
								}
							]
						}
					}
				}
			}`,
			wantFieldCount: 2,
			wantFieldNames: []string{"method", "path"},
		},
		{
			name: "root-level validation error (empty path)",
			responseBody: `{
				"body": {
					"error": {
						"code": 400,
						"message": "Invalid request structure",
						"details": {
							"constraintViolations": [
								{
									"path": "",
									"message": "Required"
								}
							]
						}
					}
				}
			}`,
			wantFieldCount: 1,
			wantFieldNames: []string{"_root_"},
		},
		{
			name: "direct constraintViolations array",
			responseBody: `{
				"constraintViolations": [
					{
						"path": "query",
						"message": "Must be a valid string"
					}
				]
			}`,
			wantFieldCount: 1,
			wantFieldNames: []string{"query"},
		},
		{
			name: "error.details.constraintViolations format",
			responseBody: `{
				"error": {
					"details": {
						"constraintViolations": [
							{
								"path": "connectionId",
								"message": "Required"
							},
							{
								"path": "channel",
								"message": "Must be defined"
							}
						]
					}
				}
			}`,
			wantFieldCount: 2,
			wantFieldNames: []string{"connectionId", "channel"},
		},
		{
			name: "nested path with dot notation",
			responseBody: `{
				"constraintViolations": [
					{
						"path": "data.field",
						"message": "Required"
					}
				]
			}`,
			wantFieldCount: 1,
			wantFieldNames: []string{"data"},
		},
		{
			name: "type inference from message",
			responseBody: `{
				"constraintViolations": [
					{
						"path": "count",
						"message": "Expected number, received string"
					},
					{
						"path": "config",
						"message": "Expected object"
					},
					{
						"path": "items",
						"message": "Expected array"
					}
				]
			}`,
			wantFieldCount: 3,
			wantFieldNames: []string{"count", "config", "items"},
		},
		{
			name:           "no constraintViolations",
			responseBody:   `{"message": "Success"}`,
			wantFieldCount: 0,
			wantFieldNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyData map[string]interface{}
			if err := json.Unmarshal([]byte(tt.responseBody), &bodyData); err != nil {
				t.Fatalf("Failed to unmarshal test data: %v", err)
			}

			fields := parseConstraintViolations(bodyData)

			if len(fields) != tt.wantFieldCount {
				t.Errorf("parseConstraintViolations() got %d fields, want %d", len(fields), tt.wantFieldCount)
			}

			// Check field names
			for i, wantName := range tt.wantFieldNames {
				if i >= len(fields) {
					t.Errorf("Missing expected field %s", wantName)
					continue
				}
				if fields[i].Name != wantName {
					t.Errorf("Field[%d].Name = %s, want %s", i, fields[i].Name, wantName)
				}
			}

			// Additional validation for type inference test
			if tt.name == "type inference from message" {
				if len(fields) >= 1 && fields[0].Type != "number" {
					t.Errorf("Field 'count' should have type 'number', got '%s'", fields[0].Type)
				}
				if len(fields) >= 2 && fields[1].Type != "object" {
					t.Errorf("Field 'config' should have type 'object', got '%s'", fields[1].Type)
				}
				if len(fields) >= 3 && fields[2].Type != "array" {
					t.Errorf("Field 'items' should have type 'array', got '%s'", fields[2].Type)
				}
			}
		})
	}
}

func TestHasRootLevelValidation(t *testing.T) {
	tests := []struct {
		name   string
		fields []SchemaField
		want   bool
	}{
		{
			name: "only root validation",
			fields: []SchemaField{
				{Name: "_root_", Type: "unknown", Required: true},
			},
			want: true,
		},
		{
			name: "multiple root validations",
			fields: []SchemaField{
				{Name: "_root_", Type: "unknown", Required: true},
				{Name: "_root_", Type: "unknown", Required: true},
			},
			want: true,
		},
		{
			name: "mixed root and field validation",
			fields: []SchemaField{
				{Name: "_root_", Type: "unknown", Required: true},
				{Name: "method", Type: "string", Required: true},
			},
			want: false,
		},
		{
			name: "only field validations",
			fields: []SchemaField{
				{Name: "method", Type: "string", Required: true},
				{Name: "path", Type: "string", Required: true},
			},
			want: false,
		},
		{
			name:   "empty fields",
			fields: []SchemaField{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasRootLevelValidation(tt.fields)
			if got != tt.want {
				t.Errorf("hasRootLevelValidation() = %v, want %v", got, tt.want)
			}
		})
	}
}
