package cmd

import (
	"fmt"
	"os"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/vfs"
)

// Session is a per-invocation target + credential override for embedded
// callers. When set on RunOptions, the invocation runs against exactly this
// environment and token: the host's config file, contexts, keyring, and
// credential env vars are all out of the picture, which is what a multi-tenant
// service needs — every request brings its own tenant
// (docs/dev/SERVICE_ENGINE_DESIGN.md).
//
// A session replaces credential *resolution*, not command *policy*: blocking
// commands that make no sense service-side (config, ctx, login) is the
// engine's job, on top of this seam.
type Session struct {
	// EnvironmentURL is the Dynatrace environment to run against
	// (e.g. https://abc12345.apps.dynatrace.com). Required.
	EnvironmentURL string
	// Token authenticates every API call of this invocation. Required.
	Token string
	// SafetyLevel bounds mutating operations for this invocation
	// (readonly | readwrite-mine | readwrite-all | dangerously-unrestricted).
	// Empty selects the dtctl default (readwrite-all).
	SafetyLevel config.SafetyLevel
}

// sessionContextName names the synthesized context and token. It appears in
// safety-check messages ("Context 'session' does not allow ...").
const sessionContextName = "session"

func (s *Session) validate() error {
	if s.EnvironmentURL == "" {
		return fmt.Errorf("session: EnvironmentURL is required")
	}
	if s.Token == "" {
		return fmt.Errorf("session: Token is required")
	}
	if s.SafetyLevel != "" {
		valid := false
		for _, l := range config.ValidSafetyLevels() {
			if s.SafetyLevel == l {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("session: invalid safety level %q", s.SafetyLevel)
		}
	}
	return nil
}

// syntheticConfig materializes the session as a single-context in-memory
// config, so everything downstream of LoadConfig (client construction, token
// resolution, safety checks) works unchanged. The token is inline in the
// Tokens list — resolution never touches a keyring or credential store.
func (s *Session) syntheticConfig() *config.Config {
	level := s.SafetyLevel
	if level == "" {
		level = config.DefaultSafetyLevel
	}
	return &config.Config{
		APIVersion:     config.CurrentAPIVersion,
		Kind:           "Config",
		CurrentContext: sessionContextName,
		Contexts: []config.NamedContext{{
			Name: sessionContextName,
			Context: config.Context{
				Environment: s.EnvironmentURL,
				TokenRef:    sessionContextName,
				SafetyLevel: level,
			},
		}},
		Tokens: []config.NamedToken{{
			Name:  sessionContextName,
			Token: s.Token,
		}},
	}
}

// runSession holds the active invocation's session override. Guarded by runMu
// like the rest of the per-invocation state; nil means normal CLI behavior
// (config file + context resolution).
var runSession *Session

// sessionScrubbedEnvVars are unset for the duration of a session-backed
// invocation, so host-level credentials and preferences cannot leak into a
// tenant's request. DTCTL_TOKEN/DT_API_TOKEN/DTCTL_ACCOUNT_TOKEN mirror
// credentialEnvVars (plugin_dispatch.go); DTCTL_CONFIG/DTCTL_CONTEXT would
// repoint config discovery, which a session fully replaces; DTCTL_PROFILE and
// DTCTL_OUTPUT would let the host's preferences shape the request's command
// surface and output format (per-request values are set explicitly via
// RunOptions.Env, which applies after this scrub).
//
// The rest are here because a request's *output bytes* must not depend on the
// host's environment: FORCE_COLOR would inject ANSI escapes into every
// response, NO_COLOR is scrubbed alongside it so colour resolves from the
// invocation alone, and DTCTL_SPILL/DTCTL_SPILL_DIR would re-enable spilling
// (or redirect it) behind the HostDiskSpill capability's back.
var sessionScrubbedEnvVars = []string{
	"DTCTL_TOKEN", "DT_API_TOKEN", "DTCTL_ACCOUNT_TOKEN",
	"DTCTL_CONFIG", "DTCTL_CONTEXT", "DTCTL_PROFILE", "DTCTL_OUTPUT",
	"FORCE_COLOR", "NO_COLOR", "DTCTL_SPILL", "DTCTL_SPILL_DIR",
}

// applyRunEnvironment installs the invocation's session and environment
// variables and returns a cleanup restoring the previous process state. The
// env mutations are process-wide, which is safe because invocations are
// serialized (runMu) — but a host application reading the same variables
// concurrently from other goroutines will observe them; hosts that care
// should not share the process with unrelated env consumers.
//
// Order matters: session scrubbing first, then opts.Env, so an embedding
// caller can explicitly re-grant a variable the scrub removed.
func applyRunEnvironment(opts RunOptions) (cleanup func(), err error) {
	if opts.Session != nil {
		if err := opts.Session.validate(); err != nil {
			return nil, err
		}
	}

	var saved []envSnapshot
	set := func(key, value string) {
		saved = append(saved, snapshotEnv(key))
		_ = os.Setenv(key, value)
	}
	unset := func(key string) {
		saved = append(saved, snapshotEnv(key))
		_ = os.Unsetenv(key)
	}

	if opts.Session != nil {
		for _, key := range sessionScrubbedEnvVars {
			unset(key)
		}
		// The synthetic config carries the token inline; keep resolution away
		// from the host OS keyring entirely (deterministic, and a service must
		// never read or populate host credential stores on a tenant's behalf).
		set(config.EnvDisableKeyring, "1")
	}
	for key, value := range opts.Env {
		set(key, value)
	}

	runSession = opts.Session
	prevFS := vfs.SetActive(opts.FS)
	return func() {
		vfs.SetActive(prevFS)
		runSession = nil
		// Restore in reverse so an opts.Env entry that overrode a scrub
		// unwinds through the scrub back to the original host value.
		for i := len(saved) - 1; i >= 0; i-- {
			saved[i].restore()
		}
	}, nil
}

type envSnapshot struct {
	key     string
	value   string
	present bool
}

func snapshotEnv(key string) envSnapshot {
	value, present := os.LookupEnv(key)
	return envSnapshot{key: key, value: value, present: present}
}

func (e envSnapshot) restore() {
	if e.present {
		_ = os.Setenv(e.key, e.value)
	} else {
		_ = os.Unsetenv(e.key)
	}
}
