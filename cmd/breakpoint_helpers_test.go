package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/resources/livedebugger"
)

func TestGetBreakpointsCommandRegistration(t *testing.T) {
	getCmd, _, err := rootCmd.Find([]string{"get"})
	if err != nil {
		t.Fatalf("expected get command to exist, got error: %v", err)
	}
	if getCmd == nil || getCmd.Name() != "get" {
		t.Fatalf("expected get command to exist")
	}

	breakpointsCmd, _, err := rootCmd.Find([]string{"get", "breakpoints"})
	if err != nil {
		t.Fatalf("expected get breakpoints command to exist, got error: %v", err)
	}
	if breakpointsCmd == nil || breakpointsCmd.Name() != "breakpoints" {
		t.Fatalf("expected get breakpoints command to exist")
	}
}

func TestExtractBreakpointRows(t *testing.T) {
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"org": map[string]interface{}{
				"workspace": map[string]interface{}{
					"rules": []interface{}{
						map[string]interface{}{
							"id":          "bp-2",
							"is_disabled": false,
							"aug_json": map[string]interface{}{
								"location": map[string]interface{}{
									"filename": "OrderController.java",
									"lineno":   float64(1337),
								},
							},
						},
						map[string]interface{}{
							"id":          "bp-1",
							"is_disabled": true,
							"aug_json": map[string]interface{}{
								"location": map[string]interface{}{
									"filename": "AController.java",
									"lineno":   float64(42),
								},
							},
						},
					},
				},
			},
		},
	}

	rows, err := extractBreakpointRows(resp)
	if err != nil {
		t.Fatalf("extractBreakpointRows returned error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].ID != "bp-1" || rows[0].Filename != "AController.java" || rows[0].Line != 42 || rows[0].Active {
		t.Fatalf("unexpected first row: %#v", rows[0])
	}

	if rows[1].ID != "bp-2" || rows[1].Filename != "OrderController.java" || rows[1].Line != 1337 || !rows[1].Active {
		t.Fatalf("unexpected second row: %#v", rows[1])
	}
}

func TestParseFilters(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    map[string][]string
	}{
		{
			name:  "single filter",
			input: "k8s.namespace.name:prod",
			want: map[string][]string{
				"k8s.namespace.name": {"prod"},
			},
		},
		{
			name:  "multiple filters and duplicate keys",
			input: "k8s.namespace.name:prod, dt.entity.host:HOST-1,k8s.namespace.name:stage",
			want: map[string][]string{
				"k8s.namespace.name": {"prod", "stage"},
				"dt.entity.host":     {"HOST-1"},
			},
		},
		{
			name:  "equals separator remains supported",
			input: "k8s.namespace.name=prod",
			want: map[string][]string{
				"k8s.namespace.name": {"prod"},
			},
		},
		{name: "missing separator", input: "k8s.namespace.name", wantErr: true},
		{name: "empty input", input: "", wantErr: true},
		{name: "empty key", input: ":prod", wantErr: true},
		{name: "empty value", input: "k8s.namespace.name:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFilters(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("unexpected map size: got=%d want=%d", len(got), len(tt.want))
			}

			for key, expectedValues := range tt.want {
				values, ok := got[key]
				if !ok {
					t.Fatalf("missing key %q", key)
				}
				if len(values) != len(expectedValues) {
					t.Fatalf("unexpected values length for %q: got=%d want=%d", key, len(values), len(expectedValues))
				}
				for i := range values {
					if values[i] != expectedValues[i] {
						t.Fatalf("unexpected value at %q[%d]: got=%q want=%q", key, i, values[i], expectedValues[i])
					}
				}
			}
		})
	}
}

func TestParseBreakpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFile string
		wantLine int
		wantErr  bool
	}{
		{name: "valid", input: "OrderController.java:306", wantFile: "OrderController.java", wantLine: 306},
		{name: "valid with spaces", input: " OrderController.java : 306 ", wantFile: "OrderController.java", wantLine: 306},
		{name: "missing separator", input: "OrderController.java", wantErr: true},
		{name: "empty file", input: ":123", wantErr: true},
		{name: "non numeric line", input: "OrderController.java:abc", wantErr: true},
		{name: "non positive line", input: "OrderController.java:0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName, lineNumber, err := parseBreakpoint(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if fileName != tt.wantFile {
				t.Fatalf("unexpected file name: got=%q want=%q", fileName, tt.wantFile)
			}

			if lineNumber != tt.wantLine {
				t.Fatalf("unexpected line number: got=%d want=%d", lineNumber, tt.wantLine)
			}
		})
	}
}

func TestBuildGraphQLResponse(t *testing.T) {
	payload := map[string]interface{}{"data": "value"}
	wrapper := buildGraphQLResponse("getWorkspaceRules", payload)

	if wrapper["operation"] != "getWorkspaceRules" {
		t.Fatalf("unexpected operation: %#v", wrapper["operation"])
	}
	response, ok := wrapper["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected response type: %#v", wrapper["response"])
	}
	if response["data"] != payload["data"] {
		t.Fatalf("unexpected response payload: %#v", response)
	}
}

func TestPrintGraphQLResponse(t *testing.T) {
	originalOut := rootCmd.OutOrStdout()
	defer rootCmd.SetOut(originalOut)

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	payload := map[string]interface{}{
		"data": map[string]interface{}{"ok": true},
	}

	if err := printGraphQLResponse("getWorkspaceRules", payload); err != nil {
		t.Fatalf("printGraphQLResponse returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "\"operation\": \"getWorkspaceRules\"") {
		t.Fatalf("missing operation in output: %q", output)
	}
	if !strings.Contains(output, "\"response\"") {
		t.Fatalf("missing response in output: %q", output)
	}
}

func TestPrintGraphQLResponse_NilPayload(t *testing.T) {
	if err := printGraphQLResponse("noop", nil); err != nil {
		t.Fatalf("expected nil payload to return nil error, got: %v", err)
	}
}

func TestCurrentProjectPath(t *testing.T) {
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() { _ = os.Chdir(originalCwd) }()

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	if got := currentProjectPath(); got == "" || got == "no-project" {
		t.Fatalf("expected project name from cwd, got %q", got)
	}
}

func TestIsDebugVerbose(t *testing.T) {
	originalDebugMode := debugMode
	originalVerbosity := verbosity
	defer func() {
		debugMode = originalDebugMode
		verbosity = originalVerbosity
	}()

	debugMode = false
	verbosity = 0
	if isDebugVerbose() {
		t.Fatalf("expected false when debugMode and verbosity are disabled")
	}

	verbosity = 1
	if !isDebugVerbose() {
		t.Fatalf("expected true when verbosity > 0")
	}

	verbosity = 0
	debugMode = true
	if !isDebugVerbose() {
		t.Fatalf("expected true when debugMode is enabled")
	}
}

func TestRunGetBreakpoints_LoadConfigError(t *testing.T) {
	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()

	cfgFile = filepath.Join(t.TempDir(), "missing-config.yaml")

	err := runGetBreakpoints(nil, nil)
	if err == nil {
		t.Fatalf("expected load config error")
	}
}

func TestRunGetBreakpoints_TableView(t *testing.T) {
	originalOutputFormat := outputFormat
	originalAgentMode := agentMode
	originalDebugMode := debugMode
	originalVerbosity := verbosity
	originalOut := rootCmd.OutOrStdout()
	defer func() {
		outputFormat = originalOutputFormat
		agentMode = originalAgentMode
		debugMode = originalDebugMode
		verbosity = originalVerbosity
		rootCmd.SetOut(originalOut)
	}()

	outputFormat = ""
	agentMode = false
	debugMode = false
	verbosity = 0

	deps := liveDebuggerDeps{}
	deps.loadConfig = func() (*config.Config, error) {
		cfg := config.NewConfig()
		cfg.SetContext("test", "https://example.invalid", "token")
		cfg.CurrentContext = "test"
		return cfg, nil
	}
	deps.newClient = func(cfg *config.Config) (*client.Client, error) { return nil, nil }
	deps.newHandler = func(c *client.Client, environment string) (*livedebugger.Handler, error) { return nil, nil }
	deps.getOrCreateWorkspace = func(handler *livedebugger.Handler, projectPath string) (map[string]interface{}, string, error) {
		return map[string]interface{}{"data": map[string]interface{}{}}, "ws-1", nil
	}
	deps.getWorkspaceRules = func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"data": map[string]interface{}{
				"org": map[string]interface{}{
					"workspace": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"id":          "bp-1",
								"is_disabled": false,
								"aug_json": map[string]interface{}{
									"location": map[string]interface{}{"filename": "OrderController.java", "lineno": float64(306)},
								},
							},
						},
					},
				},
			},
		}, nil
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	if err := runGetBreakpointsWithDeps(nil, nil, deps); err != nil {
		t.Fatalf("runGetBreakpoints returned error: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "bp-1") || !strings.Contains(text, "OrderController.java") {
		t.Fatalf("unexpected table output: %q", text)
	}
}

func TestRunGetBreakpoints_StructuredView(t *testing.T) {
	originalOutputFormat := outputFormat
	originalAgentMode := agentMode
	originalDebugMode := debugMode
	originalVerbosity := verbosity
	originalOut := rootCmd.OutOrStdout()
	defer func() {
		outputFormat = originalOutputFormat
		agentMode = originalAgentMode
		debugMode = originalDebugMode
		verbosity = originalVerbosity
		rootCmd.SetOut(originalOut)
	}()

	outputFormat = "json"
	agentMode = false
	debugMode = false
	verbosity = 0

	deps := liveDebuggerDeps{}
	deps.loadConfig = func() (*config.Config, error) {
		cfg := config.NewConfig()
		cfg.SetContext("test", "https://example.invalid", "token")
		cfg.CurrentContext = "test"
		return cfg, nil
	}
	deps.newClient = func(cfg *config.Config) (*client.Client, error) { return nil, nil }
	deps.newHandler = func(c *client.Client, environment string) (*livedebugger.Handler, error) { return nil, nil }
	deps.getOrCreateWorkspace = func(handler *livedebugger.Handler, projectPath string) (map[string]interface{}, string, error) {
		return map[string]interface{}{"data": map[string]interface{}{}}, "ws-1", nil
	}
	deps.getWorkspaceRules = func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"data": map[string]interface{}{
				"org": map[string]interface{}{
					"workspace": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"id":          "bp-1",
								"is_disabled": false,
								"aug_json": map[string]interface{}{
									"location": map[string]interface{}{"filename": "OrderController.java", "lineno": float64(306)},
								},
							},
						},
					},
				},
			},
		}, nil
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	if err := runGetBreakpointsWithDeps(nil, nil, deps); err != nil {
		t.Fatalf("runGetBreakpoints returned error: %v", err)
	}

	output := out.String()

	if !strings.Contains(output, "\"id\": \"bp-1\"") || !strings.Contains(output, "\"filename\": \"OrderController.java\"") {
		t.Fatalf("unexpected structured output: %q", output)
	}
}

func TestRunGetBreakpoints_AgentEnvelope(t *testing.T) {
	originalOutputFormat := outputFormat
	originalAgentMode := agentMode
	originalDebugMode := debugMode
	originalVerbosity := verbosity
	originalOut := rootCmd.OutOrStdout()
	defer func() {
		outputFormat = originalOutputFormat
		agentMode = originalAgentMode
		debugMode = originalDebugMode
		verbosity = originalVerbosity
		rootCmd.SetOut(originalOut)
	}()

	outputFormat = ""
	agentMode = true
	debugMode = false
	verbosity = 0

	deps := liveDebuggerDeps{}
	deps.loadConfig = func() (*config.Config, error) {
		cfg := config.NewConfig()
		cfg.SetContext("test", "https://example.invalid", "token")
		cfg.CurrentContext = "test"
		return cfg, nil
	}
	deps.newClient = func(cfg *config.Config) (*client.Client, error) { return nil, nil }
	deps.newHandler = func(c *client.Client, environment string) (*livedebugger.Handler, error) { return nil, nil }
	deps.getOrCreateWorkspace = func(handler *livedebugger.Handler, projectPath string) (map[string]interface{}, string, error) {
		return map[string]interface{}{"data": map[string]interface{}{}}, "ws-1", nil
	}
	deps.getWorkspaceRules = func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"data": map[string]interface{}{
				"org": map[string]interface{}{
					"workspace": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"id":          "bp-1",
								"is_disabled": false,
								"aug_json": map[string]interface{}{
									"location": map[string]interface{}{"filename": "OrderController.java", "lineno": float64(306)},
								},
							},
						},
					},
				},
			},
		}, nil
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)

	if err := runGetBreakpointsWithDeps(nil, nil, deps); err != nil {
		t.Fatalf("runGetBreakpoints returned error: %v", err)
	}

	output := out.String()

	if !strings.Contains(output, "\"ok\": true") || !strings.Contains(output, "\"resource\": \"breakpoint\"") {
		t.Fatalf("unexpected agent output: %q", output)
	}
}

func TestRunGetBreakpoints_GetWorkspaceRulesError(t *testing.T) {
	deps := liveDebuggerDeps{}
	deps.loadConfig = func() (*config.Config, error) {
		cfg := config.NewConfig()
		cfg.SetContext("test", "https://example.invalid", "token")
		cfg.CurrentContext = "test"
		return cfg, nil
	}
	deps.newClient = func(cfg *config.Config) (*client.Client, error) { return nil, nil }
	deps.newHandler = func(c *client.Client, environment string) (*livedebugger.Handler, error) { return nil, nil }
	deps.getOrCreateWorkspace = func(handler *livedebugger.Handler, projectPath string) (map[string]interface{}, string, error) {
		return map[string]interface{}{"data": map[string]interface{}{}}, "ws-1", nil
	}
	deps.getWorkspaceRules = func(handler *livedebugger.Handler, workspaceID string) (map[string]interface{}, error) {
		return nil, os.ErrPermission
	}

	err := runGetBreakpointsWithDeps(nil, nil, deps)
	if err == nil {
		t.Fatalf("expected get workspace rules error")
	}
}

func TestExtractWorkspaceRulesErrors(t *testing.T) {
	tests := []struct {
		name string
		resp map[string]interface{}
		want string
	}{
		{name: "missing data", resp: map[string]interface{}{}, want: "missing data object"},
		{name: "missing org", resp: map[string]interface{}{"data": map[string]interface{}{}}, want: "missing org object"},
		{name: "missing workspace", resp: map[string]interface{}{"data": map[string]interface{}{"org": map[string]interface{}{}}}, want: "missing workspace object"},
		{name: "missing rules", resp: map[string]interface{}{"data": map[string]interface{}{"org": map[string]interface{}{"workspace": map[string]interface{}{}}}}, want: "missing rules list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractWorkspaceRules(tt.resp)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestExtractBreakpointRowsSkipsInvalidRules(t *testing.T) {
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"org": map[string]interface{}{
				"workspace": map[string]interface{}{
					"rules": []interface{}{
						map[string]interface{}{"id": "bad-1"},
						map[string]interface{}{
							"id":          "ok-1",
							"is_disabled": false,
							"aug_json": map[string]interface{}{
								"location": map[string]interface{}{"filename": "A.java", "lineno": float64(2)},
							},
						},
					},
				},
			},
		},
	}

	rows, err := extractBreakpointRows(resp)
	if err != nil {
		t.Fatalf("extractBreakpointRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "ok-1" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestBreakpointRowFromRule_LinenoTypesAndFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		lineno   interface{}
		wantLine int
	}{
		{name: "int32", lineno: int32(5), wantLine: 5},
		{name: "int64", lineno: int64(6), wantLine: 6},
		{name: "invalid type uses zero", lineno: "7", wantLine: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, ok := breakpointRowFromRule(livedebugger.BreakpointRule{
				ID:         "bp-1",
				IsDisabled: false,
				AugJSON: map[string]interface{}{
					"location": map[string]interface{}{"filename": "A.java", "lineno": tt.lineno},
				},
			})
			if !ok {
				t.Fatalf("expected valid row")
			}
			if row.Line != tt.wantLine {
				t.Fatalf("line=%d want=%d", row.Line, tt.wantLine)
			}
		})
	}

	if _, ok := breakpointRowFromRule(livedebugger.BreakpointRule{ID: "bp-1"}); ok {
		t.Fatalf("expected rule without aug_json to fail")
	}
}
