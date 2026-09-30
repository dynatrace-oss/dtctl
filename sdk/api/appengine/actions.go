package appengine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// AppAction is one action of an installed app.
//
// The shape is the one the planned flat /actions collection will serve, not
// the one today's sources happen to have, so swapping a source changes no
// output:
//
//   - List reads GET /apps:search-actions and flattens it. It will read
//     GET /actions, which returns the same records already flat.
//   - Get reads the app's manifest and picks one entry out. It will read
//     GET /actions/{appId}/{actionName}.
//
// Extra follows that same split -- see its comment.
type AppAction struct {
	AppID       string `json:"appId"`
	AppName     string `json:"appName"`
	ActionName  string `json:"actionName"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Stateful    bool   `json:"stateful,omitempty"`
	FullName    string `json:"fullName"`

	// Extra holds the keys the fields above do not model -- widget, icon,
	// expressionValidation, approval, and whatever the platform adds next --
	// so a key dtctl has never heard of still reaches the caller. The
	// modelled keys are filtered out, so nothing appears twice.
	//
	// Populated by Get only, never by List. The flat /actions collection
	// serves a fixed field set widened by `add-fields`, an allowlist that
	// deliberately excludes the large fields, so a list can never carry
	// arbitrary keys; only /actions/{appId}/{actionName} returns them all.
	// Today's GET /apps:search-actions does happen to return every key, but
	// populating Extra from it would ship a field that vanishes the day the
	// source is swapped.
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// modelledActionKeys are the manifest keys AppAction gives a typed field.
// They are excluded from Extra; adding a field above means adding its key
// here, or it will be reported in both places.
var modelledActionKeys = map[string]bool{
	"name":        true,
	"title":       true,
	"description": true,
	"stateful":    true,
}

// searchActionsResponse is the wire shape of GET /apps:search-actions: actions
// grouped under the app that declares them. The flat /actions collection will
// replace it, which is why nothing outside this file sees the grouping.
type searchActionsResponse struct {
	Apps []struct {
		ID      string                   `json:"id"`
		Name    string                   `json:"name"`
		Actions []map[string]interface{} `json:"actions"`
	} `json:"apps"`
}

// MaxActionQueryLength is the longest search string the endpoint accepts.
const MaxActionQueryLength = 256

// ListActionsOptions narrows what ListActions returns.
type ListActionsOptions struct {
	// AppID keeps only the actions of one app, by exact id.
	AppID string

	// Query is a whitespace-separated list of search terms. Each term must
	// appear in the app name, app description, action name, title or
	// description; terms are case-insensitive and each one narrows further.
	Query string
}

// ListActions lists the actions of installed apps.
//
// Query is sent upstream; AppID is applied here, because the endpoint's only
// filter is the full-text Query -- an app id passed to it would also match
// every action whose description merely mentions the app.
func (h *Handler) ListActions(ctx context.Context, opts ListActionsOptions) ([]AppAction, error) {
	if len(opts.Query) > MaxActionQueryLength {
		return nil, fmt.Errorf("search is %d characters, the maximum is %d", len(opts.Query), MaxActionQueryLength)
	}

	req := h.client.HTTP().R().SetContext(ctx)
	if opts.Query != "" {
		req.SetQueryParam("query", opts.Query)
	}

	resp, err := req.Get("/platform/app-engine/registry/v1/apps:search-actions")

	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}

	var result searchActionsResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("parse actions response: %w", err)
	}

	var actions []AppAction
	for _, app := range result.Apps {
		if opts.AppID != "" && app.ID != opts.AppID {
			continue
		}
		for _, entry := range app.Actions {
			// withoutExtra: a list never carries unmodelled keys, whatever
			// this source returned. See AppAction.Extra.
			if action, ok := newAppAction(app.ID, app.Name, entry, withoutExtra); ok {
				actions = append(actions, action)
			}
		}
	}

	// Sorted so the order does not change when the source does -- the
	// endpoint groups by app, a manifest walk follows install order, and
	// /actions will do neither.
	slices.SortFunc(actions, func(a, b AppAction) int {
		return strings.Compare(a.FullName, b.FullName)
	})

	return actions, nil
}

// GetAction returns a single action addressed as "app-id/action-name".
func (h *Handler) GetAction(ctx context.Context, fullName string) (*AppAction, error) {
	// Actions are addressed exactly like functions -- app id, slash, name --
	// so they split the same way.
	appID, actionName := parseFullFunctionName(fullName)
	if appID == "" || actionName == "" {
		return nil, fmt.Errorf("invalid action name format, expected 'app-id/action-name', got %q", fullName)
	}

	app, err := h.GetApp(ctx, appID)
	if err != nil {
		return nil, err
	}

	for _, action := range extractActionsFromManifest(app) {
		if action.ActionName == actionName {
			return &action, nil
		}
	}

	return nil, fmt.Errorf("action %q not found in app %q", actionName, appID)
}

// extractActionsFromManifest reads the manifest's "actions" array. An entry
// with no name cannot be addressed as app-id/action-name, so it is skipped
// rather than emitted under an empty name.
func extractActionsFromManifest(app *App) []AppAction {
	if app.Manifest == nil {
		return nil
	}

	actionsArray, ok := app.Manifest["actions"].([]interface{})
	if !ok {
		return nil
	}

	var actions []AppAction
	for _, entry := range actionsArray {
		actionMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if action, ok := newAppAction(app.ID, app.Name, actionMap, withExtra); ok {
			actions = append(actions, action)
		}
	}

	return actions
}

// extraMode says whether an action keeps the keys AppAction does not model.
type extraMode bool

const (
	withExtra    extraMode = true
	withoutExtra extraMode = false
)

// newAppAction builds one action from a source record, whether that came from
// a manifest entry or from an endpoint -- both are the same object. It reports
// false for a record with no name: such an action cannot be addressed as
// app-id/action-name, so emitting it would produce a row no other command
// accepts.
func newAppAction(appID, appName string, entry map[string]interface{}, mode extraMode) (AppAction, bool) {
	name, _ := entry["name"].(string)
	if name == "" {
		return AppAction{}, false
	}
	title, _ := entry["title"].(string)
	description, _ := entry["description"].(string)
	stateful, _ := entry["stateful"].(bool)

	var extra map[string]interface{}
	if mode == withExtra {
		for key, value := range entry {
			if modelledActionKeys[key] {
				continue
			}
			if extra == nil {
				extra = make(map[string]interface{})
			}
			extra[key] = value
		}
	}

	return AppAction{
		AppID:       appID,
		AppName:     appName,
		ActionName:  name,
		Title:       title,
		Description: description,
		Stateful:    stateful,
		FullName:    fmt.Sprintf("%s/%s", appID, name),
		Extra:       extra,
	}, true
}
