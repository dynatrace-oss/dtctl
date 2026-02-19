package appengine

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles App Engine resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new App Engine handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// App represents an installed app
type App struct {
	ID               string                 `json:"id" table:"ID"`
	Name             string                 `json:"name" table:"NAME"`
	Version          string                 `json:"version" table:"VERSION"`
	Description      string                 `json:"description" table:"DESCRIPTION,wide"`
	IsBuiltin        bool                   `json:"isBuiltin,omitempty" table:"BUILTIN,wide"`
	ResourceStatus   *ResourceStatus        `json:"resourceStatus,omitempty" table:"-"`
	SignatureInfo    *SignatureInfo         `json:"signatureInfo,omitempty" table:"-"`
	Manifest         map[string]interface{} `json:"manifest,omitempty" table:"-"`
	ModificationInfo *ModificationInfo      `json:"modificationInfo,omitempty" table:"-"`
}

// ResourceStatus represents the status of an app's resources
type ResourceStatus struct {
	Status              string   `json:"status"`
	SubResourceTypes    []string `json:"subResourceTypes,omitempty"`
	SubResourceStatuses []string `json:"subResourceStatuses,omitempty"`
}

// SignatureInfo represents signature information for an app
type SignatureInfo struct {
	Signature string `json:"signature,omitempty"`
}

// ModificationInfo contains modification timestamps
type ModificationInfo struct {
	CreatedBy        string `json:"createdBy,omitempty"`
	CreatedTime      string `json:"createdTime,omitempty"`
	LastModifiedBy   string `json:"lastModifiedBy,omitempty"`
	LastModifiedTime string `json:"lastModifiedTime,omitempty"`
}

// AppList represents a list of apps
type AppList struct {
	Apps []App `json:"apps"`
}

// ListApps lists all installed apps
func (h *Handler) ListApps() (*AppList, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "isBuiltin,manifest,resourceStatus.subResourceTypes").
		Get("/platform/app-engine/registry/v1/apps")

	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list apps: status %d: %s", resp.StatusCode(), resp.String())
	}

	var result AppList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse apps response: %w", err)
	}

	return &result, nil
}

// GetApp gets a specific app by ID
func (h *Handler) GetApp(appID string) (*App, error) {
	resp, err := h.client.HTTP().R().
		SetQueryParam("add-fields", "isBuiltin,manifest,resourceStatus.subResourceTypes").
		Get(fmt.Sprintf("/platform/app-engine/registry/v1/apps/%s", appID))

	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("app %q not found", appID)
		default:
			return nil, fmt.Errorf("failed to get app: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	var result App
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse app response: %w", err)
	}

	return &result, nil
}

// DeleteApp uninstalls an app
func (h *Handler) DeleteApp(appID string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/app-engine/registry/v1/apps/%s", appID))

	if err != nil {
		return fmt.Errorf("failed to uninstall app: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("app %q not found", appID)
		case 403:
			return fmt.Errorf("access denied to uninstall app %q", appID)
		default:
			return fmt.Errorf("failed to uninstall app: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// AppFunction represents a function within an app
type AppFunction struct {
	AppID        string `json:"appId" table:"APP_ID,wide"`
	AppName      string `json:"appName" table:"APP"`
	FunctionName string `json:"functionName" table:"FUNCTION"`
	Title        string `json:"title,omitempty" table:"TITLE,wide"`
	Description  string `json:"description,omitempty" table:"DESCRIPTION,wide"`
	Resumable    bool   `json:"resumable" table:"RESUMABLE,wide"`
	Stateful     bool   `json:"stateful,omitempty" table:"STATEFUL,wide"`
	FullName     string `json:"fullName" table:"FULL_NAME"`
}

// ListFunctions lists all functions across apps (or filtered by app ID)
func (h *Handler) ListFunctions(appIDFilter string) ([]AppFunction, error) {
	// Get all apps with manifest and resourceStatus in a single call
	appList, err := h.ListApps()
	if err != nil {
		return nil, err
	}

	var functions []AppFunction

	// For each app, check if it has functions (no additional API calls needed)
	for _, app := range appList.Apps {
		// If filter is set, skip apps that don't match
		if appIDFilter != "" && app.ID != appIDFilter {
			continue
		}

		// Check if app has FUNCTIONS subresource type (data already fetched)
		if app.ResourceStatus == nil || !contains(app.ResourceStatus.SubResourceTypes, "FUNCTIONS") {
			continue
		}

		// Extract functions from manifest (already fetched)
		if app.Manifest != nil {
			// First, build a map of action metadata by function name
			actionMetadata := make(map[string]struct {
				title       string
				description string
				stateful    bool
			})

			if actionsArray, ok := app.Manifest["actions"].([]interface{}); ok {
				for _, action := range actionsArray {
					if actionMap, ok := action.(map[string]interface{}); ok {
						name, _ := actionMap["name"].(string)
						title, _ := actionMap["title"].(string)
						description, _ := actionMap["description"].(string)
						stateful, _ := actionMap["stateful"].(bool)

						actionMetadata[name] = struct {
							title       string
							description string
							stateful    bool
						}{title, description, stateful}
					}
				}
			}

			// Now extract functions and merge with action metadata
			if functionsMap, ok := app.Manifest["functions"].(map[string]interface{}); ok {
				for functionName, functionData := range functionsMap {
					resumable := false
					if functionDataMap, ok := functionData.(map[string]interface{}); ok {
						if res, ok := functionDataMap["resumable"].(bool); ok {
							resumable = res
						}
					}

					// Get action metadata if available
					metadata := actionMetadata[functionName]

					functions = append(functions, AppFunction{
						AppID:        app.ID,
						AppName:      app.Name,
						FunctionName: functionName,
						Title:        metadata.title,
						Description:  metadata.description,
						Resumable:    resumable,
						Stateful:     metadata.stateful,
						FullName:     fmt.Sprintf("%s/%s", app.ID, functionName),
					})
				}
			}
		}
	}

	return functions, nil
}

// GetFunction gets details about a specific function
func (h *Handler) GetFunction(fullName string) (*AppFunction, error) {
	// Parse app-id/function-name format
	appID, functionName := parseFullFunctionName(fullName)
	if appID == "" || functionName == "" {
		return nil, fmt.Errorf("invalid function name format, expected 'app-id/function-name', got %q", fullName)
	}

	// Get app details
	app, err := h.GetApp(appID)
	if err != nil {
		return nil, err
	}

	// Check if app has functions
	if app.ResourceStatus == nil || !contains(app.ResourceStatus.SubResourceTypes, "FUNCTIONS") {
		return nil, fmt.Errorf("app %q does not have functions", appID)
	}

	// Find the function in the manifest
	if app.Manifest != nil {
		// First, get action metadata if available
		var title, description string
		var stateful bool

		if actionsArray, ok := app.Manifest["actions"].([]interface{}); ok {
			for _, action := range actionsArray {
				if actionMap, ok := action.(map[string]interface{}); ok {
					if name, _ := actionMap["name"].(string); name == functionName {
						title, _ = actionMap["title"].(string)
						description, _ = actionMap["description"].(string)
						stateful, _ = actionMap["stateful"].(bool)
						break
					}
				}
			}
		}

		// Now get function data
		if functionsMap, ok := app.Manifest["functions"].(map[string]interface{}); ok {
			if functionData, ok := functionsMap[functionName]; ok {
				resumable := false
				if functionDataMap, ok := functionData.(map[string]interface{}); ok {
					if res, ok := functionDataMap["resumable"].(bool); ok {
						resumable = res
					}
				}

				return &AppFunction{
					AppID:        app.ID,
					AppName:      app.Name,
					FunctionName: functionName,
					Title:        title,
					Description:  description,
					Resumable:    resumable,
					Stateful:     stateful,
					FullName:     fullName,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("function %q not found in app %q", functionName, appID)
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func parseFullFunctionName(fullName string) (appID, functionName string) {
	for i := 0; i < len(fullName); i++ {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return "", ""
}
