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
		"tasks": []map[string]interface{}{
			{
				"name":   "test-task",
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
		"tasks": []map[string]interface{}{
			{
				"name":   "test-task",
				"action": "dynatrace.automations:run-javascript",
				"input": map[string]interface{}{
					"script": "export default async function() { return { result: 'Modified workflow' }; }",
				},
			},
			{
				"name":   "second-task",
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
				"name":      "test-tile",
				"tileType":  "DATA_EXPLORER",
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

// NotebookFixture returns a minimal notebook JSON for integration testing
func NotebookFixture(prefix string) []byte {
	notebook := map[string]interface{}{
		"name": fmt.Sprintf("%s-notebook", prefix),
		"sections": []map[string]interface{}{
			{
				"type":  "markdown",
				"title": "Test Section",
				"state":  "default",
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
				"type":  "markdown",
				"title": "Second Section",
				"state": "default",
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
		"bucketName":   BucketName(prefix),
		"table":        "logs",
		"displayName":  fmt.Sprintf("%s Integration Test Bucket", prefix),
		"retentionDays": 35,
	}
}

// BucketUpdateRequest returns a bucket update request
func BucketUpdateRequest(prefix string) map[string]interface{} {
	return map[string]interface{}{
		"displayName":  fmt.Sprintf("%s Modified Test Bucket", prefix),
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
