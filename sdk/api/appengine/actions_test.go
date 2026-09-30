package appengine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

func newTestActionHandler(t *testing.T, body string) *Handler {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"), httpclient.WithRetry(0, time.Millisecond, time.Millisecond))
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return NewHandler(c)
}

func TestExtractActionsFromManifest(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want []AppAction
	}{
		{
			name: "nil manifest",
			app:  App{ID: "acme.app", Name: "Acme"},
			want: nil,
		},
		{
			name: "manifest without actions",
			app:  App{ID: "acme.app", Name: "Acme", Manifest: map[string]interface{}{"functions": map[string]interface{}{}}},
			want: nil,
		},
		{
			name: "actions is not an array",
			app:  App{ID: "acme.app", Name: "Acme", Manifest: map[string]interface{}{"actions": "nope"}},
			want: nil,
		},
		{
			// Every modelled key gets a field and none of them reach Extra,
			// which is left nil rather than empty.
			name: "only modelled keys",
			app: App{ID: "acme.app", Name: "Acme", Manifest: map[string]interface{}{
				"actions": []interface{}{
					map[string]interface{}{
						"name":        "send-report",
						"title":       "Send report",
						"description": "Sends the daily report",
						"stateful":    true,
					},
				},
			}},
			want: []AppAction{{
				AppID:       "acme.app",
				AppName:     "Acme",
				ActionName:  "send-report",
				Title:       "Send report",
				Description: "Sends the daily report",
				Stateful:    true,
				FullName:    "acme.app/send-report",
			}},
		},
		{
			name: "optional fields absent",
			app: App{ID: "acme.app", Name: "Acme", Manifest: map[string]interface{}{
				"actions": []interface{}{
					map[string]interface{}{"name": "bare"},
				},
			}},
			want: []AppAction{{
				AppID:      "acme.app",
				AppName:    "Acme",
				ActionName: "bare",
				FullName:   "acme.app/bare",
			}},
		},
		{
			// The reason Extra exists: a key dtctl has never heard of still
			// reaches the caller, with no dtctl change -- and the modelled
			// keys beside it are not repeated there.
			name: "unmodelled keys survive in Extra",
			app: App{ID: "acme.app", Name: "Acme", Manifest: map[string]interface{}{
				"actions": []interface{}{
					map[string]interface{}{
						"name":                 "rich",
						"title":                "Rich",
						"approval":             true,
						"widget":               "/actions/rich",
						"icon":                 "AiIcon",
						"expressionValidation": map[string]interface{}{"owner": "required"},
						"somethingNewIn2027":   "surprise",
					},
				},
			}},
			want: []AppAction{{
				AppID:      "acme.app",
				AppName:    "Acme",
				ActionName: "rich",
				Title:      "Rich",
				FullName:   "acme.app/rich",
				Extra: map[string]interface{}{
					"approval":             true,
					"widget":               "/actions/rich",
					"icon":                 "AiIcon",
					"expressionValidation": map[string]interface{}{"owner": "required"},
					"somethingNewIn2027":   "surprise",
				},
			}},
		},
		{
			// An unnamed entry cannot be addressed as app-id/action-name, so
			// emitting it would produce a row no other command can accept.
			name: "entry without a name is skipped",
			app: App{ID: "acme.app", Name: "Acme", Manifest: map[string]interface{}{
				"actions": []interface{}{
					map[string]interface{}{"title": "No name"},
					"not-a-map",
					map[string]interface{}{"name": "kept"},
				},
			}},
			want: []AppAction{{
				AppID:      "acme.app",
				AppName:    "Acme",
				ActionName: "kept",
				FullName:   "acme.app/kept",
			}},
		},
		{
			// Actions are read straight off the manifest, so an app whose
			// resourceStatus never mentions FUNCTIONS still has its actions
			// listed -- unlike ListFunctions, which requires that subresource.
			name: "actions without functions",
			app: App{ID: "acme.app", Name: "Acme",
				ResourceStatus: &ResourceStatus{Status: "OK"},
				Manifest: map[string]interface{}{
					"actions": []interface{}{map[string]interface{}{"name": "standalone"}},
				}},
			want: []AppAction{{
				AppID:      "acme.app",
				AppName:    "Acme",
				ActionName: "standalone",
				FullName:   "acme.app/standalone",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractActionsFromManifest(&tt.app)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestHandler_GetAction(t *testing.T) {
	const body = `{"id":"acme.app","name":"Acme","manifest":{"actions":[
		{"name":"send-report","title":"Send report","stateful":true,"widget":"/actions/send-report"}
	]}}`

	t.Run("found", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		action, err := h.GetAction(context.Background(), "acme.app/send-report")
		if err != nil {
			t.Fatalf("GetAction: %v", err)
		}
		if action.ActionName != "send-report" || action.Title != "Send report" || !action.Stateful {
			t.Errorf("unexpected action: %+v", action)
		}
		if action.FullName != "acme.app/send-report" {
			t.Errorf("FullName = %q", action.FullName)
		}
		if action.Extra["widget"] != "/actions/send-report" {
			t.Errorf("Extra did not retain the unmodelled key: %+v", action.Extra)
		}
		if _, repeated := action.Extra["title"]; repeated {
			t.Errorf("Extra repeats a modelled key: %+v", action.Extra)
		}
	})

	t.Run("not found in app", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		_, err := h.GetAction(context.Background(), "acme.app/missing")
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), `action "missing" not found in app "acme.app"`) {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("malformed name is rejected before any request", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		_, err := h.GetAction(context.Background(), "acme.app")
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "expected 'app-id/action-name'") {
			t.Errorf("error = %v", err)
		}
	})
}

func TestHandler_ListActions(t *testing.T) {
	// GET /apps:search-actions groups actions under their app. Every entry
	// here carries an unmodelled key, because a list must drop them whatever
	// the source returned.
	const body = `{"totalCount":2,"apps":[
		{"id":"other.app","name":"Other","actions":[{"name":"b1","widget":"/w/b1"}]},
		{"id":"acme.app","name":"Acme","actions":[
			{"name":"a2","title":"A2","stateful":true,"approval":true},
			{"name":"a1","icon":"AiIcon"},
			{"title":"nameless"}
		]}
	]}`

	t.Run("flattens every app, sorted by full name", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		actions, err := h.ListActions(context.Background(), ListActionsOptions{})
		if err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		var got []string
		for _, a := range actions {
			got = append(got, a.FullName)
		}
		want := []string{"acme.app/a1", "acme.app/a2", "other.app/b1"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("modelled fields are mapped", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		actions, err := h.ListActions(context.Background(), ListActionsOptions{AppID: "acme.app"})
		if err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		if len(actions) != 2 {
			t.Fatalf("got %d actions, want 2: %+v", len(actions), actions)
		}
		a2 := actions[1]
		if a2.ActionName != "a2" || a2.Title != "A2" || !a2.Stateful {
			t.Errorf("unexpected action: %+v", a2)
		}
		if a2.AppID != "acme.app" || a2.AppName != "Acme" {
			t.Errorf("app fields not mapped: %+v", a2)
		}
	})

	t.Run("a list never carries Extra", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		actions, err := h.ListActions(context.Background(), ListActionsOptions{})
		if err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		for _, a := range actions {
			if a.Extra != nil {
				t.Errorf("%s carries Extra %+v; the flat /actions collection cannot serve it", a.FullName, a.Extra)
			}
		}
	})

	t.Run("filtered by app id", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		actions, err := h.ListActions(context.Background(), ListActionsOptions{AppID: "other.app"})
		if err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		if len(actions) != 1 || actions[0].FullName != "other.app/b1" {
			t.Fatalf("unexpected actions: %+v", actions)
		}
	})

	t.Run("filter matching no app", func(t *testing.T) {
		h := newTestActionHandler(t, body)
		actions, err := h.ListActions(context.Background(), ListActionsOptions{AppID: "nope.app"})
		if err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		if len(actions) != 0 {
			t.Fatalf("got %d actions, want 0", len(actions))
		}
	})

	t.Run("requests the search-actions endpoint", func(t *testing.T) {
		var path string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			_, _ = w.Write([]byte(`{"apps":[]}`))
		}))
		t.Cleanup(srv.Close)
		c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"), httpclient.WithRetry(0, time.Millisecond, time.Millisecond))
		if err != nil {
			t.Fatalf("httpclient.New: %v", err)
		}
		if _, err := NewHandler(c).ListActions(context.Background(), ListActionsOptions{}); err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		if path != "/platform/app-engine/registry/v1/apps:search-actions" {
			t.Errorf("requested %q", path)
		}
	})
}

func TestHandler_ListActions_Query(t *testing.T) {
	t.Run("sent upstream as the query param", func(t *testing.T) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.URL.Query().Get("query")
			_, _ = w.Write([]byte(`{"apps":[]}`))
		}))
		t.Cleanup(srv.Close)
		c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"), httpclient.WithRetry(0, time.Millisecond, time.Millisecond))
		if err != nil {
			t.Fatalf("httpclient.New: %v", err)
		}
		if _, err := NewHandler(c).ListActions(context.Background(), ListActionsOptions{Query: "jira create"}); err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		if got != "jira create" {
			t.Errorf("query = %q, want %q", got, "jira create")
		}
	})

	t.Run("omitted when empty", func(t *testing.T) {
		var present bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, present = r.URL.Query()["query"]
			_, _ = w.Write([]byte(`{"apps":[]}`))
		}))
		t.Cleanup(srv.Close)
		c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"), httpclient.WithRetry(0, time.Millisecond, time.Millisecond))
		if err != nil {
			t.Fatalf("httpclient.New: %v", err)
		}
		if _, err := NewHandler(c).ListActions(context.Background(), ListActionsOptions{}); err != nil {
			t.Fatalf("ListActions: %v", err)
		}
		if present {
			t.Error("query param sent for an empty search")
		}
	})

	t.Run("over the length limit is refused before any request", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			_, _ = w.Write([]byte(`{"apps":[]}`))
		}))
		t.Cleanup(srv.Close)
		c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"), httpclient.WithRetry(0, time.Millisecond, time.Millisecond))
		if err != nil {
			t.Fatalf("httpclient.New: %v", err)
		}
		long := strings.Repeat("x", MaxActionQueryLength+1)
		_, err = NewHandler(c).ListActions(context.Background(), ListActionsOptions{Query: long})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "the maximum is 256") {
			t.Errorf("error = %v", err)
		}
		if called {
			t.Error("request sent despite an over-long search")
		}
	})
}
