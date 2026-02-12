package azureconnection

import (
	"strings"
	"testing"
)

// TestValueString_MasksSecret verifies that the String() method masks the client secret
func TestValueString_MasksSecret(t *testing.T) {
	tests := []struct {
		name     string
		value    Value
		wantSubstr string
		notWantSubstr string
	}{
		{
			name: "masks non-empty client secret",
			value: Value{
				Name: "test-connection",
				Type: "client-secret",
				ClientSecret: &ClientSecretCredential{
					ApplicationID: "app-123",
					DirectoryID:   "dir-456",
					ClientSecret:  "super-secret-value",
					Consumers:     []string{"consumer1"},
				},
			},
			wantSubstr: "secret=[REDACTED]",
			notWantSubstr: "super-secret-value",
		},
		{
			name: "shows empty string for empty secret",
			value: Value{
				Name: "test-connection",
				Type: "client-secret",
				ClientSecret: &ClientSecretCredential{
					ApplicationID: "app-123",
					DirectoryID:   "dir-456",
					ClientSecret:  "",
					Consumers:     []string{"consumer1"},
				},
			},
			wantSubstr: "secret=",
			notWantSubstr: "[REDACTED]",
		},
		{
			name: "federated identity credential without secret",
			value: Value{
				Name: "test-connection",
				Type: "federated-identity",
				FederatedIdentityCredential: &FederatedIdentityCredential{
					Consumers: []string{"consumer1", "consumer2"},
				},
			},
			wantSubstr: "name=test-connection type=federated-identity",
			notWantSubstr: "secret=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.value.String()
			
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Value.String() = %v, want substring %v", got, tt.wantSubstr)
			}
			
			if tt.notWantSubstr != "" && strings.Contains(got, tt.notWantSubstr) {
				t.Errorf("Value.String() = %v, should not contain %v", got, tt.notWantSubstr)
			}
		})
	}
}
