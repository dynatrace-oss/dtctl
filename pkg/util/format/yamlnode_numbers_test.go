package format

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLNodeFromJSONKeepsNumberTokens(t *testing.T) {
	type doc struct {
		Big   json.RawMessage `json:"big"`
		Ratio float64         `json:"ratio"`
		Count int             `json:"count"`
		Label string          `json:"label"`
		Nest  map[string]any  `json:"nest"`
	}
	in := doc{
		Big:   json.RawMessage("9007199254740993"),
		Ratio: 0.25,
		Count: 3,
		Label: "123", // a numeric-looking string must stay a string
		Nest:  map[string]any{"list": []any{json.Number("18446744073709551615")}},
	}

	node, err := YAMLNodeFromJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	back, err := YAMLToJSON(out)
	if err != nil {
		t.Fatalf("YAMLToJSON: %v\n%s", err, out)
	}

	for _, want := range []string{`"big":9007199254740993`, `"ratio":0.25`, `"count":3`, `"label":"123"`} {
		if !strings.Contains(string(back), want) {
			t.Errorf("round trip lost %s:\nyaml:\n%s\njson: %s", want, out, back)
		}
	}
	if !strings.Contains(string(out), "18446744073709551615") {
		t.Errorf("yaml output rounded an integer beyond int64:\n%s", out)
	}
}
