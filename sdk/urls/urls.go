package urls

import (
	"fmt"
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

// Normalize returns the environment URL with an "https://" scheme prepended when
// no scheme is present. A URL that already carries an http:// or https:// scheme,
// or is empty, is returned unchanged. Without a scheme Go's HTTP client rejects
// requests with "unsupported protocol scheme", so a context stored from a bare
// host (e.g. "abc12345.apps.dynatrace.com") would authenticate yet fail on every
// query. Normalize does not correct the domain-level mistakes reported by Check.
func Normalize(environmentURL string) string {
	if environmentURL == "" {
		return environmentURL
	}
	lower := strings.ToLower(environmentURL)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return "https://" + environmentURL
	}
	return environmentURL
}

// Check inspects a Dynatrace environment URL for common mistakes.
// It returns a list of problems found (empty if the URL looks correct).
func Check(environmentURL string) []Problem {
	if environmentURL == "" {
		return nil
	}

	var problems []Problem

	lower := strings.ToLower(environmentURL)

	// Missing scheme: URL without https:// causes Go's HTTP client to return
	// "unsupported protocol scheme" at runtime. Detect early and suggest the fix.
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		problems = append(problems, Problem{
			Message:      "environment URL is missing the https:// scheme",
			SuggestedURL: "https://" + environmentURL,
		})
		return problems
	}

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

// Host returns the lowercase hostname of an environment URL, or "" if the URL
// cannot be parsed. Used to compare canonical origins across config entries.
func Host(environmentURL string) string {
	u, err := url.Parse(environmentURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// IsDynatraceEnvironmentURL returns an error if environmentURL is not a valid
// Dynatrace environment URL. A valid URL must use https and have a hostname
// that is a subdomain of dynatrace.com or dynatracelabs.com. This is enforced
// on auto-discovered local configs to prevent credential exfiltration to
// attacker-controlled hosts.
func IsDynatraceEnvironmentURL(environmentURL string) error {
	u, err := url.Parse(environmentURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if strings.ToLower(u.Scheme) != "https" {
		return fmt.Errorf("URL must use https (got %q)", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, ".dynatrace.com") && !strings.HasSuffix(host, ".dynatracelabs.com") {
		return fmt.Errorf("URL must target a dynatrace.com or dynatracelabs.com host (got %q)", host)
	}
	return nil
}

// IsDynatraceEnvironmentOrigin returns an error if environmentURL is not a
// bare Dynatrace environment origin: a valid https Dynatrace host (see
// IsDynatraceEnvironmentURL) carrying nothing else — no userinfo, path, query
// or fragment.
//
// A Dynatrace SaaS environment URL is always a bare origin; dtctl appends the
// API path itself. Requiring that shape on auto-discovered local configs is
// what keeps ${VAR} expansion safe there: expansion can only fill in the
// destination, never smuggle an unrelated host secret out in a path or query
// (e.g. "https://<bound-tenant>.apps.dynatrace.com/$AWS_SECRET_ACCESS_KEY").
func IsDynatraceEnvironmentOrigin(environmentURL string) error {
	if err := IsDynatraceEnvironmentURL(environmentURL); err != nil {
		return err
	}
	u, err := url.Parse(environmentURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.User != nil {
		return fmt.Errorf("URL must not embed credentials (got %q)", u.Redacted())
	}
	if u.RawQuery != "" {
		return fmt.Errorf("URL must not carry a query string (got %q)", u.RawQuery)
	}
	if u.Fragment != "" {
		return fmt.Errorf("URL must not carry a fragment (got %q)", u.Fragment)
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		return fmt.Errorf("URL must be a bare environment origin with no path (got %q)", u.Path)
	}
	return nil
}
