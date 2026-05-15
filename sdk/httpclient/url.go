package httpclient

import (
	"fmt"
	"net/url"
	"strings"
)

// ExtractSubdomain extracts the first subdomain (typically the environment or
// org ID) from a Dynatrace environment URL. For example, given
// "https://abc12345.apps.dynatrace.com" it returns "abc12345".
//
// The URL must include a scheme (e.g. "https://"). Bare hostnames like
// "abc12345.apps.dynatrace.com" are not supported and will return an error.
func ExtractSubdomain(environmentURL string) (string, error) {
	trimmed := strings.TrimSpace(environmentURL)

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid environment URL %q: %w", environmentURL, err)
	}

	hostname := u.Hostname()
	if hostname == "" {
		// Common mistake: URL without scheme gets parsed as a path, not a host.
		return "", fmt.Errorf("environment URL has no host (missing https:// scheme?): %q", environmentURL)
	}

	parts := strings.Split(hostname, ".")
	if strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("failed to extract subdomain from host %q", hostname)
	}

	return parts[0], nil
}
