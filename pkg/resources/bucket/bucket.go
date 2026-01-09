package bucket

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles Grail bucket resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new bucket handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// Bucket represents a Grail storage bucket
type Bucket struct {
	BucketName                 string `json:"bucketName" table:"NAME"`
	Table                      string `json:"table" table:"TABLE"`
	DisplayName                string `json:"displayName" table:"DISPLAY_NAME"`
	Status                     string `json:"status" table:"STATUS"`
	RetentionDays              int    `json:"retentionDays" table:"RETENTION_DAYS"`
	IncludedQueryLimitDays     int    `json:"includedQueryLimitDays,omitempty" table:"-"`
	MetricInterval             string `json:"metricInterval,omitempty" table:"INTERVAL,wide"`
	Version                    int    `json:"version" table:"-"`
	Updatable                  bool   `json:"updatable" table:"UPDATABLE,wide"`
	Records                    *int64 `json:"records,omitempty" table:"RECORDS,wide"`
	EstimatedUncompressedBytes *int64 `json:"estimatedUncompressedBytes,omitempty" table:"-"`
}

// BucketList represents a list of buckets
type BucketList struct {
	Buckets []Bucket `json:"buckets"`
}

// BucketCreate represents the request body for creating a bucket
type BucketCreate struct {
	BucketName             string `json:"bucketName"`
	Table                  string `json:"table"`
	DisplayName            string `json:"displayName,omitempty"`
	RetentionDays          int    `json:"retentionDays"`
	IncludedQueryLimitDays int    `json:"includedQueryLimitDays,omitempty"`
}

// BucketUpdate represents the request body for updating a bucket
type BucketUpdate struct {
	DisplayName            string `json:"displayName,omitempty"`
	RetentionDays          int    `json:"retentionDays,omitempty"`
	IncludedQueryLimitDays int    `json:"includedQueryLimitDays,omitempty"`
}

// List lists all bucket definitions
func (h *Handler) List() (*BucketList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "records").
		Get("/platform/storage/management/v1/bucket-definitions")

	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list buckets: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result BucketList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse buckets response: %w", err)
	}

	return &result, nil
}

// Get gets a specific bucket by name
func (h *Handler) Get(bucketName string) (*Bucket, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "records,estimatedUncompressedBytes").
		Get(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s", bucketName))

	if err != nil {
		return nil, fmt.Errorf("failed to get bucket: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("bucket %q not found", bucketName)
		default:
			return nil, fmt.Errorf("failed to get bucket: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result Bucket
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse bucket response: %w", err)
	}

	return &result, nil
}

// Create creates a new bucket
func (h *Handler) Create(req BucketCreate) (*Bucket, error) {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/storage/management/v1/bucket-definitions")

	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid bucket configuration: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create bucket")
		case 409:
			return nil, fmt.Errorf("bucket %q already exists", req.BucketName)
		default:
			return nil, fmt.Errorf("failed to create bucket: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result Bucket
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	return &result, nil
}

// Update updates an existing bucket
func (h *Handler) Update(bucketName string, version int, req BucketUpdate) error {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetQueryParam("optimistic-locking-version", fmt.Sprintf("%d", version)).
		Patch(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s", bucketName))

	if err != nil {
		return fmt.Errorf("failed to update bucket: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return fmt.Errorf("invalid bucket configuration: %s", resp.String())
		case 403:
			return fmt.Errorf("bucket %q is read-only or access denied", bucketName)
		case 404:
			return fmt.Errorf("bucket %q not found", bucketName)
		case 409:
			return fmt.Errorf("bucket version conflict (bucket was modified)")
		default:
			return fmt.Errorf("failed to update bucket: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Delete deletes a bucket
func (h *Handler) Delete(bucketName string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s", bucketName))

	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 403:
			return fmt.Errorf("bucket %q is read-only or access denied", bucketName)
		case 404:
			return fmt.Errorf("bucket %q not found", bucketName)
		case 409:
			return fmt.Errorf("bucket %q is still in use and cannot be deleted", bucketName)
		default:
			return fmt.Errorf("failed to delete bucket: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// Truncate empties a bucket (removes all data)
func (h *Handler) Truncate(bucketName string) error {
	resp, err := h.client.HTTP().R().
		Post(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s:truncate", bucketName))

	if err != nil {
		return fmt.Errorf("failed to truncate bucket: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 403:
			return fmt.Errorf("access denied to truncate bucket %q", bucketName)
		case 404:
			return fmt.Errorf("bucket %q not found", bucketName)
		default:
			return fmt.Errorf("failed to truncate bucket: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// GetRaw gets a bucket as raw JSON bytes (for editing)
func (h *Handler) GetRaw(bucketName string) ([]byte, error) {
	bucket, err := h.Get(bucketName)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(bucket, "", "  ")
}
