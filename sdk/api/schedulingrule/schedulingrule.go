// Package schedulingrule provides access to the Dynatrace Automation Scheduling Rules API.
package schedulingrule

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const basePath = "/platform/automation/v1/scheduling-rules"

// Handler handles scheduling rule resources.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new scheduling rule handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// SchedulingRule represents a scheduling rule resource.
type SchedulingRule struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Rule        string `json:"rule"`
	Timezone    string `json:"timezone"`
	Owner       string `json:"owner,omitempty"`
	OwnerType   string `json:"ownerType,omitempty"`
}

// SchedulingRuleList represents a list of scheduling rules.
type SchedulingRuleList struct {
	Count   int              `json:"count"`
	Results []SchedulingRule `json:"results"`
}

// List retrieves all scheduling rules.
func (h *Handler) List(ctx context.Context) (*SchedulingRuleList, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).Get(basePath)
	if err != nil {
		return nil, fmt.Errorf("list scheduling rules: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list scheduling rules: %w", err)
	}
	var result SchedulingRuleList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list scheduling rules: parse response: %w", err)
	}
	return &result, nil
}

// Get retrieves a specific scheduling rule.
func (h *Handler) Get(ctx context.Context, id string) (*SchedulingRule, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).Get(fmt.Sprintf("%s/%s", basePath, id))
	if err != nil {
		return nil, fmt.Errorf("get scheduling rule: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get scheduling rule: %w", err)
	}
	var result SchedulingRule
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get scheduling rule: parse response: %w", err)
	}
	return &result, nil
}

// GetRaw retrieves a scheduling rule as raw JSON bytes.
func (h *Handler) GetRaw(ctx context.Context, id string) ([]byte, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).Get(fmt.Sprintf("%s/%s", basePath, id))
	if err != nil {
		return nil, fmt.Errorf("get scheduling rule: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get scheduling rule: %w", err)
	}
	return resp.Body(), nil
}

// Create creates a new scheduling rule.
func (h *Handler) Create(ctx context.Context, data []byte) (*SchedulingRule, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		Post(basePath)
	if err != nil {
		return nil, fmt.Errorf("create scheduling rule: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create scheduling rule: %w", err)
	}
	var result SchedulingRule
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("create scheduling rule: parse response: %w", err)
	}
	return &result, nil
}

// Update updates an existing scheduling rule.
func (h *Handler) Update(ctx context.Context, id string, data []byte) (*SchedulingRule, error) {
	resp, err := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(data).
		Put(fmt.Sprintf("%s/%s", basePath, id))
	if err != nil {
		return nil, fmt.Errorf("update scheduling rule: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("update scheduling rule: %w", err)
	}
	var result SchedulingRule
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("update scheduling rule: parse response: %w", err)
	}
	return &result, nil
}

// Delete deletes a scheduling rule.
func (h *Handler) Delete(ctx context.Context, id string) error {
	resp, err := h.client.HTTP().R().SetContext(ctx).Delete(fmt.Sprintf("%s/%s", basePath, id))
	if err != nil {
		return fmt.Errorf("delete scheduling rule: %w", err)
	}
	return httpclient.CheckResponse(resp)
}
