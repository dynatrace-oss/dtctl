package httpclient

import (
	"fmt"
	"net/url"
)

// PaginationStyle controls how page-key and page-size interact.
//
// Dynatrace APIs have three distinct pagination behaviors:
//   - Default: page-size must NOT be sent with page-key.
//   - DocumentAPI: page-size CAN be sent with page-key.
//   - SettingsAPI: when nextPageKey is present, NO other params may be sent.
type PaginationStyle int

const (
	// PaginationDefault is the standard style used by most Dynatrace APIs.
	PaginationDefault PaginationStyle = iota

	// PaginationDocumentAPI is used by the Document API.
	PaginationDocumentAPI

	// PaginationSettingsAPI is used by the Settings API.
	PaginationSettingsAPI
)

// PaginationParams configures how pagination query parameters are applied.
type PaginationParams struct {
	Style         PaginationStyle
	PageKeyParam  string
	PageSizeParam string
	NextPageKey   string
	PageSize      int64
	Filters       map[string]string
}

// QueryParams returns the query parameters to apply for the current page.
// This is a pure function with no resty dependency, suitable for any HTTP client.
func (p PaginationParams) QueryParams() url.Values {
	params := url.Values{}

	switch p.Style {
	case PaginationDefault:
		if p.NextPageKey != "" {
			params.Set(p.PageKeyParam, p.NextPageKey)
		} else if p.PageSize > 0 && p.PageSizeParam != "" {
			params.Set(p.PageSizeParam, fmt.Sprintf("%d", p.PageSize))
		}
		for k, v := range p.Filters {
			if v != "" {
				params.Set(k, v)
			}
		}

	case PaginationDocumentAPI:
		if p.NextPageKey != "" {
			params.Set(p.PageKeyParam, p.NextPageKey)
		}
		if p.PageSize > 0 && p.PageSizeParam != "" {
			params.Set(p.PageSizeParam, fmt.Sprintf("%d", p.PageSize))
		}
		for k, v := range p.Filters {
			if v != "" {
				params.Set(k, v)
			}
		}

	case PaginationSettingsAPI:
		if p.NextPageKey != "" {
			params.Set(p.PageKeyParam, p.NextPageKey)
		} else {
			if p.PageSize > 0 && p.PageSizeParam != "" {
				params.Set(p.PageSizeParam, fmt.Sprintf("%d", p.PageSize))
			}
			for k, v := range p.Filters {
				if v != "" {
					params.Set(k, v)
				}
			}
		}
	}

	return params
}
