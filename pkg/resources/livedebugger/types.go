package livedebugger

import sdkld "github.com/dynatrace-oss/dtctl/sdk/api/livedebugger"

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	GraphQLWorkspaceResponse = sdkld.GraphQLWorkspaceResponse
	Workspace                = sdkld.Workspace
	BreakpointRule           = sdkld.BreakpointRule
	RuleStatusNode           = sdkld.RuleStatusNode
	DeleteAllRulesResponse   = sdkld.DeleteAllRulesResponse
)

// Re-export SDK helper functions.
var (
	ExtractWorkspaceID    = sdkld.ExtractWorkspaceID
	ExtractWorkspaceRules = sdkld.ExtractWorkspaceRules
	ExtractRuleStatuses   = sdkld.ExtractRuleStatuses
	ExtractDeletedRuleIDs = sdkld.ExtractDeletedRuleIDs
)
