package template

import (
	"testing"
)

func TestParseSetFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		want    map[string]interface{}
		wantErr bool
	}{
		{
			name:  "single flag",
			flags: []string{"host=h-123"},
			want:  map[string]interface{}{"host": "h-123"},
		},
		{
			name:  "multiple flags",
			flags: []string{"host=h-123", "timerange=1h", "limit=100"},
			want: map[string]interface{}{
				"host":      "h-123",
				"timerange": "1h",
				"limit":     "100",
			},
		},
		{
			name:  "value with spaces",
			flags: []string{"message=hello world"},
			want:  map[string]interface{}{"message": "hello world"},
		},
		{
			name:  "value with equals sign",
			flags: []string{"filter=status=ERROR"},
			want:  map[string]interface{}{"filter": "status=ERROR"},
		},
		{
			name:    "invalid format - no equals",
			flags:   []string{"invalid"},
			wantErr: true,
		},
		{
			name:    "invalid format - empty key",
			flags:   []string{"=value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSetFlags(tt.flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSetFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				for k, v := range tt.want {
					if got[k] != v {
						t.Errorf("ParseSetFlags() got[%s] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]interface{}
		want     string
		wantErr  bool
	}{
		{
			name:     "simple substitution",
			template: "fetch logs | filter host = \"{{.host}}\"",
			vars:     map[string]interface{}{"host": "h-123"},
			want:     "fetch logs | filter host = \"h-123\"",
		},
		{
			name:     "multiple variables",
			template: "fetch logs | filter host = \"{{.host}}\" | limit {{.limit}}",
			vars:     map[string]interface{}{"host": "h-123", "limit": "100"},
			want:     "fetch logs | filter host = \"h-123\" | limit 100",
		},
		{
			name:     "default value used when variable missing",
			template: "fetch logs | filter timestamp > now() - {{.timerange | default \"1h\"}}",
			vars:     map[string]interface{}{},
			want:     "fetch logs | filter timestamp > now() - 1h",
		},
		{
			name:     "default value not used when variable provided",
			template: "fetch logs | limit {{.limit | default 100}}",
			vars:     map[string]interface{}{"limit": "50"},
			want:     "fetch logs | limit 50",
		},
		{
			name:     "no variables",
			template: "fetch logs | limit 10",
			vars:     map[string]interface{}{},
			want:     "fetch logs | limit 10",
		},
		{
			name:     "missing variable outputs no value marker",
			template: "fetch logs | filter host = \"{{.host}}\"",
			vars:     map[string]interface{}{},
			want:     "fetch logs | filter host = \"<no value>\"",
		},
		{
			name:     "invalid template syntax",
			template: "fetch logs | filter {{.unclosed",
			vars:     map[string]interface{}{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTemplate(tt.template, tt.vars)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("RenderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContainsTemplate(t *testing.T) {
	tests := []struct {
		name string
		str  string
		want bool
	}{
		{
			name: "contains template",
			str:  "fetch logs | filter host = \"{{.host}}\"",
			want: true,
		},
		{
			name: "no template",
			str:  "fetch logs | limit 10",
			want: false,
		},
		{
			name: "only opening braces",
			str:  "fetch logs {{",
			want: false,
		},
		{
			name: "only closing braces",
			str:  "fetch logs }}",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsTemplate(tt.str); got != tt.want {
				t.Errorf("ContainsTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantVars []string
		wantErr  bool
	}{
		{
			name:     "single variable",
			template: "fetch logs | filter host = \"{{.host}}\"",
			wantVars: []string{"host"},
		},
		{
			name:     "multiple variables",
			template: "fetch logs | filter host = \"{{.host}}\" and status = \"{{.status}}\"",
			wantVars: []string{"host", "status"},
		},
		{
			name:     "variable with default",
			template: "fetch logs | limit {{.limit | default 100}}",
			wantVars: []string{"limit"},
		},
		{
			name:     "no variables",
			template: "fetch logs | limit 10",
			wantVars: []string{},
		},
		{
			name:     "invalid syntax",
			template: "fetch logs | filter {{.unclosed",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVars, err := ValidateTemplate(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(gotVars) != len(tt.wantVars) {
					t.Errorf("ValidateTemplate() found %d vars, want %d", len(gotVars), len(tt.wantVars))
				}
				// Check all expected vars are present
				for _, wantVar := range tt.wantVars {
					found := false
					for _, gotVar := range gotVars {
						if gotVar == wantVar {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("ValidateTemplate() missing variable %q", wantVar)
					}
				}
			}
		})
	}
}
