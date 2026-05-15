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
