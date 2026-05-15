package urls

import (
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		wantProblems  int
		wantMessage   string
		wantSuggested string
	}{
		{
			name:         "valid apps URL",
			url:          "https://abc12345.apps.dynatrace.com",
			wantProblems: 0,
		},
		{
			name:         "empty URL",
			url:          "",
			wantProblems: 0,
		},
		{
			name:          "live domain",
			url:           "https://abc12345.live.dynatrace.com",
			wantProblems:  1,
			wantMessage:   "live.dynatrace.com",
			wantSuggested: "https://abc12345.apps.dynatrace.com",
		},
		{
			name:          "bare production domain",
			url:           "https://abc12345.dynatrace.com",
			wantProblems:  1,
			wantMessage:   "bare",
			wantSuggested: "https://abc12345.apps.dynatrace.com",
		},
		{
			name:          "dev without apps",
			url:           "https://abc12345.dev.dynatracelabs.com",
			wantProblems:  1,
			wantSuggested: "https://abc12345.dev.apps.dynatracelabs.com",
		},
		{
			name:         "dev with apps is fine",
			url:          "https://abc12345.dev.apps.dynatracelabs.com",
			wantProblems: 0,
		},
		{
			name:          "sprint without apps",
			url:           "https://abc12345.sprint.dynatracelabs.com",
			wantProblems:  1,
			wantSuggested: "https://abc12345.sprint.apps.dynatracelabs.com",
		},
		{
			name:         "managed /e/ URL",
			url:          "https://myhost.example.com/e/abc12345",
			wantProblems: 1,
			wantMessage:  "Managed",
		},
		{
			name:         "managed /e/ URL with apps is fine",
			url:          "https://abc12345.apps.dynatrace.com/e/something",
			wantProblems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := Check(tt.url)
			if len(problems) != tt.wantProblems {
				t.Errorf("Check(%q) returned %d problems, want %d: %+v", tt.url, len(problems), tt.wantProblems, problems)
				return
			}
			if tt.wantProblems > 0 {
				p := problems[0]
				if tt.wantMessage != "" && !contains(p.Message, tt.wantMessage) {
					t.Errorf("problem message %q does not contain %q", p.Message, tt.wantMessage)
				}
				if tt.wantSuggested != "" && p.SuggestedURL != tt.wantSuggested {
					t.Errorf("suggested URL = %q, want %q", p.SuggestedURL, tt.wantSuggested)
				}
			}
		})
	}
}

func TestSuggestions(t *testing.T) {
	s := Suggestions("https://abc12345.live.dynatrace.com")
	if len(s) == 0 {
		t.Error("expected suggestions for live URL, got none")
	}

	s = Suggestions("https://abc12345.apps.dynatrace.com")
	if len(s) != 0 {
		t.Errorf("expected no suggestions for valid URL, got %v", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
