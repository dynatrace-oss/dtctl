package profile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkae "github.com/dynatrace-oss/dtctl/sdk/api/appengine"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const (
	DefaultAppID = "dynatrace.profiling"
	functionName = "codeLevelAnalysis"
)

// Kind maps user-facing shorthand to the API kind strings.
var Kind = map[string]string{
	"hotspots":       "methodHotspots",
	"threads":        "threadAnalysis",
	"memory":         "memoryAllocation",
	"memory-details": "memoryAllocationDetails",
}

type Payload struct {
	Kind            string `json:"kind"`
	EntityID        string `json:"entityId"`
	From            int64  `json:"from"`
	To              int64  `json:"to"`
	ServiceFilter   string `json:"serviceFilter,omitempty"`
	ShowWaiting     bool   `json:"showWaiting,omitempty"`
	ProblemID       string `json:"problemId,omitempty"`
	SurvivorsOnly   bool   `json:"survivorsOnly,omitempty"`
	TypeFilter      string `json:"typeFilter,omitempty"`
	APIFilter       string `json:"apiFilter,omitempty"`
	MethodFQNFilter string `json:"methodFqnFilter,omitempty"`
	Type            string `json:"type,omitempty"`
	Method          string `json:"method,omitempty"`
}

type Handler struct {
	fn *sdkae.FunctionHandler
}

func NewHandler(c *client.Client) *Handler {
	return &Handler{fn: sdkae.NewFunctionHandler(httpclient.Wrap(c.HTTP()))}
}

func (h *Handler) Run(ctx context.Context, appID string, p Payload) (interface{}, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("profile: marshal payload: %w", err)
	}

	resp, err := h.fn.InvokeFunction(ctx, &sdkae.FunctionInvokeRequest{
		Method:       "POST",
		AppID:        appID,
		FunctionName: functionName,
		Payload:      string(body),
		Headers:      map[string]string{"Content-Type": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}

	if resp.RawBody != nil {
		return resp.RawBody, nil
	}
	var result interface{}
	if err := json.Unmarshal([]byte(resp.Body), &result); err != nil {
		return resp.Body, nil
	}
	return result, nil
}

