// Package schedulingrule provides the CLI resource handler for Dynatrace scheduling rules.
package schedulingrule

import (
	"context"
	"errors"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkschedulingrule "github.com/dynatrace-oss/dtctl/sdk/api/schedulingrule"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// SchedulingRule represents a scheduling rule (CLI version with table tags).
type SchedulingRule struct {
	ID          string `json:"id,omitempty"          yaml:"id,omitempty"          table:"ID"`
	Title       string `json:"title"                 yaml:"title"                 table:"TITLE"`
	Timezone    string `json:"timezone"              yaml:"timezone"              table:"TIMEZONE"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" table:"DESCRIPTION,wide"`
	Rule        string `json:"rule"                  yaml:"rule"                  table:"RULE,wide"`
	Owner       string `json:"owner,omitempty"       yaml:"owner,omitempty"       table:"-"`
	OwnerType   string `json:"ownerType,omitempty"   yaml:"ownerType,omitempty"   table:"-"`
}

// SchedulingRuleList represents a list of scheduling rules.
type SchedulingRuleList struct {
	Count   int              `json:"count"`
	Results []SchedulingRule `json:"results"`
}

// fromSDKSchedulingRule converts an SDK SchedulingRule to a CLI SchedulingRule.
func fromSDKSchedulingRule(s *sdkschedulingrule.SchedulingRule) SchedulingRule {
	return SchedulingRule{
		ID:          s.ID,
		Title:       s.Title,
		Timezone:    s.Timezone,
		Description: s.Description,
		Rule:        s.Rule,
		Owner:       s.Owner,
		OwnerType:   s.OwnerType,
	}
}

// Handler handles scheduling rule resources.
// It delegates to the SDK handler and adds CLI-specific convenience methods.
type Handler struct {
	sdk *sdkschedulingrule.Handler
}

// NewHandler creates a new scheduling rule handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkschedulingrule.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List retrieves scheduling rules.
// chunkSize controls page size; 0 returns only the first page.
// limit caps the total number of results; 0 means unlimited.
func (h *Handler) List(chunkSize, limit int64) (*SchedulingRuleList, error) {
	sdkResult, err := h.sdk.List(context.Background(), chunkSize, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SchedulingRule, len(sdkResult.Results))
	for i := range sdkResult.Results {
		results[i] = fromSDKSchedulingRule(&sdkResult.Results[i])
	}
	return &SchedulingRuleList{Count: sdkResult.Count, Results: results}, nil
}

// Get retrieves a specific scheduling rule.
func (h *Handler) Get(id string) (*SchedulingRule, error) {
	sdkResult, err := h.sdk.Get(context.Background(), id)
	if err != nil {
		return nil, err
	}
	r := fromSDKSchedulingRule(sdkResult)
	return &r, nil
}

// Create creates a new scheduling rule.
func (h *Handler) Create(data []byte) (*SchedulingRule, error) {
	sdkResult, err := h.sdk.Create(context.Background(), data)
	if err != nil {
		return nil, err
	}
	r := fromSDKSchedulingRule(sdkResult)
	return &r, nil
}

// Update updates an existing scheduling rule.
func (h *Handler) Update(id string, data []byte) (*SchedulingRule, error) {
	sdkResult, err := h.sdk.Update(context.Background(), id, data)
	if err != nil {
		return nil, err
	}
	r := fromSDKSchedulingRule(sdkResult)
	return &r, nil
}

// Delete deletes a scheduling rule.
func (h *Handler) Delete(id string) error {
	return h.sdk.Delete(context.Background(), id)
}

// IsNotFound reports whether err indicates the scheduling rule does not exist
// (HTTP 404), as opposed to a transient, auth, or other failure.
func IsNotFound(err error) bool {
	return errors.Is(err, httpclient.ErrNotFound)
}
