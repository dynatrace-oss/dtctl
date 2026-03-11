package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/spf13/cobra"
)

var (
	debugFilters string
)

type breakpointRow struct {
	ID       string
	Filename string
	Line     int
	Active   bool
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Manage Live Debugger workspace filters",
	Long: `Configure Live Debugger workspace filters for the current project.

Examples:
  dtctl debug --filters k8s.namespace.name=prod
  dtctl debug --filters k8s.namespace.name=prod,dt.entity.host=HOST-123
	dtctl create breakpoint OrderController.java:306
	dtctl get breakpoints`,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose := isDebugVerbose()

		if strings.TrimSpace(debugFilters) == "" {
			return fmt.Errorf("--filters is required")
		}

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

		if strings.TrimSpace(debugFilters) != "" {
			checker, err := NewSafetyChecker(cfg)
			if err != nil {
				return err
			}
			if err := checker.CheckError(safety.OperationUpdate, safety.OwnershipUnknown); err != nil {
				return err
			}

			parsedFilters, err := parseFilters(debugFilters)
			if err != nil {
				return err
			}

			updateResp, err := handler.UpdateWorkspaceFilters(workspaceID, livedebugger.BuildFilterSets(parsedFilters))
			if err != nil {
				if verbose {
					_ = printGraphQLResponse("updateWorkspaceV2", updateResp)
				}
				return err
			}
			if verbose {
				if err := printGraphQLResponse("updateWorkspaceV2", updateResp); err != nil {
					return err
				}
			}
		}

		return nil
	},
}

func runGetBreakpoints(cmd *cobra.Command, args []string) error {
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
		return printGraphQLResponse("getWorkspaceRules", workspaceRulesResp)
	}

	if !useBreakpointTableView() {
		printer := NewPrinter()
		return printer.Print(buildGraphQLResponse("getWorkspaceRules", workspaceRulesResp))
	}

	rows, err := extractBreakpointRows(workspaceRulesResp)
	if err != nil {
		return err
	}

	printBreakpointsTable(rows)
	return nil
}

func useBreakpointTableView() bool {
	return outputFormat == "" || outputFormat == "table" || outputFormat == "wide" || outputFormat == "csv"
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

func extractWorkspaceRules(workspaceRulesResp map[string]interface{}) ([]map[string]interface{}, error) {
	dataObj, ok := workspaceRulesResp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing data object")
	}

	orgObj, ok := dataObj["org"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing org object")
	}

	workspaceObj, ok := orgObj["workspace"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing workspace object")
	}

	rulesIfc, ok := workspaceObj["rules"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("graphql response missing rules list")
	}

	rules := make([]map[string]interface{}, 0, len(rulesIfc))
	for _, ruleIfc := range rulesIfc {
		rule, ok := ruleIfc.(map[string]interface{})
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func breakpointRowFromRule(rule map[string]interface{}) (breakpointRow, bool) {
	augJSON, ok := rule["aug_json"].(map[string]interface{})
	if !ok {
		return breakpointRow{}, false
	}

	location, ok := augJSON["location"].(map[string]interface{})
	if !ok {
		return breakpointRow{}, false
	}

	id, _ := rule["id"].(string)
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

	isDisabled, _ := rule["is_disabled"].(bool)
	return breakpointRow{ID: id, Filename: filename, Line: line, Active: !isDisabled}, true
}

func printBreakpointsTable(rows []breakpointRow) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "id\tfilename\tline number\tactive")
	for _, row := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%t\n", row.ID, row.Filename, row.Line, row.Active)
	}
	_ = tw.Flush()
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

		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			return nil, fmt.Errorf("invalid filter %q: expected key=value", trimmed)
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

	fmt.Println(string(encoded))
	return nil
}

func buildGraphQLResponse(operation string, payload map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"operation": operation,
		"response":  payload,
	}
}

func init() {
	debugCmd.Flags().StringVar(&debugFilters, "filters", "", "filters to apply (comma-separated key=value pairs)")
	rootCmd.AddCommand(debugCmd)
}
