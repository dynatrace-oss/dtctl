package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
)

// resetExecFunctionFlags resets exec function command flags to defaults.
// Cobra retains flag values across Execute() calls on the global rootCmd.
func resetExecFunctionFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"method", "payload", "data", "code", "file", "defer", "outfile"} {
		if f := execFunctionCmd.Flags().Lookup(name); f != nil {
			if err := f.Value.Set(f.DefValue); err != nil {
				t.Logf("warning: could not reset flag %q: %v", name, err)
			}
			f.Changed = false
		}
	}
}

// newExecFunctionTestConfig creates a config file pointing at the given server URL
// and returns the path. The config uses a plain API token stored inline.
func newExecFunctionTestConfig(t *testing.T, serverURL string) string {
	t.Helper()
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := config.NewConfig()
	cfg.SetContext("test", serverURL, "test-token")
	cfg.CurrentContext = "test"
	cfg.Tokens = append(cfg.Tokens, config.NamedToken{Name: "test-token", Token: "dt0s01.FAKE"})

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}
	return configPath
}

// --- Unit tests for writeFunctionResultToFile ---

func TestWriteFunctionResultToFile_InvokeResponse_JSONBody(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "result.json")

	result := &appengine.FunctionInvokeResponse{
		StatusCode: 200,
		Body:       `{"key":"value","count":42}`,
	}

	if err := writeFunctionResultToFile(result, outfile); err != nil {
		t.Fatalf("writeFunctionResultToFile() error = %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("failed to read outfile: %v", err)
	}
	if string(got) != result.Body {
		t.Errorf("file content = %q, want %q", string(got), result.Body)
	}
	info, err := os.Stat(outfile)
	if err != nil {
		t.Fatalf("failed to stat outfile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}
}

func TestWriteFunctionResultToFile_InvokeResponse_PlainTextBody(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "result.txt")

	result := &appengine.FunctionInvokeResponse{
		StatusCode: 200,
		Body:       "hello world",
	}

	if err := writeFunctionResultToFile(result, outfile); err != nil {
		t.Fatalf("writeFunctionResultToFile() error = %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("failed to read outfile: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("file content = %q, want %q", string(got), "hello world")
	}
	info, err := os.Stat(outfile)
	if err != nil {
		t.Fatalf("failed to stat outfile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}
}

func TestWriteFunctionResultToFile_ExecutorResponse(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "result.json")

	result := &appengine.FunctionExecutorResponse{
		Result: map[string]interface{}{"answer": 42, "ok": true},
		Logs:   "some log output",
	}

	if err := writeFunctionResultToFile(result, outfile); err != nil {
		t.Fatalf("writeFunctionResultToFile() error = %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("failed to read outfile: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("file content is not valid JSON: %v\ncontent: %s", err, got)
	}
	if parsed["answer"] != float64(42) {
		t.Errorf("answer = %v, want 42", parsed["answer"])
	}
	if parsed["ok"] != true {
		t.Errorf("ok = %v, want true", parsed["ok"])
	}
	info, err := os.Stat(outfile)
	if err != nil {
		t.Fatalf("failed to stat outfile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}
}

func TestWriteFunctionResultToFile_ExecutorResponse_NoLogs(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "result.json")

	result := &appengine.FunctionExecutorResponse{
		Result: "simple string result",
		Logs:   "",
	}

	if err := writeFunctionResultToFile(result, outfile); err != nil {
		t.Fatalf("writeFunctionResultToFile() error = %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("failed to read outfile: %v", err)
	}
	if !strings.Contains(string(got), "simple string result") {
		t.Errorf("file content %q does not contain expected result", string(got))
	}
	info, err := os.Stat(outfile)
	if err != nil {
		t.Fatalf("failed to stat outfile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}
}

func TestWriteFunctionResultToFile_FallbackType(t *testing.T) {
	dir := t.TempDir()
	outfile := filepath.Join(dir, "result.json")

	type customResult struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	result := &customResult{ID: "abc-123", Name: "test"}

	if err := writeFunctionResultToFile(result, outfile); err != nil {
		t.Fatalf("writeFunctionResultToFile() error = %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("failed to read outfile: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("file content is not valid JSON: %v", err)
	}
	if parsed["id"] != "abc-123" {
		t.Errorf("id = %v, want %q", parsed["id"], "abc-123")
	}
	info, err := os.Stat(outfile)
	if err != nil {
		t.Fatalf("failed to stat outfile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}
}

func TestWriteFunctionResultToFile_FileCreationError(t *testing.T) {
	outfile := "/nonexistent-dir-dtctl-test/result.json"

	result := &appengine.FunctionInvokeResponse{
		StatusCode: 200,
		Body:       "test",
	}

	err := writeFunctionResultToFile(result, outfile)
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create output file") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "failed to create output file")
	}
}

// --- Unit tests for outfileSummary ---

func TestOutfileSummary_InvokeResponse(t *testing.T) {
	result := &appengine.FunctionInvokeResponse{StatusCode: 201, Body: `{"key":"value"}`}
	got, ok := outfileSummary(result, "out.json").(invokeOutfileSummary)
	if !ok {
		t.Fatal("outfileSummary() did not return invokeOutfileSummary")
	}
	if got.StatusCode != 201 {
		t.Errorf("StatusCode = %v, want 201", got.StatusCode)
	}
	if got.Result != "written to out.json" {
		t.Errorf("Result = %q, want %q", got.Result, "written to out.json")
	}
}

func TestOutfileSummary_ExecutorResponse_WithLogs(t *testing.T) {
	result := &appengine.FunctionExecutorResponse{Result: "ignored", Logs: "some log"}
	got, ok := outfileSummary(result, "out.json").(executorOutfileSummary)
	if !ok {
		t.Fatal("outfileSummary() did not return executorOutfileSummary")
	}
	if got.Result != "written to out.json" {
		t.Errorf("Result = %q, want %q", got.Result, "written to out.json")
	}
	if got.Logs != "some log" {
		t.Errorf("Logs = %q, want %q", got.Logs, "some log")
	}
}

func TestOutfileSummary_ExecutorResponse_NoLogs(t *testing.T) {
	result := &appengine.FunctionExecutorResponse{Result: "ignored", Logs: ""}
	got, ok := outfileSummary(result, "out.json").(genericOutfileSummary)
	if !ok {
		t.Fatal("outfileSummary() did not return genericOutfileSummary when logs is empty")
	}
	if got.Result != "written to out.json" {
		t.Errorf("Result = %q, want %q", got.Result, "written to out.json")
	}
}

func TestOutfileSummary_FallbackType(t *testing.T) {
	got, ok := outfileSummary("anything", "out.json").(genericOutfileSummary)
	if !ok {
		t.Fatal("outfileSummary() did not return genericOutfileSummary for unknown type")
	}
	if got.Result != "written to out.json" {
		t.Errorf("Result = %q, want %q", got.Result, "written to out.json")
	}
}

// --- Integration tests via rootCmd.Execute() ---

// TestExecFunction_OutfileFlag_FlagRegistered verifies the --outfile flag exists
// on the exec function command.
func TestExecFunction_OutfileFlag_FlagRegistered(t *testing.T) {
	f := execFunctionCmd.Flags().Lookup("outfile")
	if f == nil {
		t.Fatal("--outfile flag not registered on exec function command")
	}
	if f.DefValue != "" {
		t.Errorf("--outfile default value = %q, want %q", f.DefValue, "")
	}
}

// TestExecFunction_Outfile_WritesBodyToFile exercises the full exec function path:
// mock HTTP server → rootCmd.Execute() with --outfile → verify file content.
func TestExecFunction_Outfile_WritesBodyToFile(t *testing.T) {
	responseBody := `{"status":"ok","value":123}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/platform/app-engine/app-functions/v1/apps/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	}))
	defer server.Close()

	viper.Reset()
	cfgFile = newExecFunctionTestConfig(t, server.URL)
	defer func() {
		cfgFile = ""
		viper.Reset()
		resetExecFunctionFlags(t)
	}()

	dir := t.TempDir()
	outfile := filepath.Join(dir, "result.json")

	rootCmd.SetArgs([]string{
		"exec", "function", "my.app/my-function",
		"--method", "GET",
		"--outfile", outfile,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}

	got, err := os.ReadFile(outfile)
	if err != nil {
		t.Fatalf("failed to read outfile: %v", err)
	}
	if string(got) != responseBody {
		t.Errorf("outfile content = %q, want %q", string(got), responseBody)
	}
	info, err := os.Stat(outfile)
	if err != nil {
		t.Fatalf("failed to stat outfile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}
}

// TestExecFunction_NoOutfile_DoesNotCreateFile verifies that without --outfile
// no file is created (and the command does not error on a valid response).
func TestExecFunction_NoOutfile_DoesNotCreateFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	viper.Reset()
	cfgFile = newExecFunctionTestConfig(t, server.URL)
	defer func() {
		cfgFile = ""
		viper.Reset()
		resetExecFunctionFlags(t)
	}()

	dir := t.TempDir()
	unexpectedFile := filepath.Join(dir, "should-not-exist.json")

	rootCmd.SetArgs([]string{
		"exec", "function", "my.app/my-function",
		"--method", "GET",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}

	if _, err := os.Stat(unexpectedFile); err == nil {
		t.Error("no outfile was requested but a file was created")
	}
}
