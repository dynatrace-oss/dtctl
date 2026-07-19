package inventory

import (
	"strings"
	"testing"
)

func TestBuiltinDefinitionsAreValid(t *testing.T) {
	for name, def := range BuiltinDefinitions() {
		if err := validateDef(def); err != nil {
			t.Errorf("builtin %q: %v", name, err)
		}
		if def.Probe != "" {
			t.Errorf("builtin %q is probe-shaped — builtins are structural-only by design", name)
		}
	}
}

func TestParseDefinitions(t *testing.T) {
	defs, err := ParseDefinitions([]byte(`
apiVersion: dtctl.dev/v1alpha1
kind: InventoryDefinitions
capabilities:
  postgres:
    entityTypes: [DB_INSTANCE_POSTGRES, "*_DBFORPOSTGRESQL_*"]
  genai:
    probe: 'fetch spans, from:now()-24h | filter isNotNull(gen_ai.system) | limit 1'
    window: 24h
  aws: null
`))
	if err != nil {
		t.Fatalf("ParseDefinitions() error: %v", err)
	}
	if len(defs.Capabilities) != 3 {
		t.Errorf("capabilities = %d, want 3", len(defs.Capabilities))
	}
	if defs.Capabilities["aws"] != nil {
		t.Errorf("null capability must load as nil (a removal marker)")
	}
}

func TestParseDefinitionsRejectsInvalid(t *testing.T) {
	cases := []struct {
		name, content, wantErr string
	}{
		{"wrong kind", "kind: RecipeBook\ncapabilities: {}", `expected "InventoryDefinitions"`},
		{"no shape", "capabilities:\n  broken: {}", "exactly one discovery shape"},
		{"two shapes", "capabilities:\n  broken: {dataObject: logs, metricKey: 'dt.*'}", "exactly one discovery shape"},
		{"probe without window", "capabilities:\n  broken: {probe: 'fetch logs | limit 1'}", "must declare window"},
		{"window without probe", "capabilities:\n  broken: {dataObject: logs, window: 24h}", "only valid with a probe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDefinitions([]byte(tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want mention of %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateDefinitions covers the Go-construction path: sets built as
// struct literals bypass ParseDefinitions, so ValidateDefinitions must catch
// what YAML validation would have — including the nil that only an overlay
// may carry.
func TestValidateDefinitions(t *testing.T) {
	valid := map[string]*CapabilityDef{
		"logs":  {DataObject: "logs"},
		"genai": {Probe: "fetch spans | limit 1", Window: "24h"},
	}
	if err := ValidateDefinitions(valid); err != nil {
		t.Errorf("valid set rejected: %v", err)
	}

	cases := []struct {
		name    string
		defs    map[string]*CapabilityDef
		wantErr string
	}{
		{"nil definition", map[string]*CapabilityDef{"aws": nil}, "null removes a capability only"},
		{"no shape", map[string]*CapabilityDef{"broken": {}}, "exactly one discovery shape"},
		{"two shapes", map[string]*CapabilityDef{"broken": {DataObject: "logs", MetricKey: "dt.*"}}, "exactly one discovery shape"},
		{"probe without window", map[string]*CapabilityDef{"broken": {Probe: "fetch logs | limit 1"}}, "must declare window"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDefinitions(tc.defs)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want mention of %q", err, tc.wantErr)
			}
		})
	}

	// Deterministic reporting: with several offenders, the alphabetically
	// first capability is named, so error output is stable across runs.
	err := ValidateDefinitions(map[string]*CapabilityDef{
		"zeta":  {},
		"alpha": {},
	})
	if err == nil || !strings.Contains(err.Error(), `"alpha"`) {
		t.Errorf("error = %v, want the alphabetically first offender %q", err, "alpha")
	}
}

func TestMergeDefinitions(t *testing.T) {
	base := map[string]*CapabilityDef{
		"aws":  {EntityTypes: []string{"AWS_*"}},
		"logs": {DataObject: "logs"},
	}
	merged := MergeDefinitions(base,
		&Definitions{Capabilities: map[string]*CapabilityDef{
			"aws":      nil, // removal
			"postgres": {EntityTypes: []string{"DB_INSTANCE_POSTGRES"}},
		}},
		&Definitions{Capabilities: map[string]*CapabilityDef{
			"logs": {DataObject: "logs.custom"}, // later overlay wins
		}},
	)
	if _, ok := merged["aws"]; ok {
		t.Errorf("aws should be removed by the null overlay")
	}
	if merged["postgres"] == nil {
		t.Errorf("postgres should be added")
	}
	if merged["logs"].DataObject != "logs.custom" {
		t.Errorf("logs = %+v, want the later overlay to win", merged["logs"])
	}
	if base["logs"].DataObject != "logs" {
		t.Errorf("merge must not mutate the base set")
	}
}
