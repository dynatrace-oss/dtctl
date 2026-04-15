package cmd

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
)

func TestStripSchemaFluff_RemovesTopLevelKeys(t *testing.T) {
	input := map[string]interface{}{
		"type":          "object",
		"displayName":   "My Extension",
		"documentation": "Some docs",
		"customMessage": "A message",
		"description":   "kept",
	}

	result := extension.StripSchemaFluff(input).(map[string]interface{})

	for _, removed := range []string{"displayName", "documentation", "customMessage"} {
		if _, ok := result[removed]; ok {
			t.Errorf("expected key %q to be removed but it was not", removed)
		}
	}
	if result["type"] != "object" {
		t.Errorf("expected type to be kept, got %v", result["type"])
	}
	if result["description"] != "kept" {
		t.Errorf("expected description to be kept, got %v", result["description"])
	}
}

func TestStripSchemaFluff_RecursiveProperties(t *testing.T) {
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host": map[string]interface{}{
				"type":          "string",
				"displayName":   "Hostname",
				"documentation": "The host.",
				"description":   "kept",
			},
		},
	}

	result := extension.StripSchemaFluff(input).(map[string]interface{})
	props := result["properties"].(map[string]interface{})
	host := props["host"].(map[string]interface{})

	if _, ok := host["displayName"]; ok {
		t.Error("expected displayName to be removed from nested property")
	}
	if _, ok := host["documentation"]; ok {
		t.Error("expected documentation to be removed from nested property")
	}
	if host["type"] != "string" {
		t.Errorf("expected type to be kept in nested property, got %v", host["type"])
	}
	if host["description"] != "kept" {
		t.Errorf("expected description to be kept in nested property, got %v", host["description"])
	}
}

func TestStripSchemaFluff_RecursiveArray(t *testing.T) {
	input := map[string]interface{}{
		"type": "array",
		"items": []interface{}{
			map[string]interface{}{
				"type":        "string",
				"displayName": "Item label",
				"description": "kept",
			},
		},
	}

	result := extension.StripSchemaFluff(input).(map[string]interface{})
	items := result["items"].([]interface{})
	item := items[0].(map[string]interface{})

	if _, ok := item["displayName"]; ok {
		t.Error("expected displayName to be removed from array item")
	}
	if item["description"] != "kept" {
		t.Errorf("expected description to be kept in array item, got %v", item["description"])
	}
}

func TestStripSchemaFluff_LeavesPrimitivesUntouched(t *testing.T) {
	cases := []interface{}{
		"a string",
		42,
		true,
		nil,
	}
	for _, c := range cases {
		result := extension.StripSchemaFluff(c)
		if !reflect.DeepEqual(result, c) {
			t.Errorf("expected primitive %v to be unchanged, got %v", c, result)
		}
	}
}

func TestStripSchemaFluff_EmptyObject(t *testing.T) {
	input := map[string]interface{}{}
	result := extension.StripSchemaFluff(input).(map[string]interface{})
	if len(result) != 0 {
		t.Errorf("expected empty map to remain empty, got %v", result)
	}
}

func TestStripSchemaFluff_NoFluffKeys(t *testing.T) {
	input := map[string]interface{}{
		"type":        "object",
		"description": "no fluff here",
		"properties":  map[string]interface{}{},
	}
	// Deep-copy via JSON round-trip to compare
	before, _ := json.Marshal(input)
	result := extension.StripSchemaFluff(input)
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Errorf("expected no change when no fluff keys present\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestFluffKeys_ContainsExpectedKeys(t *testing.T) {
	expected := []string{"documentation", "customMessage", "displayName"}
	for _, k := range expected {
		if !extension.FluffKeys[k] {
			t.Errorf("expected FluffKeys to contain %q", k)
		}
	}
	if len(extension.FluffKeys) != len(expected) {
		t.Errorf("expected FluffKeys to have exactly %d entries, got %d", len(expected), len(extension.FluffKeys))
	}
}
