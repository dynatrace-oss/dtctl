package client

import "github.com/dynatrace-oss/dtctl/pkg/auth"

// AccountBaseURLForEnvironment maps a tier to the account API base URL.
func AccountBaseURLForEnvironment(env auth.Environment) string {
	switch env {
	case auth.EnvironmentDev:
		return "https://api-dev.internal.dynatracelabs.com"
	case auth.EnvironmentHard:
		return "https://api-hardening.internal.dynatracelabs.com"
	default: // prod
		return "https://api.dynatrace.com"
	}
}

// IAMBaseURLForEnvironment maps a tier to the base URL hosting the IAM
// access-info endpoint (/iam/v1/access-info). This endpoint lives on the
// Account Management API host, not on a separate iam.* host — so it returns
// the same hosts as AccountBaseURLForEnvironment.
func IAMBaseURLForEnvironment(env auth.Environment) string {
	return AccountBaseURLForEnvironment(env)
}
