package bucket

import (
	"encoding/json"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkbucket "github.com/dynatrace-oss/dtctl/sdk/api/bucket"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	Bucket       = sdkbucket.Bucket
	BucketList   = sdkbucket.BucketList
	BucketCreate = sdkbucket.BucketCreate
	BucketUpdate = sdkbucket.BucketUpdate
)

// Handler handles Grail bucket resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	sdk *sdkbucket.Handler
}

// NewHandler creates a new bucket handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkbucket.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List lists all bucket definitions.
func (h *Handler) List() (*BucketList, error) {
	return h.sdk.List()
}

// Get gets a specific bucket by name.
func (h *Handler) Get(bucketName string) (*Bucket, error) {
	return h.sdk.Get(bucketName)
}

// Create creates a new bucket.
func (h *Handler) Create(req BucketCreate) (*Bucket, error) {
	return h.sdk.Create(req)
}

// Update updates an existing bucket.
func (h *Handler) Update(bucketName string, version int, req BucketUpdate) error {
	return h.sdk.Update(bucketName, version, req)
}

// Delete deletes a bucket.
func (h *Handler) Delete(bucketName string) error {
	return h.sdk.Delete(bucketName)
}

// Truncate empties a bucket (removes all data).
func (h *Handler) Truncate(bucketName string) error {
	return h.sdk.Truncate(bucketName)
}

// GetRaw gets a bucket as raw JSON bytes (for editing).
func (h *Handler) GetRaw(bucketName string) ([]byte, error) {
	bucket, err := h.sdk.Get(bucketName)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(bucket, "", "  ")
}
