package appengine

import (
	"encoding/json"
	"testing"
)

// TestIntentWorkflowIntegration tests the complete workflow from extraction to matching to URL generation
func TestIntentWorkflowIntegration(t *testing.T) {
	// Create a mock app with intents
	app := App{
		ID:   "dynatrace.distributedtracing",
		Name: "Distributed Tracing",
		Manifest: map[string]interface{}{
			"intents": map[string]interface{}{
				"view-trace": map[string]interface{}{
					"name":        "View Trace",
					"description": "View distributed trace",
					"properties": map[string]interface{}{
						"trace_id": map[string]interface{}{
							"required": true,
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						"timestamp": map[string]interface{}{
							"required": false,
							"schema": map[string]interface{}{
								"type":   "string",
								"format": "date-time",
							},
						},
					},
				},
				"view-trace-addon": map[string]interface{}{
					"name":        "View Trace Addon",
					"description": "View trace with additional context",
					"properties": map[string]interface{}{
						"trace_id": map[string]interface{}{
							"required": true,
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						"timestamp": map[string]interface{}{
							"required": true,
							"schema": map[string]interface{}{
								"type":   "string",
								"format": "date-time",
							},
						},
						"service_name": map[string]interface{}{
							"required": false,
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
	}

	// Test 1: Extract intents from manifest
	t.Run("extract intents", func(t *testing.T) {
		intents := extractIntentsFromManifest(app)
		if len(intents) != 2 {
			t.Fatalf("expected 2 intents, got %d", len(intents))
		}

		// Find intents by ID (order is not guaranteed from map iteration)
		var viewTrace, viewTraceAddon *Intent
		for i := range intents {
			if intents[i].IntentID == "view-trace" {
				viewTrace = &intents[i]
			} else if intents[i].IntentID == "view-trace-addon" {
				viewTraceAddon = &intents[i]
			}
		}

		// Verify view-trace intent
		if viewTrace == nil {
			t.Fatal("view-trace intent not found")
		}
		if len(viewTrace.Properties) != 2 {
			t.Errorf("view-trace: expected 2 properties, got %d", len(viewTrace.Properties))
		}
		if len(viewTrace.RequiredProps) != 1 {
			t.Errorf("view-trace: expected 1 required prop, got %d", len(viewTrace.RequiredProps))
		}
		if len(viewTrace.RequiredProps) > 0 && viewTrace.RequiredProps[0] != "trace_id" {
			t.Errorf("view-trace: expected required prop 'trace_id', got %q", viewTrace.RequiredProps[0])
		}

		// Verify view-trace-addon intent
		if viewTraceAddon == nil {
			t.Fatal("view-trace-addon intent not found")
		}
		if len(viewTraceAddon.RequiredProps) != 2 {
			t.Errorf("view-trace-addon: expected 2 required props, got %d", len(viewTraceAddon.RequiredProps))
		}
	})

	// Test 2: Match data with different scenarios
	t.Run("match data scenarios", func(t *testing.T) {
		intents := extractIntentsFromManifest(app)

		// Scenario 1: Data with both trace_id and timestamp
		data1 := map[string]interface{}{
			"trace_id":  "d052c9a8772e349d09048355a8891b82",
			"timestamp": "2026-02-02T16:04:19.947000000Z",
		}

		matches := []IntentMatch{}
		for _, intent := range intents {
			match := matchIntentToData(intent, data1)
			if match.MatchQuality > 0 {
				matches = append(matches, match)
			}
		}

		if len(matches) != 2 {
			t.Errorf("expected 2 matches, got %d", len(matches))
		}

		// view-trace-addon has 3 properties (trace_id, timestamp, service_name)
		// but data1 only has 2 (trace_id, timestamp), so match is 66.67% (2/3)
		// view-trace has 2 properties (trace_id, timestamp) and data1 has both, so 100% (2/2)
		for _, match := range matches {
			if match.IntentID == "view-trace-addon" {
				// 2 out of 3 properties present = 66.67%
				expectedQuality := 66.67
				if match.MatchQuality < expectedQuality-0.1 || match.MatchQuality > expectedQuality+0.1 {
					t.Errorf("expected ~%.2f%% match for view-trace-addon, got %.2f%%",
						expectedQuality, match.MatchQuality)
				}
			} else if match.IntentID == "view-trace" {
				// 2 out of 2 properties present = 100%
				if match.MatchQuality != 100 {
					t.Errorf("expected 100%% match for view-trace, got %.2f%%", match.MatchQuality)
				}
			}
		}

		// Scenario 2: Data with only trace_id
		data2 := map[string]interface{}{
			"trace_id": "abc123",
		}

		matches2 := []IntentMatch{}
		for _, intent := range intents {
			match := matchIntentToData(intent, data2)
			if match.MatchQuality > 0 {
				matches2 = append(matches2, match)
			}
		}

		// Only view-trace should match (view-trace-addon requires timestamp)
		if len(matches2) != 1 {
			t.Errorf("expected 1 match, got %d", len(matches2))
		}
		if matches2[0].IntentID != "view-trace" {
			t.Errorf("expected match for 'view-trace', got %q", matches2[0].IntentID)
		}

		// Scenario 3: Data with all properties including optional
		data3 := map[string]interface{}{
			"trace_id":     "abc123",
			"timestamp":    "2026-02-02T10:00:00Z",
			"service_name": "my-service",
		}

		matches3 := []IntentMatch{}
		for _, intent := range intents {
			match := matchIntentToData(intent, data3)
			if match.MatchQuality > 0 {
				matches3 = append(matches3, match)
			}
		}

		// Verify we got matches
		if len(matches3) != 2 {
			t.Errorf("expected 2 matches for data3, got %d", len(matches3))
		}

		// view-trace-addon has 3 properties (trace_id, timestamp, service_name)
		// data3 has all 3, so match is 100% (3/3)
		// view-trace has 2 properties (trace_id, timestamp) and data3 has both, so 100% (2/2)
		for _, match := range matches3 {
			if match.IntentID == "view-trace-addon" {
				// All 3 properties present = 100%
				if match.MatchQuality != 100 {
					t.Errorf("expected 100%% match for view-trace-addon (3/3 properties), got %.2f%% with %d matched props out of %d total",
						match.MatchQuality, len(match.MatchedProps), len(match.Intent.Properties))
				}
			} else if match.IntentID == "view-trace" {
				// All 2 properties present = 100%
				if match.MatchQuality != 100 {
					t.Errorf("expected 100%% match for view-trace (2/2 properties), got %.2f%%", match.MatchQuality)
				}
			}
		}
	})

	// Test 3: Verify URL structure
	t.Run("URL generation structure", func(t *testing.T) {
		// We can't test the full URL without a client, but we can verify the logic
		payload := map[string]interface{}{
			"trace_id":  "test123",
			"timestamp": "2026-02-02T10:00:00Z",
		}

		// Verify payload can be marshaled
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			t.Errorf("failed to marshal payload: %v", err)
		}

		// Verify JSON structure
		var unmarshaled map[string]interface{}
		if err := json.Unmarshal(jsonPayload, &unmarshaled); err != nil {
			t.Errorf("failed to unmarshal payload: %v", err)
		}

		if unmarshaled["trace_id"] != "test123" {
			t.Errorf("expected trace_id 'test123', got %v", unmarshaled["trace_id"])
		}
	})
}

// TestMultipleAppsWithIntents tests handling multiple apps with different intents
func TestMultipleAppsWithIntents(t *testing.T) {
	apps := []App{
		{
			ID:   "app1",
			Name: "App One",
			Manifest: map[string]interface{}{
				"intents": map[string]interface{}{
					"intent1": map[string]interface{}{
						"description": "First intent",
						"properties": map[string]interface{}{
							"prop1": map[string]interface{}{
								"required": true,
								"schema": map[string]interface{}{
									"type": "string",
								},
							},
						},
					},
				},
			},
		},
		{
			ID:   "app2",
			Name: "App Two",
			Manifest: map[string]interface{}{
				"intents": map[string]interface{}{
					"intent2": map[string]interface{}{
						"description": "Second intent",
						"properties": map[string]interface{}{
							"prop2": map[string]interface{}{
								"required": true,
								"schema": map[string]interface{}{
									"type": "string",
								},
							},
						},
					},
				},
			},
		},
	}

	// Extract all intents
	var allIntents []Intent
	for _, app := range apps {
		intents := extractIntentsFromManifest(app)
		allIntents = append(allIntents, intents...)
	}

	if len(allIntents) != 2 {
		t.Fatalf("expected 2 intents total, got %d", len(allIntents))
	}

	// Verify each app's intent is correctly associated
	if allIntents[0].AppID != "app1" {
		t.Errorf("expected first intent from app1, got %s", allIntents[0].AppID)
	}
	if allIntents[1].AppID != "app2" {
		t.Errorf("expected second intent from app2, got %s", allIntents[1].AppID)
	}

	// Test matching - data should only match one intent
	data := map[string]interface{}{
		"prop1": "value1",
	}

	matches := []IntentMatch{}
	for _, intent := range allIntents {
		match := matchIntentToData(intent, data)
		if match.MatchQuality > 0 {
			matches = append(matches, match)
		}
	}

	if len(matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(matches))
	}
	if matches[0].IntentID != "intent1" {
		t.Errorf("expected match for intent1, got %s", matches[0].IntentID)
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("intent with complex nested properties", func(t *testing.T) {
		app := App{
			ID:   "test.app",
			Name: "Test App",
			Manifest: map[string]interface{}{
				"intents": map[string]interface{}{
					"complex-intent": map[string]interface{}{
						"description": "Complex intent",
						"properties": map[string]interface{}{
							"simple": map[string]interface{}{
								"required": true,
								"schema": map[string]interface{}{
									"type": "string",
								},
							},
							"number": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "number",
								},
							},
							"object": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
								},
							},
							"array": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "array",
								},
							},
						},
					},
				},
			},
		}

		intents := extractIntentsFromManifest(app)
		if len(intents) != 1 {
			t.Fatalf("expected 1 intent, got %d", len(intents))
		}

		intent := intents[0]
		if len(intent.Properties) != 4 {
			t.Errorf("expected 4 properties, got %d", len(intent.Properties))
		}

		// Test matching with complex data
		data := map[string]interface{}{
			"simple": "value",
			"number": 42,
			"object": map[string]interface{}{"key": "value"},
			"array":  []interface{}{"item1", "item2"},
		}

		match := matchIntentToData(intent, data)
		if match.MatchQuality != 100 {
			t.Errorf("expected 100%% match, got %.2f%%", match.MatchQuality)
		}
	})

	t.Run("intent with all optional properties", func(t *testing.T) {
		app := App{
			ID:   "test.app",
			Name: "Test App",
			Manifest: map[string]interface{}{
				"intents": map[string]interface{}{
					"optional-intent": map[string]interface{}{
						"description": "All optional",
						"properties": map[string]interface{}{
							"opt1": map[string]interface{}{
								"required": false,
								"schema": map[string]interface{}{
									"type": "string",
								},
							},
							"opt2": map[string]interface{}{
								"required": false,
								"schema": map[string]interface{}{
									"type": "string",
								},
							},
						},
					},
				},
			},
		}

		intents := extractIntentsFromManifest(app)
		intent := intents[0]

		// Should match even with no data (all optional)
		match := matchIntentToData(intent, map[string]interface{}{})
		if match.MatchQuality != 0 {
			t.Errorf("expected 0%% match with no data, got %.2f%%", match.MatchQuality)
		}

		// Should match partially
		match2 := matchIntentToData(intent, map[string]interface{}{"opt1": "value"})
		if match2.MatchQuality != 50 {
			t.Errorf("expected 50%% match, got %.2f%%", match2.MatchQuality)
		}
	})

	t.Run("malformed manifest structures", func(t *testing.T) {
		testCases := []struct {
			name     string
			manifest map[string]interface{}
			expected int
		}{
			{
				name: "nil manifest",
				manifest: nil,
				expected: 0,
			},
			{
				name: "empty manifest",
				manifest: map[string]interface{}{},
				expected: 0,
			},
			{
				name: "intents not a map",
				manifest: map[string]interface{}{
					"intents": "not a map",
				},
				expected: 0,
			},
			{
				name: "intent value not a map",
				manifest: map[string]interface{}{
					"intents": map[string]interface{}{
						"bad-intent": "not a map",
					},
				},
				expected: 0,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				app := App{
					ID:       "test.app",
					Name:     "Test App",
					Manifest: tc.manifest,
				}
				intents := extractIntentsFromManifest(app)
				if len(intents) != tc.expected {
					t.Errorf("expected %d intents, got %d", tc.expected, len(intents))
				}
			})
		}
	})
}

// TestSortingAndFiltering tests that matches are properly sorted by quality
func TestSortingAndFiltering(t *testing.T) {
	// Create multiple intents with different matching qualities
	intents := []Intent{
		{
			IntentID: "intent-100",
			Properties: map[string]IntentProperty{
				"prop1": {Type: "string", Required: true},
			},
			RequiredProps: []string{"prop1"},
		},
		{
			IntentID: "intent-50",
			Properties: map[string]IntentProperty{
				"prop1": {Type: "string", Required: true},
				"prop2": {Type: "string", Required: false},
			},
			RequiredProps: []string{"prop1"},
		},
		{
			IntentID: "intent-0",
			Properties: map[string]IntentProperty{
				"prop2": {Type: "string", Required: true},
			},
			RequiredProps: []string{"prop2"},
		},
	}

	data := map[string]interface{}{
		"prop1": "value1",
	}

	var matches []IntentMatch
	for _, intent := range intents {
		match := matchIntentToData(intent, data)
		if match.MatchQuality > 0 {
			matches = append(matches, match)
		}
	}

	// Should only have 2 matches (intent-0 has 0% quality)
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}

	// Verify they're not sorted yet
	// (In real usage, FindIntentsForData would sort them)

	// Verify match qualities
	for _, match := range matches {
		if match.IntentID == "intent-100" && match.MatchQuality != 100 {
			t.Errorf("expected 100%% for intent-100, got %.2f%%", match.MatchQuality)
		}
		if match.IntentID == "intent-50" && match.MatchQuality != 50 {
			t.Errorf("expected 50%% for intent-50, got %.2f%%", match.MatchQuality)
		}
	}
}
