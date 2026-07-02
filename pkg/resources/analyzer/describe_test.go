package analyzer

import "testing"

func TestFlattenSchema_RequiredFirstThenAlpha(t *testing.T) {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"logQuery"},
		"properties": map[string]interface{}{
			"maxPatterns": map[string]interface{}{"type": "integer", "description": "max"},
			"logQuery":    map[string]interface{}{"type": "string", "description": "the query"},
			"timeframe":   map[string]interface{}{"type": "string"},
		},
	}

	fields, ok := FlattenSchema(schema)
	if !ok {
		t.Fatal("expected schema to be introspectable")
	}
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	// Required field sorts first.
	if fields[0].Name != "logQuery" || !fields[0].Required {
		t.Errorf("expected logQuery required first, got %+v", fields[0])
	}
	if fields[0].Type != "string" || fields[0].Description != "the query" {
		t.Errorf("unexpected type/description for logQuery: %+v", fields[0])
	}
	// Optional fields follow, alphabetically.
	if fields[1].Name != "maxPatterns" || fields[1].Required {
		t.Errorf("expected maxPatterns optional second, got %+v", fields[1])
	}
	if fields[2].Name != "timeframe" {
		t.Errorf("expected timeframe third, got %+v", fields[2])
	}
}

func TestFlattenSchema_CompositeProperty(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	fields, ok := FlattenSchema(schema)
	if !ok {
		t.Fatal("expected schema to be introspectable")
	}
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if !fields[0].Composite {
		t.Errorf("expected composite field, got %+v", fields[0])
	}
	if fields[0].Type != "composite" {
		t.Errorf("expected type 'composite', got %q", fields[0].Type)
	}
}

func TestFlattenSchema_UnionType(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"value": map[string]interface{}{"type": []interface{}{"string", "null"}},
		},
	}
	fields, ok := FlattenSchema(schema)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected 1 introspectable field, got ok=%v len=%d", ok, len(fields))
	}
	if fields[0].Type != "string|null" {
		t.Errorf("expected union type 'string|null', got %q", fields[0].Type)
	}
}

func TestFlattenSchema_NotIntrospectable(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"nil":              nil,
		"no properties":    {"type": "object"},
		"empty properties": {"type": "object", "properties": map[string]interface{}{}},
	}
	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			fields, ok := FlattenSchema(schema)
			if ok {
				t.Errorf("expected not introspectable")
			}
			if fields != nil {
				t.Errorf("expected nil fields, got %+v", fields)
			}
		})
	}
}
