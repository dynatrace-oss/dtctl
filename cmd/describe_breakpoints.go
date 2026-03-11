package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
	"github.com/spf13/cobra"
)

type breakpointStatusResult struct {
	ID                 string                  `json:"id" yaml:"id"`
	Location           string                  `json:"location,omitempty" yaml:"location,omitempty"`
	Enabled            bool                    `json:"enabled" yaml:"enabled"`
	DisableReason      string                  `json:"disableReason,omitempty" yaml:"disableReason,omitempty"`
	Status             string                  `json:"status" yaml:"status"`
	ActiveRooks        []breakpointRookInfo    `json:"activeRooks,omitempty" yaml:"activeRooks,omitempty"`
	ActiveTips         []breakpointTip         `json:"activeTips,omitempty" yaml:"activeTips,omitempty"`
	PendingRooks       []breakpointRookInfo    `json:"pendingRooks,omitempty" yaml:"pendingRooks,omitempty"`
	PendingTips        []breakpointTip         `json:"pendingTips,omitempty" yaml:"pendingTips,omitempty"`
	Warnings           []breakpointStatusIssue `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Errors             []breakpointStatusIssue `json:"errors,omitempty" yaml:"errors,omitempty"`
	ControllerWarnings []breakpointStatusIssue `json:"controllerWarnings,omitempty" yaml:"controllerWarnings,omitempty"`
	ControllerErrors   []breakpointStatusIssue `json:"controllerErrors,omitempty" yaml:"controllerErrors,omitempty"`
}

type breakpointRookInfo struct {
	ID         string `json:"id,omitempty" yaml:"id,omitempty"`
	Hostname   string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Executable string `json:"executable,omitempty" yaml:"executable,omitempty"`
}

type breakpointTip struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	DocsLink    string `json:"docsLink,omitempty" yaml:"docsLink,omitempty"`
}

type breakpointStatusIssue struct {
	Title       string               `json:"title" yaml:"title"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	DocsLink    string               `json:"docsLink,omitempty" yaml:"docsLink,omitempty"`
	Args        interface{}          `json:"args,omitempty" yaml:"args,omitempty"`
	Rooks       []breakpointRookInfo `json:"rooks,omitempty" yaml:"rooks,omitempty"`
	Controllers []string             `json:"controllers,omitempty" yaml:"controllers,omitempty"`
}

func runDescribeCommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return requireSubcommand(cmd, args)
	}

	if !shouldHandleAsBreakpointDescribe(args[0]) {
		return requireSubcommand(cmd, args)
	}

	return runDescribeBreakpoint(cmd, args[0])
}

func shouldHandleAsBreakpointDescribe(identifier string) bool {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return false
	}

	if _, _, err := parseBreakpoint(trimmed); err == nil {
		return true
	}

	if strings.HasPrefix(trimmed, "dtctl-rule-") {
		return true
	}

	if strings.HasPrefix(trimmed, "bp-") {
		return true
	}

	if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return true
	}

	return false
}

func runDescribeBreakpoint(cmd *cobra.Command, identifier string) error {
	verbose := isDebugVerbose()

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	ctx, err := cfg.CurrentContextObj()
	if err != nil {
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

	targetRules, _, allowDirectID, err := resolveBreakpointRulesForEdit(rules, strings.TrimSpace(identifier))
	if err != nil {
		return err
	}
	if allowDirectID {
		targetRules = []map[string]interface{}{{"id": strings.TrimSpace(identifier)}}
	}

	results := make([]breakpointStatusResult, 0, len(targetRules))
	for _, rule := range targetRules {
		ruleID := stringValue(rule["id"])
		statusResp, err := handler.GetRuleStatusBreakdown(ruleID)
		if err != nil {
			if verbose {
				_ = printGraphQLResponse("GetRuleStatusBreakdown", statusResp)
			}
			return err
		}
		if verbose {
			if err := printGraphQLResponse("GetRuleStatusBreakdown", statusResp); err != nil {
				return err
			}
		}

		result, err := buildBreakpointStatusResult(rule, statusResp)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	if !useBreakpointDescribeTextView() {
		printer := NewPrinter()
		if len(results) == 1 {
			return printer.Print(results[0])
		}
		return printer.Print(results)
	}

	for i, result := range results {
		if i > 0 {
			fmt.Println()
		}
		printBreakpointStatusResult(result)
	}

	return nil
}

func useBreakpointDescribeTextView() bool {
	return outputFormat == "" || outputFormat == "table" || outputFormat == "wide" || outputFormat == "csv"
}

func buildBreakpointStatusResult(rule map[string]interface{}, statusResp map[string]interface{}) (breakpointStatusResult, error) {
	result := breakpointStatusResult{
		ID:      stringValue(rule["id"]),
		Enabled: !boolValue(rule["is_disabled"]),
	}
	if row, ok := breakpointRowFromRule(rule); ok {
		result.ID = row.ID
		result.Location = fmt.Sprintf("%s:%d", row.Filename, row.Line)
		result.Enabled = row.Active
	}
	result.DisableReason = stringValue(rule["disable_reason"])

	ruleStatuses, err := extractRuleStatuses(statusResp)
	if err != nil {
		return result, err
	}

	result.ActiveRooks, result.ActiveTips = extractRooksAndTipsByStatus(ruleStatuses, "Active")
	result.PendingRooks, result.PendingTips = extractRooksAndTipsByStatus(ruleStatuses, "Pending", "None")
	result.Warnings = summarizeRookIssues(ruleStatuses, "Warning")
	result.Errors = summarizeRookIssues(ruleStatuses, "Error")
	result.ControllerWarnings = summarizeControllerIssues(ruleStatuses, "Warning")
	result.ControllerErrors = summarizeControllerIssues(ruleStatuses, "Error")
	result.Status = deriveOverallBreakpointStatus(result)

	return result, nil
}

func deriveOverallBreakpointStatus(result breakpointStatusResult) string {
	if !result.Enabled {
		return "Disabled"
	}
	if len(result.Errors) > 0 || len(result.ControllerErrors) > 0 {
		return "Error"
	}
	if len(result.Warnings) > 0 || len(result.ControllerWarnings) > 0 {
		return "Warning"
	}
	if len(result.ActiveRooks) > 0 {
		return "Active"
	}
	return "Pending"
}

func extractRuleStatuses(statusResp map[string]interface{}) ([]map[string]interface{}, error) {
	dataObj, ok := statusResp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing data object")
	}
	orgObj, ok := dataObj["org"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing org object")
	}
	statusesIfc, ok := orgObj["ruleStatuses"].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}
	statuses := make([]map[string]interface{}, 0, len(statusesIfc))
	for _, statusIfc := range statusesIfc {
		status, ok := statusIfc.(map[string]interface{})
		if !ok {
			continue
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func extractRooksAndTipsByStatus(ruleStatuses []map[string]interface{}, statuses ...string) ([]breakpointRookInfo, []breakpointTip) {
	statusSet := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}

	rooks := make([]breakpointRookInfo, 0)
	tipsByKey := make(map[string]breakpointTip)
	for _, ruleStatus := range ruleStatuses {
		if _, ok := statusSet[stringValue(ruleStatus["status"])]; !ok {
			continue
		}
		for _, rookStatus := range interfaceMaps(ruleStatus["rookStatuses"]) {
			if rookInfo, ok := extractRookInfo(rookStatus["rook"]); ok {
				rooks = append(rooks, rookInfo)
			}
			for _, tip := range interfaceMaps(rookStatus["tips"]) {
				tipInfo := breakpointTip{Description: stringValue(tip["description"]), DocsLink: stringValue(tip["docsLink"])}
				key := tipInfo.Description + "|" + tipInfo.DocsLink
				if tipInfo.Description != "" {
					tipsByKey[key] = tipInfo
				}
			}
		}
	}

	sort.Slice(rooks, func(i, j int) bool {
		if rooks[i].Hostname == rooks[j].Hostname {
			return rooks[i].Executable < rooks[j].Executable
		}
		return rooks[i].Hostname < rooks[j].Hostname
	})

	tips := make([]breakpointTip, 0, len(tipsByKey))
	for _, tip := range tipsByKey {
		tips = append(tips, tip)
	}
	sort.Slice(tips, func(i, j int) bool { return tips[i].Description < tips[j].Description })

	return uniqueRooks(rooks), tips
}

func summarizeRookIssues(ruleStatuses []map[string]interface{}, wantedStatus string) []breakpointStatusIssue {
	issueMap := make(map[string]*breakpointStatusIssue)
	order := make([]string, 0)
	for _, ruleStatus := range ruleStatuses {
		if stringValue(ruleStatus["status"]) != wantedStatus {
			continue
		}
		for _, rookStatus := range interfaceMaps(ruleStatus["rookStatuses"]) {
			issue := issueFromError(rookStatus["error"])
			key := issueKey(issue.Title, issue.Args)
			if _, ok := issueMap[key]; !ok {
				copy := issue
				issueMap[key] = &copy
				order = append(order, key)
			}
			if rookInfo, ok := extractRookInfo(rookStatus["rook"]); ok {
				issueMap[key].Rooks = append(issueMap[key].Rooks, rookInfo)
			}
		}
	}

	issues := make([]breakpointStatusIssue, 0, len(order))
	for _, key := range order {
		issue := *issueMap[key]
		issue.Rooks = uniqueRooks(issue.Rooks)
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Title < issues[j].Title })
	return issues
}

func summarizeControllerIssues(ruleStatuses []map[string]interface{}, wantedStatus string) []breakpointStatusIssue {
	issueMap := make(map[string]*breakpointStatusIssue)
	order := make([]string, 0)
	for _, ruleStatus := range ruleStatuses {
		if stringValue(ruleStatus["status"]) != wantedStatus {
			continue
		}
		controllerStatuses := interfaceMaps(ruleStatus["controllerStatuses"])
		if len(controllerStatuses) == 0 {
			controllerStatuses = interfaceMaps(ruleStatus["agentStatuses"])
		}
		for _, controllerStatus := range controllerStatuses {
			issue := issueFromError(controllerStatus["error"])
			key := issueKey(issue.Title, issue.Args)
			if _, ok := issueMap[key]; !ok {
				copy := issue
				issueMap[key] = &copy
				order = append(order, key)
			}
			if controllerID := stringValue(controllerStatus["controllerId"]); controllerID != "" {
				issueMap[key].Controllers = append(issueMap[key].Controllers, controllerID)
			}
		}
	}

	issues := make([]breakpointStatusIssue, 0, len(order))
	for _, key := range order {
		issue := *issueMap[key]
		issue.Controllers = uniqueStrings(issue.Controllers)
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Title < issues[j].Title })
	return issues
}

func issueFromError(value interface{}) breakpointStatusIssue {
	errorObj, _ := value.(map[string]interface{})
	summaryObj, _ := errorObj["summary"].(map[string]interface{})
	issue := breakpointStatusIssue{
		Title:       stringValue(summaryObj["title"]),
		Description: stringValue(summaryObj["description"]),
		DocsLink:    stringValue(summaryObj["docsLink"]),
		Args:        summaryObj["args"],
	}
	if issue.Title == "" {
		issue.Title = stringValue(errorObj["type"])
	}
	if issue.Description == "" {
		issue.Description = stringValue(errorObj["message"])
	}
	if issue.Title == "" {
		issue.Title = "Unknown issue"
	}
	return issue
}

func issueKey(title string, args interface{}) string {
	encoded, _ := json.Marshal(args)
	return title + "|" + string(encoded)
}

func extractRookInfo(value interface{}) (breakpointRookInfo, bool) {
	rookObj, ok := value.(map[string]interface{})
	if !ok {
		return breakpointRookInfo{}, false
	}
	result := breakpointRookInfo{
		ID:         stringValue(rookObj["id"]),
		Hostname:   stringValue(rookObj["hostname"]),
		Executable: stringValue(rookObj["executable"]),
	}
	if result.ID == "" && result.Hostname == "" && result.Executable == "" {
		return breakpointRookInfo{}, false
	}
	return result, true
}

func interfaceMaps(value interface{}) []map[string]interface{} {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		mapItem, ok := item.(map[string]interface{})
		if ok {
			result = append(result, mapItem)
		}
	}
	return result
}

func uniqueRooks(rooks []breakpointRookInfo) []breakpointRookInfo {
	seen := make(map[string]struct{}, len(rooks))
	result := make([]breakpointRookInfo, 0, len(rooks))
	for _, rook := range rooks {
		key := rook.ID + "|" + rook.Hostname + "|" + rook.Executable
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, rook)
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok || value == "" {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func boolValue(value interface{}) bool {
	result, _ := value.(bool)
	return result
}

func printBreakpointStatusResult(result breakpointStatusResult) {
	fmt.Printf("ID:            %s\n", result.ID)
	if result.Location != "" {
		fmt.Printf("Location:      %s\n", result.Location)
	}
	fmt.Printf("Enabled:       %t\n", result.Enabled)
	if result.DisableReason != "" {
		fmt.Printf("Disable reason:%s%s\n", strings.Repeat(" ", 1), result.DisableReason)
	}
	fmt.Printf("Status:        %s\n", result.Status)
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Active rooks:\t%d\n", len(result.ActiveRooks))
	fmt.Fprintf(w, "Pending rooks:\t%d\n", len(result.PendingRooks))
	fmt.Fprintf(w, "Warnings:\t%d\n", len(result.Warnings))
	fmt.Fprintf(w, "Errors:\t%d\n", len(result.Errors))
	fmt.Fprintf(w, "Controller warnings:\t%d\n", len(result.ControllerWarnings))
	fmt.Fprintf(w, "Controller errors:\t%d\n", len(result.ControllerErrors))
	_ = w.Flush()

	printBreakpointRooksSection("Active rooks", result.ActiveRooks)
	printBreakpointTipsSection("Active tips", result.ActiveTips)
	printBreakpointRooksSection("Pending rooks", result.PendingRooks)
	printBreakpointTipsSection("Pending tips", result.PendingTips)
	printBreakpointIssuesSection("Warnings", result.Warnings)
	printBreakpointIssuesSection("Errors", result.Errors)
	printBreakpointIssuesSection("Controller warnings", result.ControllerWarnings)
	printBreakpointIssuesSection("Controller errors", result.ControllerErrors)
}

func printBreakpointRooksSection(title string, rooks []breakpointRookInfo) {
	if len(rooks) == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("%s:\n", title)
	for _, rook := range rooks {
		label := strings.TrimSpace(strings.Join([]string{rook.Hostname, rook.Executable}, " / "))
		if label == "/" || label == "" {
			label = rook.ID
		}
		if rook.ID != "" && rook.ID != label {
			fmt.Printf("  - %s (%s)\n", label, rook.ID)
			continue
		}
		fmt.Printf("  - %s\n", label)
	}
}

func printBreakpointTipsSection(title string, tips []breakpointTip) {
	if len(tips) == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("%s:\n", title)
	for _, tip := range tips {
		if tip.DocsLink != "" {
			fmt.Printf("  - %s (%s)\n", tip.Description, tip.DocsLink)
			continue
		}
		fmt.Printf("  - %s\n", tip.Description)
	}
}

func printBreakpointIssuesSection(title string, issues []breakpointStatusIssue) {
	if len(issues) == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("%s:\n", title)
	for _, issue := range issues {
		fmt.Printf("  - %s\n", issue.Title)
		if issue.Description != "" {
			fmt.Printf("    Description: %s\n", issue.Description)
		}
		if issue.DocsLink != "" {
			fmt.Printf("    Docs:        %s\n", issue.DocsLink)
		}
		if len(issue.Rooks) > 0 {
			fmt.Printf("    Rooks:       %d\n", len(issue.Rooks))
			for _, rook := range issue.Rooks {
				label := strings.TrimSpace(strings.Join([]string{rook.Hostname, rook.Executable}, " / "))
				if label == "/" || label == "" {
					label = rook.ID
				}
				fmt.Printf("      - %s\n", label)
			}
		}
		if len(issue.Controllers) > 0 {
			fmt.Printf("    Controllers: %d\n", len(issue.Controllers))
			for _, controller := range issue.Controllers {
				fmt.Printf("      - %s\n", controller)
			}
		}
	}
}
