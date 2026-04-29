package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// setupAuthTestConfig creates a temporary config with the given context and returns the path.
func setupAuthTestConfig(t *testing.T, contextName, environment, tokenRef string) string {
	t.Helper()
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := config.NewConfig()
	cfg.SetContext(contextName, environment, tokenRef)
	cfg.CurrentContext = contextName

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}
	return configPath
}

// resetAuthLoginFlags resets all auth login command flags to their default values.
// This is needed because cobra/pflag retains flag values across Execute() calls when
// the global rootCmd is reused in tests — a flag not present in the args of a new
// Execute() call keeps the value set by the previous call.
func resetAuthLoginFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"context", "environment", "token-name", "timeout", "safety-level"} {
		if f := authLoginCmd.Flags().Lookup(name); f != nil {
			if err := f.Value.Set(f.DefValue); err != nil {
				t.Logf("warning: could not reset flag %q: %v", name, err)
			}
			f.Changed = false
		}
	}
}

// TestAuthLogin_FlagValidation checks that the login command fails correctly when
// neither flags nor a current context are available.
func TestAuthLogin_FlagValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setupConfig bool // whether to write a config with a current context
		wantErrSub  string
		wantHint    string
	}{
		{
			name:        "no flags no config errors helpfully",
			args:        []string{"auth", "login"},
			setupConfig: false,
			wantErrSub:  "--context and --environment are required",
			wantHint:    "dtctl ctx",
		},
		{
			name:        "no flags empty current context errors helpfully",
			args:        []string{"auth", "login"},
			setupConfig: true, // config exists but CurrentContext is empty (handled below)
			wantErrSub:  "--context and --environment are required when no current context is set",
			wantHint:    "dtctl ctx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			if tt.name == "no flags no config errors helpfully" {
				// Point to a non-existent config file
				tmpDir := t.TempDir()
				cfgFile = filepath.Join(tmpDir, "nonexistent.yaml")
			} else {
				// Config with no current context set
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.yaml")
				cfg := config.NewConfig()
				// Deliberately leave cfg.CurrentContext empty
				if err := cfg.SaveTo(configPath); err != nil {
					t.Fatalf("failed to save config: %v", err)
				}
				cfgFile = configPath
			}

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErrSub, err)
			}
			if !strings.Contains(err.Error(), tt.wantHint) {
				t.Errorf("expected error to contain hint %q, got: %v", tt.wantHint, err)
			}

			// Reset
			cfgFile = ""
		})
	}
}

// TestAuthLogin_CurrentContextFallback verifies that the login command derives
// context name, environment URL and token name from the active context when no
// flags are provided.  The test stops before the actual OAuth flow (keyring
// unavailable) but checks the context resolution logic produces the expected
// error – i.e. it gets past flag validation.
func TestAuthLogin_CurrentContextFallback(t *testing.T) {
	viper.Reset()

	const (
		ctxName  = "my-context"
		envURL   = "https://abc12345.apps.dynatrace.com"
		tokenRef = "my-context-oauth"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, tokenRef)
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	// Execute with no flags – should pass flag validation and reach the
	// keyring check (which fails in a test environment).
	rootCmd.SetArgs([]string{"auth", "login"})
	err := rootCmd.Execute()

	// We expect the command to fail, but NOT because of missing flags.
	// It should fail later (keyring unavailable or similar infrastructure error).
	if err == nil {
		// Unlikely in a unit test environment without a real keyring/browser,
		// but not a failure of the logic we are testing.
		return
	}

	if strings.Contains(err.Error(), "--context and --environment are required") {
		t.Errorf("expected current-context fallback to work, but got flag validation error: %v", err)
	}

	// The error should reference the token storage env var or the keyring disable env var
	// as a hint for how to resolve the issue.
	errMsg := err.Error()
	hasStorageHint := strings.Contains(errMsg, config.EnvTokenStorage) || strings.Contains(errMsg, config.EnvDisableKeyring)
	if strings.Contains(errMsg, "keyring") && !hasStorageHint {
		t.Errorf("expected error to include storage hint (%s or %s), got: %v", config.EnvTokenStorage, config.EnvDisableKeyring, err)
	}
}

// TestAuthLogin_PartialFlags verifies that supplying only --context (without
// --environment) causes the missing value to be filled from the current context.
func TestAuthLogin_PartialFlags_EnvironmentFromContext(t *testing.T) {
	viper.Reset()
	resetAuthLoginFlags(t)

	const (
		ctxName  = "my-context"
		envURL   = "https://abc12345.apps.dynatrace.com"
		tokenRef = "my-context-oauth"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, tokenRef)
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	// --context is the active context, so environment resolution uses its own URL.
	rootCmd.SetArgs([]string{"auth", "login", "--context", ctxName})
	err := rootCmd.Execute()

	if err != nil && strings.Contains(err.Error(), "--context and --environment are required") {
		t.Errorf("expected environment to be filled from named context, got: %v", err)
	}
}

// TestAuthLogin_KeyringRecovery verifies that the auth login command
// attempts to create a keyring collection when CheckKeyring returns an
// error containing ErrMsgCollectionUnlock, and that a successful recovery
// allows the flow to continue past the keyring gate.
func TestAuthLogin_KeyringRecovery(t *testing.T) {
	viper.Reset()

	const (
		ctxName = "recover-ctx"
		envURL  = "https://abc12345.apps.dynatrace.com"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, ctxName+"-oauth")
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	// Track whether EnsureKeyringCollection was called.
	ensureCalled := false

	// First call to CheckKeyring returns the unlock error; after recovery
	// it returns nil (simulating a fixed keyring).
	calls := 0
	origCheck := authCheckKeyringFunc
	origEnsure := authEnsureKeyringFunc
	defer func() {
		authCheckKeyringFunc = origCheck
		authEnsureKeyringFunc = origEnsure
	}()

	authCheckKeyringFunc = func() error {
		calls++
		if calls == 1 {
			return fmt.Errorf("keyring probe failed: %s", config.ErrMsgCollectionUnlock)
		}
		return nil // recovered
	}
	authEnsureKeyringFunc = func(_ context.Context) error {
		ensureCalled = true
		return nil
	}

	rootCmd.SetArgs([]string{"auth", "login", "--context", ctxName, "--environment", envURL})
	err := rootCmd.Execute()

	if !ensureCalled {
		t.Fatal("expected EnsureKeyringCollection to be called during recovery")
	}

	// After recovery the command should proceed past the keyring gate.
	// It will eventually fail further along (no real keyring for token
	// storage, or OAuth infrastructure issues), but not with the initial
	// keyring gate error about requiring a working keyring.
	if err != nil && strings.Contains(err.Error(), "OAuth login requires a working system keyring") {
		t.Errorf("expected recovery to succeed and proceed past keyring gate, got: %v", err)
	}
}

// TestFinalizeLoginConfig verifies that after a successful login the config is
// updated correctly and any template placeholder contexts (empty environment)
// are pruned, except for the context that was just logged into.
func TestFinalizeLoginConfig(t *testing.T) {
	tests := []struct {
		name          string
		setupContexts []struct{ name, env, tok string }
		// placeholderNames overrides which context names are treated as prunable
		// placeholders. nil means: derive from setupContexts where env == "".
		// Set explicitly to map[string]bool{} to simulate a context whose env is
		// empty only because its env var was unset at load time (not a true placeholder).
		placeholderNames map[string]bool
		loginContext     string
		loginEnv         string
		loginToken       string
		wantContexts     []string
		wantCurrent      string
	}{
		{
			name: "prunes my-environment placeholder after login",
			setupContexts: []struct{ name, env, tok string }{
				{"my-environment", "", "my-token"},
			},
			loginContext: "dev",
			loginEnv:     "https://abc12345.apps.dynatrace.com/",
			loginToken:   "dev-oauth",
			wantContexts: []string{"dev"},
			wantCurrent:  "dev",
		},
		{
			name: "prunes multiple placeholder contexts",
			setupContexts: []struct{ name, env, tok string }{
				{"my-environment", "", "my-token"},
				{"staging", "", "staging-token"},
				{"prod", "https://prod.apps.dynatrace.com/", "prod-oauth"},
			},
			loginContext: "dev",
			loginEnv:     "https://dev.apps.dynatracelabs.com/",
			loginToken:   "dev-oauth",
			wantContexts: []string{"prod", "dev"},
			wantCurrent:  "dev",
		},
		{
			name: "keeps all contexts with real environments",
			setupContexts: []struct{ name, env, tok string }{
				{"prod", "https://prod.apps.dynatrace.com/", "prod-oauth"},
				{"staging", "https://staging.apps.dynatrace.com/", "staging-oauth"},
			},
			loginContext: "dev",
			loginEnv:     "https://dev.apps.dynatracelabs.com/",
			loginToken:   "dev-oauth",
			wantContexts: []string{"prod", "staging", "dev"},
			wantCurrent:  "dev",
		},
		{
			name:          "no existing contexts — fresh config",
			setupContexts: []struct{ name, env, tok string }{},
			loginContext:  "dev",
			loginEnv:      "https://abc12345.apps.dynatrace.com/",
			loginToken:    "dev-oauth",
			wantContexts:  []string{"dev"},
			wantCurrent:   "dev",
		},
		{
			// Simulates: config has `environment: ${CI_DT_URL}` and CI_DT_URL is
			// unset at login time, so os.ExpandEnv produces Environment=="". The
			// raw config has a non-empty template value, so ci-env is NOT in
			// placeholderNames and must be preserved.
			name: "preserves context backed by unset env var",
			setupContexts: []struct{ name, env, tok string }{
				{"ci-env", "", "ci-token"}, // expanded from ${CI_DT_URL} which is unset
			},
			placeholderNames: map[string]bool{}, // ci-env is NOT a placeholder in raw config
			loginContext:     "dev",
			loginEnv:         "https://abc12345.apps.dynatrace.com/",
			loginToken:       "dev-oauth",
			wantContexts:     []string{"ci-env", "dev"},
			wantCurrent:      "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			for _, c := range tt.setupContexts {
				cfg.SetContext(c.name, c.env, c.tok)
			}

			placeholderNames := tt.placeholderNames
			if placeholderNames == nil {
				placeholderNames = make(map[string]bool)
				for _, c := range tt.setupContexts {
					if c.env == "" {
						placeholderNames[c.name] = true
					}
				}
			}

			finalizeLoginConfig(cfg, tt.loginContext, tt.loginEnv, tt.loginToken, config.SafetyLevelReadWriteAll, placeholderNames)

			if cfg.CurrentContext != tt.wantCurrent {
				t.Errorf("CurrentContext = %q, want %q", cfg.CurrentContext, tt.wantCurrent)
			}
			if len(cfg.Contexts) != len(tt.wantContexts) {
				got := make([]string, len(cfg.Contexts))
				for i, nc := range cfg.Contexts {
					got[i] = nc.Name
				}
				t.Fatalf("context count = %d %v, want %d %v", len(cfg.Contexts), got, len(tt.wantContexts), tt.wantContexts)
			}
			for i, want := range tt.wantContexts {
				if cfg.Contexts[i].Name != want {
					t.Errorf("contexts[%d].Name = %q, want %q", i, cfg.Contexts[i].Name, want)
				}
			}
			// The login context must always be present with the correct environment.
			nc, err := cfg.GetContext(tt.loginContext)
			if err != nil {
				t.Fatalf("login context %q not found after finalize: %v", tt.loginContext, err)
			}
			if nc.Context.Environment != tt.loginEnv {
				t.Errorf("login context environment = %q, want %q", nc.Context.Environment, tt.loginEnv)
			}
		})
	}
}

// TestResolveLoginContext tests the resolveLoginContext helper that determines
// contextName, environment, and tokenName from an existing config when not all
// values are supplied as explicit CLI flags.
func TestResolveLoginContext(t *testing.T) {
	const (
		prodURL = "https://irc65933.apps.dynatrace.com/"
		hardURL = "https://eva38390.sprint.apps.dynatracelabs.com/"
		prodTok = "prod1sfm-oauth"
		hardTok = "hardsfm-oauth"
	)

	makeCfg := func() *config.Config {
		cfg := config.NewConfig()
		cfg.SetContext("prod1sfm", prodURL, prodTok)
		cfg.SetContext("hardsfm", hardURL, hardTok)
		cfg.CurrentContext = "prod1sfm"
		return cfg
	}

	tests := []struct {
		name        string
		contextName string
		environment string
		tokenName   string
		currentCtx  string // override CurrentContext ("" means use default "prod1sfm")
		wantContext string
		wantEnv     string
		wantToken   string
		wantErr     bool
	}{
		{
			name:        "no flags uses current context",
			wantContext: "prod1sfm",
			wantEnv:     prodURL,
			wantToken:   prodTok,
		},
		{
			// Regression: before the fix, this would return prodURL for environment.
			name:        "explicit context uses named context URL not current context URL",
			contextName: "hardsfm",
			wantContext: "hardsfm",
			wantEnv:     hardURL,
			wantToken:   hardTok,
		},
		{
			name:        "explicit context and explicit environment uses provided values",
			contextName: "hardsfm",
			environment: "https://custom.example.invalid/",
			wantContext: "hardsfm",
			wantEnv:     "https://custom.example.invalid/",
			wantToken:   hardTok, // token still resolved from named context
		},
		{
			name:        "all flags explicit skips lookup entirely",
			contextName: "hardsfm",
			environment: "https://custom.example.invalid/",
			tokenName:   "my-explicit-token",
			wantContext: "hardsfm",
			wantEnv:     "https://custom.example.invalid/",
			wantToken:   "my-explicit-token",
		},
		{
			name:        "explicit context not in config returns empty environment",
			contextName: "new-context",
			wantContext: "new-context",
			wantEnv:     "", // caller must check and error
			wantToken:   "",
		},
		{
			name:       "no flags no current context returns error",
			currentCtx: "none", // sentinel: clear CurrentContext
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeCfg()
			if tt.currentCtx == "none" {
				cfg.CurrentContext = ""
			}

			gotCtx, gotEnv, gotToken, err := resolveLoginContext(cfg, tt.contextName, tt.environment, tt.tokenName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotCtx != tt.wantContext {
				t.Errorf("context = %q, want %q", gotCtx, tt.wantContext)
			}
			if gotEnv != tt.wantEnv {
				t.Errorf("environment = %q, want %q", gotEnv, tt.wantEnv)
			}
			if gotToken != tt.wantToken {
				t.Errorf("tokenName = %q, want %q", gotToken, tt.wantToken)
			}
		})
	}
}

// TestAuthLogin_ContextOnly_UsesNamedContextURL is the integration-level regression
// test for the bug where dtctl auth login --context <non-active-context> would
// silently overwrite the target context's environment URL with the active context's URL.
//
// Config: prod1sfm (active, prod URL) + hardsfm (sprint URL)
// Command: auth login --context hardsfm   (no --environment)
// Expected: hardsfm's sprint URL is used, command fails at keyring gate (not URL resolution).
func TestAuthLogin_ContextOnly_UsesNamedContextURL(t *testing.T) {
	viper.Reset()
	resetAuthLoginFlags(t)
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := config.NewConfig()
	cfg.SetContext("prod1sfm", "https://irc65933.apps.dynatrace.com/", "prod1sfm-oauth")
	cfg.SetContext("hardsfm", "https://eva38390.sprint.apps.dynatracelabs.com/", "hardsfm-oauth")
	cfg.CurrentContext = "prod1sfm"

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	rootCmd.SetArgs([]string{"auth", "login", "--context", "hardsfm"})
	err := rootCmd.Execute()

	// The command must NOT fail with a context/environment validation error —
	// that would indicate hardsfm's URL was not resolved from config.
	if err != nil && strings.Contains(err.Error(), "--context and --environment are required") {
		t.Errorf("expected hardsfm's URL to be resolved from config, got: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "not found in config") {
		t.Errorf("expected hardsfm to be found in config, got: %v", err)
	}
	// The command should fail at the keyring gate (disabled in tests), not earlier.
	if err != nil && !strings.Contains(err.Error(), "keyring") {
		t.Errorf("expected keyring error after successful URL resolution, got: %v", err)
	}
}

// TestAuthLogin_NewContext_RequiresEnvironment verifies that when --context names
// a context that does not exist in config yet, --environment must be supplied.
func TestAuthLogin_NewContext_RequiresEnvironment(t *testing.T) {
	viper.Reset()
	resetAuthLoginFlags(t)
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := config.NewConfig()
	cfg.SetContext("existing", "https://abc12345.apps.dynatrace.com/", "existing-oauth")
	cfg.CurrentContext = "existing"

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	rootCmd.SetArgs([]string{"auth", "login", "--context", "brand-new-context"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error for new context without --environment, got nil")
	}
	if !strings.Contains(err.Error(), "--environment is required") {
		t.Errorf("expected --environment required error, got: %v", err)
	}
}

// TestAuthLogin_KeyringRecoveryFailure verifies that when
// EnsureKeyringCollection fails, the command returns an actionable
// diagnostic error with suggestions including file-based storage.
func TestAuthLogin_KeyringRecoveryFailure(t *testing.T) {
	viper.Reset()

	const (
		ctxName = "fail-ctx"
		envURL  = "https://abc12345.apps.dynatrace.com"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, ctxName+"-oauth")
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	origCheck := authCheckKeyringFunc
	origEnsure := authEnsureKeyringFunc
	defer func() {
		authCheckKeyringFunc = origCheck
		authEnsureKeyringFunc = origEnsure
	}()

	// CheckKeyring always fails with the unlock error.
	authCheckKeyringFunc = func() error {
		return fmt.Errorf("keyring probe failed: %s", config.ErrMsgCollectionUnlock)
	}
	// EnsureKeyringCollection fails too.
	authEnsureKeyringFunc = func(_ context.Context) error {
		return fmt.Errorf("D-Bus connection refused")
	}

	rootCmd.SetArgs([]string{"auth", "login", "--context", ctxName, "--environment", envURL})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error when keyring recovery fails")
	}
	if !strings.Contains(err.Error(), "keyring") {
		t.Errorf("expected keyring-related error, got: %v", err)
	}
	// Should suggest file-based storage as the primary alternative
	if !strings.Contains(err.Error(), config.EnvTokenStorage) {
		t.Errorf("expected suggestion about %s, got: %v", config.EnvTokenStorage, err)
	}
}

// TestAuthLogin_FileStorage_PassesKeyringGate verifies that when the keyring
// is unavailable but DTCTL_TOKEN_STORAGE=file is set, auth login proceeds
// past the keyring gate (it will fail later at the actual OAuth flow, but
// the keyring gate itself should not block).
func TestAuthLogin_FileStorage_PassesKeyringGate(t *testing.T) {
	viper.Reset()
	resetAuthLoginFlags(t)
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")
	t.Setenv(config.EnvTokenStorage, "file")

	const (
		ctxName = "file-ctx"
		envURL  = "https://abc12345.apps.dynatrace.com"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, ctxName+"-oauth")
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	// Override keyring check to always fail (deterministic across OSes)
	origCheck := authCheckKeyringFunc
	defer func() { authCheckKeyringFunc = origCheck }()
	authCheckKeyringFunc = func() error {
		return fmt.Errorf("keyring disabled via %s environment variable", config.EnvDisableKeyring)
	}

	// Use a very short timeout so the OAuth flow fails quickly instead of hanging
	rootCmd.SetArgs([]string{"auth", "login", "--context", ctxName, "--environment", envURL, "--timeout", "1s"})
	err := rootCmd.Execute()

	// The command should NOT fail at the keyring gate.
	// It will fail further along (OAuth timeout or no browser),
	// but the error should NOT be about requiring a keyring.
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "requires a token storage backend") {
			t.Errorf("expected file storage to pass keyring gate, got blocking error: %v", err)
		}
		if strings.Contains(errMsg, "requires a working system keyring") {
			t.Errorf("expected file storage to pass keyring gate, got old-style keyring error: %v", err)
		}
	}
}

// TestAuthLogin_KeyringRecovery_WithFileStorage verifies that the keyring
// gate in auth login produces a warning (not an error) when keyring is
// unavailable but DTCTL_DISABLE_KEYRING is set together with DTCTL_TOKEN_STORAGE=file.
func TestAuthLogin_KeyringRecovery_WithFileStorage(t *testing.T) {
	viper.Reset()
	resetAuthLoginFlags(t)
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")
	t.Setenv(config.EnvTokenStorage, "file")

	const (
		ctxName = "file-recovery-ctx"
		envURL  = "https://abc12345.apps.dynatrace.com"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, ctxName+"-oauth")
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	// Override keyring check to always fail (simulates headless Linux)
	origCheck := authCheckKeyringFunc
	defer func() { authCheckKeyringFunc = origCheck }()
	authCheckKeyringFunc = func() error {
		return fmt.Errorf("keyring disabled via %s environment variable", config.EnvDisableKeyring)
	}

	// Use a very short timeout so the OAuth flow fails quickly instead of hanging
	rootCmd.SetArgs([]string{"auth", "login", "--context", ctxName, "--environment", envURL, "--timeout", "1s"})
	err := rootCmd.Execute()

	// Should NOT fail at the keyring gate — file storage should let it through.
	if err != nil && strings.Contains(err.Error(), "requires a token storage backend") {
		t.Errorf("expected file storage to bypass keyring gate, got: %v", err)
	}
}
