package session

import (
	"testing"
)

func TestParseStabilityLevel(t *testing.T) {
	for _, c := range []struct {
		in      string
		want    StabilityLevel
		wantErr bool
	}{
		{"", "", false}, // unset: the caller substitutes its own default
		{"stable", StabilityStable, false},
		{" EXPERIMENTAL ", StabilityExperimental, false},
		{"development", StabilityDevelopment, false},
		{"beta", "", true},
		{"ga", "", true},
	} {
		got, err := ParseStabilityLevel(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseStabilityLevel(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("ParseStabilityLevel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveMinStabilityPrecedence(t *testing.T) {
	cfg := NewConfig()
	cfg.SetContextWithOptions("prod", "https://env.example.com", "tok", &ContextOptions{
		MinStability: StabilityStable,
	})
	cfg.CurrentContext = "prod"

	// Context binding wins over the default.
	got, err := cfg.resolveMinStability("")
	if err != nil || got != StabilityStable {
		t.Errorf("context floor = %q (%v), want stable", got, err)
	}

	// The environment wins over the context, so a CI job can tighten the floor
	// without editing a shared config.
	got, err = cfg.resolveMinStability("experimental")
	if err != nil || got != StabilityExperimental {
		t.Errorf("env floor = %q (%v), want experimental", got, err)
	}

	// An invalid value is an explicit error rather than a silent fallback:
	// for a floor, falling back would *widen* the accepted surface.
	if _, err := cfg.resolveMinStability("beta"); err == nil {
		t.Error("an invalid floor was accepted")
	}
}

func TestResolveMinStabilityDefaultsToExperimental(t *testing.T) {
	cfg := NewConfig()
	cfg.SetContext("prod", "https://env.example.com", "tok")
	cfg.CurrentContext = "prod"

	// Experimental rather than stable, so humans and interactive agents keep
	// getting new (badged) surface — the whole point of having a middle tier.
	// Only the platform service and CI pipelines pin stable.
	got, err := cfg.resolveMinStability("")
	if err != nil {
		t.Fatalf("resolveMinStability: %v", err)
	}
	if got != StabilityExperimental {
		t.Errorf("default floor = %q, want experimental", got)
	}
	if DefaultMinStability != StabilityExperimental {
		t.Errorf("DefaultMinStability = %q, want experimental", DefaultMinStability)
	}
}

func TestEnabledDevelopmentFeatures(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDevelopmentFeature("account", true)
	cfg.SetDevelopmentFeature("declined", false)

	got := cfg.enabledDevelopmentFeatures("")
	if !got["account"] {
		t.Error("a config opt-in was not honored")
	}
	if got["declined"] {
		t.Error("an explicit off was treated as on")
	}

	// The environment merges with config, per key.
	got = cfg.enabledDevelopmentFeatures("serve, development.other ")
	if !got["account"] || !got["serve"] || !got["other"] {
		t.Errorf("merged set = %v, want account, serve and other", got)
	}

	// An explicit off in the environment withdraws a config opt-in, so a
	// single process can drop one feature without editing the file.
	got = cfg.enabledDevelopmentFeatures("account=off")
	if got["account"] {
		t.Error("an environment off-value did not withdraw the config opt-in")
	}
}

func TestSetDevelopmentFeatureRecordsAnExplicitOff(t *testing.T) {
	cfg := NewConfig()
	cfg.SetDevelopmentFeature("development.account", false)

	// Recorded rather than deleted, so `config view` shows what was considered
	// and then declined. The dotted config form normalizes to the bare key.
	on, ok := cfg.Development["account"]
	if !ok {
		t.Fatal("turning a feature off deleted the key instead of recording it")
	}
	if on {
		t.Error("the feature was recorded as on")
	}
}

func TestStabilityExceptionsComeFromTheContext(t *testing.T) {
	cfg := NewConfig()
	cfg.SetContextWithOptions("agent", "https://env.example.com", "tok", &ContextOptions{
		MinStability:        StabilityStable,
		StabilityExceptions: []string{"ingest", "query --spill"},
	})
	cfg.CurrentContext = "agent"

	// On the context, not in a profile: a profile that named a tier would be
	// encoding another axis's intent, and because profiles are reusable topical
	// sets, one deployment's accepted risk would propagate to every context
	// that binds the profile.
	got := cfg.StabilityExceptions()
	if len(got) != 2 || got[0] != "ingest" || got[1] != "query --spill" {
		t.Errorf("StabilityExceptions() = %v", got)
	}
}
