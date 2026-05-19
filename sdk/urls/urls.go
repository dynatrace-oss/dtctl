package urls

import (
	"net/url"
	"strings"
)

// Problem describes a detected issue with a Dynatrace environment URL
// and optionally suggests a correction.
type Problem struct {
	// Message describes the problem in human-readable form.
	Message string
	// SuggestedURL is the corrected URL, if one can be derived.
	// Empty if no correction is possible.
	SuggestedURL string
}

// Check inspects a Dynatrace environment URL for common mistakes.
// It returns a list of problems found (empty if the URL looks correct).
func Check(environmentURL string) []Problem {
	if environmentURL == "" {
		return nil
	}

	var problems []Problem

	lower := strings.ToLower(environmentURL)

	// SaaS production: <tenant>.live.dynatrace.com instead of <tenant>.apps.dynatrace.com
	if strings.Contains(lower, ".live.dynatrace.com") {
		suggested := fixDomain(environmentURL, ".live.dynatrace.com", ".apps.dynatrace.com")
		problems = append(problems, Problem{
			Message:      "environment URL uses 'live.dynatrace.com' which is the classic API domain; the platform API is at 'apps.dynatrace.com'",
			SuggestedURL: suggested,
		})
	}

	// SaaS production: <tenant>.dynatrace.com (bare domain without .apps.)
	if !strings.Contains(lower, ".apps.dynatrace.com") &&
		!strings.Contains(lower, ".live.dynatrace.com") &&
		matchesBareProductionDomain(lower) {
		suggested := fixDomain(environmentURL, ".dynatrace.com", ".apps.dynatrace.com")
		problems = append(problems, Problem{
			Message:      "environment URL uses the bare 'dynatrace.com' domain; the platform API is at 'apps.dynatrace.com'",
			SuggestedURL: suggested,
		})
	}

	// DEV: <tenant>.dev.dynatracelabs.com instead of <tenant>.dev.apps.dynatracelabs.com
	if strings.Contains(lower, ".dev.dynatracelabs.com") &&
		!strings.Contains(lower, ".dev.apps.dynatracelabs.com") {
		suggested := fixDomain(environmentURL, ".dev.dynatracelabs.com", ".dev.apps.dynatracelabs.com")
		problems = append(problems, Problem{
			Message:      "environment URL uses 'dev.dynatracelabs.com' without '.apps.' in the domain; the platform API is at 'dev.apps.dynatracelabs.com'",
			SuggestedURL: suggested,
		})
	}

	// SPRINT/HARD: <tenant>.sprint.dynatracelabs.com instead of <tenant>.sprint.apps.dynatracelabs.com
	if strings.Contains(lower, ".sprint.dynatracelabs.com") &&
		!strings.Contains(lower, ".sprint.apps.dynatracelabs.com") {
		suggested := fixDomain(environmentURL, ".sprint.dynatracelabs.com", ".sprint.apps.dynatracelabs.com")
		problems = append(problems, Problem{
			Message:      "environment URL uses 'sprint.dynatracelabs.com' without '.apps.' in the domain; the platform API is at 'sprint.apps.dynatracelabs.com'",
			SuggestedURL: suggested,
		})
	}

	// Managed/ActiveGate: <host>/e/<envid> pattern (classic managed URL)
	if strings.Contains(lower, "/e/") && !strings.Contains(lower, "apps.") {
		problems = append(problems, Problem{
			Message: "environment URL looks like a Dynatrace Managed or ActiveGate URL (/e/<envid> path); the Dynatrace Platform (SaaS) 'apps' URL is required",
		})
	}

	return problems
}

// Suggestions returns human-readable troubleshooting strings for URL problems.
// Returns nil if no problems are detected.
func Suggestions(environmentURL string) []string {
	problems := Check(environmentURL)
	if len(problems) == 0 {
		return nil
	}

	var suggestions []string
	for _, p := range problems {
		suggestions = append(suggestions, "Possible wrong environment URL: "+p.Message)
		if p.SuggestedURL != "" {
			suggestions = append(suggestions, "Did you mean "+p.SuggestedURL+"?")
		}
	}
	return suggestions
}

// matchesBareProductionDomain checks if a lowercased URL ends with
// "<something>.dynatrace.com" without any known subdomain prefix.
func matchesBareProductionDomain(lower string) bool {
	u, err := url.Parse(lower)
	if err != nil {
		return false
	}

	host := u.Hostname()
	if host == "" {
		host = lower
		if idx := strings.Index(host, "/"); idx >= 0 {
			host = host[:idx]
		}
	}

	if !strings.HasSuffix(host, ".dynatrace.com") {
		return false
	}

	prefix := strings.TrimSuffix(host, ".dynatrace.com")
	return !strings.Contains(prefix, ".")
}

// fixDomain replaces oldSuffix with newSuffix in the URL, case-insensitively.
func fixDomain(rawURL, oldSuffix, newSuffix string) string {
	lower := strings.ToLower(rawURL)
	idx := strings.Index(lower, strings.ToLower(oldSuffix))
	if idx < 0 {
		return rawURL
	}
	return rawURL[:idx] + newSuffix + rawURL[idx+len(oldSuffix):]
}
