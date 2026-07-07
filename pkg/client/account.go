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

// IAMBaseURLForEnvironment maps a tier to the IAM service URL (for access-info).
func IAMBaseURLForEnvironment(env auth.Environment) string {
	switch env {
	case auth.EnvironmentDev:
		return "https://iam-dev.dynatracelabs.com"
	case auth.EnvironmentHard:
		return "https://iam-hardening.dynatracelabs.com"
	default: // prod
		return "https://iam.dynatrace.com"
	}
}
