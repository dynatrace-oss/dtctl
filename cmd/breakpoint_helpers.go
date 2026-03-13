package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
)

type liveDebuggerDeps struct {
	loadConfig             func() (*config.Config, error)
	newClient              func(*config.Config) (*client.Client, error)
	newHandler             func(*client.Client, string) (*livedebugger.Handler, error)
	getOrCreateWorkspace   func(*livedebugger.Handler, string) (map[string]interface{}, string, error)
	getWorkspaceRules      func(*livedebugger.Handler, string) (map[string]interface{}, error)
	getRuleStatusBreakdown func(*livedebugger.Handler, string) (map[string]interface{}, error)
}

func defaultLiveDebuggerDeps() liveDebuggerDeps {
	return liveDebuggerDeps{
		loadConfig: LoadConfig,
		newClient:  NewClientFromConfig,
		newHandler: livedebugger.NewHandler,
		getOrCreateWorkspace: func(handler *livedebugger.Handler, projectPath string) (map[string]interface{}, string, error) {
			return handler.GetOrCreateWorkspace(projectPath)
		},
		getWorkspaceRules: func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
			return handler.GetWorkspaceRules(workspaceID)
		},
		getRuleStatusBreakdown: func(handler *livedebugger.Handler, ruleID string) (map[string]interface{}, error) {
			return handler.GetRuleStatusBreakdown(ruleID)
		},
	}
}

type breakpointRow struct {
	ID       string `table:"ID" json:"id" yaml:"id"`
	Filename string `table:"FILENAME" json:"filename" yaml:"filename"`
	Line     int    `table:"LINE NUMBER" json:"lineNumber" yaml:"lineNumber"`
	Active   bool   `table:"ACTIVE" json:"active" yaml:"active"`
}

func runGetBreakpoints(cmd *cobra.Command, args []string) error {
	return runGetBreakpointsWithDeps(cmd, args, defaultLiveDebuggerDeps())
}

func runGetBreakpointsWithDeps(cmd *cobra.Command, args []string, deps liveDebuggerDeps) error {
	verbose := isDebugVerbose()

	cfg, err := deps.loadConfig()
	if err != nil {
		return err
	}

	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return err
	}

	c, err := deps.newClient(cfg)
	if err != nil {
		return err
	}

	handler, err := deps.newHandler(c, ctx.Environment)
	if err != nil {
		return err
	}

	workspaceResp, workspaceID, err := deps.getOrCreateWorkspace(handler, currentProjectPath())
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

	workspaceRulesResp, err := deps.getWorkspaceRules(handler, workspaceID)
	if err != nil {
		if verbose {
			_ = printGraphQLResponse("getWorkspaceRules", workspaceRulesResp)
		}
		return err
	}

	if verbose {
		return printGraphQLResponse("getWorkspaceRules", workspaceRulesResp)
	}

	rows, err := extractBreakpointRows(workspaceRulesResp)
	if err != nil {
		return err
	}

	var printer output.Printer
	if agentMode {
		printer = output.NewAgentPrinter(rootCmd.OutOrStdout(), &output.ResponseContext{})
	} else {
		printer = output.NewPrinterWithOptions(outputFormat, rootCmd.OutOrStdout(), plainMode)
	}
	_ = enrichAgent(printer, "get", "breakpoint")
	return printer.PrintList(rows)
}

func isDebugVerbose() bool {
	return debugMode || verbosity > 0
}

func extractBreakpointRows(workspaceRulesResp map[string]interface{}) ([]breakpointRow, error) {
	rules, err := extractWorkspaceRules(workspaceRulesResp)
	if err != nil {
		return nil, err
	}

	rows := make([]breakpointRow, 0, len(rules))
	for _, rule := range rules {
		row, ok := breakpointRowFromRule(rule)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Filename == rows[j].Filename {
			return rows[i].Line < rows[j].Line
		}
		return rows[i].Filename < rows[j].Filename
	})

	return rows, nil
}

func extractWorkspaceRules(workspaceRulesResp map[string]interface{}) ([]livedebugger.BreakpointRule, error) {
	return livedebugger.ExtractWorkspaceRules(workspaceRulesResp)
}

func breakpointRowFromRule(rule livedebugger.BreakpointRule) (breakpointRow, bool) {
	augJSON := rule.AugJSON
	if augJSON == nil {
		return breakpointRow{}, false
	}

	location, ok := augJSON["location"].(map[string]interface{})
	if !ok {
		return breakpointRow{}, false
	}

	id := rule.ID
	filename, _ := location["filename"].(string)
	if filename == "" {
		return breakpointRow{}, false
	}

	line := 0
	switch lineno := location["lineno"].(type) {
	case int:
		line = lineno
	case int32:
		line = int(lineno)
	case int64:
		line = int(lineno)
	case float64:
		line = int(lineno)
	}

	isDisabled := rule.IsDisabled
	return breakpointRow{ID: id, Filename: filename, Line: line, Active: !isDisabled}, true
}

func currentProjectPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "no-project"
	}
	project := filepath.Base(cwd)
	if project == "" || project == "." || project == string(filepath.Separator) {
		return "no-project"
	}
	return project
}

func parseFilters(input string) (map[string][]string, error) {
	filters := map[string][]string{}
	parts := strings.Split(input, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			key, value, found = strings.Cut(trimmed, "=")
		}
		if !found {
			return nil, fmt.Errorf("invalid filter %q: expected key:value", trimmed)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, fmt.Errorf("invalid filter %q: key and value are required", trimmed)
		}

		filters[key] = append(filters[key], value)
	}

	if len(filters) == 0 {
		return nil, fmt.Errorf("no valid filters provided")
	}

	for key := range filters {
		sort.Strings(filters[key])
	}

	return filters, nil
}

func parseBreakpoint(input string) (string, int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", 0, fmt.Errorf("breakpoint cannot be empty")
	}

	fileName, lineString, found := strings.Cut(trimmed, ":")
	if !found {
		return "", 0, fmt.Errorf("invalid breakpoint %q: expected File.java:line", trimmed)
	}

	fileName = strings.TrimSpace(fileName)
	lineString = strings.TrimSpace(lineString)
	if fileName == "" || lineString == "" {
		return "", 0, fmt.Errorf("invalid breakpoint %q: file and line are required", trimmed)
	}

	lineNumber, err := strconv.Atoi(lineString)
	if err != nil || lineNumber <= 0 {
		return "", 0, fmt.Errorf("invalid breakpoint line %q: must be a positive integer", lineString)
	}

	return fileName, lineNumber, nil
}

func printGraphQLResponse(operation string, payload map[string]interface{}) error {
	if payload == nil {
		return nil
	}

	wrapper := buildGraphQLResponse(operation, payload)

	encoded, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode %s response: %w", operation, err)
	}

	_, _ = fmt.Fprintln(rootCmd.OutOrStdout(), string(encoded))
	return nil
}

func buildGraphQLResponse(operation string, payload map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"operation": operation,
		"response":  payload,
	}
}
