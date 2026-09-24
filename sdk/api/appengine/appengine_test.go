package appengine

import "testing"

func TestParseFullFunctionName(t *testing.T) {
	tests := []struct {
		name             string
		fullName         string
		wantAppID        string
		wantFunctionName string
	}{
		{
			name:             "valid full name",
			fullName:         "my-app/my-function",
			wantAppID:        "my-app",
			wantFunctionName: "my-function",
		},
		{
			name:             "full name with nested path",
			fullName:         "my-app/api/v1/handler",
			wantAppID:        "my-app",
			wantFunctionName: "api/v1/handler",
		},
		{
			name:             "no separator",
			fullName:         "my-app",
			wantAppID:        "",
			wantFunctionName: "",
		},
		{
			name:             "empty string",
			fullName:         "",
			wantAppID:        "",
			wantFunctionName: "",
		},
		{
			name:             "separator at start",
			fullName:         "/my-function",
			wantAppID:        "",
			wantFunctionName: "my-function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appID, functionName := parseFullFunctionName(tt.fullName)
			if appID != tt.wantAppID {
				t.Errorf("appID = %q, want %q", appID, tt.wantAppID)
			}
			if functionName != tt.wantFunctionName {
				t.Errorf("functionName = %q, want %q", functionName, tt.wantFunctionName)
			}
		})
	}
}

// TestEscapeFunctionName pins the one id dtctl interpolates that is a path by
// design: the separators a nested name carries have to survive, while anything
// that would end a segment must not.
func TestEscapeFunctionName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain name", "my-function", "my-function"},
		{"nested name keeps its separators", "api/v1/handler", "api/v1/handler"},
		{"fragment stays inside its part", "handler#x", "handler%23x"},
		{"query stays inside its part", "api/handler?x=1", "api/handler%3Fx=1"},
		{"space", "my handler", "my%20handler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeFunctionName(tt.in); got != tt.want {
				t.Errorf("escapeFunctionName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
