package inventory

import (
	"os"
	"path/filepath"
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

func writeDefs(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "defs.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefinitionsFile(t *testing.T) {
	path := writeDefs(t, `
apiVersion: dtctl.dev/v1alpha1
kind: InventoryDefinitions
capabilities:
  postgres:
    entityTypes: [DB_INSTANCE_POSTGRES, "*_DBFORPOSTGRESQL_*"]
  genai:
    probe: 'fetch spans, from:now()-24h | filter isNotNull(gen_ai.system) | limit 1'
    window: 24h
  aws: null
`)
	defs, err := LoadDefinitionsFile(path)
	if err != nil {
		t.Fatalf("LoadDefinitionsFile() error: %v", err)
	}
	if len(defs.Capabilities) != 3 {
		t.Errorf("capabilities = %d, want 3", len(defs.Capabilities))
	}
	if defs.Capabilities["aws"] != nil {
		t.Errorf("null capability must load as nil (a removal marker)")
	}
}

func TestLoadDefinitionsFileRejectsInvalid(t *testing.T) {
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
			_, err := LoadDefinitionsFile(writeDefs(t, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want mention of %q", err, tc.wantErr)
			}
		})
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
