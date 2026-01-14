package settings

import (
	"encoding/base64"
	"fmt"
)

// DecodedObjectID contains the decoded components of a settings object ID
type DecodedObjectID struct {
	SchemaID  string
	ScopeType string
	ScopeID   string
	UID       string
}

// DecodeObjectID decodes a Dynatrace settings object ID into its components.
//
// The objectId is a RawURLEncoding base64-encoded (URL-safe, no padding) binary structure with the format:
//
//	[8-byte magic header][4-byte version][length:uint16][string]...
//
// Where the strings are: schemaId, scopeType, scopeId, uid
//
// Example:
//
//	Input:  "vu9U3hXa3q0AAAABABRidWlsdGluOnJ1bS53ZWIubmFtZQAL..."
//	Output: SchemaID="builtin:rum.web.name", ScopeType="APPLICATION",
//	        ScopeID="5C9B9BB1B4546855", UID="e4c6742f-47f9-3b14-8348-59cbe32f7980"
func DecodeObjectID(objectID string) (*DecodedObjectID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	// The structure starts with an 8-byte magic header and 4-byte version
	// At offset 12 (0x0c), we have length-prefixed strings
	const headerSize = 12
	if len(decoded) < headerSize {
		return nil, fmt.Errorf("object ID too short (len=%d)", len(decoded))
	}

	result := &DecodedObjectID{}
	offset := headerSize

	// Read length-prefixed strings: schemaId, scopeType, scopeId, uid
	// Note: Not all objectIds have all fields (e.g., environment-scoped settings may lack scopeId/uid)
	fields := []*string{
		&result.SchemaID,
		&result.ScopeType,
		&result.ScopeID,
		&result.UID,
	}

	for _, field := range fields {
		// Try to read the next field, but don't fail if we reach the end
		value, newOffset, err := readLengthPrefixedString(decoded, offset)
		if err != nil {
			// If we can't read more fields, that's okay - some objectIds are shorter
			// Just return what we've got so far
			break
		}
		*field = value
		offset = newOffset
	}

	return result, nil
}

// readLengthPrefixedString reads a big-endian uint16 length followed by a UTF-8 string
func readLengthPrefixedString(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", offset, fmt.Errorf("insufficient data for length at offset %d", offset)
	}

	// Read big-endian uint16 length
	length := int(data[offset])<<8 | int(data[offset+1])
	offset += 2

	if offset+length > len(data) {
		return "", offset, fmt.Errorf("insufficient data for string of length %d at offset %d", length, offset)
	}

	value := string(data[offset : offset+length])
	offset += length

	return value, offset, nil
}

// FormattedScope returns the scope in "TYPE-ID" format (e.g., "APPLICATION-5C9B9BB1B4546855")
func (d *DecodedObjectID) FormattedScope() string {
	if d.ScopeType == "" && d.ScopeID == "" {
		return ""
	}
	return d.ScopeType + "-" + d.ScopeID
}
