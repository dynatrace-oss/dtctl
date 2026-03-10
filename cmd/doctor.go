package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// checkResult represents the outcome of a single doctor check
type checkResult struct {
	Name   string
	Status string // "ok", "warn", "fail"
	Detail string
}

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
  4. Token is retrievable (keyring or config)
  5. Environment URL is reachable (HTTP connectivity)
  6. API authentication works (user identity)`,
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

	// 4. Token retrieval
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

	// 5. Environment connectivity
	httpClient := &http.Client{Timeout: 10 * time.Second}
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
		} else {
			resp.Body.Close()
			results = append(results, checkResult{
				Name:   "Connectivity",
				Status: "ok",
				Detail: fmt.Sprintf("%s is reachable", ctx.Environment),
			})
		}
	}

	// 6. API authentication
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
		// Try fallback to user ID extraction from JWT
		userID, jwtErr := c.CurrentUserID()
		if jwtErr != nil {
			results = append(results, checkResult{
				Name:   "Authentication",
				Status: "fail",
				Detail: fmt.Sprintf("API call failed: %s", authErr.Error()),
			})
		} else {
			results = append(results, checkResult{
				Name:   "Authentication",
				Status: "warn",
				Detail: fmt.Sprintf("metadata API unavailable, but token is valid (user: %s)", userID),
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
			icon = "[OK]  "
		case "warn":
			icon = "[WARN]"
		case "fail":
			icon = "[FAIL]"
		}
		fmt.Printf("%s %-16s %s\n", icon, r.Name, r.Detail)
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
