//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// WorkflowFixture returns a minimal workflow JSON for integration testing
func WorkflowFixture(prefix string) []byte {
	workflow := map[string]interface{}{
		"title":       fmt.Sprintf("%s-workflow", prefix),
		"description": "Integration test workflow",
		"tasks": map[string]interface{}{
			"test_task": map[string]interface{}{
				"name":   "test_task",
				"action": "dynatrace.automations:run-javascript",
				"input": map[string]interface{}{
					"script": "export default async function() { return { result: 'Integration test success' }; }",
				},
			},
		},
	}

	data, _ := json.Marshal(workflow)
	return data
}

// WorkflowFixtureModified returns a modified version of the workflow for edit testing
func WorkflowFixtureModified(prefix string) []byte {
	workflow := map[string]interface{}{
		"title":       fmt.Sprintf("%s-workflow-modified", prefix),
		"description": "Modified integration test workflow",
		"tasks": map[string]interface{}{
			"test_task": map[string]interface{}{
				"name":   "test_task",
				"action": "dynatrace.automations:run-javascript",
				"input": map[string]interface{}{
					"script": "export default async function() { return { result: 'Modified workflow' }; }",
				},
			},
			"second_task": map[string]interface{}{
				"name":   "second_task",
				"action": "dynatrace.automations:run-javascript",
				"input": map[string]interface{}{
					"script": "export default async function() { return { result: 'Second task added' }; }",
				},
			},
		},
	}

	data, _ := json.Marshal(workflow)
	return data
}

// DashboardFixture returns a minimal dashboard JSON for integration testing
func DashboardFixture(prefix string) []byte {
	dashboard := map[string]interface{}{
		"dashboardMetadata": map[string]interface{}{
			"name":   fmt.Sprintf("%s-dashboard", prefix),
			"shared": false,
		},
		"tiles": []map[string]interface{}{
			{
				"name":       "test-tile",
				"tileType":   "DATA_EXPLORER",
				"configured": true,
				"bounds": map[string]interface{}{
					"top":    0,
					"left":   0,
					"width":  400,
					"height": 200,
				},
			},
		},
	}

	data, _ := json.Marshal(dashboard)
	return data
}

// DashboardFixtureModified returns a modified dashboard for edit testing
func DashboardFixtureModified(prefix string) []byte {
	dashboard := map[string]interface{}{
		"dashboardMetadata": map[string]interface{}{
			"name":        fmt.Sprintf("%s-dashboard-modified", prefix),
			"description": "Modified dashboard",
			"shared":      false,
		},
		"tiles": []map[string]interface{}{
			{
				"name":       "test-tile",
				"tileType":   "DATA_EXPLORER",
				"configured": true,
				"bounds": map[string]interface{}{
					"top":    0,
					"left":   0,
					"width":  400,
					"height": 200,
				},
			},
			{
				"name":       "second-tile",
				"tileType":   "MARKDOWN",
				"configured": true,
				"bounds": map[string]interface{}{
					"top":    0,
					"left":   400,
					"width":  400,
					"height": 200,
				},
				"markdown": "# Modified Dashboard",
			},
		},
	}

	data, _ := json.Marshal(dashboard)
	return data
}

// DashboardFixtureLarge returns a large dashboard to test truncation issues
// This creates a dashboard with many tiles to ensure the response is > 10KB
func DashboardFixtureLarge(prefix string) []byte {
	tiles := make([]map[string]interface{}, 0)

	// Create 50 tiles to make a reasonably large dashboard (>10KB)
	for i := 0; i < 50; i++ {
		tile := map[string]interface{}{
			"name":       fmt.Sprintf("test-tile-%d", i),
			"tileType":   "DATA_EXPLORER",
			"configured": true,
			"bounds": map[string]interface{}{
				"top":    (i / 5) * 200,
				"left":   (i % 5) * 400,
				"width":  400,
				"height": 200,
			},
			"queries": []map[string]interface{}{
				{
					"id":    fmt.Sprintf("query-%d", i),
					"query": fmt.Sprintf("fetch logs | filter status == \"ERROR\" | filter tile == %d | summarize count = count(), by: {status}", i),
				},
			},
			// Add some padding to make each tile larger
			"customName": fmt.Sprintf("Custom Tile %d - This is a longer description to add more data", i),
		}
		tiles = append(tiles, tile)
	}

	dashboard := map[string]interface{}{
		"dashboardMetadata": map[string]interface{}{
			"name":        fmt.Sprintf("%s-dashboard-large", prefix),
			"description": "Large dashboard for testing content retrieval and truncation issues. This dashboard contains many tiles with queries.",
			"shared":      false,
		},
		"tiles": tiles,
	}

	data, _ := json.Marshal(dashboard)
	return data
}

// NotebookFixture returns a minimal notebook JSON for integration testing
func NotebookFixture(prefix string) []byte {
	notebook := map[string]interface{}{
		"name": fmt.Sprintf("%s-notebook", prefix),
		"sections": []map[string]interface{}{
			{
				"type":     "markdown",
				"title":    "Test Section",
				"state":    "default",
				"markdown": "# Integration Test Notebook\n\nThis is a test notebook.",
			},
		},
	}

	data, _ := json.Marshal(notebook)
	return data
}

// NotebookFixtureModified returns a modified notebook for edit testing
func NotebookFixtureModified(prefix string) []byte {
	notebook := map[string]interface{}{
		"name": fmt.Sprintf("%s-notebook-modified", prefix),
		"sections": []map[string]interface{}{
			{
				"type":     "markdown",
				"title":    "Test Section",
				"state":    "default",
				"markdown": "# Modified Integration Test Notebook\n\nThis notebook has been modified.",
			},
			{
				"type":     "markdown",
				"title":    "Second Section",
				"state":    "default",
				"markdown": "## New Section\n\nAdded during edit test.",
			},
		},
	}

	data, _ := json.Marshal(notebook)
	return data
}

// BucketName returns a unique bucket name for testing
func BucketName(prefix string) string {
	return fmt.Sprintf("%s_bucket", prefix)
}

// BucketCreateRequest returns a bucket creation request
func BucketCreateRequest(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"bucketName":    BucketName(prefix),
		"table":         "logs",
		"displayName":   fmt.Sprintf("%s Integration Test Bucket", prefix),
		"retentionDays": 35,
	}
}

// BucketUpdateRequest returns a bucket update request
func BucketUpdateRequest(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"displayName":   fmt.Sprintf("%s Modified Test Bucket", prefix),
		"retentionDays": 60,
	}
}

// WorkflowExecutionParams returns sample workflow execution parameters
func WorkflowExecutionParams() map[string]interface{} {
	return map[string]interface{}{
		"testParam": "testValue",
	}
}

// ToYAML converts a map to YAML bytes
func ToYAML(data map[string]interface{}) []byte {
	bytes, _ := yaml.Marshal(data)
	return bytes
}

// ToJSON converts a map to JSON bytes
func ToJSON(data map[string]interface{}) []byte {
	bytes, _ := json.Marshal(data)
	return bytes
}

// SettingsObjectFixture returns a minimal settings object for integration testing
// Uses builtin:loadtest.5k.owner-based schema which is a simple owner-based schema
func SettingsObjectFixture(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"schemaId": "builtin:loadtest.5k.owner-based",
		"scope":    "environment",
		"value": map[string]interface{}{
			"text": fmt.Sprintf("%s-integration-test-settings", prefix),
		},
	}
}

// SettingsObjectFixtureModified returns a modified settings object for edit testing
func SettingsObjectFixtureModified(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"text": fmt.Sprintf("%s-integration-test-settings-modified", prefix),
	}
}

// SLOFixture returns a minimal SLO for integration testing
func SLOFixture(prefix string) []byte {
	slo := map[string]interface{}{
		"name":        fmt.Sprintf("%s-slo", prefix),
		"description": "Integration test SLO",
		"criteria": []map[string]interface{}{
			{
				"timeframeFrom": "-1h",
				"target":        95.0,
				"warning":       98.0,
			},
		},
		"customSli": map[string]interface{}{
			"indicator": "fetch logs | filter status == \"INFO\" | summarize count = count()",
			"type":      "QUERY_RATIO",
		},
	}

	data, _ := json.Marshal(slo)
	return data
}

// SLOFixtureModified returns a modified SLO for edit testing
func SLOFixtureModified(prefix string) []byte {
	slo := map[string]interface{}{
		"name":        fmt.Sprintf("%s-slo-modified", prefix),
		"description": "Modified integration test SLO",
		"criteria": []map[string]interface{}{
			{
				"timeframeFrom": "-1h",
				"target":        90.0,
				"warning":       95.0,
			},
		},
		"customSli": map[string]interface{}{
			"indicator": "fetch logs | filter status == \"INFO\" | summarize count = count()",
			"type":      "QUERY_RATIO",
		},
	}

	data, _ := json.Marshal(slo)
	return data
}

// EdgeConnectFixture returns an EdgeConnect configuration for integration testing
func EdgeConnectFixture(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("%s-edgeconnect", prefix),
		"hostPatterns": []string{
			fmt.Sprintf("*.%s.test.invalid", prefix), // Use .invalid TLD to ensure non-routable
		},
		"oauthClientId": fmt.Sprintf("%s-client-id", prefix),
	}
}

// EdgeConnectFixtureModified returns a modified EdgeConnect configuration for edit testing
func EdgeConnectFixtureModified(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"name": fmt.Sprintf("%s-edgeconnect-modified", prefix),
		"hostPatterns": []string{
			fmt.Sprintf("*.%s.test.invalid", prefix),
			fmt.Sprintf("*.%s.modified.test.invalid", prefix),
		},
		"oauthClientId": fmt.Sprintf("%s-client-id-modified", prefix),
	}
}
