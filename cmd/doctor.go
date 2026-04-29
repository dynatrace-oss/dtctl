package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// checkResult represents the outcome of a single doctor check
type checkResult struct {
	Name   string
	Status string // "ok", "warn", "fail"
	Detail string
}

// checkKeyringFunc is the function used to probe keyring availability.
// It defaults to config.CheckKeyring and can be overridden in tests.
var checkKeyringFunc = config.CheckKeyring

// doctorCmd runs health checks on the dtctl configuration and connectivity
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration, connectivity, and authentication health",
	Long: `Run a series of diagnostic checks to verify that dtctl is properly configured
and can communicate with the Dynatrace environment.

Checks performed:
  1. dtctl version
  2. Configuration file exists and is valid
  3. Current context is set
  4. Environment URL pattern is valid (detects common domain mistakes)
  5. Keyring status (secure token storage)
  6. Token is retrievable (keyring or config)
  7. Environment URL is reachable (HTTP connectivity)
  8. API authentication works (user identity)`,
	Example: `  # Run all checks
  dtctl doctor

  # Run checks for a specific context
  dtctl doctor --context production`,
	RunE: func(cmd *cobra.Command, args []string) error {
		results := runDoctorChecks()
		printDoctorResults(results)

		// Return error if any check failed
		for _, r := range results {
			if r.Status == "fail" {
				return fmt.Errorf("one or more checks failed")
			}
		}
		return nil
	},
}

func runDoctorChecks() []checkResult {
	return runDoctorChecksWithClient(&http.Client{Timeout: 10 * time.Second})
}

func runDoctorChecksWithClient(httpClient *http.Client) []checkResult {
	var results []checkResult

	// 1. Version
	results = append(results, checkResult{
		Name:   "dtctl version",
		Status: "ok",
		Detail: fmt.Sprintf("%s (commit: %s, built: %s)", version.Version, version.Commit, version.Date),
	})

	// 2. Configuration file
	cfg, cfgErr := LoadConfig()
	if cfgErr != nil {
		results = append(results, checkResult{
			Name:   "Configuration",
			Status: "fail",
			Detail: cfgErr.Error(),
		})
		return results // Cannot continue without config
	}

	configPath := config.FindLocalConfig()
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}
	results = append(results, checkResult{
		Name:   "Configuration",
		Status: "ok",
		Detail: configPath,
	})

	// 3. Current context
	if cfg.CurrentContext == "" {
		results = append(results, checkResult{
			Name:   "Current context",
			Status: "fail",
			Detail: "no current context set (use 'dtctl ctx set <name>' or 'dtctl config set-context')",
		})
		return results
	}

	ctx, ctxErr := cfg.CurrentContextObj()
	if ctxErr != nil {
		results = append(results, checkResult{
			Name:   "Current context",
			Status: "fail",
			Detail: ctxErr.Error(),
		})
		return results
	}

	safetyLevel := ctx.GetEffectiveSafetyLevel().String()
	results = append(results, checkResult{
		Name:   "Current context",
		Status: "ok",
		Detail: fmt.Sprintf("%s (environment: %s, safety: %s)", cfg.CurrentContext, ctx.Environment, safetyLevel),
	})

	// 4. Environment URL validation
	if urlProblems := diagnostic.CheckEnvironmentURL(ctx.Environment); len(urlProblems) > 0 {
		for _, p := range urlProblems {
			detail := p.Message
			if p.SuggestedURL != "" {
				detail += fmt.Sprintf(". Suggested fix: dtctl ctx set %s --environment %s", cfg.CurrentContext, p.SuggestedURL)
			}
			results = append(results, checkResult{
				Name:   "Environment URL",
				Status: "warn",
				Detail: detail,
			})
		}
	} else {
		results = append(results, checkResult{
			Name:   "Environment URL",
			Status: "ok",
			Detail: "URL pattern looks correct",
		})
	}

	// 5. Keyring status
	if keyringErr := checkKeyringFunc(); keyringErr != nil {
		if config.IsFileTokenStorage() {
			results = append(results, checkResult{
				Name:   "Token storage",
				Status: "ok",
				Detail: fmt.Sprintf("file-based (%s=file); keyring unavailable: %v", config.EnvTokenStorage, keyringErr),
			})
		} else {
			detail := fmt.Sprintf("%s: %v", config.KeyringBackend(), keyringErr)
			if strings.Contains(keyringErr.Error(), config.ErrMsgCollectionUnlock) {
				detail += " (run 'dtctl auth login' to create the collection automatically)"
			}
			detail += fmt.Sprintf(" (set %s=file to use file-based token storage)", config.EnvTokenStorage)
			results = append(results, checkResult{
				Name:   "Token storage",
				Status: "warn",
				Detail: detail,
			})
		}
	} else {
		results = append(results, checkResult{
			Name:   "Token storage",
			Status: "ok",
			Detail: config.KeyringBackend(),
		})
	}

	// 6. Token retrieval
	token, tokenErr := client.GetTokenWithOAuthSupport(cfg, ctx.TokenRef)
	if tokenErr != nil || token == "" {
		detail := "token not found"
		if tokenErr != nil {
			detail = tokenErr.Error()
		}
		results = append(results, checkResult{
			Name:   "Token",
			Status: "fail",
			Detail: fmt.Sprintf("cannot retrieve token %q: %s", ctx.TokenRef, detail),
		})
		return results
	}

	tokenSource := "config file"
	if config.IsKeyringAvailable() {
		tokenSource = fmt.Sprintf("keyring (%s)", config.KeyringBackend())
	} else if config.IsFileTokenStorage() {
		tokenSource = fmt.Sprintf("file store (%s)", config.OAuthStorageBackend())
	}
	// Mask token for display
	maskedToken := token
	if len(token) > 8 {
		maskedToken = token[:4] + "..." + token[len(token)-4:]
	}
	results = append(results, checkResult{
		Name:   "Token",
		Status: "ok",
		Detail: fmt.Sprintf("retrieved from %s (%s)", tokenSource, maskedToken),
	})

	// OAuth session state (only applies when the stored token is OAuth)
	session, sessionErr := buildSessionStatusFunc(cfg.CurrentContext, ctx, ctx.TokenRef)
	if sessionErr != nil {
		results = append(results, checkResult{
			Name:   "OAuth session",
			Status: "fail",
			Detail: fmt.Sprintf("could not read session state: %s", sessionErr.Error()),
		})
	} else if session.IsOAuth {
		results = append(results, oauthSessionCheckResult(session))
	}

	// 6. Environment connectivity
	req, reqErr := http.NewRequest(http.MethodHead, ctx.Environment, nil)
	if reqErr != nil {
		results = append(results, checkResult{
			Name:   "Connectivity",
			Status: "fail",
			Detail: fmt.Sprintf("invalid environment URL %s: %s", ctx.Environment, reqErr.Error()),
		})
	} else {
		resp, connErr := httpClient.Do(req)
		if connErr != nil {
			results = append(results, checkResult{
				Name:   "Connectivity",
				Status: "fail",
				Detail: fmt.Sprintf("cannot reach %s: %s", ctx.Environment, connErr.Error()),
			})
			// No point testing authentication if the server is unreachable
			return results
		} else {
			resp.Body.Close()
			results = append(results, checkResult{
				Name:   "Connectivity",
				Status: "ok",
				Detail: fmt.Sprintf("%s is reachable", ctx.Environment),
			})
		}
	}

	// 7. API authentication
	c, clientErr := NewClientFromConfig(cfg)
	if clientErr != nil {
		results = append(results, checkResult{
			Name:   "Authentication",
			Status: "fail",
			Detail: clientErr.Error(),
		})
		return results
	}

	userInfo, authErr := c.CurrentUser()
	if authErr != nil {
		// Platform tokens cannot be granted iam:users:read, so the metadata API
		// returns 403 by design — surface this as a known limitation, not a
		// failure. For other (OAuth/JWT) tokens, fall back to decoding the user
		// ID directly out of the token; CurrentUserID would re-issue the same
		// metadata call we already failed, so go straight to the JWT path.
		if client.IsPlatformToken(token) {
			results = append(results, checkResult{
				Name:   "Authentication",
				Status: "warn",
				Detail: "platform token: user identity unavailable via metadata API (requires iam:users:read scope, not grantable to platform tokens)",
			})
		} else if userID, jwtErr := client.ExtractUserIDFromToken(token); jwtErr == nil {
			results = append(results, checkResult{
				Name:   "Authentication",
				Status: "warn",
				Detail: fmt.Sprintf("metadata API unavailable, but token is valid (user: %s)", userID),
			})
		} else {
			results = append(results, checkResult{
				Name:   "Authentication",
				Status: "fail",
				Detail: fmt.Sprintf("API call failed: %s", authErr.Error()),
			})
		}
	} else {
		detail := fmt.Sprintf("authenticated as %s", userInfo.UserID)
		if userInfo.EmailAddress != "" {
			detail = fmt.Sprintf("authenticated as %s (%s)", userInfo.UserID, userInfo.EmailAddress)
		}
		results = append(results, checkResult{
			Name:   "Authentication",
			Status: "ok",
			Detail: detail,
		})
	}

	return results
}

func printDoctorResults(results []checkResult) {
	for _, r := range results {
		var icon string
		switch r.Status {
		case "ok":
			icon = output.DoctorOK()
		case "warn":
			icon = output.DoctorWarn()
		case "fail":
			icon = output.DoctorFail()
		}
		fmt.Printf("%s %-16s %s\n", icon, r.Name, r.Detail)
	}
}

func oauthSessionCheckResult(session *SessionStatus) checkResult {
	if !session.RefreshTokenPresent {
		return checkResult{
			Name:   "OAuth session",
			Status: "warn",
			Detail: "no refresh token; run 'dtctl auth login' to enable automatic refresh",
		}
	}

	accessExpired := session.AccessTokenExpiresAt != nil && time.Now().After(*session.AccessTokenExpiresAt)
	if accessExpired {
		return checkResult{
			Name:   "OAuth session",
			Status: "warn",
			Detail: "access token expired; will refresh on next API call",
		}
	}

	var parts []string
	if session.AccessTokenExpiresAt != nil {
		parts = append(parts, fmt.Sprintf("access expires in %s", time.Until(*session.AccessTokenExpiresAt).Round(time.Second)))
	}
	parts = append(parts, "refresh token present")

	return checkResult{
		Name:   "OAuth session",
		Status: "ok",
		Detail: strings.Join(parts, "; "),
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
