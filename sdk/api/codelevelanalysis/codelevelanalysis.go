package codelevelanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const (
	basePath        = "/platform-reserved/codelevelanalysis/v0.1"
	pollInterval    = 1500 * time.Millisecond
	maxPollAttempts = 40
	maxPollErrors   = 40
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

type Response struct {
	Status    string      `json:"status"` // "completed" | "no-data"
	Kind      string      `json:"kind"`
	EntityID  string      `json:"entityId"`
	PollCount int         `json:"pollCount"`
	Result    interface{} `json:"result"`
}

type progressToken struct {
	Token string `json:"token"`
}

type Handler struct {
	client *httpclient.Client
}

func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

func (h *Handler) Run(ctx context.Context, p Payload) (*Response, error) {
	if err := validate(p); err != nil {
		return nil, err
	}

	submitPath := buildSubmitPath(p)
	resp, err := h.client.HTTP().R().SetContext(ctx).Get(submitPath)
	if err != nil {
		return nil, fmt.Errorf("codelevelanalysis: submit: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		// Allow 202 through — CheckResponse only errors on 4xx/5xx
		// so this path is only hit on genuine errors.
		return nil, fmt.Errorf("codelevelanalysis: submit: %w", err)
	}

	if resp.StatusCode() == 200 {
		return parseResult(p, 0, resp.Body())
	}

	// 202: async task started — extract token and poll
	var pt progressToken
	if err := json.Unmarshal(resp.Body(), &pt); err != nil || pt.Token == "" {
		return nil, fmt.Errorf("codelevelanalysis: submit returned 202 without a progress token")
	}
	return h.poll(ctx, p, pt.Token)
}

func (h *Handler) poll(ctx context.Context, p Payload, token string) (*Response, error) {
	resultPath := fmt.Sprintf("%s/result?token=%s", basePath, url.QueryEscape(token))
	errCount := 0

	for attempt := 1; attempt <= maxPollAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		resp, err := h.client.HTTP().R().SetContext(ctx).Get(resultPath)
		if err != nil {
			errCount++
			if errCount >= maxPollErrors {
				return nil, fmt.Errorf("codelevelanalysis: poll failed %d times (token %s): %w", maxPollErrors, token, err)
			}
			continue
		}

		sc := resp.StatusCode()
		if sc != 200 && sc != 202 && sc != 204 {
			errCount++
			if errCount >= maxPollErrors {
				return nil, fmt.Errorf("codelevelanalysis: poll returned %d %d times (token %s)", sc, maxPollErrors, token)
			}
			continue
		}

		if sc == 204 {
			return &Response{Status: "no-data", Kind: p.Kind, EntityID: p.EntityID, PollCount: attempt}, nil
		}
		if sc == 200 {
			return parseResult(p, attempt, resp.Body())
		}
		// 202: still running
	}

	return nil, fmt.Errorf("codelevelanalysis: analysis did not complete within %ds (token %s)",
		int(maxPollAttempts*pollInterval/time.Second), token)
}

func parseResult(p Payload, pollCount int, body []byte) (*Response, error) {
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("codelevelanalysis: parse result: %w", err)
	}
	return &Response{Status: "completed", Kind: p.Kind, EntityID: p.EntityID, PollCount: pollCount, Result: result}, nil
}

func validate(p Payload) error {
	if p.EntityID == "" {
		return fmt.Errorf("codelevelanalysis: entityId is required")
	}
	if p.From == 0 || p.To == 0 || p.To <= p.From {
		return fmt.Errorf("codelevelanalysis: from/to must be epoch-millis with to > from")
	}
	if p.To-p.From > 2*60*60*1000 {
		return fmt.Errorf("codelevelanalysis: timeframe must not exceed 2 hours")
	}
	if p.Kind == "memoryAllocationDetails" && (p.Type == "" || p.Method == "") {
		return fmt.Errorf("codelevelanalysis: memoryAllocationDetails requires type and method")
	}
	return nil
}

func buildSubmitPath(p Payload) string {
	params := url.Values{}
	params.Set("from", fmt.Sprintf("%d", p.From))
	params.Set("to", fmt.Sprintf("%d", p.To))
	entity := url.PathEscape(p.EntityID)

	setIfTrue := func(key string, v bool) {
		if v {
			params.Set(key, "true")
		}
	}
	setIfSet := func(key, v string) {
		if v != "" {
			params.Set(key, v)
		}
	}

	switch p.Kind {
	case "methodHotspots", "threadAnalysis":
		setIfSet("servicefilter", p.ServiceFilter)
		setIfTrue("showWaiting", p.ShowWaiting)
		path := "methodhotspots"
		if p.Kind == "threadAnalysis" {
			path = "threadanalysis"
		}
		return fmt.Sprintf("%s/%s/%s?%s", basePath, path, entity, params.Encode())

	case "memoryAllocation":
		setIfSet("problemId", p.ProblemID)
		setIfTrue("survivorsOnly", p.SurvivorsOnly)
		setIfSet("typeFilter", p.TypeFilter)
		setIfSet("apiFilter", p.APIFilter)
		setIfSet("methodFqnFilter", p.MethodFQNFilter)
		return fmt.Sprintf("%s/memoryallocation/%s?%s", basePath, entity, params.Encode())

	case "memoryAllocationDetails":
		setIfSet("problemId", p.ProblemID)
		setIfSet("type", p.Type)
		setIfSet("method", p.Method)
		setIfTrue("survivorsOnly", p.SurvivorsOnly)
		setIfSet("typeFilter", p.TypeFilter)
		setIfSet("apiFilter", p.APIFilter)
		setIfSet("methodFqnFilter", p.MethodFQNFilter)
		return fmt.Sprintf("%s/memoryallocationdetails/%s?%s", basePath, entity, params.Encode())
	}

	panic(fmt.Sprintf("codelevelanalysis: unsupported kind %q", p.Kind))
}
