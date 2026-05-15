package httpclient

import (
	"fmt"
	"net/url"
	"strings"
)

// ExtractSubdomain extracts the first subdomain (typically the environment or
// org ID) from a Dynatrace environment URL. For example, given
// "https://abc12345.apps.dynatrace.com" it returns "abc12345".
func ExtractSubdomain(environmentURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(environmentURL))
	if err != nil {
		return "", fmt.Errorf("invalid environment URL %q: %w", environmentURL, err)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("environment URL has no host: %q", environmentURL)
	}

	parts := strings.Split(hostname, ".")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("failed to extract subdomain from host %q", hostname)
	}

	return parts[0], nil
}
