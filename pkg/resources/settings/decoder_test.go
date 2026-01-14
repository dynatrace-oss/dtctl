package settings

import (
	"testing"
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
