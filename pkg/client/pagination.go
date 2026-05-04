package client

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

// PaginationStyle controls how page-key and page-size interact.
//
// Dynatrace APIs have three distinct pagination behaviors:
//
//   - Default: page-size must NOT be sent with page-key (HTTP 400).
//     Filter/search params must be resent on every request.
//
//   - DocumentAPI: page-size CAN be sent with page-key.
//     Filter params must be resent (the page token does not embed them).
//
//   - SettingsAPI: when nextPageKey is present, NO other params may be sent
//     (pageSize, schemaIds, scopes, and fields are all embedded in the page token).
type PaginationStyle int

const (
	// PaginationDefault is the standard style used by most Dynatrace APIs.
	// page-size is only sent on the first request; filter params are sent on every request.
	PaginationDefault PaginationStyle = iota

	// PaginationDocumentAPI is used by the Document API (/platform/document/v1/).
	// page-size and filter params are sent on every request alongside page-key.
	PaginationDocumentAPI

	// PaginationSettingsAPI is used by the Settings API (/platform/classic/environment-api/v2/settings/).
	// When nextPageKey is present, no other query params may be sent (not even fields).
	PaginationSettingsAPI
)

// PaginationParams configures how pagination query parameters are applied to a request.
type PaginationParams struct {
	// Style determines the pagination variant.
	Style PaginationStyle

	// PageKeyParam is the query parameter name for the page key
	// (e.g. "page-key", "next-page-key", "nextPageKey").
	PageKeyParam string

	// PageSizeParam is the query parameter name for the page size
	// (e.g. "page-size", "pageSize"). May be empty if no page size is used.
	PageSizeParam string

	// NextPageKey is the current page key (empty string for the first page).
	NextPageKey string

	// PageSize is the requested page size. Zero means "use API default / no pagination".
	PageSize int64

	// Filters are additional query parameters (e.g. filter, name, schemaIds, scopes)
	// that should be sent according to the pagination style rules.
	// For Default and DocumentAPI styles, filters are sent on every request.
	// For SettingsAPI style, filters are only sent on the first request (page token embeds them).
	Filters map[string]string
}

// Apply sets the pagination query parameters on the given resty request.
// It returns the request for chaining.
//
// This helper encapsulates the three Dynatrace pagination styles so that
// callers cannot accidentally send page-size alongside page-key on APIs
// that reject that combination.
func (p PaginationParams) Apply(req *resty.Request) *resty.Request {
	switch p.Style {
	case PaginationDefault:
		// page-size must NOT be sent with page-key.
		// Filters must be sent on every request.
		if p.NextPageKey != "" {
			req.SetQueryParam(p.PageKeyParam, p.NextPageKey)
		} else if p.PageSize > 0 && p.PageSizeParam != "" {
			req.SetQueryParam(p.PageSizeParam, fmt.Sprintf("%d", p.PageSize))
		}
		for k, v := range p.Filters {
			if v != "" {
				req.SetQueryParam(k, v)
			}
		}

	case PaginationDocumentAPI:
		// page-size and filters are sent on every request alongside page-key.
		if p.NextPageKey != "" {
			req.SetQueryParam(p.PageKeyParam, p.NextPageKey)
		}
		if p.PageSize > 0 && p.PageSizeParam != "" {
			req.SetQueryParam(p.PageSizeParam, fmt.Sprintf("%d", p.PageSize))
		}
		for k, v := range p.Filters {
			if v != "" {
				req.SetQueryParam(k, v)
			}
		}

	case PaginationSettingsAPI:
		// When nextPageKey is present, NO other params may be sent.
		if p.NextPageKey != "" {
			req.SetQueryParam(p.PageKeyParam, p.NextPageKey)
		} else {
			if p.PageSize > 0 && p.PageSizeParam != "" {
				req.SetQueryParam(p.PageSizeParam, fmt.Sprintf("%d", p.PageSize))
			}
			for k, v := range p.Filters {
				if v != "" {
					req.SetQueryParam(k, v)
				}
			}
		}
	}

	return req
}
