package document

import "testing"

func TestUIURL(t *testing.T) {
	const base = "https://env.example.invalid"
	tests := []struct {
		name    string
		docType string
		id      string
		want    string
	}{
		{"dashboard", "dashboard", "dash-1", base + "/ui/apps/dynatrace.dashboards/dashboard/dash-1"},
		{"notebook", "notebook", "nb-1", base + "/ui/apps/dynatrace.notebooks/notebook/nb-1"},
		// Launchpads are served by the launcher app; "dynatrace.launchpads" 404s.
		{"launchpad", "launchpad", "team-launchpad", base + "/ui/apps/dynatrace.launcher/launchpad/team-launchpad"},
		{"id is escaped as one path segment", "launchpad", "team/a?b#c:d e", base + "/ui/apps/dynatrace.launcher/launchpad/team%2Fa%3Fb%23c%3Ad%20e"},
		{"custom type has no known viewer", "acme:config", "cfg-1", ""},
		{"no id", "dashboard", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UIURL(base, tt.docType, tt.id); got != tt.want {
				t.Errorf("UIURL(%q, %q) = %q, want %q", tt.docType, tt.id, got, tt.want)
			}
		})
	}
}
