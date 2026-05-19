package httpclient

import (
	"testing"
)

func TestPaginationParams_Default_FirstPage(t *testing.T) {
	p := PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      100,
		Filters:       map[string]string{"filter": "status=active"},
	}
	params := p.QueryParams()
	if params.Get("page-size") != "100" {
		t.Errorf("page-size = %q", params.Get("page-size"))
	}
	if params.Get("filter") != "status=active" {
		t.Errorf("filter = %q", params.Get("filter"))
	}
	if params.Get("page-key") != "" {
		t.Error("page-key should not be set on first page")
	}
}

func TestPaginationParams_Default_NextPage(t *testing.T) {
	p := PaginationParams{
		Style:         PaginationDefault,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      100,
		NextPageKey:   "abc123",
		Filters:       map[string]string{"filter": "status=active"},
	}
	params := p.QueryParams()
	if params.Get("page-key") != "abc123" {
		t.Errorf("page-key = %q", params.Get("page-key"))
	}
	// page-size must NOT be sent with page-key for Default style
	if params.Get("page-size") != "" {
		t.Error("page-size should not be set with page-key for Default style")
	}
	// filters should still be sent
	if params.Get("filter") != "status=active" {
		t.Errorf("filter = %q", params.Get("filter"))
	}
}

func TestPaginationParams_DocumentAPI_NextPage(t *testing.T) {
	p := PaginationParams{
		Style:         PaginationDocumentAPI,
		PageKeyParam:  "page-key",
		PageSizeParam: "page-size",
		PageSize:      50,
		NextPageKey:   "abc123",
		Filters:       map[string]string{"filter": "type=dashboard"},
	}
	params := p.QueryParams()
	if params.Get("page-key") != "abc123" {
		t.Errorf("page-key = %q", params.Get("page-key"))
	}
	// DocumentAPI: page-size CAN be sent with page-key
	if params.Get("page-size") != "50" {
		t.Errorf("page-size = %q, want 50", params.Get("page-size"))
	}
	if params.Get("filter") != "type=dashboard" {
		t.Errorf("filter = %q", params.Get("filter"))
	}
}

func TestPaginationParams_SettingsAPI_NextPage(t *testing.T) {
	p := PaginationParams{
		Style:         PaginationSettingsAPI,
		PageKeyParam:  "nextPageKey",
		PageSizeParam: "pageSize",
		PageSize:      500,
		NextPageKey:   "abc123",
		Filters:       map[string]string{"schemaIds": "builtin:alerting.profile"},
	}
	params := p.QueryParams()
	if params.Get("nextPageKey") != "abc123" {
		t.Errorf("nextPageKey = %q", params.Get("nextPageKey"))
	}
	// SettingsAPI: NO other params with nextPageKey
	if params.Get("pageSize") != "" {
		t.Error("pageSize should not be set with nextPageKey for SettingsAPI")
	}
	if params.Get("schemaIds") != "" {
		t.Error("schemaIds should not be set with nextPageKey for SettingsAPI")
	}
}
