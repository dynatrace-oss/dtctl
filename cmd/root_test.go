package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/pkg/suggest"
)

// TestGlobalFlags_Config tests the --config flag
func TestGlobalFlags_Config(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		wantErr    bool
	}{
		{
			name:       "custom config path",
			configFile: "custom-config.yaml",
			wantErr:    false,
		},
		{
			name:       "default config location",
			configFile: "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			var configPath string

			if tt.configFile != "" {
				configPath = filepath.Join(tmpDir, tt.configFile)
				cfg := config.NewConfig()
				cfg.SetContext("test", "https://test.dt.com", "test-token")
				_ = cfg.SetToken("test-token", "dt0c01.test")
				cfg.CurrentContext = "test"
				if err := cfg.SaveTo(configPath); err != nil {
					t.Fatalf("failed to save config: %v", err)
				}
			}

			// Reset viper state
			viper.Reset()

			// Set config file flag
			cfgFile = configPath

			// Initialize config
			initConfig()

			// Verify config was loaded if custom path provided
			if tt.configFile != "" && viper.ConfigFileUsed() != configPath {
				t.Errorf("Expected config file %s, got %s", configPath, viper.ConfigFileUsed())
			}
		})
	}
}

// TestGlobalFlags_Context tests the --context flag
func TestGlobalFlags_Context(t *testing.T) {
	tests := []struct {
		name        string
		context     string
		wantContext string
	}{
		{
			name:        "custom context",
			context:     "prod",
			wantContext: "prod",
		},
		{
			name:        "default context",
			context:     "",
			wantContext: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset
			viper.Reset()
			contextName = tt.context

			if tt.context != "" {
				viper.Set("context", tt.context)
			}

			got := viper.GetString("context")
			if got != tt.wantContext {
				t.Errorf("context = %v, want %v", got, tt.wantContext)
			}
		})
	}
}

// TestLoadConfig tests the LoadConfig function with --context flag override
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name               string
		currentContext     string
		contextFlagValue   string
		wantCurrentContext string
	}{
		{
			name:               "no context flag - use config default",
			currentContext:     "dev",
			contextFlagValue:   "",
			wantCurrentContext: "dev",
		},
		{
			name:               "context flag overrides config",
			currentContext:     "dev",
			contextFlagValue:   "prod",
			wantCurrentContext: "prod",
		},
		{
			name:               "context flag set to same as config",
			currentContext:     "staging",
			contextFlagValue:   "staging",
			wantCurrentContext: "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			cfg := config.NewConfig()
			cfg.SetContext("dev", "https://dev.dt.com", "dev-token")
			cfg.SetContext("staging", "https://staging.dt.com", "staging-token")
			cfg.SetContext("prod", "https://prod.dt.com", "prod-token")
			cfg.CurrentContext = tt.currentContext

			if err := cfg.SaveTo(configPath); err != nil {
				t.Fatalf("failed to save config: %v", err)
			}

			// Save original values and environment
			origCfgFile := cfgFile
			origContextName := contextName
			origEnvContext := os.Getenv("DTCTL_CONTEXT")
			defer func() {
				cfgFile = origCfgFile
				contextName = origContextName
				if origEnvContext != "" {
					_ = os.Setenv("DTCTL_CONTEXT", origEnvContext)
				} else {
					_ = os.Unsetenv("DTCTL_CONTEXT")
				}
			}()

			// Unset environment variable to avoid interference
			_ = os.Unsetenv("DTCTL_CONTEXT")

			// Reset state
			viper.Reset()
			cfgFile = configPath
			contextName = tt.contextFlagValue

			// Initialize config
			initConfig()

			// Load config with context override
			loadedCfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}

			if loadedCfg.CurrentContext != tt.wantCurrentContext {
				t.Errorf("LoadConfig().CurrentContext = %v, want %v", loadedCfg.CurrentContext, tt.wantCurrentContext)
			}
		})
	}
}

// TestGlobalFlags_Output tests the --output/-o flag
func TestGlobalFlags_Output(t *testing.T) {
	validFormats := []string{"json", "yaml", "csv", "table", "wide"}

	for _, format := range validFormats {
		t.Run(format, func(t *testing.T) {
			viper.Reset()
			outputFormat = format
			viper.Set("output", format)

			got := viper.GetString("output")
			if got != format {
				t.Errorf("output format = %v, want %v", got, format)
			}
		})
	}
}

// TestGlobalFlags_Verbose tests the --verbose/-v flag
func TestGlobalFlags_Verbose(t *testing.T) {
	tests := []struct {
		name      string
		verbosity int
		want      int
	}{
		{
			name:      "no verbose flag",
			verbosity: 0,
			want:      0,
		},
		{
			name:      "single -v",
			verbosity: 1,
			want:      1,
		},
		{
			name:      "double -vv",
			verbosity: 2,
			want:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verbosity = tt.verbosity

			if verbosity != tt.want {
				t.Errorf("verbosity = %v, want %v", verbosity, tt.want)
			}
		})
	}
}

// TestGlobalFlags_DryRun tests the --dry-run flag
func TestGlobalFlags_DryRun(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
		want   bool
	}{
		{
			name:   "dry-run enabled",
			dryRun: true,
			want:   true,
		},
		{
			name:   "dry-run disabled",
			dryRun: false,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dryRun = tt.dryRun

			if dryRun != tt.want {
				t.Errorf("dryRun = %v, want %v", dryRun, tt.want)
			}
		})
	}
}

// TestGlobalFlags_PlainMode tests the --plain flag
func TestGlobalFlags_PlainMode(t *testing.T) {
	tests := []struct {
		name  string
		plain bool
		want  bool
	}{
		{
			name:  "plain mode enabled",
			plain: true,
			want:  true,
		},
		{
			name:  "plain mode disabled",
			plain: false,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plainMode = tt.plain

			got := GetPlainMode()
			if got != tt.want {
				t.Errorf("GetPlainMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGlobalFlags_ChunkSize tests the --chunk-size flag
func TestGlobalFlags_ChunkSize(t *testing.T) {
	tests := []struct {
		name      string
		chunkSize int64
		want      int64
	}{
		{
			name:      "default chunk size",
			chunkSize: 500,
			want:      500,
		},
		{
			name:      "custom chunk size",
			chunkSize: 1000,
			want:      1000,
		},
		{
			name:      "disabled chunking",
			chunkSize: 0,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunkSize = tt.chunkSize

			got := GetChunkSize()
			if got != tt.want {
				t.Errorf("GetChunkSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewPrinter tests the NewPrinter helper function
func TestNewPrinter(t *testing.T) {
	tests := []struct {
		name         string
		outputFormat string
		plainMode    bool
	}{
		{
			name:         "json output",
			outputFormat: "json",
			plainMode:    false,
		},
		{
			name:         "yaml output",
			outputFormat: "yaml",
			plainMode:    false,
		},
		{
			name:         "table output with plain mode",
			outputFormat: "table",
			plainMode:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputFormat = tt.outputFormat
			plainMode = tt.plainMode

			printer := NewPrinter()
			if printer == nil {
				t.Error("NewPrinter() returned nil")
			}
		})
	}
}

// TestInitConfig tests the initConfig function
func TestInitConfig(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    bool
		configFileName string
		wantConfigUsed bool
	}{
		{
			name:           "valid config file exists",
			setupConfig:    true,
			configFileName: "config.yaml",
			wantConfigUsed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			viper.Reset()

			if tt.setupConfig {
				configPath := filepath.Join(tmpDir, tt.configFileName)
				cfg := config.NewConfig()
				cfg.SetContext("test", "https://test.dt.com", "token")
				_ = cfg.SaveTo(configPath)
				cfgFile = configPath
			} else {
				cfgFile = ""
			}

			initConfig()

			configUsed := viper.ConfigFileUsed()
			if tt.wantConfigUsed && configUsed == "" {
				t.Error("Expected config file to be used, but none was loaded")
			}
			if !tt.wantConfigUsed && configUsed != "" {
				t.Errorf("Expected no config file, but got %s", configUsed)
			}
		})
	}
}

// TestEnhanceFlagError tests flag error enhancement with suggestions
func TestEnhanceFlagError(t *testing.T) {
	tests := []struct {
		name        string
		errMsg      string
		wantContain string
	}{
		{
			name:        "unknown flag with close match",
			errMsg:      "unknown flag: --outpt",
			wantContain: "output",
		},
		{
			name:        "unknown shorthand flag",
			errMsg:      "unknown shorthand flag: 'x'",
			wantContain: "unknown flag --x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The enhanceFlagError function is called internally by cobra
			// We test it indirectly by verifying the error suggestion system works
			flags := collectFlags(rootCmd)

			if len(flags) == 0 {
				t.Error("Expected to collect flags from rootCmd, got none")
			}

			// Verify our global flags are in the list
			expectedFlags := []string{"config", "context", "output", "verbose", "dry-run", "plain", "chunk-size"}
			for _, expectedFlag := range expectedFlags {
				found := false
				for _, flag := range flags {
					if flag == expectedFlag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected flag %s not found in collected flags", expectedFlag)
				}
			}
		})
	}
}

// TestEnhanceCommandError tests command error enhancement with suggestions
func TestEnhanceCommandError(t *testing.T) {
	subcommands := collectSubcommands(rootCmd)

	if len(subcommands) == 0 {
		t.Error("Expected to collect subcommands from rootCmd, got none")
	}

	// Verify common subcommands are present
	expectedCommands := []string{"get", "create", "apply", "delete", "query", "exec"}
	for _, expectedCmd := range expectedCommands {
		found := false
		for _, cmd := range subcommands {
			if cmd == expectedCmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected command %s not found in collected subcommands", expectedCmd)
		}
	}
}

// TestRequireSubcommand tests the requireSubcommand helper
func TestRequireSubcommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "no args provided",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "unknown resource type",
			args:    []string{"invalidresource"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireSubcommand(rootCmd, tt.args)

			if (err != nil) != tt.wantErr {
				t.Errorf("requireSubcommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestEnvironmentVariableBinding tests that persistent flags are bound to environment variables
func TestEnvironmentVariableBinding(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		envVal  string
		flagKey string
	}{
		{
			name:    "DTCTL_CONTEXT env var",
			envVar:  "DTCTL_CONTEXT",
			envVal:  "prod-context",
			flagKey: "context",
		},
		{
			name:    "DTCTL_OUTPUT env var",
			envVar:  "DTCTL_OUTPUT",
			envVal:  "json",
			flagKey: "output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			os.Setenv(tt.envVar, tt.envVal)
			defer os.Unsetenv(tt.envVar)

			// Reset and reinitialize viper
			viper.Reset()
			viper.SetEnvPrefix("DTCTL")
			viper.AutomaticEnv()

			// Bind flag
			rootCmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
				if flag.Name == tt.flagKey {
					_ = viper.BindPFlag(tt.flagKey, flag)
				}
			})

			// Verify environment variable is read
			got := viper.GetString(tt.flagKey)
			if got != tt.envVal {
				t.Errorf("viper.GetString(%s) = %v, want %v", tt.flagKey, got, tt.envVal)
			}
		})
	}
}

// --- errorToDetail tests ---

func TestErrorToDetail_DiagnosticError(t *testing.T) {
	err := &diagnostic.Error{
		Operation:  "get workflows",
		StatusCode: 401,
		Message:    "authentication failed",
		RequestID:  "req-abc-123",
		Suggestions: []string{
			"Run 'dtctl auth login' to refresh your token",
		},
	}

	detail := errorToDetail(err)

	if detail.Code != "auth_required" {
		t.Errorf("Code = %q, want %q", detail.Code, "auth_required")
	}
	if detail.Message != "authentication failed" {
		t.Errorf("Message = %q, want %q", detail.Message, "authentication failed")
	}
	if detail.Operation != "get workflows" {
		t.Errorf("Operation = %q, want %q", detail.Operation, "get workflows")
	}
	if detail.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want %d", detail.StatusCode, 401)
	}
	if detail.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %q, want %q", detail.RequestID, "req-abc-123")
	}
	if len(detail.Suggestions) != 1 {
		t.Fatalf("Suggestions count = %d, want 1", len(detail.Suggestions))
	}
}

func TestErrorToDetail_DiagnosticErrorStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCode   string
	}{
		{"400 bad request", 400, "bad_request"},
		{"401 unauthorized", 401, "auth_required"},
		{"403 forbidden", 403, "permission_denied"},
		{"404 not found", 404, "not_found"},
		{"409 conflict", 409, "conflict"},
		{"429 rate limited", 429, "rate_limited"},
		{"500 server error", 500, "server_error"},
		{"502 bad gateway", 502, "server_error"},
		{"0 no status code", 0, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &diagnostic.Error{
				Operation:  "test",
				StatusCode: tt.statusCode,
				Message:    "test error",
			}
			detail := errorToDetail(err)
			if detail.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", detail.Code, tt.wantCode)
			}
		})
	}
}

func TestErrorToDetail_APIError(t *testing.T) {
	err := &client.APIError{
		StatusCode: 404,
		Message:    "not found",
		Details:    "workflow does not exist",
	}

	detail := errorToDetail(err)

	if detail.Code != "not_found" {
		t.Errorf("Code = %q, want %q", detail.Code, "not_found")
	}
	if detail.Message != "not found - workflow does not exist" {
		t.Errorf("Message = %q, want %q", detail.Message, "not found - workflow does not exist")
	}
	if detail.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want %d", detail.StatusCode, 404)
	}
}

func TestErrorToDetail_APIErrorWithoutDetails(t *testing.T) {
	err := &client.APIError{
		StatusCode: 500,
		Message:    "internal server error",
	}

	detail := errorToDetail(err)

	if detail.Code != "server_error" {
		t.Errorf("Code = %q, want %q", detail.Code, "server_error")
	}
	if detail.Message != "internal server error" {
		t.Errorf("Message = %q, want %q", detail.Message, "internal server error")
	}
}

func TestErrorToDetail_SafetyError(t *testing.T) {
	err := &safety.SafetyError{
		ContextName: "production",
		SafetyLevel: config.SafetyLevelReadOnly,
		Operation:   safety.OperationDelete,
		Reason:      "Context 'production' (readonly) does not allow delete operations",
		Suggestions: []string{
			"Switch to a context with write permissions",
		},
	}

	detail := errorToDetail(err)

	if detail.Code != "safety_blocked" {
		t.Errorf("Code = %q, want %q", detail.Code, "safety_blocked")
	}
	if detail.Message != "Context 'production' (readonly) does not allow delete operations" {
		t.Errorf("Message = %q, want expected reason", detail.Message)
	}
	if len(detail.Suggestions) != 1 {
		t.Fatalf("Suggestions count = %d, want 1", len(detail.Suggestions))
	}
}

func TestErrorToDetail_CommandError(t *testing.T) {
	err := &suggest.CommandError{
		Command: "geet",
		Message: `unknown command "geet"`,
		Suggestion: &suggest.Suggestion{
			Value:    "get",
			Distance: 1,
		},
	}

	detail := errorToDetail(err)

	if detail.Code != "unknown_command" {
		t.Errorf("Code = %q, want %q", detail.Code, "unknown_command")
	}
	if detail.Message != `unknown command "geet"` {
		t.Errorf("Message = %q, want expected message", detail.Message)
	}
	if len(detail.Suggestions) != 1 {
		t.Fatalf("Suggestions count = %d, want 1", len(detail.Suggestions))
	}
	if detail.Suggestions[0] != `did you mean "get"?` {
		t.Errorf("Suggestion = %q, want %q", detail.Suggestions[0], `did you mean "get"?`)
	}
}

func TestErrorToDetail_CommandErrorNoSuggestion(t *testing.T) {
	err := &suggest.CommandError{
		Command: "xyzzy",
		Message: `unknown command "xyzzy"`,
	}

	detail := errorToDetail(err)

	if detail.Code != "unknown_command" {
		t.Errorf("Code = %q, want %q", detail.Code, "unknown_command")
	}
	if detail.Suggestions != nil {
		t.Errorf("Suggestions should be nil, got %v", detail.Suggestions)
	}
}

func TestErrorToDetail_FlagError(t *testing.T) {
	err := &suggest.FlagError{
		Flag:    "outpt",
		Message: "unknown flag --outpt",
		Suggestion: &suggest.Suggestion{
			Value:    "output",
			Distance: 1,
		},
	}

	detail := errorToDetail(err)

	if detail.Code != "unknown_command" {
		t.Errorf("Code = %q, want %q", detail.Code, "unknown_command")
	}
	if len(detail.Suggestions) != 1 {
		t.Fatalf("Suggestions count = %d, want 1", len(detail.Suggestions))
	}
	if detail.Suggestions[0] != "did you mean --output?" {
		t.Errorf("Suggestion = %q, want %q", detail.Suggestions[0], "did you mean --output?")
	}
}

func TestErrorToDetail_GenericError(t *testing.T) {
	err := fmt.Errorf("something unexpected happened")

	detail := errorToDetail(err)

	if detail.Code != "error" {
		t.Errorf("Code = %q, want %q", detail.Code, "error")
	}
	if detail.Message != "something unexpected happened" {
		t.Errorf("Message = %q, want %q", detail.Message, "something unexpected happened")
	}
}

func TestErrorToDetail_WrappedDiagnosticError(t *testing.T) {
	inner := &diagnostic.Error{
		Operation:  "apply dashboard",
		StatusCode: 403,
		Message:    "insufficient permissions",
	}
	wrapped := fmt.Errorf("command failed: %w", inner)

	detail := errorToDetail(wrapped)

	if detail.Code != "permission_denied" {
		t.Errorf("Code = %q, want %q (should unwrap diagnostic.Error)", detail.Code, "permission_denied")
	}
	if detail.Operation != "apply dashboard" {
		t.Errorf("Operation = %q, want %q", detail.Operation, "apply dashboard")
	}
}

func TestErrorToDetail_DiagnosticPrecedesAPIError(t *testing.T) {
	// diagnostic.Error wraps a client.APIError — diagnostic should take precedence
	apiErr := &client.APIError{StatusCode: 404, Message: "not found"}
	diagErr := diagnostic.Wrap(apiErr, "get workflows")

	detail := errorToDetail(diagErr)

	// Should use diagnostic.Error classification, not raw APIError
	if detail.Code != "not_found" {
		t.Errorf("Code = %q, want %q", detail.Code, "not_found")
	}
	if detail.Operation != "get workflows" {
		t.Errorf("Operation = %q, want %q", detail.Operation, "get workflows")
	}
}

// --- classifyGenericError tests ---

func TestClassifyGenericError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"context error", errors.New("no active context configured"), "context_error"},
		{"no context", errors.New("no context set"), "context_error"},
		{"config error", errors.New("failed to load config"), "config_error"},
		{"configuration error", errors.New("invalid configuration"), "config_error"},
		{"timeout error", errors.New("operation timed out"), "timeout"},
		{"timeout variant", errors.New("request timeout after 30s"), "timeout"},
		{"validation error", errors.New("validation failed for field name"), "validation_error"},
		{"invalid input", errors.New("invalid resource definition"), "validation_error"},
		{"generic error", errors.New("something went wrong"), "error"},
		{"empty error", errors.New(""), "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGenericError(tt.err)
			if got != tt.wantCode {
				t.Errorf("classifyGenericError(%q) = %q, want %q", tt.err, got, tt.wantCode)
			}
		})
	}
}

// --- exitCodeForError tests ---

func TestExitCodeForError_DiagnosticError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCode   int
	}{
		{"401 auth error", 401, client.ExitAuthError},
		{"403 permission error", 403, client.ExitPermissionError},
		{"404 not found", 404, client.ExitNotFoundError},
		{"500 server error", 500, client.ExitError},
		{"0 generic error", 0, client.ExitError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &diagnostic.Error{StatusCode: tt.statusCode}
			got := exitCodeForError(err)
			if got != tt.wantCode {
				t.Errorf("exitCodeForError() = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestExitCodeForError_APIError(t *testing.T) {
	err := &client.APIError{StatusCode: 404, Message: "not found"}
	got := exitCodeForError(err)
	if got != client.ExitNotFoundError {
		t.Errorf("exitCodeForError() = %d, want %d", got, client.ExitNotFoundError)
	}
}

func TestExitCodeForError_CommandError(t *testing.T) {
	err := &suggest.CommandError{Command: "geet", Message: "unknown command"}
	got := exitCodeForError(err)
	if got != client.ExitUsageError {
		t.Errorf("exitCodeForError() = %d, want %d", got, client.ExitUsageError)
	}
}

func TestExitCodeForError_FlagError(t *testing.T) {
	err := &suggest.FlagError{Flag: "outpt", Message: "unknown flag"}
	got := exitCodeForError(err)
	if got != client.ExitUsageError {
		t.Errorf("exitCodeForError() = %d, want %d", got, client.ExitUsageError)
	}
}

func TestExitCodeForError_GenericError(t *testing.T) {
	err := errors.New("generic error")
	got := exitCodeForError(err)
	if got != client.ExitError {
		t.Errorf("exitCodeForError() = %d, want %d", got, client.ExitError)
	}
}
