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
