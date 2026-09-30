package appengine

import (
	"context"

	sdkae "github.com/dynatrace-oss/dtctl/sdk/api/appengine"
)

// AppAction represents an action declared by an app (CLI version with table
// tags).
type AppAction struct {
	AppID       string `json:"appId" table:"APP_ID,wide"`
	AppName     string `json:"appName" table:"APP"`
	ActionName  string `json:"actionName" table:"ACTION"`
	Title       string `json:"title,omitempty" table:"TITLE"`
	Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
	Stateful    bool   `json:"stateful,omitempty" table:"STATEFUL,wide"`
	FullName    string `json:"fullName" table:"FULL_NAME"`

	// Extra holds the manifest keys with no typed field above. Hidden from
	// every table -- it is nested and arbitrarily wide -- and rendered by
	// `describe action`.
	Extra map[string]interface{} `json:"extra,omitempty" table:"-"`
}

// fromSDKAppAction converts an SDK AppAction to a CLI AppAction.
func fromSDKAppAction(s *sdkae.AppAction) AppAction {
	return AppAction{
		AppID:       s.AppID,
		AppName:     s.AppName,
		ActionName:  s.ActionName,
		Title:       s.Title,
		Description: s.Description,
		Stateful:    s.Stateful,
		FullName:    s.FullName,
		Extra:       s.Extra,
	}
}

// ListActionsOptions narrows what ListActions returns.
type ListActionsOptions = sdkae.ListActionsOptions

// ListActions lists all actions across apps, narrowed by opts
func (h *Handler) ListActions(opts ListActionsOptions) ([]AppAction, error) {
	sdkResult, err := h.sdk.ListActions(context.Background(), opts)
	if err != nil {
		return nil, err
	}
	actions := make([]AppAction, len(sdkResult))
	for i := range sdkResult {
		actions[i] = fromSDKAppAction(&sdkResult[i])
	}
	return actions, nil
}

// GetAction gets details about a specific action
func (h *Handler) GetAction(fullName string) (*AppAction, error) {
	sdkResult, err := h.sdk.GetAction(context.Background(), fullName)
	if err != nil {
		return nil, err
	}
	a := fromSDKAppAction(sdkResult)
	return &a, nil
}
