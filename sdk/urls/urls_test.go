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
			name:          "missing scheme - apps domain",
			url:           "abc12345.apps.dynatrace.com",
			wantProblems:  1,
			wantMessage:   "missing the https:// scheme",
			wantSuggested: "https://abc12345.apps.dynatrace.com",
		},
		{
			name:          "missing scheme - dev domain",
			url:           "abc12345.dev.apps.dynatracelabs.com",
			wantProblems:  1,
			wantMessage:   "missing the https:// scheme",
			wantSuggested: "https://abc12345.dev.apps.dynatracelabs.com",
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

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "empty stays empty",
			url:  "",
			want: "",
		},
		{
			name: "bare host gets https",
			url:  "abc12345.apps.dynatrace.com",
			want: "https://abc12345.apps.dynatrace.com",
		},
		{
			name: "host with path gets https",
			url:  "abc12345.apps.dynatrace.com/platform",
			want: "https://abc12345.apps.dynatrace.com/platform",
		},
		{
			name: "https untouched",
			url:  "https://abc12345.apps.dynatrace.com",
			want: "https://abc12345.apps.dynatrace.com",
		},
		{
			name: "http untouched",
			url:  "http://127.0.0.1:8080",
			want: "http://127.0.0.1:8080",
		},
		{
			name: "mixed-case scheme untouched",
			url:  "HTTPS://abc12345.apps.dynatrace.com",
			want: "HTTPS://abc12345.apps.dynatrace.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.url); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestIsDynatraceEnvironmentURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "valid prod", url: "https://abc12345.apps.dynatrace.com"},
		{name: "valid dev", url: "https://abc12345.dev.apps.dynatracelabs.com"},
		{name: "valid sprint", url: "https://abc12345.sprint.apps.dynatracelabs.com"},
		{
			name:    "http rejected",
			url:     "http://abc12345.apps.dynatrace.com",
			wantErr: "https",
		},
		{
			name:    "foreign host rejected",
			url:     "https://evil.example.com",
			wantErr: "dynatrace.com",
		},
		{
			name:    "dynatrace.com lookalike rejected",
			url:     "https://evil-dynatrace.com",
			wantErr: "dynatrace.com",
		},
		{
			name:    "dynatrace.com suffix in path rejected",
			url:     "https://evil.example.com/?x=apps.dynatrace.com",
			wantErr: "dynatrace.com",
		},
		{
			name:    "bare dynatrace.com rejected (no subdomain)",
			url:     "https://dynatrace.com",
			wantErr: "dynatrace.com",
		},
		{
			name:    "no scheme rejected",
			url:     "abc12345.apps.dynatrace.com",
			wantErr: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsDynatraceEnvironmentURL(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("IsDynatraceEnvironmentURL(%q) unexpected error: %v", tt.url, err)
				}
			} else {
				if err == nil {
					t.Errorf("IsDynatraceEnvironmentURL(%q) = nil, want error containing %q", tt.url, tt.wantErr)
				} else if !containsStr(err.Error(), tt.wantErr) {
					t.Errorf("IsDynatraceEnvironmentURL(%q) error %q does not contain %q", tt.url, err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestHost(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "valid URL", url: "https://abc12345.apps.dynatrace.com", want: "abc12345.apps.dynatrace.com"},
		{name: "uppercase scheme", url: "HTTPS://Abc.apps.dynatrace.com", want: "abc.apps.dynatrace.com"},
		{name: "with path", url: "https://abc.apps.dynatrace.com/platform", want: "abc.apps.dynatrace.com"},
		{name: "with port", url: "https://abc.apps.dynatrace.com:8443", want: "abc.apps.dynatrace.com"},
		{name: "empty", url: "", want: ""},
		{name: "unparseable", url: "://bad", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Host(tt.url); got != tt.want {
				t.Errorf("Host(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
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

func TestIsDynatraceEnvironmentOrigin(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "bare origin", url: "https://abc12345.apps.dynatrace.com"},
		{name: "trailing slash is bare", url: "https://abc12345.apps.dynatrace.com/"},
		{name: "explicit port", url: "https://abc12345.apps.dynatrace.com:443"},
		{
			name:    "path rejected",
			url:     "https://abc12345.apps.dynatrace.com/platform",
			wantErr: "no path",
		},
		{
			// The residual this check exists for: an expanded ${VAR} riding
			// along in the path of an otherwise allowlisted host.
			name:    "secret smuggled in path rejected",
			url:     "https://abc12345.apps.dynatrace.com/AKIAIOSFODNN7EXAMPLE",
			wantErr: "no path",
		},
		{
			name:    "query rejected",
			url:     "https://abc12345.apps.dynatrace.com?x=1",
			wantErr: "query",
		},
		{
			name:    "fragment rejected",
			url:     "https://abc12345.apps.dynatrace.com#x",
			wantErr: "fragment",
		},
		{
			name:    "credentials rejected",
			url:     "https://user:pass@abc12345.apps.dynatrace.com",
			wantErr: "credentials",
		},
		{
			name:    "foreign host still rejected",
			url:     "https://evil.example.com",
			wantErr: "dynatrace.com",
		},
		{
			name:    "http still rejected",
			url:     "http://abc12345.apps.dynatrace.com",
			wantErr: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsDynatraceEnvironmentOrigin(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("IsDynatraceEnvironmentOrigin(%q) unexpected error: %v", tt.url, err)
				}
				return
			}
			if err == nil {
				t.Errorf("IsDynatraceEnvironmentOrigin(%q) = nil, want error containing %q", tt.url, tt.wantErr)
			} else if !containsStr(err.Error(), tt.wantErr) {
				t.Errorf("IsDynatraceEnvironmentOrigin(%q) error %q does not contain %q", tt.url, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestIsDynatraceEnvironmentOrigin_RedactsCredentials guards against the
// validator echoing a password from the rejected URL into an error string that
// callers print and log.
func TestIsDynatraceEnvironmentOrigin_RedactsCredentials(t *testing.T) {
	err := IsDynatraceEnvironmentOrigin("https://user:sup3rs3cret@abc12345.apps.dynatrace.com")
	if err == nil {
		t.Fatal("IsDynatraceEnvironmentOrigin() = nil, want error")
	}
	if containsStr(err.Error(), "sup3rs3cret") {
		t.Errorf("error leaks the password: %q", err.Error())
	}
}
