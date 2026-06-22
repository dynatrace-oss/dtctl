package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/cmd/testutil"
)

const classicPipelinesTranslateEndpoint = "/platform/openpipeline/v1/classic-pipelines/translate"

// captureStdout (defined in breakpoint_output_test.go) runs a function with
// os.Stdout redirected to a pipe and returns what was written.

func TestGetClassicPipelinesTranslationCmd_Success(t *testing.T) {
	var gotConfiguration string
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		classicPipelinesTranslateEndpoint: func(w http.ResponseWriter, r *http.Request) {
			gotConfiguration = r.URL.Query().Get("configuration")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":{"id":"pipe-1"},"withWarning":false}`))
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	origAgent := agentMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
		agentMode = origAgent
	}()

	cfgFile = configPath
	plainMode = true
	agentMode = false

	testutil.ResetCommandFlags(getClassicPipelinesTranslationCmd)

	if err := getClassicPipelinesTranslationCmd.RunE(getClassicPipelinesTranslationCmd, []string{"logs"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if gotConfiguration != "logs" {
		t.Errorf("configuration sent = %q, want %q", gotConfiguration, "logs")
	}
}

func TestGetClassicPipelinesTranslationCmd_PassesFlags(t *testing.T) {
	gotParams := map[string]string{}
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		classicPipelinesTranslateEndpoint: func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			for _, p := range []string{"configuration", "includeSampleData", "skipDisabledRules", "skipBuiltinProcessingRules"} {
				gotParams[p] = q.Get(p)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":{},"withWarning":false}`))
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()

	cfgFile = configPath
	plainMode = true

	testutil.ResetCommandFlags(getClassicPipelinesTranslationCmd)
	_ = getClassicPipelinesTranslationCmd.Flags().Set("include-sample-data", "true")
	_ = getClassicPipelinesTranslationCmd.Flags().Set("skip-disabled-rules", "false")
	_ = getClassicPipelinesTranslationCmd.Flags().Set("skip-builtin-processing-rules", "true")

	if err := getClassicPipelinesTranslationCmd.RunE(getClassicPipelinesTranslationCmd, []string{"bizevents"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}

	want := map[string]string{
		"configuration":              "bizevents",
		"includeSampleData":          "true",
		"skipDisabledRules":          "false",
		"skipBuiltinProcessingRules": "true",
	}
	for p, w := range want {
		if gotParams[p] != w {
			t.Errorf("query %q = %q, want %q", p, gotParams[p], w)
		}
	}
}

func TestGetClassicPipelinesTranslationCmd_InvalidScope(t *testing.T) {
	// An invalid scope must fail fast without making an HTTP call.
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		classicPipelinesTranslateEndpoint: func(w http.ResponseWriter, _ *http.Request) {
			t.Error("endpoint should not be called for an invalid scope")
			w.WriteHeader(http.StatusOK)
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
	}()

	cfgFile = configPath
	plainMode = true

	testutil.ResetCommandFlags(getClassicPipelinesTranslationCmd)

	err := getClassicPipelinesTranslationCmd.RunE(getClassicPipelinesTranslationCmd, []string{"metrics"})
	if err == nil {
		t.Fatal("expected an error for an invalid scope")
	}
	if !strings.Contains(err.Error(), "invalid configuration scope") {
		t.Errorf("error = %q, want it to mention invalid configuration scope", err.Error())
	}
}

// runWithOutput sets up a mock server returning body, runs the command for the
// given output format, and returns captured stdout.
func runWithOutput(t *testing.T, format, body string, args []string) string {
	t.Helper()
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		classicPipelinesTranslateEndpoint: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	origAgent := agentMode
	origFormat := outputFormat
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
		agentMode = origAgent
		outputFormat = origFormat
	}()

	cfgFile = configPath
	plainMode = false
	agentMode = false
	outputFormat = format

	testutil.ResetCommandFlags(getClassicPipelinesTranslationCmd)

	return captureStdout(t, func() {
		if err := getClassicPipelinesTranslationCmd.RunE(getClassicPipelinesTranslationCmd, args); err != nil {
			t.Fatalf("RunE() error = %v", err)
		}
	})
}

func TestGetClassicPipelinesTranslationCmd_DefaultPrintsValueOnly(t *testing.T) {
	out := runWithOutput(t, "table",
		`{"value":{"id":"pipe-1","processors":[]},"withWarning":false}`,
		[]string{"logs"})

	// Default output is the pipeline document (value) as indented JSON — not
	// the full result, so withWarning must not appear.
	if !strings.Contains(out, `"id": "pipe-1"`) {
		t.Errorf("default output missing indented value; got:\n%s", out)
	}
	if strings.Contains(out, "withWarning") {
		t.Errorf("default output should not include withWarning; got:\n%s", out)
	}
}

func TestGetClassicPipelinesTranslationCmd_JSONPrintsFullResult(t *testing.T) {
	out := runWithOutput(t, "json",
		`{"value":{"id":"pipe-1"},"withWarning":true}`,
		[]string{"logs"})

	// -o json returns the full {value, withWarning} result.
	if !strings.Contains(out, `"withWarning": true`) {
		t.Errorf("json output missing withWarning; got:\n%s", out)
	}
	if !strings.Contains(out, `"id": "pipe-1"`) {
		t.Errorf("json output missing value; got:\n%s", out)
	}
}

func TestGetClassicPipelinesTranslationCmd_YAMLRendersStructured(t *testing.T) {
	out := runWithOutput(t, "yaml",
		`{"value":{"id":"pipe-1","processors":[{"type":"fieldsAdd"}]},"withWarning":false}`,
		[]string{"bizevents"})

	// -o yaml renders the document structurally (keys as YAML), not as an
	// escaped JSON string.
	if !strings.Contains(out, "value:") || !strings.Contains(out, "id: pipe-1") {
		t.Errorf("yaml output not structured; got:\n%s", out)
	}
	if !strings.Contains(out, "withWarning: false") {
		t.Errorf("yaml output missing withWarning; got:\n%s", out)
	}
}

func TestGetClassicPipelinesTranslationCmd_AgentMode(t *testing.T) {
	ms := testutil.NewMockServer(t, map[string]http.HandlerFunc{
		classicPipelinesTranslateEndpoint: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":{"id":"pipe-2"},"withWarning":true}`))
		},
	})
	defer ms.Close()

	configPath, cleanup := testutil.SetupTestConfig(t, ms.URL)
	defer cleanup()

	origCfgFile := cfgFile
	origPlain := plainMode
	origAgent := agentMode
	defer func() {
		cfgFile = origCfgFile
		plainMode = origPlain
		agentMode = origAgent
	}()

	cfgFile = configPath
	plainMode = true
	agentMode = true

	testutil.ResetCommandFlags(getClassicPipelinesTranslationCmd)

	if err := getClassicPipelinesTranslationCmd.RunE(getClassicPipelinesTranslationCmd, []string{"logs"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}
