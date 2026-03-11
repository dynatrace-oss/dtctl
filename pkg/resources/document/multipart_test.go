package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
)

// TestParseMultipartDocument_LargeContent tests that large multipart responses are not truncated
func TestParseMultipartDocument_LargeContent(t *testing.T) {
	// Create a large content payload (>10KB)
	largeContent := make(map[string]interface{})
	tiles := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		tiles[i] = map[string]interface{}{
			"id":    fmt.Sprintf("tile-%d", i),
			"name":  fmt.Sprintf("Tile %d with some additional text to make it larger", i),
			"query": fmt.Sprintf("fetch logs | filter status == 'ERROR' | filter id == %d | summarize count = count()", i),
		}
	}
	largeContent["tiles"] = tiles
	largeContent["metadata"] = map[string]interface{}{
		"name":        "Test Dashboard",
		"description": "This is a test dashboard with lots of tiles to test large content handling",
	}

	contentBytes, err := json.Marshal(largeContent)
	if err != nil {
		t.Fatalf("Failed to marshal test content: %v", err)
	}

	t.Logf("Test content size: %d bytes", len(contentBytes))

	// Create a multipart response
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add metadata part
	metadata := Document{
		ID:      "test-id-123",
		Name:    "Test Dashboard",
		Type:    "dashboard",
		Version: 1,
	}
	metadataBytes, _ := json.Marshal(metadata)

	metadataPart, err := writer.CreateFormField("metadata")
	if err != nil {
		t.Fatalf("Failed to create metadata part: %v", err)
	}
	if _, err := metadataPart.Write(metadataBytes); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	// Add content part
	contentPart, err := writer.CreateFormField("content")
	if err != nil {
		t.Fatalf("Failed to create content part: %v", err)
	}
	if _, err := contentPart.Write(contentBytes); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}

	_ = writer.Close()

	// Create a mock resty response
	resp := &resty.Response{
		RawResponse: &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"Content-Type": []string{fmt.Sprintf("multipart/form-data; boundary=%s", writer.Boundary())},
			},
			Body: io.NopCloser(&buf),
		},
	}
	resp.SetBody(buf.Bytes())

	// Parse the multipart document
	doc, err := ParseMultipartDocument(resp)
	if err != nil {
		t.Fatalf("ParseMultipartDocument failed: %v", err)
	}

	// Verify the document was parsed correctly
	if doc.ID != "test-id-123" {
		t.Errorf("ID mismatch: got %s, want test-id-123", doc.ID)
	}
	if doc.Name != "Test Dashboard" {
		t.Errorf("Name mismatch: got %s, want Test Dashboard", doc.Name)
	}

	// CRITICAL: Verify content was not truncated
	if len(doc.Content) == 0 {
		t.Fatal("Content is empty - this indicates truncation!")
	}

	if len(doc.Content) != len(contentBytes) {
		t.Errorf("Content size mismatch: got %d bytes, want %d bytes - content was truncated!",
			len(doc.Content), len(contentBytes))
	}

	// Verify content is valid JSON
	var parsedContent map[string]interface{}
	if err := json.Unmarshal(doc.Content, &parsedContent); err != nil {
		t.Fatalf("Content is not valid JSON: %v", err)
	}

	// Verify tiles are present and count matches
	tilesArray, ok := parsedContent["tiles"].([]interface{})
	if !ok {
		t.Fatal("Content does not have tiles array")
	}
	if len(tilesArray) != 100 {
		t.Errorf("Tiles count mismatch: got %d, want 100 - content was truncated!", len(tilesArray))
	}

	t.Logf("✓ Large content parsed successfully: %d bytes, %d tiles", len(doc.Content), len(tilesArray))
}

// TestParseMultipartDocument_Empty tests handling of empty content
func TestParseMultipartDocument_Empty(t *testing.T) {
	// Create a multipart response with empty content
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add metadata part
	metadata := Document{
		ID:      "test-id-empty",
		Name:    "Empty Dashboard",
		Type:    "dashboard",
		Version: 1,
	}
	metadataBytes, _ := json.Marshal(metadata)

	metadataPart, err := writer.CreateFormField("metadata")
	if err != nil {
		t.Fatalf("Failed to create metadata part: %v", err)
	}
	if _, err := metadataPart.Write(metadataBytes); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	// Add empty content part
	contentPart, err := writer.CreateFormField("content")
	if err != nil {
		t.Fatalf("Failed to create content part: %v", err)
	}
	if _, err := contentPart.Write([]byte("{}")); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}

	_ = writer.Close()

	// Create a mock resty response
	resp := &resty.Response{
		RawResponse: &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"Content-Type": []string{fmt.Sprintf("multipart/form-data; boundary=%s", writer.Boundary())},
			},
			Body: io.NopCloser(&buf),
		},
	}
	resp.SetBody(buf.Bytes())

	// Parse the multipart document
	doc, err := ParseMultipartDocument(resp)
	if err != nil {
		t.Fatalf("ParseMultipartDocument failed: %v", err)
	}

	// Verify metadata was parsed
	if doc.ID != "test-id-empty" {
		t.Errorf("ID mismatch: got %s, want test-id-empty", doc.ID)
	}

	// Content should be present (even if minimal)
	if len(doc.Content) == 0 {
		t.Error("Content should not be empty")
	}
}

// TestParseMultipartDocument_MissingBoundary tests error handling for missing boundary
func TestParseMultipartDocument_MissingBoundary(t *testing.T) {
	resp := &resty.Response{
		RawResponse: &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"Content-Type": []string{"multipart/form-data"},
			},
		},
	}

	_, err := ParseMultipartDocument(resp)
	if err == nil {
		t.Fatal("Expected error for missing boundary, got nil")
	}
	if !strings.Contains(err.Error(), "boundary") {
		t.Errorf("Error should mention 'boundary', got: %v", err)
	}
}

// TestParseMultipartDocument_MissingMetadata tests error handling for missing metadata
func TestParseMultipartDocument_MissingMetadata(t *testing.T) {
	// Create a multipart response without metadata part
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Only add content part, no metadata
	contentPart, err := writer.CreateFormField("content")
	if err != nil {
		t.Fatalf("Failed to create content part: %v", err)
	}
	if _, err := contentPart.Write([]byte(`{"test": "data"}`)); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}

	_ = writer.Close()

	resp := &resty.Response{
		RawResponse: &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"Content-Type": []string{fmt.Sprintf("multipart/form-data; boundary=%s", writer.Boundary())},
			},
		},
	}
	resp.SetBody(buf.Bytes())

	_, err = ParseMultipartDocument(resp)
	if err == nil {
		t.Fatal("Expected error for missing metadata, got nil")
	}
	if !strings.Contains(err.Error(), "metadata") {
		t.Errorf("Error should mention 'metadata', got: %v", err)
	}
}

// TestDocumentMetadata_VersionAsString tests that version can be unmarshaled from a string
// (some API versions return version as "1" instead of 1).
func TestDocumentMetadata_VersionAsString(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    int
		wantErr bool
	}{
		{
			name: "version as int",
			json: `{"id":"doc-1","name":"Test","type":"dashboard","version":3,"owner":"user-1","isPrivate":false,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z","lastModifiedBy":"user-1","lastModifiedTime":"2025-01-01T00:00:00Z"}}`,
			want: 3,
		},
		{
			name: "version as string",
			json: `{"id":"doc-1","name":"Test","type":"dashboard","version":"5","owner":"user-1","isPrivate":false,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z","lastModifiedBy":"user-1","lastModifiedTime":"2025-01-01T00:00:00Z"}}`,
			want: 5,
		},
		{
			name: "version as zero",
			json: `{"id":"doc-1","name":"Test","type":"dashboard","version":0,"owner":"user-1","isPrivate":false,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z","lastModifiedBy":"user-1","lastModifiedTime":"2025-01-01T00:00:00Z"}}`,
			want: 0,
		},
		{
			name: "version as string zero",
			json: `{"id":"doc-1","name":"Test","type":"dashboard","version":"0","owner":"user-1","isPrivate":false,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z","lastModifiedBy":"user-1","lastModifiedTime":"2025-01-01T00:00:00Z"}}`,
			want: 0,
		},
		{
			name:    "version as non-numeric string",
			json:    `{"id":"doc-1","name":"Test","type":"dashboard","version":"abc","owner":"user-1","isPrivate":false,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z","lastModifiedBy":"user-1","lastModifiedTime":"2025-01-01T00:00:00Z"}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m DocumentMetadata
			err := json.Unmarshal([]byte(tt.json), &m)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Version != tt.want {
				t.Errorf("Version = %d, want %d", m.Version, tt.want)
			}
		})
	}
}

// TestDocument_VersionAsString tests that Document can unmarshal version from string.
func TestDocument_VersionAsString(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int
	}{
		{
			name: "version as int",
			json: `{"id":"doc-1","name":"Test","type":"dashboard","version":2,"owner":"user-1","isPrivate":false}`,
			want: 2,
		},
		{
			name: "version as string",
			json: `{"id":"doc-1","name":"Test","type":"dashboard","version":"7","owner":"user-1","isPrivate":false}`,
			want: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Document
			if err := json.Unmarshal([]byte(tt.json), &d); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.Version != tt.want {
				t.Errorf("Version = %d, want %d", d.Version, tt.want)
			}
		})
	}
}

// TestDocumentList_VersionAsString tests that a list response with string versions parses correctly.
func TestDocumentList_VersionAsString(t *testing.T) {
	listJSON := `{
		"documents": [
			{"id":"doc-1","name":"Dashboard 1","type":"dashboard","version":"3","owner":"user-1","isPrivate":false,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z","lastModifiedBy":"user-1","lastModifiedTime":"2025-01-01T00:00:00Z"}},
			{"id":"doc-2","name":"Notebook 1","type":"notebook","version":5,"owner":"user-2","isPrivate":true,"modificationInfo":{"createdBy":"user-2","createdTime":"2025-02-01T00:00:00Z","lastModifiedBy":"user-2","lastModifiedTime":"2025-02-01T00:00:00Z"}}
		],
		"totalCount": 2
	}`

	var list DocumentList
	if err := json.Unmarshal([]byte(listJSON), &list); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(list.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(list.Documents))
	}
	if list.Documents[0].Version != 3 {
		t.Errorf("Documents[0].Version = %d, want 3", list.Documents[0].Version)
	}
	if list.Documents[1].Version != 5 {
		t.Errorf("Documents[1].Version = %d, want 5", list.Documents[1].Version)
	}
}

// TestSnapshot_VersionAsString tests that Snapshot can unmarshal version fields from strings.
func TestSnapshot_VersionAsString(t *testing.T) {
	tests := []struct {
		name           string
		json           string
		wantSnapshot   int
		wantDocVersion int
	}{
		{
			name:           "versions as ints",
			json:           `{"snapshotVersion":1,"documentVersion":2,"modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z"}}`,
			wantSnapshot:   1,
			wantDocVersion: 2,
		},
		{
			name:           "versions as strings",
			json:           `{"snapshotVersion":"3","documentVersion":"4","modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z"}}`,
			wantSnapshot:   3,
			wantDocVersion: 4,
		},
		{
			name:           "mixed versions",
			json:           `{"snapshotVersion":5,"documentVersion":"6","modificationInfo":{"createdBy":"user-1","createdTime":"2025-01-01T00:00:00Z"}}`,
			wantSnapshot:   5,
			wantDocVersion: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Snapshot
			if err := json.Unmarshal([]byte(tt.json), &s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.SnapshotVersion != tt.wantSnapshot {
				t.Errorf("SnapshotVersion = %d, want %d", s.SnapshotVersion, tt.wantSnapshot)
			}
			if s.DocumentVersion != tt.wantDocVersion {
				t.Errorf("DocumentVersion = %d, want %d", s.DocumentVersion, tt.wantDocVersion)
			}
		})
	}
}
