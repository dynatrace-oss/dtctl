package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

const (
	breakpointDefaultOutputMessage   = "Hit on {store.rookout.frame.filename}:{store.rookout.frame.line}"
	breakpointRookoutTargetID        = "Rookout"
	breakpointRookoutTargetName      = "send_rookout"
	breakpointRookoutOnPremTargetName = "send_rookout_data_on_prem"
)

var editBreakpointCmd = &cobra.Command{
	Use:     "breakpoint <id|filename:line>",
	Aliases: []string{"breakpoints", "bp"},
	Short:   "Edit a Live Debugger breakpoint",
	Long: `Edit Live Debugger breakpoints by mutable rule ID or by source location.

Examples:
  # Add or update a condition
  dtctl edit breakpoint dtctl-rule-123 --condition "value>othervalue"

  # Enable a breakpoint
  dtctl edit breakpoint OrderController.java:306 --enabled true

  # Disable a breakpoint
  dtctl edit breakpoint OrderController.java:306 --enabled false
`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		conditionChanged := cmd.Flags().Changed("condition")
		enabled, enabledChanged, err := getOptionalBoolFlag(cmd, "enabled", args[1:])
		if err != nil {
			return err
		}
		if len(args) > 1 && !enabledChanged {
			return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
		}
		if !conditionChanged && !enabledChanged {
			return fmt.Errorf("at least one of --condition or --enabled is required")
		}

		condition, _ := cmd.Flags().GetString("condition")
		identifier := strings.TrimSpace(args[0])
		verbose := isDebugVerbose()

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		ctx, err := cfg.CurrentContextObj()
		if err != nil {
			return err
		}

		if dryRun {
			changes := describeBreakpointEdits(conditionChanged, condition, enabledChanged, enabled)
			printOutf("Dry run: would edit breakpoint %s (%s)\n", identifier, changes)
			return nil
		}

		checker, err := NewSafetyChecker(cfg)
		if err != nil {
			return err
		}
		if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler, err := livedebugger.NewHandler(c, ctx.Environment)
		if err != nil {
			return err
		}

		workspaceResp, workspaceID, err := handler.GetOrCreateWorkspace(currentProjectPath())
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("getOrCreateWorkspaceV2", workspaceResp)
			}
			return err
		}
		if verbose {
			if err := printGraphQLResponse("getOrCreateWorkspaceV2", workspaceResp); err != nil {
				return err
			}
		}

		workspaceRulesResp, err := handler.GetWorkspaceRules(workspaceID)
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("getWorkspaceRules", workspaceRulesResp)
			}
			return err
		}
		if verbose {
			if err := printGraphQLResponse("getWorkspaceRules", workspaceRulesResp); err != nil {
				return err
			}
		}

		rules, err := extractWorkspaceRules(workspaceRulesResp)
		if err != nil {
			return err
		}

		targetRules, targetDescription, allowDirectID, err := resolveBreakpointRulesForEdit(rules, identifier)
		if err != nil {
			return err
		}

		if conditionChanged {
			if len(targetRules) == 0 {
				return fmt.Errorf("breakpoint %q not found in the current workspace", identifier)
			}
			for _, rule := range targetRules {
				ruleSettings, err := buildEditBreakpointSettings(rule, condition, true)
				if err != nil {
					return err
				}
				editResp, err := handler.EditBreakpoint(workspaceID, ruleSettings)
				if err != nil {
					if verbose {
						_ = printGraphQLResponse("editRuleV2", editResp)
					}
					return err
				}
				if verbose {
					if err := printGraphQLResponse("editRuleV2", editResp); err != nil {
						return err
					}
				}
			}
		}

		if enabledChanged {
			ruleIDs := make([]string, 0, len(targetRules))
			for _, rule := range targetRules {
				if row, ok := breakpointRowFromRule(rule); ok && row.ID != "" {
					ruleIDs = append(ruleIDs, row.ID)
				}
			}
			if len(ruleIDs) == 0 && allowDirectID {
				ruleIDs = append(ruleIDs, identifier)
			}
			if len(ruleIDs) == 0 {
				return fmt.Errorf("breakpoint %q not found in the current workspace", identifier)
			}
			enableResp, err := handler.EnableOrDisableBreakpoints(workspaceID, ruleIDs, !enabled)
			if err != nil {
				if verbose {
					_ = printGraphQLResponse("enableOrDisableRules", enableResp)
				}
				return err
			}
			if verbose {
				if err := printGraphQLResponse("enableOrDisableRules", enableResp); err != nil {
					return err
				}
			}
		}

		printOutf("Edited breakpoint %s (%s)\n", targetDescription, describeBreakpointEdits(conditionChanged, condition, enabledChanged, enabled))
		return nil
	},
}

func resolveBreakpointRulesForEdit(rules []map[string]interface{}, identifier string) ([]map[string]interface{}, string, bool, error) {
	if fileName, lineNumber, err := parseBreakpoint(identifier); err == nil {
		matches := findBreakpointRulesByLocation(rules, fileName, lineNumber)
		if len(matches) == 0 {
			return nil, "", false, fmt.Errorf("no breakpoints found at %s:%d", fileName, lineNumber)
		}
		return matches, fmt.Sprintf("%s:%d", fileName, lineNumber), false, nil
	}

	if rule, ok := findBreakpointRuleByID(rules, identifier); ok {
		return []map[string]interface{}{rule}, identifier, false, nil
	}

	return nil, identifier, true, nil
}

func buildEditBreakpointSettings(rule map[string]interface{}, condition string, conditionChanged bool) (map[string]interface{}, error) {
	row, ok := breakpointRowFromRule(rule)
	if !ok || row.ID == "" {
		return nil, fmt.Errorf("rule missing mutable rule id")
	}

	aug, ok := rule["aug_json"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("rule %s missing aug_json", row.ID)
	}

	paths := extractBreakpointOperationPaths(aug)
	currentCondition := stringValue(aug["conditional"])
	if conditionChanged {
		currentCondition = condition
	}

	settings := map[string]interface{}{
		"mutableRuleId":             row.ID,
		"collectLocalsMethod":       stringValue(paths["store.rookout.frame"]),
		"stackTraceCollection":      stringValue(paths["store.rookout.traceback"]) == "stack.traceback()",
		"ttlHitLimit":               intValue(aug["globalHitLimit"], 100),
		"ttlTimeLimit":              stringValue(aug["globalDisableAfterTime"]),
		"ttlTimeLimitInterval":      0,
		"collectedVariables":        extractCollectedVariables(paths),
		"targetConfiguration":       map[string]interface{}{"targetId": extractBreakpointTargetID(rule)},
		"outputMessage":             extractBreakpointOutputMessage(rule),
		"condition":                 currentCondition,
		"rateLimit":                 stringValue(aug["rateLimit"]),
		"tracingCollection":         stringValue(paths["store.rookout.tracing"]) == "trace.dump()",
		"processMonitoringCollection": stringValue(paths["store.rookout.processMonitoring"]) == "state.dump()",
	}

	return settings, nil
}

func extractBreakpointOperationPaths(aug map[string]interface{}) map[string]interface{} {
	paths := map[string]interface{}{}
	action, ok := aug["action"].(map[string]interface{})
	if !ok {
		return paths
	}
	operations, ok := action["operations"].([]interface{})
	if !ok {
		return paths
	}
	for _, operationIfc := range operations {
		operation, ok := operationIfc.(map[string]interface{})
		if !ok {
			continue
		}
		opPaths, ok := operation["paths"].(map[string]interface{})
		if !ok {
			continue
		}
		for key, value := range opPaths {
			paths[key] = value
		}
	}
	return paths
}

func extractCollectedVariables(paths map[string]interface{}) []string {
	variables := make([]string, 0)
	for key, value := range paths {
		if !strings.HasPrefix(key, "store.rookout.variables.") {
			continue
		}
		pathValue := stringValue(value)
		if pathValue == "" {
			continue
		}
		variables = append(variables, formatCollectedVariable(pathValue))
	}
	sort.Strings(variables)
	return variables
}

func formatCollectedVariable(path string) string {
	trimmed := strings.TrimPrefix(path, "frame.")
	if className, member, ok := parseThreadLocalPath(trimmed); ok {
		return className + "." + member
	}
	return trimmed
}

func parseThreadLocalPath(path string) (string, string, bool) {
	if !strings.HasPrefix(path, "utils.class(\"") {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, "utils.class(\"")
	className, remainder, found := strings.Cut(rest, "\")")
	if !found || className == "" {
		return "", "", false
	}
	member := strings.TrimPrefix(remainder, ".")
	if member == "" {
		return "", "", false
	}
	return className, member, true
}

func extractBreakpointOutputMessage(rule map[string]interface{}) string {
	processing, ok := rule["processing"].(map[string]interface{})
	if !ok {
		return breakpointDefaultOutputMessage
	}
	operations, ok := processing["operations"].([]interface{})
	if !ok {
		return breakpointDefaultOutputMessage
	}
	for _, operationIfc := range operations {
		operation, ok := operationIfc.(map[string]interface{})
		if !ok {
			continue
		}
		if stringValue(operation["path"]) == "temp.message.rookout.message" {
			format := stringValue(operation["format"])
			if format != "" {
				return format
			}
		}
	}
	return breakpointDefaultOutputMessage
}

func extractBreakpointTargetID(rule map[string]interface{}) string {
	processing, ok := rule["processing"].(map[string]interface{})
	if !ok {
		return breakpointRookoutTargetID
	}
	operations, ok := processing["operations"].([]interface{})
	if !ok || len(operations) == 0 {
		return breakpointRookoutTargetID
	}
	lastOperation, ok := operations[len(operations)-1].(map[string]interface{})
	if !ok {
		return breakpointRookoutTargetID
	}
	name := stringValue(lastOperation["name"])
	if name == breakpointRookoutTargetName || name == breakpointRookoutOnPremTargetName {
		return breakpointRookoutTargetID
	}
	if targetID := stringValue(lastOperation["target_id"]); targetID != "" {
		return targetID
	}
	return breakpointRookoutTargetID
}

func findBreakpointRulesByLocation(rules []map[string]interface{}, fileName string, lineNumber int) []map[string]interface{} {
	matches := make([]map[string]interface{}, 0)
	for _, rule := range rules {
		row, ok := breakpointRowFromRule(rule)
		if !ok {
			continue
		}
		if row.Filename == fileName && row.Line == lineNumber {
			matches = append(matches, rule)
		}
	}
	return matches
}

func findBreakpointRuleByID(rules []map[string]interface{}, id string) (map[string]interface{}, bool) {
	for _, rule := range rules {
		row, ok := breakpointRowFromRule(rule)
		if ok && row.ID == id {
			return rule, true
		}
	}
	return nil, false
}

func stringValue(value interface{}) string {
	stringVal, _ := value.(string)
	return stringVal
}

func intValue(value interface{}, defaultValue int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return defaultValue
	}
}

func describeBreakpointEdits(conditionChanged bool, condition string, enabledChanged bool, enabled bool) string {
	changes := make([]string, 0, 2)
	if conditionChanged {
		changes = append(changes, fmt.Sprintf("condition=%q", condition))
	}
	if enabledChanged {
		changes = append(changes, fmt.Sprintf("enabled=%t", enabled))
	}
	return strings.Join(changes, ", ")
}

func getOptionalBoolFlag(cmd *cobra.Command, flagName string, trailingArgs []string) (bool, bool, error) {
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil || !flag.Changed {
		if len(trailingArgs) > 0 {
			return false, false, fmt.Errorf("accepts 1 arg(s), received %d", 1+len(trailingArgs))
		}
		return false, false, nil
	}
	if len(trailingArgs) > 1 {
		return false, true, fmt.Errorf("accepts 1 arg(s), received %d", 1+len(trailingArgs))
	}
	if len(trailingArgs) == 1 {
		parsed, err := strconv.ParseBool(strings.TrimSpace(trailingArgs[0]))
		if err != nil {
			return false, true, fmt.Errorf("invalid boolean value %q for --%s", trailingArgs[0], flagName)
		}
		return parsed, true, nil
	}

	value := strings.TrimSpace(flag.Value.String())
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, true, fmt.Errorf("invalid boolean value %q for --%s", value, flagName)
	}

	return parsed, true, nil
}

func init() {
	editCmd.AddCommand(editBreakpointCmd)
	editBreakpointCmd.Flags().String("condition", "", "Condition expression for the breakpoint")
	editBreakpointCmd.Flags().String("enabled", "", "Enable or disable the breakpoint")
	editBreakpointCmd.Flags().Lookup("enabled").NoOptDefVal = "true"
}
