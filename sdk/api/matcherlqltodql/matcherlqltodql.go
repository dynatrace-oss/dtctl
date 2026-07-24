// Package matcherlqltodql is a thin, read-only SDK client for the OpenPipeline
// matcher LQL-to-DQL translation endpoint, which converts a single LQL matcher
// expression into its semantically equivalent DQL matcher expression.
//
// The translated DQL string is forwarded verbatim; this package does not
// interpret, reshape, or validate the result.
package matcherlqltodql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

const basePath = "/platform/openpipeline/v1/matcher/lqlToDql"

// Handler calls the LQL-to-DQL translation endpoint.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new LQL-to-DQL translation handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// TranslationResult holds the translated DQL matcher expression.
type TranslationResult struct {
	Query string `json:"query"`
}

// Translate POSTs the given LQL matcher expression to the translation endpoint
// and returns the DQL equivalent verbatim.
func (h *Handler) Translate(ctx context.Context, lql string) (*TranslationResult, error) {
	body, err := json.Marshal(map[string]string{"query": lql})
	if err != nil {
		return nil, fmt.Errorf("translate lql-to-dql: marshal request: %w", err)
	}

	resp, err := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(basePath)
	if err != nil {
		return nil, fmt.Errorf("translate lql-to-dql: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("translate lql-to-dql: %w", err)
	}

	var result TranslationResult
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("translate lql-to-dql: parse response: %w", err)
	}

	return &result, nil
}
