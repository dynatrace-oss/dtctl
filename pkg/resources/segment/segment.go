package segment

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

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

	// Populate VariablesDisplay for wide table output
	for i := range result.FilterSegments {
		result.FilterSegments[i].VariablesDisplay = variablesDisplay(result.FilterSegments[i].Variables)
	}

	return &result, nil
}

// variablesDisplay returns a human-readable summary of a segment's variables.
func variablesDisplay(v *Variables) string {
	if v == nil || v.Type == "" {
		return ""
	}
	return v.Type
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
			return nil, fmt.Errorf("segment %q not found", uid)
		default:
			return nil, fmt.Errorf("failed to get segment: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result FilterSegment
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse segment response: %w", err)
	}

	return &result, nil
}

// Create creates a new filter segment from raw JSON/YAML bytes.
func (h *Handler) Create(data []byte) (*FilterSegment, error) {
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetBody(data).
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
	resp, err := h.client.HTTP().R().
		SetHeader("Content-Type", "application/json").
		SetQueryParam("optimistic-locking-version", fmt.Sprintf("%d", version)).
		SetBody(data).
		Put(fmt.Sprintf("%s/%s", basePath, uid))

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
			return fmt.Errorf("segment %q not found", uid)
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
			return fmt.Errorf("segment %q not found", uid)
		default:
			return fmt.Errorf("failed to delete segment: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetRaw gets a segment as raw JSON bytes (for edit command).
func (h *Handler) GetRaw(uid string) ([]byte, error) {
	seg, err := h.Get(uid)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(seg, "", "  ")
}

// IsNotFound returns true if the error indicates a segment was not found (404).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}
