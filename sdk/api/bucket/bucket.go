package bucket

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

type Handler struct {
	client *httpclient.Client
}

func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

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

type BucketList struct {
	Buckets []Bucket `json:"buckets"`
}

type BucketCreate struct {
	BucketName             string `json:"bucketName"`
	Table                  string `json:"table"`
	DisplayName            string `json:"displayName,omitempty"`
	RetentionDays          int    `json:"retentionDays"`
	IncludedQueryLimitDays int    `json:"includedQueryLimitDays,omitempty"`
}

type BucketUpdate struct {
	DisplayName            string `json:"displayName,omitempty"`
	RetentionDays          int    `json:"retentionDays,omitempty"`
	IncludedQueryLimitDays int    `json:"includedQueryLimitDays,omitempty"`
}

func (h *Handler) List() (*BucketList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "records").
		Get("/platform/storage/management/v1/bucket-definitions")
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	var result BucketList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list buckets: parse response: %w", err)
	}
	return &result, nil
}

func (h *Handler) Get(bucketName string) (*Bucket, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "records,estimatedUncompressedBytes").
		Get(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s", bucketName))
	if err != nil {
		return nil, fmt.Errorf("get bucket: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get bucket %q: %w", bucketName, err)
	}
	var result Bucket
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get bucket: parse response: %w", err)
	}
	return &result, nil
}

func (h *Handler) Create(req BucketCreate) (*Bucket, error) {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		Post("/platform/storage/management/v1/bucket-definitions")
	if err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	var result Bucket
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("create bucket: parse response: %w", err)
	}
	return &result, nil
}

func (h *Handler) Update(bucketName string, version int, req BucketUpdate) error {
	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetQueryParam("optimistic-locking-version", fmt.Sprintf("%d", version)).
		Patch(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s", bucketName))
	if err != nil {
		return fmt.Errorf("update bucket: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("update bucket %q: %w", bucketName, err)
	}
	return nil
}

func (h *Handler) Delete(bucketName string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s", bucketName))
	if err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete bucket %q: %w", bucketName, err)
	}
	return nil
}

func (h *Handler) Truncate(bucketName string) error {
	resp, err := h.client.HTTP().R().
		Post(fmt.Sprintf("/platform/storage/management/v1/bucket-definitions/%s:truncate", bucketName))
	if err != nil {
		return fmt.Errorf("truncate bucket: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("truncate bucket %q: %w", bucketName, err)
	}
	return nil
}

func (h *Handler) GetRaw(bucketName string) ([]byte, error) {
	bucket, err := h.Get(bucketName)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(bucket, "", "  ")
}
