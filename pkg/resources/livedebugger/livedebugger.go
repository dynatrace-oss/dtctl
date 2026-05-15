package livedebugger

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkld "github.com/dynatrace-oss/dtctl/sdk/api/livedebugger"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles live debugger resources.
type Handler struct {
	sdk *sdkld.Handler
}

func NewHandler(c *client.Client, environmentURL string) (*Handler, error) {
	sdkHandler, err := sdkld.NewHandler(httpclient.Wrap(c.HTTP()), environmentURL)
	if err != nil {
		return nil, err
	}
	return &Handler{sdk: sdkHandler}, nil
}

func (h *Handler) GetOrCreateWorkspace(projectPath string) (map[string]interface{}, string, error) {
	return h.sdk.GetOrCreateWorkspace(projectPath)
}

func (h *Handler) UpdateWorkspaceFilters(workspaceID string, filterSets []map[string]interface{}) (map[string]interface{}, error) {
	return h.sdk.UpdateWorkspaceFilters(workspaceID, filterSets)
}

func (h *Handler) CreateBreakpoint(workspaceID, fileName string, lineNumber int) (map[string]interface{}, error) {
	return h.sdk.CreateBreakpoint(workspaceID, fileName, lineNumber)
}

func (h *Handler) GetWorkspaceRules(workspaceID string) (map[string]interface{}, error) {
	return h.sdk.GetWorkspaceRules(workspaceID)
}

func (h *Handler) DeleteBreakpoint(workspaceID, ruleID string) (map[string]interface{}, error) {
	return h.sdk.DeleteBreakpoint(workspaceID, ruleID)
}

func (h *Handler) GetRuleStatusBreakdown(ruleID string) (map[string]interface{}, error) {
	return h.sdk.GetRuleStatusBreakdown(ruleID)
}

func (h *Handler) EditBreakpoint(workspaceID string, ruleSettings map[string]interface{}) (map[string]interface{}, error) {
	return h.sdk.EditBreakpoint(workspaceID, ruleSettings)
}

func (h *Handler) EnableOrDisableBreakpoints(workspaceID string, ruleIDs []string, isDisabled bool) (map[string]interface{}, error) {
	return h.sdk.EnableOrDisableBreakpoints(workspaceID, ruleIDs, isDisabled)
}

func (h *Handler) DeleteAllBreakpoints(workspaceID string) (map[string]interface{}, error) {
	return h.sdk.DeleteAllBreakpoints(workspaceID)
}

// BuildFilterSets delegates to the SDK implementation.
var BuildFilterSets = sdkld.BuildFilterSets
