package segment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// ErrNotFound is returned when a segment is not found (HTTP 404).
var ErrNotFound = errors.New("segment not found")

const basePath = "/platform/storage/filter-segments/v1/filter-segments"

// Handler handles Grail filter segment resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new segment handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// FilterSegment is the read model for a Grail filter segment.
type FilterSegment struct {
	UID               string     `json:"uid" table:"UID"`
	Name              string     `json:"name" table:"NAME"`
	Description       string     `json:"description,omitempty" table:"DESCRIPTION,wide"`
	IsPublic          bool       `json:"isPublic" table:"PUBLIC"`
	VariablesDisplay  string     `json:"-" yaml:"-" table:"VARIABLES,wide"`
	Owner             string     `json:"owner,omitempty" table:"OWNER,wide"`
	Version           int        `json:"version,omitempty" table:"-"`
	IsReadyMade       bool       `json:"isReadyMade,omitempty" table:"-"`
	Includes          []Include  `json:"includes,omitempty" table:"-"`
	Variables         *Variables `json:"variables,omitempty" table:"-"`
	AllowedOperations []string   `json:"allowedOperations,omitempty" table:"-"`
}

// Include represents a single include rule within a segment.
type Include struct {
	DataObject string `json:"dataObject"` // "logs", "spans", etc. Use "_all_data_object" for all.
	Filter     string `json:"filter"`
}

// Variables holds the variable configuration for a segment.
type Variables struct {
	Type  string `json:"type"`  // Variable type, e.g. "query"
	Value string `json:"value"` // Variable value, e.g. a DQL expression
}

// FilterSegmentList represents a list of filter segments.
// The filter-segments API does not support pagination; all segments are
// returned in a single response.
type FilterSegmentList struct {
	FilterSegments []FilterSegment `json:"filterSegments"`
	TotalCount     int             `json:"totalCount,omitempty"`
}

// List lists all filter segments.
// The filter-segments API returns all segments in one response (no pagination).
// Variables are requested so the wide table view can show whether each segment
// requires variable bindings.
func (h *Handler) List() (*FilterSegmentList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParamsFromValues(map[string][]string{
			"add-fields": {"VARIABLES"},
		}).
		Get(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list segments: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list segments: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result FilterSegmentList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse segments response: %w", err)
	}

	// Convert AST filters to human-readable DQL for display, and
	// populate VariablesDisplay for wide table output.
	for i := range result.FilterSegments {
		convertIncludesForDisplay(&result.FilterSegments[i])
		result.FilterSegments[i].VariablesDisplay = variablesDisplay(result.FilterSegments[i].Variables)
	}

	return &result, nil
}

// variablesDisplay returns a human-readable summary of a segment's variables.
func variablesDisplay(v *Variables) string {
	if v == nil || v.Type == "" {
		return ""
	}
	return "Yes"
}

// Get gets a specific filter segment by UID.
func (h *Handler) Get(uid string) (*FilterSegment, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParamsFromValues(map[string][]string{
			"add-fields": {"INCLUDES", "VARIABLES"},
		}).
		Get(fmt.Sprintf("%s/%s", basePath, uid))

	if err != nil {
		return nil, fmt.Errorf("failed to get segment: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("segment %q: %w", uid, ErrNotFound)
		default:
			return nil, fmt.Errorf("failed to get segment: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result FilterSegment
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse segment response: %w", err)
	}

	// Convert AST filters to human-readable DQL for display
	convertIncludesForDisplay(&result)

	return &result, nil
}

// Create creates a new filter segment from raw JSON/YAML bytes.
func (h *Handler) Create(data []byte) (*FilterSegment, error) {
	// Convert DQL filters to AST for the API
	converted, err := convertIncludesForAPI(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert filter expressions: %w", err)
	}

	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(converted).
		Post(basePath)

	if err != nil {
		return nil, fmt.Errorf("failed to create segment: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid segment definition: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create segment")
		case 409:
			return nil, fmt.Errorf("segment already exists: %s", resp.String())
		default:
			return nil, fmt.Errorf("failed to create segment: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result FilterSegment
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	return &result, nil
}

// Update updates an existing filter segment.
// The version parameter is required for optimistic locking.
func (h *Handler) Update(uid string, version int, data []byte) error {
	// Convert DQL filters to AST for the API
	converted, err := convertIncludesForAPI(data)
	if err != nil {
		return fmt.Errorf("failed to convert filter expressions: %w", err)
	}

	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetQueryParam("optimistic-locking-version", fmt.Sprintf("%d", version)).
		SetBody(converted).
		Patch(fmt.Sprintf("%s/%s", basePath, uid))

	if err != nil {
		return fmt.Errorf("failed to update segment: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("invalid segment definition: %s", resp.String())
		case 403:
			return fmt.Errorf("access denied to update segment %q", uid)
		case 404:
			return fmt.Errorf("segment %q: %w", uid, ErrNotFound)
		case 409:
			return fmt.Errorf("segment version conflict (segment was modified)")
		default:
			return fmt.Errorf("failed to update segment: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Delete deletes a filter segment by UID.
func (h *Handler) Delete(uid string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("%s/%s", basePath, uid))

	if err != nil {
		return fmt.Errorf("failed to delete segment: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 403:
			return fmt.Errorf("access denied to delete segment %q", uid)
		case 404:
			return fmt.Errorf("segment %q: %w", uid, ErrNotFound)
		default:
			return fmt.Errorf("failed to delete segment: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetRaw gets a segment as pretty-printed JSON bytes (for edit command).
// Note: the returned JSON contains DQL filter expressions (not raw API AST)
// because it delegates to Get, which converts AST filters to DQL.
func (h *Handler) GetRaw(uid string) ([]byte, error) {
	seg, err := h.Get(uid)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(seg, "", "  ")
}

// IsNotFound returns true if the error indicates a segment was not found (404).
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// ---------------------------------------------------------------------------
// Filter format conversion (DQL ↔ AST)
// ---------------------------------------------------------------------------

// convertIncludesForAPI converts include filters from DQL to AST before
// sending to the API. It operates on raw JSON bytes so it works with both
// create and update payloads. If a filter is already JSON AST (starts with
// '{'), it is passed through unchanged.
//
// The function preserves the original JSON field order by splicing the
// converted includes array back into the original bytes rather than
// re-marshaling the entire payload through a map (which would alphabetize
// keys).
func convertIncludesForAPI(data []byte) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	includesRaw, ok := payload["includes"]
	if !ok {
		return data, nil // no includes field — pass through unchanged
	}

	var includes []map[string]json.RawMessage
	if err := json.Unmarshal(includesRaw, &includes); err != nil {
		return nil, fmt.Errorf("failed to parse includes: %w", err)
	}

	changed := false
	for i, inc := range includes {
		filterRaw, ok := inc["filter"]
		if !ok {
			continue
		}
		var filter string
		if err := json.Unmarshal(filterRaw, &filter); err != nil {
			continue // not a string — skip
		}

		ast, err := FilterToAST(filter)
		if err != nil {
			return nil, fmt.Errorf("include[%d]: %w", i, err)
		}
		if ast != filter {
			newFilterJSON, err := json.Marshal(ast)
			if err != nil {
				return nil, fmt.Errorf("include[%d]: failed to marshal AST: %w", i, err)
			}
			inc["filter"] = newFilterJSON
			changed = true
		}
	}

	if !changed {
		return data, nil
	}

	// Re-marshal only the includes array, then splice it into the original
	// JSON to preserve field order of the top-level object.
	newIncludes, err := json.Marshal(includes)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal includes: %w", err)
	}

	return replaceJSONField(data, "includes", newIncludes)
}

// replaceJSONField replaces the value of a top-level JSON object field while
// preserving the original field order and formatting. It scans the raw JSON
// bytes for the field key, locates its value span, and splices in newValue.
func replaceJSONField(data []byte, field string, newValue json.RawMessage) ([]byte, error) {
	// Build the key pattern to search for: "field":
	keyPattern := []byte(`"` + field + `"`)

	idx := bytes.Index(data, keyPattern)
	if idx < 0 {
		return nil, fmt.Errorf("field %q not found in JSON", field)
	}

	// Skip past the key and the colon
	valueStart := idx + len(keyPattern)
	for valueStart < len(data) && (data[valueStart] == ' ' || data[valueStart] == '\t' || data[valueStart] == '\n' || data[valueStart] == '\r' || data[valueStart] == ':') {
		valueStart++
	}
	if valueStart >= len(data) {
		return nil, fmt.Errorf("field %q: unexpected end of JSON after key", field)
	}

	// Find the end of the value by counting balanced brackets/braces and
	// skipping strings. The value starts at data[valueStart].
	valueEnd, err := findJSONValueEnd(data, valueStart)
	if err != nil {
		return nil, fmt.Errorf("field %q: %w", field, err)
	}

	// Splice: data[:valueStart] + newValue + data[valueEnd:]
	var buf bytes.Buffer
	buf.Grow(valueStart + len(newValue) + (len(data) - valueEnd))
	buf.Write(data[:valueStart])
	buf.Write(newValue)
	buf.Write(data[valueEnd:])
	return buf.Bytes(), nil
}

// findJSONValueEnd returns the byte offset just past the JSON value starting
// at data[start]. It handles objects, arrays, strings, and primitives.
func findJSONValueEnd(data []byte, start int) (int, error) {
	if start >= len(data) {
		return 0, fmt.Errorf("unexpected end of JSON")
	}

	switch data[start] {
	case '{', '[':
		return findJSONBalancedEnd(data, start)
	case '"':
		return findJSONStringEnd(data, start)
	default:
		// Primitive (number, boolean, null) — ends at comma, }, ], or whitespace
		i := start
		for i < len(data) {
			switch data[i] {
			case ',', '}', ']', ' ', '\t', '\n', '\r':
				return i, nil
			}
			i++
		}
		return i, nil
	}
}

// findJSONBalancedEnd finds the closing bracket/brace that matches the opener
// at data[start], correctly skipping nested structures and strings.
func findJSONBalancedEnd(data []byte, start int) (int, error) {
	open := data[start]
	var close byte
	if open == '{' {
		close = '}'
	} else {
		close = ']'
	}

	depth := 1
	i := start + 1
	for i < len(data) && depth > 0 {
		switch data[i] {
		case '"':
			end, err := findJSONStringEnd(data, i)
			if err != nil {
				return 0, err
			}
			i = end
			continue
		case open:
			depth++
		case close:
			depth--
		}
		i++
	}
	if depth != 0 {
		return 0, fmt.Errorf("unbalanced JSON starting at offset %d", start)
	}
	return i, nil
}

// findJSONStringEnd returns the offset just past the closing quote of a JSON
// string starting at data[start] (which must be '"').
func findJSONStringEnd(data []byte, start int) (int, error) {
	i := start + 1 // skip opening quote
	for i < len(data) {
		if data[i] == '\\' {
			i += 2 // skip escaped character
			continue
		}
		if data[i] == '"' {
			return i + 1, nil
		}
		i++
	}
	return 0, fmt.Errorf("unterminated JSON string starting at offset %d", start)
}

// convertIncludesForDisplay converts include filters from AST to
// human-readable DQL after receiving from the API. It modifies the
// FilterSegment in place. If a filter is already plain DQL (doesn't
// start with '{'), it is left unchanged.
func convertIncludesForDisplay(seg *FilterSegment) {
	for i := range seg.Includes {
		dql, err := FilterFromAST(seg.Includes[i].Filter)
		if err != nil {
			continue // leave as-is if conversion fails
		}
		seg.Includes[i].Filter = dql
	}
}
