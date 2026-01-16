package settings

import (
	"testing"
	"time"
)

func TestDecodeObjectID(t *testing.T) {
	tests := []struct {
		name     string
		objectID string
		want     *DecodedObjectID
		wantErr  bool
	}{
		{
			name:     "valid RUM web name setting",
			objectID: "vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQALQVBQTElDQVRJT04AEDVDOUI5QkIxQjQ1NDY4NTUAJGU0YzY3NDJmLTQ3ZjktM2IxNC04MzQ4LTU5Y2JlMzJmNzk4ML7vVN4V2t6t",
			want: &DecodedObjectID{
				SchemaID:  "builtin:rum.web.name",
				ScopeType: "APPLICATION",
				ScopeID:   "5C9B9BB1B4546855",
				UID:       "e4c6742f-47f9-3b14-8348-59cbe32f7980",
			},
			wantErr: false,
		},
		{
			name:     "invalid base64",
			objectID: "not-valid-base64!",
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "too short",
			objectID: "YWJj", // "abc" in base64, only 3 bytes
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeObjectID(tt.objectID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeObjectID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.SchemaID != tt.want.SchemaID {
				t.Errorf("SchemaID = %v, want %v", got.SchemaID, tt.want.SchemaID)
			}
			if got.ScopeType != tt.want.ScopeType {
				t.Errorf("ScopeType = %v, want %v", got.ScopeType, tt.want.ScopeType)
			}
			if got.ScopeID != tt.want.ScopeID {
				t.Errorf("ScopeID = %v, want %v", got.ScopeID, tt.want.ScopeID)
			}
			if got.UID != tt.want.UID {
				t.Errorf("UID = %v, want %v", got.UID, tt.want.UID)
			}
		})
	}
}

func TestDecodedObjectID_FormattedScope(t *testing.T) {
	tests := []struct {
		name string
		d    *DecodedObjectID
		want string
	}{
		{
			name: "application scope",
			d: &DecodedObjectID{
				ScopeType: "APPLICATION",
				ScopeID:   "5C9B9BB1B4546855",
			},
			want: "APPLICATION-5C9B9BB1B4546855",
		},
		{
			name: "empty scope",
			d: &DecodedObjectID{
				ScopeType: "",
				ScopeID:   "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.FormattedScope(); got != tt.want {
				t.Errorf("FormattedScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodedObjectID_FormattedScope_PartialData(t *testing.T) {
	// Test edge cases with partial data
	tests := []struct {
		name string
		d    *DecodedObjectID
		want string
	}{
		{
			name: "only scope type",
			d: &DecodedObjectID{
				ScopeType: "HOST",
				ScopeID:   "",
			},
			want: "HOST-",
		},
		{
			name: "only scope ID",
			d: &DecodedObjectID{
				ScopeType: "",
				ScopeID:   "12345",
			},
			want: "-12345",
		},
		{
			name: "environment scope",
			d: &DecodedObjectID{
				SchemaID:  "builtin:alerting.profile",
				ScopeType: "environment",
				ScopeID:   "",
			},
			want: "environment-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.FormattedScope(); got != tt.want {
				t.Errorf("FormattedScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadLengthPrefixedString(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		offset  int
		want    string
		wantErr bool
	}{
		{
			name:    "valid string",
			data:    []byte{0x00, 0x05, 'h', 'e', 'l', 'l', 'o'},
			offset:  0,
			want:    "hello",
			wantErr: false,
		},
		{
			name:    "empty string",
			data:    []byte{0x00, 0x00},
			offset:  0,
			want:    "",
			wantErr: false,
		},
		{
			name:    "insufficient data for length",
			data:    []byte{0x00},
			offset:  0,
			wantErr: true,
		},
		{
			name:    "insufficient data for string",
			data:    []byte{0x00, 0x10, 'a', 'b', 'c'}, // Claims 16 bytes but only has 3
			offset:  0,
			wantErr: true,
		},
		{
			name:    "offset past end",
			data:    []byte{0x00, 0x05, 'h', 'e', 'l', 'l', 'o'},
			offset:  10,
			wantErr: true,
		},
		{
			name:    "read from middle",
			data:    []byte{0xFF, 0xFF, 0x00, 0x03, 'a', 'b', 'c'},
			offset:  2,
			want:    "abc",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := readLengthPrefixedString(tt.data, tt.offset)
			if (err != nil) != tt.wantErr {
				t.Errorf("readLengthPrefixedString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("readLengthPrefixedString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractUUIDv1Timestamp(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid v1 UUID",
			uuid:    "9bea8bdc-ae5f-11f0-8001-adbc4e9fd7b3",
			wantErr: false,
		},
		{
			name:    "valid v1 UUID without dashes",
			uuid:    "9bea8bdcae5f11f08001adbc4e9fd7b3",
			wantErr: false,
		},
		{
			name:    "invalid UUID - too short",
			uuid:    "9bea8bdc-ae5f-11f0",
			wantErr: true,
			errMsg:  "invalid UUID length",
		},
		{
			name:    "invalid UUID - bad hex",
			uuid:    "ZZZZ8bdc-ae5f-11f0-8001-adbc4e9fd7b3",
			wantErr: true,
			errMsg:  "invalid UUID hex",
		},
		{
			name:    "v4 UUID (not v1)",
			uuid:    "550e8400-e29b-41d4-a716-446655440000", // Version 4
			wantErr: true,
			errMsg:  "not a v1 UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractUUIDv1Timestamp(tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractUUIDv1Timestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("extractUUIDv1Timestamp() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDecodeVersion(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		wantUID      string
		wantRevision string
		wantTime     time.Time
		wantErr      bool
	}{
		{
			name:         "valid SLO version",
			version:      "vu9U3hXY3q0ATAAkMDAwY2YzZGEtMDdkNC0zZmMxLTk0MzUtZTkwNmFlYTY0MGExACQ5YmVhOGJkYy1hZTVmLTExZjAtODAwMS1hZGJjNGU5ZmQ3YjO-71TeFdjerQ",
			wantUID:      "000cf3da-07d4-3fc1-9435-e906aea640a1",
			wantRevision: "9bea8bdc-ae5f-11f0-8001-adbc4e9fd7b3",
			wantTime:     time.Date(2025, 10, 21, 9, 23, 30, 945122800, time.UTC),
			wantErr:      false,
		},
		{
			name:    "invalid base64",
			version: "not-valid!",
			wantErr: true,
		},
		{
			name:    "too short",
			version: "YWJj",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.UID != tt.wantUID {
				t.Errorf("UID = %v, want %v", got.UID, tt.wantUID)
			}
			if got.RevisionUUID != tt.wantRevision {
				t.Errorf("RevisionUUID = %v, want %v", got.RevisionUUID, tt.wantRevision)
			}
			if got.Timestamp == nil {
				t.Error("Timestamp is nil, expected a value")
			} else if !got.Timestamp.Equal(tt.wantTime) {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.wantTime)
			}
		})
	}
}
