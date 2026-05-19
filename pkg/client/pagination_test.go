package client

import (
	"testing"

	"github.com/go-resty/resty/v2"
)

// getQueryParams extracts query parameters from a resty request by inspecting
// the FormData (which resty uses for query params internally). Since resty
// doesn't expose query params directly before execution, we use a test
// HTTP client with a capturing transport.
func buildQueryParams(t *testing.T, p PaginationParams) map[string]string {
	t.Helper()

	// Create a resty request and apply pagination params
	r := resty.New().R()
	p.Apply(r)

	// resty stores query params internally; we can access them via QueryParam
	result := make(map[string]string)
	for k, v := range r.QueryParam {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

func TestPaginationParams_Default_FirstPage(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      100,
		Filters:       map[string]string{"filter": "status==\"active\""},
	})

	if _, ok := params["page-key"]; ok {
		t.Error("page-key should not be set on first page")
	}
	if params["page-size"] != "100" {
		t.Errorf("expected page-size=100, got %q", params["page-size"])
	}
	if params["filter"] != "status==\"active\"" {
		t.Errorf("expected filter on first page, got %q", params["filter"])
	}
}

func TestPaginationParams_Default_SubsequentPage(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		NextPageKey:   "abc123",
		PageSize:      100,
		Filters:       map[string]string{"filter": "status==\"active\""},
	})

	if params["page-key"] != "abc123" {
		t.Errorf("expected page-key=abc123, got %q", params["page-key"])
	}
	if _, ok := params["page-size"]; ok {
		t.Error("page-size must NOT be sent with page-key on Default style")
	}
	if params["filter"] != "status==\"active\"" {
		t.Errorf("expected filter on subsequent pages, got %q", params["filter"])
	}
}

func TestPaginationParams_Default_NextPageKeyParam(t *testing.T) {
	// Extension API uses "next-page-key" instead of "page-key"
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "next-page-key",
		PageSizeParam: "page-size",
		NextPageKey:   "ext-page-2",
		PageSize:      50,
		Filters:       map[string]string{"name": "my-extension"},
	})

	if params["next-page-key"] != "ext-page-2" {
		t.Errorf("expected next-page-key=ext-page-2, got %q", params["next-page-key"])
	}
	if _, ok := params["page-size"]; ok {
		t.Error("page-size must NOT be sent with next-page-key on Default style")
	}
	if params["name"] != "my-extension" {
		t.Errorf("expected name filter on subsequent page, got %q", params["name"])
	}
}

func TestPaginationParams_DocumentAPI_FirstPage(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDocumentAPI,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      50,
		Filters:       map[string]string{"filter": "type=='dashboard'"},
	})

	if _, ok := params["page-key"]; ok {
		t.Error("page-key should not be set on first page")
	}
	if params["page-size"] != "50" {
		t.Errorf("expected page-size=50, got %q", params["page-size"])
	}
	if params["filter"] != "type=='dashboard'" {
		t.Errorf("expected filter, got %q", params["filter"])
	}
}

func TestPaginationParams_DocumentAPI_SubsequentPage(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDocumentAPI,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		NextPageKey:   "doc-page-2",
		PageSize:      50,
		Filters:       map[string]string{"filter": "type=='dashboard'"},
	})

	if params["page-key"] != "doc-page-2" {
		t.Errorf("expected page-key=doc-page-2, got %q", params["page-key"])
	}
	// Document API: page-size IS sent alongside page-key
	if params["page-size"] != "50" {
		t.Errorf("Document API should send page-size on subsequent pages, got %q", params["page-size"])
	}
	if params["filter"] != "type=='dashboard'" {
		t.Errorf("expected filter on subsequent pages, got %q", params["filter"])
	}
}

func TestPaginationParams_SettingsAPI_FirstPage(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationSettingsAPI,
		PageKeyParam:  "nextPageKey",
		PageSizeParam: "pageSize",
		PageSize:      500,
		Filters:       map[string]string{"schemaIds": "builtin:alerting.profile", "scopes": "environment"},
	})

	if _, ok := params["nextPageKey"]; ok {
		t.Error("nextPageKey should not be set on first page")
	}
	if params["pageSize"] != "500" {
		t.Errorf("expected pageSize=500, got %q", params["pageSize"])
	}
	if params["schemaIds"] != "builtin:alerting.profile" {
		t.Errorf("expected schemaIds on first page, got %q", params["schemaIds"])
	}
	if params["scopes"] != "environment" {
		t.Errorf("expected scopes on first page, got %q", params["scopes"])
	}
}

func TestPaginationParams_SettingsAPI_SubsequentPage(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationSettingsAPI,
		PageKeyParam:  "nextPageKey",
		PageSizeParam: "pageSize",
		NextPageKey:   "settings-page-2",
		PageSize:      500,
		Filters: map[string]string{
			"schemaIds": "builtin:alerting.profile",
			"scopes":    "environment",
			"fields":    "objectId,scope,value",
		},
	})

	if params["nextPageKey"] != "settings-page-2" {
		t.Errorf("expected nextPageKey=settings-page-2, got %q", params["nextPageKey"])
	}
	// Settings API: NOTHING else may be sent with nextPageKey
	for _, forbidden := range []string{"pageSize", "schemaIds", "scopes", "fields"} {
		if _, ok := params[forbidden]; ok {
			t.Errorf("Settings API must NOT send %q with nextPageKey", forbidden)
		}
	}
}

func TestPaginationParams_EmptyFilters(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      100,
		Filters:       map[string]string{"filter": "", "name": ""},
	})

	if _, ok := params["filter"]; ok {
		t.Error("empty filter values should not be set as query params")
	}
	if _, ok := params["name"]; ok {
		t.Error("empty name values should not be set as query params")
	}
}

func TestPaginationParams_NilFilters(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      100,
	})

	if params["page-size"] != "100" {
		t.Errorf("expected page-size=100, got %q", params["page-size"])
	}
}

func TestPaginationParams_ZeroPageSize(t *testing.T) {
	params := buildQueryParams(t, PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      0,
	})

	if _, ok := params["page-size"]; ok {
		t.Error("page-size should not be set when PageSize is 0")
	}
}

func TestPaginationParams_NoPageSizeParam(t *testing.T) {
	// Some endpoints (e.g. extension versions, snapshots) don't use page-size at all
	params := buildQueryParams(t, PaginationParams{
		Style:        PaginationDefault,
		PageKeyParam: "next-page-key",
		NextPageKey:  "page-2",
	})

	if params["next-page-key"] != "page-2" {
		t.Errorf("expected next-page-key=page-2, got %q", params["next-page-key"])
	}
	if len(params) != 1 {
		t.Errorf("expected exactly 1 param, got %d: %v", len(params), params)
	}
}
