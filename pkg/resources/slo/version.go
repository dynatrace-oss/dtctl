package slo

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// DecodeVersionTimestamp extracts the last-modified timestamp from a Dynatrace SLO version string.
// The version is a RawURLEncoding base64-encoded binary structure containing a v1 UUID revision.
// Returns nil if the version cannot be decoded or does not contain a v1 UUID.
func DecodeVersionTimestamp(version string) *time.Time {
	decoded, err := base64.RawURLEncoding.DecodeString(version)
	if err != nil {
		return nil
	}

	// Version structure: 8-byte magic header + 2-byte field + length-prefixed strings (UID, revisionUUID)
	const headerSize = 10
	if len(decoded) < headerSize {
		return nil
	}

	offset := headerSize

	// Skip UID string
	_, offset, err = readLPS(decoded, offset)
	if err != nil {
		return nil
	}

	// Read revision UUID
	revisionUUID, _, err := readLPS(decoded, offset)
	if err != nil {
		return nil
	}

	ts, err := uuidV1Timestamp(revisionUUID)
	if err != nil {
		return nil
	}
	return &ts
}

// readLPS reads a big-endian uint16 length followed by a UTF-8 string.
func readLPS(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", offset, fmt.Errorf("insufficient data")
	}
	length := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	if offset+length > len(data) {
		return "", offset, fmt.Errorf("insufficient data for string")
	}
	return string(data[offset : offset+length]), offset + length, nil
}

// uuidV1Timestamp extracts the timestamp from a v1 UUID string.
func uuidV1Timestamp(uuidStr string) (time.Time, error) {
	uuidStr = strings.ReplaceAll(uuidStr, "-", "")
	if len(uuidStr) != 32 {
		return time.Time{}, fmt.Errorf("invalid UUID length")
	}
	b, err := hex.DecodeString(uuidStr)
	if err != nil {
		return time.Time{}, err
	}
	if (b[6]>>4)&0x0F != 1 {
		return time.Time{}, fmt.Errorf("not a v1 UUID")
	}
	timeLow := uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
	timeMid := uint64(b[4])<<8 | uint64(b[5])
	timeHi := uint64(b[6]&0x0F)<<8 | uint64(b[7])
	t := timeLow | (timeMid << 32) | (timeHi << 48)
	const gregorianToUnix = 122192928000000000
	return time.Unix(0, int64(t-gregorianToUnix)*100).UTC(), nil
}
