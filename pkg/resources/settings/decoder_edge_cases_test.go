package settings

import (
	"encoding/base64"
	"testing"
)

// TestDecodeObjectID_PartialFields tests objectIds that don't have all 4 fields
func TestDecodeObjectID_PartialFields(t *testing.T) {
	// Create a minimal objectId with only schemaId and scopeType
	// Format: [magic 8 bytes][version 4 bytes][length][string][length][string][magic footer]
	data := []byte{
		0xbe, 0xef, 0x54, 0xde, 0x15, 0xda, 0xde, 0xad, // magic header
		0x00, 0x00, 0x00, 0x01, // version
		0x00, 0x0b, // length = 11
		'e', 'n', 'v', 'i', 'r', 'o', 'n', 'm', 'e', 'n', 't', // "environment" (schemaId)
		0x00, 0x0b, // length = 11
		'E', 'N', 'V', 'I', 'R', 'O', 'N', 'M', 'E', 'N', 'T', // "ENVIRONMENT" (scopeType)
		// No scopeId or UID
		0xbe, 0xef, 0x54, 0xde, 0x15, 0xda, 0xde, 0xad, // magic footer
	}

	objectID := base64.RawURLEncoding.EncodeToString(data)

	decoded, err := DecodeObjectID(objectID)
	if err != nil {
		t.Fatalf("DecodeObjectID failed: %v", err)
	}

	// Should have schemaId and scopeType
	if decoded.SchemaID != "environment" {
		t.Errorf("SchemaID = %q, want %q", decoded.SchemaID, "environment")
	}
	if decoded.ScopeType != "ENVIRONMENT" {
		t.Errorf("ScopeType = %q, want %q", decoded.ScopeType, "ENVIRONMENT")
	}

	// Should have empty scopeId and UID (they weren't present)
	if decoded.ScopeID != "" {
		t.Errorf("ScopeID = %q, want empty", decoded.ScopeID)
	}
	if decoded.UID != "" {
		t.Errorf("UID = %q, want empty", decoded.UID)
	}
}

// TestDecodeObjectID_ThreeFields tests objectIds with only 3 fields (no UID)
func TestDecodeObjectID_ThreeFields(t *testing.T) {
	// Create objectId with schemaId, scopeType, scopeId, but no UID
	data := []byte{
		0xbe, 0xef, 0x54, 0xde, 0x15, 0xda, 0xde, 0xad, // magic header
		0x00, 0x00, 0x00, 0x01, // version
		0x00, 0x0a, // length = 10
		't', 'e', 's', 't', '.', 's', 'c', 'h', 'e', 'm', // "test.schem" (schemaId)
		0x00, 0x0b, // length = 11
		'A', 'P', 'P', 'L', 'I', 'C', 'A', 'T', 'I', 'O', 'N', // "APPLICATION" (scopeType)
		0x00, 0x08, // length = 8
		'S', 'C', 'O', 'P', 'E', '1', '2', '3', // "SCOPE123" (scopeId)
		// No UID
		0xbe, 0xef, 0x54, 0xde, 0x15, 0xda, 0xde, 0xad, // magic footer
	}

	objectID := base64.RawURLEncoding.EncodeToString(data)

	decoded, err := DecodeObjectID(objectID)
	if err != nil {
		t.Fatalf("DecodeObjectID failed: %v", err)
	}

	// Should have first 3 fields
	if decoded.SchemaID != "test.schem" {
		t.Errorf("SchemaID = %q, want %q", decoded.SchemaID, "test.schem")
	}
	if decoded.ScopeType != "APPLICATION" {
		t.Errorf("ScopeType = %q, want %q", decoded.ScopeType, "APPLICATION")
	}
	if decoded.ScopeID != "SCOPE123" {
		t.Errorf("ScopeID = %q, want %q", decoded.ScopeID, "SCOPE123")
	}

	// Should have empty UID (it wasn't present)
	if decoded.UID != "" {
		t.Errorf("UID = %q, want empty", decoded.UID)
	}
}
