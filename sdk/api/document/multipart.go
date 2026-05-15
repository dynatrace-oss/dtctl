package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/go-resty/resty/v2"
)

// ParseMultipartDocument parses a multipart response into a Document
func ParseMultipartDocument(resp *resty.Response) (*Document, error) {
	contentType := resp.Header().Get("Content-Type")
	if contentType == "" {
		return nil, fmt.Errorf("missing Content-Type header")
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Content-Type: %w", err)
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("expected multipart Content-Type, got %s", mediaType)
	}

	boundary, ok := params["boundary"]
	if !ok {
		return nil, fmt.Errorf("missing boundary in Content-Type")
	}

	// Use bytes.NewReader with resp.Body() to avoid any potential truncation
	// that resp.String() might impose. This ensures we read the complete response.
	reader := multipart.NewReader(bytes.NewReader(resp.Body()), boundary)

	var doc Document
	var metadataParsed bool

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read multipart part: %w", err)
		}

		partName := part.FormName()

		switch partName {
		case "metadata", "document":
			// Metadata part (JSON) - API uses "metadata" as the field name
			data, err := io.ReadAll(part)
			if err != nil {
				return nil, fmt.Errorf("failed to read metadata part: %w", err)
			}

			if err := json.Unmarshal(data, &doc); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
			metadataParsed = true

		case "content":
			// Content part (binary)
			content, err := io.ReadAll(part)
			if err != nil {
				return nil, fmt.Errorf("failed to read content part: %w", err)
			}
			doc.Content = content

		default:
			// Unknown part, skip
			_, _ = io.Copy(io.Discard, part)
		}
	}

	if !metadataParsed {
		return nil, fmt.Errorf("metadata part not found in multipart response")
	}

	return &doc, nil
}

// CreateUpdateRequest creates form data for updating a document
func CreateUpdateRequest(content []byte, contentType string) (map[string]string, error) {
	if contentType == "" {
		// Default content type for update operations
		contentType = "application/json" //nolint:ineffassign,wastedassign // Variable may be used in future updates
	}

	// For update operations, we only need to send the content
	// The metadata is updated separately if needed
	formData := map[string]string{
		"content": string(content),
	}

	return formData, nil
}
