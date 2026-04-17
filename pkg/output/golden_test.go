package output

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/resources/anomalydetector"
	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azureconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/azuremonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/edgeconnect"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/gcpmonitoringconfig"
	"github.com/dynatrace-oss/dtctl/pkg/resources/hub"
	"github.com/dynatrace-oss/dtctl/pkg/resources/iam"
	"github.com/dynatrace-oss/dtctl/pkg/resources/segment"
	"github.com/dynatrace-oss/dtctl/pkg/resources/settings"
	"github.com/dynatrace-oss/dtctl/pkg/resources/slo"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
)

// -update flag: regenerate golden files
// Run: go test ./pkg/output/ -update
var updateGolden = flag.Bool("update", false, "update golden files")

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name+".golden")

	if *updateGolden {
		dir := filepath.Dir(goldenPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found: %s\nRun with -update to create:\n  go test ./pkg/output/ -update\n\nActual output:\n%s", goldenPath, actual)
	}

	if string(expected) != actual {
		t.Errorf("output does not match golden file %s\n\n--- expected ---\n%s\n--- actual ---\n%s\n--- diff hint ---\nRun with -update to accept the new output:\n  go test ./pkg/output/ -update",
			goldenPath, string(expected), actual)
	}
}

// ---------------------------------------------------------------------------
// Test fixtures using REAL production structs with realistic synthetic data.
//
// All data is fictional — no real customer names, environment IDs, or tokens.
// IDs use realistic formats (UUIDs, base64-encoded settings object IDs).
// ---------------------------------------------------------------------------

var fixedTime = time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)

func workflowFixtures() []workflow.Workflow {
	return []workflow.Workflow{
		{
			ID:          "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
			Title:       "Deploy to Production",
			Owner:       "7a8b9c0d-1e2f-4a3b-8c4d-5e6f7a8b9c0d",
			OwnerType:   "USER",
			Description: "Deploys latest build to prod environment",
			Private:     false,
			IsDeployed:  true,
			Tasks: map[string]interface{}{
				"deploy": map[string]interface{}{
					"action": "dynatrace.automations:run-javascript",
					"input":  map[string]interface{}{"script": "// deploy logic"},
				},
			},
			Trigger: map[string]interface{}{
				"schedule": map[string]interface{}{
					"trigger": map[string]interface{}{"type": "cron", "cron": "0 9 * * 1-5"},
				},
			},
		},
		{
			ID:          "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e",
			Title:       "Daily Cleanup",
			Owner:       "00000000-0000-0000-0000-000000000000",
			OwnerType:   "USER",
			Description: "Removes stale resources older than 30 days",
			Private:     false,
			IsDeployed:  true,
		},
		{
			ID:          "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f",
			Title:       "Incident Response",
			Owner:       "8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e",
			OwnerType:   "USER",
			Description: "",
			Private:     true,
			IsDeployed:  false,
		},
	}
}

func sloFixtures() []slo.SLO {
	return []slo.SLO{
		{
			ID:          "a1b2c3d4-0001-4000-8000-000000000001",
			Name:        "API Availability",
			Description: "99.9% availability for public API endpoints",
			Version:     "3",
			Criteria: []slo.Criteria{
				{TimeframeFrom: "-7d", Target: 99.9, Warning: float64Ptr(99.5)},
			},
			Tags: []string{"service:api", "tier:1"},
		},
		{
			ID:          "a1b2c3d4-0002-4000-8000-000000000002",
			Name:        "Checkout Latency",
			Description: "P95 response time under 500ms for checkout flow",
			Version:     "1",
			Criteria: []slo.Criteria{
				{TimeframeFrom: "-30d", Target: 95.0},
			},
			Tags: []string{"service:checkout"},
		},
		{
			ID:          "a1b2c3d4-0003-4000-8000-000000000003",
			Name:        "Error Rate",
			Description: "Error rate below 0.1% across all services",
			Version:     "5",
		},
	}
}

func float64Ptr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64       { return &v }

func bucketFixtures() []bucket.Bucket {
	return []bucket.Bucket{
		{
			BucketName:    "default_logs",
			Table:         "logs",
			DisplayName:   "Default Logs",
			Status:        "active",
			RetentionDays: 35,
			Updatable:     true,
			Version:       1,
			Records:       int64Ptr(1250000),
		},
		{
			BucketName:             "custom_metrics",
			Table:                  "metrics",
			DisplayName:            "Custom Metrics",
			Status:                 "active",
			RetentionDays:          90,
			IncludedQueryLimitDays: 30,
			MetricInterval:         "PT1M",
			Updatable:              true,
			Version:                3,
			Records:                int64Ptr(8750000),
		},
		{
			BucketName:    "security_events",
			Table:         "logs",
			DisplayName:   "Security Events",
			Status:        "active",
			RetentionDays: 365,
			Updatable:     false,
			Version:       2,
			Records:       int64Ptr(42000),
		},
	}
}

func documentFixtures() []document.Document {
	return []document.Document{
		{
			ID:          "b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d5e",
			Name:        "Production Overview",
			Type:        "dashboard",
			Owner:       "7a8b9c0d-1e2f-4a3b-8c4d-5e6f7a8b9c0d",
			IsPrivate:   false,
			Created:     fixedTime,
			Description: "Main production monitoring dashboard",
			Version:     3,
			Modified:    fixedTime.Add(2 * time.Hour),
		},
		{
			ID:        "c2d3e4f5-a6b7-4c8d-9e0f-1a2b3c4d5e6f",
			Name:      "Runbook: Incident Response",
			Type:      "notebook",
			Owner:     "8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e",
			IsPrivate: true,
			Created:   fixedTime.Add(-24 * time.Hour),
			Version:   1,
			Modified:  fixedTime.Add(-12 * time.Hour),
		},
		{
			ID:          "d3e4f5a6-b7c8-4d9e-0f1a-2b3c4d5e6f7a",
			Name:        "Performance Dashboard",
			Type:        "dashboard",
			Owner:       "9c0d1e2f-3a4b-5c6d-7e8f-9a0b1c2d3e4f",
			IsPrivate:   false,
			Created:     fixedTime.Add(-72 * time.Hour),
			Description: "Service performance metrics and SLOs",
			Version:     7,
			Modified:    fixedTime.Add(-1 * time.Hour),
		},
	}
}

func settingsFixtures() []settings.SettingsObject {
	return []settings.SettingsObject{
		{
			ObjectID:      "vu9U3hXa3q0AAAABABhidWlsdGluOmFsZXJ0aW5nLnByb2ZpbGUABnRlbmFudAAGdGVuYW50ACRhMWIyYzNkNC1lNWY2LTRhN2ItOGM5ZC0wZTFmMmEzYjRjNWQ",
			SchemaID:      "builtin:alerting.profile",
			SchemaVersion: "1.0.5",
			Scope:         "environment",
			Summary:       "Default Alerting Profile",
			Value: map[string]any{
				"name":            "Default",
				"severityRules":   []any{},
				"eventTypeFilter": []any{},
			},
			ObjectIDShort: "vu9U3hXa3q0AAAABAB...",
			UID:           "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
			ScopeType:     "tenant",
			ScopeID:       "tenant",
		},
		{
			ObjectID:      "vu9U3hXa3q0AAAABABxidWlsdGluOnByb2JsZW0ubm90aWZpY2F0aW9ucwAGdGVuYW50AAZ0ZW5hbnQAJGIyYzNkNGU1LWY2YTctNGI4Yy05ZDBlLTFmMmEzYjRjNWQ2ZQ",
			SchemaID:      "builtin:problem.notifications",
			SchemaVersion: "2.1.0",
			Scope:         "environment",
			Summary:       "Email Notification",
			Value: map[string]any{
				"enabled":    true,
				"type":       "EMAIL",
				"recipients": "oncall-team@example.invalid",
			},
			ObjectIDShort: "vu9U3hXa3q0AAAABAB...",
			UID:           "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e",
			ScopeType:     "tenant",
			ScopeID:       "tenant",
		},
		{
			ObjectID:      "vu9U3hXa3q0AAAABABlidWlsdGluOnRhZ3MuYXV0by10YWdnaW5nAAZ0ZW5hbnQABnRlbmFudAAkYzNkNGU1ZjYtYTdiOC00YzlkLTBlMWYtMmEzYjRjNWQ2ZTdm",
			SchemaID:      "builtin:tags.auto-tagging",
			SchemaVersion: "3.0.2",
			Scope:         "environment",
			Summary:       "Environment Tag Rule",
			Value: map[string]any{
				"name":  "environment",
				"rules": []any{},
			},
			ObjectIDShort: "vu9U3hXa3q0AAAABAB...",
			UID:           "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f",
			ScopeType:     "tenant",
			ScopeID:       "tenant",
		},
	}
}

func executionFixtures() []workflow.Execution {
	return []workflow.Execution{
		{
			ID:          "d4e5f6a7-b8c9-4d0e-1f2a-3b4c5d6e7f8a",
			Workflow:    "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
			Title:       "Deploy to Production",
			State:       "SUCCEEDED",
			StartedAt:   fixedTime,
			Runtime:     45,
			TriggerType: "Schedule",
		},
		{
			ID:          "e5f6a7b8-c9d0-4e1f-2a3b-4c5d6e7f8a9b",
			Workflow:    "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
			Title:       "Deploy to Production",
			State:       "FAILED",
			StartedAt:   fixedTime.Add(-time.Hour),
			Runtime:     12,
			TriggerType: "Manual",
		},
		{
			ID:          "f6a7b8c9-d0e1-4f2a-3b4c-5d6e7f8a9b0c",
			Workflow:    "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e",
			Title:       "Daily Cleanup",
			State:       "RUNNING",
			StartedAt:   fixedTime.Add(-5 * time.Minute),
			Runtime:     0,
			TriggerType: "Schedule",
		},
	}
}

func dqlRecordsFixture() []map[string]interface{} {
	return []map[string]interface{}{
		{"timestamp": "2025-03-15T10:30:00Z", "host.name": "web-server-01", "status": "ERROR", "content": "Connection timeout to database"},
		{"timestamp": "2025-03-15T10:29:55Z", "host.name": "web-server-02", "status": "WARN", "content": "High memory usage detected"},
		{"timestamp": "2025-03-15T10:29:50Z", "host.name": "api-gateway", "status": "INFO", "content": "Request processed successfully"},
	}
}

func dqlTimeseriesFixture() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"timeframe": map[string]interface{}{
				"start": "2025-03-15T09:00:00Z",
				"end":   "2025-03-15T10:00:00Z",
			},
			"interval": 300000000000, // 5 min in nanoseconds
			"avg(dt.host.cpu.usage)": []interface{}{
				12.5, 15.3, 18.7, 22.1, 35.6,
				42.3, 38.9, 25.4, 19.8, 14.2,
				11.1, 13.7,
			},
		},
	}
}

func extensionFixtures() []extension.Extension {
	return []extension.Extension{
		{
			ExtensionName: "com.dynatrace.extension.host-monitoring",
			Version:       "1.2.3",
		},
		{
			ExtensionName: "com.dynatrace.extension.jmx",
			Version:       "2.0.1",
		},
		{
			ExtensionName: "custom:my-custom-extension",
			Version:       "",
		},
	}
}

func extensionVersionFixtures() []extension.ExtensionVersion {
	return []extension.ExtensionVersion{
		{
			Version:       "1.2.3",
			ExtensionName: "com.dynatrace.extension.host-monitoring",
			Active:        true,
		},
		{
			Version:       "1.2.2",
			ExtensionName: "com.dynatrace.extension.host-monitoring",
			Active:        false,
		},
		{
			Version:       "1.1.0",
			ExtensionName: "com.dynatrace.extension.host-monitoring",
			Active:        false,
		},
	}
}

func segmentFixtures() []segment.FilterSegment {
	return []segment.FilterSegment{
		{
			UID:         "a1b2c3d4-e5f6-4a7b-8c9d-seg000000001",
			Name:        "Kubernetes Alpha",
			Description: "Filters data for Kubernetes cluster alpha",
			IsPublic:    true,
			Owner:       "admin@example.invalid",
			Version:     3,
			Includes: []segment.Include{
				{DataObject: "_all_data_object", Filter: `k8s.cluster.name = "alpha"`},
				{DataObject: "logs", Filter: `dt.system.bucket = "custom-logs"`},
			},
			Variables: &segment.Variables{
				Type:  "query",
				Value: `fetch logs | limit 1`,
			},
			VariablesDisplay:  "Yes",
			AllowedOperations: []string{"READ", "WRITE", "DELETE"},
		},
		{
			UID:         "b2c3d4e5-f6a7-4b8c-9d0e-seg000000002",
			Name:        "Production Services",
			Description: "All production microservices",
			IsPublic:    false,
			Owner:       "platform-team@example.invalid",
			Version:     1,
			Includes: []segment.Include{
				{DataObject: "spans", Filter: `environment = "production"`},
			},
			AllowedOperations: []string{"READ"},
		},
		{
			UID:      "c3d4e5f6-a7b8-4c9d-0e1f-seg000000003",
			Name:     "Error Logs",
			IsPublic: true,
			Owner:    "sre-team@example.invalid",
			Version:  7,
			Includes: []segment.Include{
				{DataObject: "logs", Filter: `status = "ERROR"`},
			},
			AllowedOperations: []string{"READ", "WRITE", "DELETE"},
		},
	}
}

func describeSegmentFixture() segment.FilterSegment {
	return segmentFixtures()[0]
}

func anomalyDetectorFixtures() []anomalydetector.AnomalyDetector {
	return []anomalydetector.AnomalyDetector{
		{
			ObjectID:      "vu9U3hXa3q0AAAABACdidWlsdGluOmRhdmlzLmFub21hbHktZGV0ZWN0b3JzAA",
			Title:         "Aurora cluster CPU utilization",
			Enabled:       true,
			AnalyzerShort: "static (>90)",
			EventType:     "PERFORMANCE_EVENT",
			Source:        "Clouds",
			Description:   "Monitors Aurora cluster CPU utilization to detect performance bottlenecks",
			Value: map[string]any{
				"title":       "Aurora cluster CPU utilization",
				"enabled":     true,
				"description": "Monitors Aurora cluster CPU utilization to detect performance bottlenecks",
				"source":      "Clouds",
				"analyzer": map[string]any{
					"name": "dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer",
					"input": []any{
						map[string]any{"key": "alertCondition", "value": "ABOVE"},
						map[string]any{"key": "threshold", "value": "90"},
						map[string]any{"key": "slidingWindow", "value": "5"},
						map[string]any{"key": "violatingSamples", "value": "3"},
						map[string]any{"key": "dealertingSamples", "value": "5"},
						map[string]any{"key": "query", "value": "timeseries cpu=avg(cloud.aws.rds.CPUUtilization), interval:1m"},
					},
				},
				"eventTemplate": map[string]any{
					"properties": []any{
						map[string]any{"key": "event.type", "value": "PERFORMANCE_EVENT"},
						map[string]any{"key": "event.name", "value": "Aurora cluster high CPU"},
						map[string]any{"key": "event.description", "value": "CPU utilization is high"},
					},
				},
			},
			SchemaVersion: "1.0.15",
		},
		{
			ObjectID:      "xw0V4iYb4r1BBBBBAC1idWlsdGluOmRhdmlzLmFub21hbHktZGV0ZWN0b3JzAB",
			Title:         "VPC endpoint packet drops",
			Enabled:       true,
			AnalyzerShort: "auto-adaptive",
			EventType:     "AVAILABILITY_EVENT",
			Source:        "Clouds",
			Value: map[string]any{
				"title":   "VPC endpoint packet drops",
				"enabled": true,
				"source":  "Clouds",
				"analyzer": map[string]any{
					"name":  "dt.statistics.ui.anomaly_detection.AutoAdaptiveAnomalyDetectionAnalyzer",
					"input": []any{},
				},
				"eventTemplate": map[string]any{
					"properties": []any{
						map[string]any{"key": "event.type", "value": "AVAILABILITY_EVENT"},
						map[string]any{"key": "event.name", "value": "VPC packet drops detected"},
					},
				},
			},
			SchemaVersion: "1.0.15",
		},
		{
			ObjectID:      "yz1W5jZc5s2CCCCCAD2idWlsdGluOmRhdmlzLmFub21hbHktZGV0ZWN0b3JzAC",
			Title:         "EC2 instance CPU utilization",
			Enabled:       false,
			AnalyzerShort: "static (>85)",
			EventType:     "PERFORMANCE_EVENT",
			Source:        "dtctl",
			Description:   "Disabled detector for EC2 CPU monitoring",
			Value: map[string]any{
				"title":       "EC2 instance CPU utilization",
				"enabled":     false,
				"description": "Disabled detector for EC2 CPU monitoring",
				"source":      "dtctl",
				"analyzer": map[string]any{
					"name": "dt.statistics.ui.anomaly_detection.StaticThresholdAnomalyDetectionAnalyzer",
					"input": []any{
						map[string]any{"key": "alertCondition", "value": "ABOVE"},
						map[string]any{"key": "threshold", "value": "85"},
					},
				},
				"eventTemplate": map[string]any{
					"properties": []any{
						map[string]any{"key": "event.type", "value": "PERFORMANCE_EVENT"},
						map[string]any{"key": "event.name", "value": "EC2 high CPU utilization"},
					},
				},
			},
			SchemaVersion: "1.0.15",
		},
	}
}

func describeAnomalyDetectorFixture() anomalydetector.AnomalyDetector {
	return anomalyDetectorFixtures()[0]
}

func monitoringConfigFixtures() []extension.MonitoringConfiguration {
	return []extension.MonitoringConfiguration{
		{
			ExtensionName: "com.dynatrace.extension.host-monitoring",
			ObjectID:      "a1b2c3d4-e5f6-4a7b-8c9d-000000000001",
			Scope:         "environment",
		},
		{
			ExtensionName: "com.dynatrace.extension.host-monitoring",
			ObjectID:      "a1b2c3d4-e5f6-4a7b-8c9d-000000000002",
			Scope:         "HOST-ABC123",
		},
		{
			ExtensionName: "com.dynatrace.extension.jmx",
			ObjectID:      "b2c3d4e5-f6a7-4b8c-9d0e-000000000003",
			Scope:         "HOST_GROUP-XYZ789",
		},
	}
}

// ---------------------------------------------------------------------------
// Golden tests: text formats (table, wide, json, yaml, csv)
// ---------------------------------------------------------------------------

func TestGolden_GetWorkflows(t *testing.T) {
	workflows := workflowFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(workflows); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/workflows-"+name, buf.String())
		})
	}
}

func TestGolden_GetSLOs(t *testing.T) {
	slos := sloFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(slos); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/slos-"+name, buf.String())
		})
	}
}

func TestGolden_GetBuckets(t *testing.T) {
	buckets := bucketFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(buckets); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/buckets-"+name, buf.String())
		})
	}
}

func TestGolden_GetDocuments(t *testing.T) {
	docs := documentFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(docs); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/documents-"+name, buf.String())
		})
	}
}

func genericDocumentFixtures() []document.Document {
	return []document.Document{
		{
			ID:        "aaaaaaaa-1111-2222-3333-444444444444",
			Name:      "My Launchpad",
			Type:      "launchpad",
			Owner:     "user-a@example.invalid",
			IsPrivate: false,
			Created:   fixedTime,
			Version:   1,
			Modified:  fixedTime.Add(time.Hour),
		},
		{
			ID:          "bbbbbbbb-2222-3333-4444-555555555555",
			Name:        "App Config",
			Type:        "my-app:config",
			Owner:       "user-b@example.invalid",
			IsPrivate:   true,
			Created:     fixedTime.Add(-48 * time.Hour),
			Description: "Configuration for my custom app",
			Version:     2,
			Modified:    fixedTime.Add(-24 * time.Hour),
		},
		{
			ID:        "cccccccc-3333-4444-5555-666666666666",
			Name:      "Production Overview",
			Type:      "dashboard",
			Owner:     "user-c@example.invalid",
			IsPrivate: false,
			Created:   fixedTime.Add(-72 * time.Hour),
			Version:   5,
			Modified:  fixedTime.Add(-2 * time.Hour),
		},
	}
}

func TestGolden_GetDocumentsGeneric(t *testing.T) {
	docs := genericDocumentFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(docs); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/documents-generic-"+name, buf.String())
		})
	}
}

func TestGolden_GetSettings(t *testing.T) {
	objs := settingsFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(objs); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/settings-"+name, buf.String())
		})
	}
}

func TestGolden_GetExtensions(t *testing.T) {
	extensions := extensionFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(extensions); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/extensions-"+name, buf.String())
		})
	}
}

func TestGolden_GetExtensionVersions(t *testing.T) {
	versions := extensionVersionFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(versions); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/extension-versions-"+name, buf.String())
		})
	}
}

func TestGolden_GetExtensionConfigs(t *testing.T) {
	configs := monitoringConfigFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(configs); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/extension-configs-"+name, buf.String())
		})
	}
}

func TestGolden_GetExecutions(t *testing.T) {
	executions := executionFixtures()

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(executions); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/executions-"+name, buf.String())
		})
	}
}

func TestGolden_GetSegments(t *testing.T) {
	segments := segmentFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(segments); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/segments-"+name, buf.String())
		})
	}
}

func TestGolden_GetAnomalyDetectors(t *testing.T) {
	detectors := anomalyDetectorFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(detectors); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/anomalydetectors-"+name, buf.String())
		})
	}
}

func taskExecutionFixtures() []workflow.TaskExecution {
	started1 := fixedTime
	ended1 := fixedTime.Add(5 * time.Second)
	started2 := fixedTime.Add(5 * time.Second)
	ended2 := fixedTime.Add(12 * time.Second)
	started3 := fixedTime.Add(12 * time.Second)
	ended3 := fixedTime.Add(15 * time.Second)
	return []workflow.TaskExecution{
		{
			ID:        "a1b2c3d4-task-0001",
			Name:      "fetch_active_events",
			State:     "SUCCESS",
			StartedAt: &started1,
			EndedAt:   &ended1,
			Runtime:   5,
			Result:    nil, // DQL task — no structured return value
		},
		{
			ID:        "a1b2c3d4-task-0002",
			Name:      "rca_analysis",
			State:     "SUCCESS",
			StartedAt: &started2,
			EndedAt:   &ended2,
			Runtime:   7,
			Result: map[string]any{
				"results": []any{
					map[string]any{
						"serviceId":  "SERVICE-BE4453718DDF0511",
						"eventStart": "2025-03-15T10:30:00.000Z",
					},
				},
			},
		},
		{
			ID:        "a1b2c3d4-task-0003",
			Name:      "send_notification",
			State:     "ERROR",
			StartedAt: &started3,
			EndedAt:   &ended3,
			Runtime:   3,
			StateInfo: func() *string { s := "HTTP 503 from notification endpoint"; return &s }(),
		},
	}
}

func taskResultFixture() any {
	return map[string]any{
		"results": []any{
			map[string]any{
				"serviceId":  "SERVICE-BE4453718DDF0511",
				"eventStart": "2025-03-15T10:30:00.000Z",
			},
			map[string]any{
				"serviceId":  "SERVICE-F19A3CC2E0B74E10",
				"eventStart": "2025-03-15T09:15:00.000Z",
			},
		},
	}
}

func TestGolden_GetTaskExecutions(t *testing.T) {
	tasks := taskExecutionFixtures()

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(tasks); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/task-executions-"+name, buf.String())
		})
	}
}

func TestGolden_GetTaskResult(t *testing.T) {
	result := taskResultFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(result); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "get/task-result-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: describe (single-object Print)
// ---------------------------------------------------------------------------

func TestGolden_DescribeWorkflow(t *testing.T) {
	wf := workflowFixtures()[0]

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(wf); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/workflow-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeBucket(t *testing.T) {
	b := bucketFixtures()[0]

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(b); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/bucket-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Describe fixtures: additional resource types
// ---------------------------------------------------------------------------

func describeSLOFixture() slo.SLO {
	return slo.SLO{
		ID:          "a1b2c3d4-0001-4000-8000-000000000001",
		Name:        "API Availability",
		Description: "99.9% availability for public API endpoints",
		Version:     "3",
		Criteria: []slo.Criteria{
			{TimeframeFrom: "-7d", Target: 99.9, Warning: float64Ptr(99.5)},
			{TimeframeFrom: "-30d", Target: 99.0},
		},
		Tags: []string{"service:api", "tier:1"},
	}
}

func describeDocumentFixture() document.DocumentMetadata {
	return document.DocumentMetadata{
		ID:          "b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d5e",
		Name:        "Production Overview",
		Type:        "dashboard",
		Owner:       "7a8b9c0d-1e2f-4a3b-8c4d-5e6f7a8b9c0d",
		IsPrivate:   false,
		Description: "Main production monitoring dashboard",
		Version:     3,
		ModificationInfo: document.ModificationInfo{
			CreatedBy:        "admin@example.invalid",
			CreatedTime:      fixedTime,
			LastModifiedBy:   "editor@example.invalid",
			LastModifiedTime: fixedTime.Add(2 * time.Hour),
		},
		Access: []string{"read", "write"},
	}
}

func describeAppFixture() appengine.App {
	return appengine.App{
		ID:          "my.custom-app",
		Name:        "Custom App",
		Version:     "1.4.2",
		Description: "A custom monitoring application",
		IsBuiltin:   false,
		ResourceStatus: &appengine.ResourceStatus{
			Status:           "OK",
			SubResourceTypes: []string{"function", "action"},
		},
		ModificationInfo: &appengine.ModificationInfo{
			CreatedBy:        "user-a@example.invalid",
			CreatedTime:      "2025-01-10T08:00:00Z",
			LastModifiedBy:   "user-b@example.invalid",
			LastModifiedTime: "2025-03-15T10:30:00Z",
		},
	}
}

func describeSettingsFixture() settings.SettingsObject {
	return settings.SettingsObject{
		ObjectID:      "vu9U3hXa3q0AAAABABhidWlsdGluOmFsZXJ0aW5nLnByb2ZpbGUABnRlbmFudAAGdGVuYW50ACRhMWIyYzNkNC1lNWY2LTRhN2ItOGM5ZC0wZTFmMmEzYjRjNWQ",
		SchemaID:      "builtin:alerting.profile",
		SchemaVersion: "1.0.5",
		Scope:         "environment",
		Summary:       "Default Alerting Profile",
		Value: map[string]any{
			"name":            "Default",
			"severityRules":   []any{},
			"eventTypeFilter": []any{},
		},
		ObjectIDShort: "vu9U3hXa3q0AAAABAB...",
		UID:           "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
		ScopeType:     "tenant",
		ScopeID:       "tenant",
	}
}

func describeEdgeConnectFixture() edgeconnect.EdgeConnect {
	return edgeconnect.EdgeConnect{
		ID:           "ec-a1b2c3d4",
		Name:         "production-edge",
		HostPatterns: []string{"*.prod.example.invalid", "api.example.invalid"},
		ModificationInfo: &edgeconnect.ModificationInfo{
			CreatedBy:        "admin@example.invalid",
			CreatedTime:      "2025-01-15T09:00:00Z",
			LastModifiedBy:   "admin@example.invalid",
			LastModifiedTime: "2025-03-10T14:30:00Z",
		},
		ManagedByDynatraceOperator: true,
		Metadata: &edgeconnect.Metadata{
			Version: "5",
		},
	}
}

func describeUserFixture() iam.User {
	return iam.User{
		UID:         "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
		Email:       "jane.doe@example.invalid",
		Name:        "Jane",
		Surname:     "Doe",
		Description: "Platform engineering team lead",
	}
}

func describeGroupFixture() iam.Group {
	return iam.Group{
		UUID:      "g1h2i3j4-k5l6-4m7n-8o9p-0q1r2s3t4u5v",
		GroupName: "platform-admins",
		Type:      "LOCAL",
	}
}

// lookupDescribeFixture is an inline struct to avoid import cycle with lookup package.
type lookupDescribeFixture struct {
	Path        string                   `json:"path"`
	DisplayName string                   `json:"displayName,omitempty"`
	Description string                   `json:"description,omitempty"`
	FileSize    int64                    `json:"fileSize,omitempty"`
	Records     int                      `json:"records,omitempty"`
	LookupField string                   `json:"lookupField,omitempty"`
	Columns     []string                 `json:"columns,omitempty"`
	Modified    string                   `json:"modified,omitempty"`
	PreviewData []map[string]interface{} `json:"previewData"`
}

func describeLookupFixture() lookupDescribeFixture {
	return lookupDescribeFixture{
		Path:        "/lookups/grail/pm/error_codes",
		DisplayName: "Error Codes",
		Description: "Mapping of error codes to descriptions",
		FileSize:    2048,
		Records:     42,
		LookupField: "code",
		Columns:     []string{"code", "description", "severity"},
		Modified:    "2025-03-15T10:30:00Z",
		PreviewData: []map[string]interface{}{
			{"code": "E001", "description": "Connection timeout", "severity": "HIGH"},
			{"code": "E002", "description": "Invalid input", "severity": "LOW"},
		},
	}
}

func describeAzureConnectionFixture() azureconnection.AzureConnection {
	return azureconnection.AzureConnection{
		ObjectID: "azure-conn-a1b2c3d4",
		Value: azureconnection.Value{
			Name: "production-azure",
			Type: "CLIENT_SECRET",
			ClientSecret: &azureconnection.ClientSecretCredential{
				ApplicationID: "app-id-1234-5678",
				DirectoryID:   "dir-id-abcd-efgh",
				Consumers:     []string{"ME", "DA"},
			},
		},
		Name: "production-azure",
		Type: "CLIENT_SECRET",
	}
}

func describeAzureMonitoringConfigFixture() azuremonitoringconfig.AzureMonitoringConfig {
	return azuremonitoringconfig.AzureMonitoringConfig{
		ObjectID:    "azmon-a1b2c3d4",
		Description: "Production Azure monitoring",
		Enabled:     true,
		Version:     "1.0",
		Value: azuremonitoringconfig.Value{
			Enabled:     true,
			Description: "Production Azure monitoring",
			Version:     "1.0",
			Azure: azuremonitoringconfig.AzureConfig{
				DeploymentScope:           "MANAGEMENT_GROUP",
				SubscriptionFilteringMode: "ALL",
				ConfigurationMode:         "STANDARD",
				DeploymentMode:            "FULL",
				Credentials: []azuremonitoringconfig.Credential{
					{
						Enabled:      true,
						Description:  "Main credential",
						ConnectionId: "azure-conn-a1b2c3d4",
						Type:         "CLIENT_SECRET",
					},
				},
			},
			FeatureSets: []string{"default", "advanced"},
		},
	}
}

func describeGCPConnectionFixture() gcpconnection.GCPConnection {
	return gcpconnection.GCPConnection{
		ObjectID: "gcp-conn-x1y2z3",
		Value: gcpconnection.Value{
			Name: "production-gcp",
			Type: "SERVICE_ACCOUNT_IMPERSONATION",
			ServiceAccountImpersonation: &gcpconnection.ServiceAccountImpersonation{
				ServiceAccountID: "sa-monitor@project-id.iam.gserviceaccount.com",
				Consumers:        []string{"ME", "DA"},
			},
		},
		Name: "production-gcp",
		Type: "SERVICE_ACCOUNT_IMPERSONATION",
	}
}

func describeGCPMonitoringConfigFixture() gcpmonitoringconfig.GCPMonitoringConfig {
	return gcpmonitoringconfig.GCPMonitoringConfig{
		ObjectID:    "gcpmon-a1b2c3d4",
		Description: "Production GCP monitoring",
		Enabled:     true,
		Version:     "2.0",
		Value: gcpmonitoringconfig.Value{
			Enabled:     true,
			Description: "Production GCP monitoring",
			Version:     "2.0",
			GoogleCloud: gcpmonitoringconfig.GoogleCloudConfig{
				LocationFiltering: []string{"us-east1", "eu-west1"},
				ProjectFiltering:  []string{"my-project-123"},
				FolderFiltering:   []string{},
				Credentials: []gcpmonitoringconfig.Credential{
					{
						Description:    "Main SA credential",
						Enabled:        true,
						ConnectionID:   "gcp-conn-x1y2z3",
						ServiceAccount: "sa-monitor@project-id.iam.gserviceaccount.com",
					},
				},
			},
			FeatureSets: []string{"default"},
		},
	}
}

func describeExecutionFixture() workflow.Execution {
	endedAt := fixedTime.Add(45 * time.Second)
	triggerStr := "schedule"
	return workflow.Execution{
		ID:          "d4e5f6a7-b8c9-4d0e-1f2a-3b4c5d6e7f8a",
		Workflow:    "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
		Title:       "Deploy to Production",
		State:       "SUCCEEDED",
		StartedAt:   fixedTime,
		EndedAt:     &endedAt,
		Runtime:     45,
		Trigger:     &triggerStr,
		TriggerType: "Schedule",
	}
}

// ---------------------------------------------------------------------------
// Golden tests: describe (additional resource types)
// ---------------------------------------------------------------------------

func TestGolden_DescribeSLO(t *testing.T) {
	s := describeSLOFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(s); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/slo-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeDocument(t *testing.T) {
	d := describeDocumentFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(d); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/document-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeApp(t *testing.T) {
	a := describeAppFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(a); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/app-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeSettings(t *testing.T) {
	s := describeSettingsFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(s); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/settings-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeEdgeConnect(t *testing.T) {
	ec := describeEdgeConnectFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(ec); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/edgeconnect-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeUser(t *testing.T) {
	u := describeUserFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(u); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/user-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeGroup(t *testing.T) {
	g := describeGroupFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(g); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/group-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeLookup(t *testing.T) {
	lu := describeLookupFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(lu); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/lookup-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeAzureConnection(t *testing.T) {
	ac := describeAzureConnectionFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(ac); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/azure-connection-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeAzureMonitoring(t *testing.T) {
	am := describeAzureMonitoringConfigFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(am); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/azure-monitoring-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeGCPConnection(t *testing.T) {
	gc := describeGCPConnectionFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(gc); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/gcp-connection-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeGCPMonitoring(t *testing.T) {
	gm := describeGCPMonitoringConfigFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(gm); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/gcp-monitoring-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeExecution(t *testing.T) {
	e := describeExecutionFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(e); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/execution-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeSegment(t *testing.T) {
	s := describeSegmentFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(s); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/segment-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeAnomalyDetector(t *testing.T) {
	d := describeAnomalyDetectorFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
		"toon": "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(d); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/anomalydetector-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: DQL query results (map-based records)
// ---------------------------------------------------------------------------

func TestGolden_QueryDQL(t *testing.T) {
	records := dqlRecordsFixture()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"csv":   "csv",
		"toon":  "toon",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(records); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "query/dql-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: DQL query results with metadata
// ---------------------------------------------------------------------------

func metadataFixture() *QueryMetadata {
	return &QueryMetadata{
		ExecutionTimeMilliseconds: 47,
		ScannedRecords:            42351,
		ScannedBytes:              2982690,
		ScannedDataPoints:         0,
		Sampled:                   false,
		QueryID:                   "27c4daf9-2619-4ba1-b1ad-9e276c75a351",
		DQLVersion:                "V1_0",
		Query:                     "fetch logs | limit 3 | fields timestamp, host.name, status, content",
		CanonicalQuery:            "fetch logs\n| limit 3\n| fields timestamp, host.name, status, content",
		Timezone:                  "Z",
		Locale:                    "und",
		AnalysisTimeframe: &MetadataTimeframe{
			Start: "2025-03-15T08:30:00.000000000Z",
			End:   "2025-03-15T10:30:00.000000000Z",
		},
		Contributions: &MetadataContribs{
			Buckets: []MetadataBucket{
				{
					Name:                "default_logs",
					Table:               "logs",
					ScannedBytes:        2982690,
					MatchedRecordsRatio: 1.0,
				},
			},
		},
	}
}

func TestGolden_QueryDQL_Metadata_JSON(t *testing.T) {
	records := dqlRecordsFixture()
	meta := metadataFixture()

	t.Run("all", func(t *testing.T) {
		// "all" returns the struct (omitempty suppresses zeros)
		payload := map[string]interface{}{
			"records":  records,
			"metadata": MetadataToMap(meta, []string{"all"}),
		}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("json", &buf)
		if err := printer.Print(payload); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		assertGolden(t, "query/dql-metadata-json", buf.String())
	})

	t.Run("filtered", func(t *testing.T) {
		// Explicit field selection including a zero-value field (scannedDataPoints=0)
		fields := []string{"executionTimeMilliseconds", "scannedRecords", "scannedDataPoints", "sampled", "queryId"}
		payload := map[string]interface{}{
			"records":  records,
			"metadata": MetadataToMap(meta, fields),
		}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("json", &buf)
		if err := printer.Print(payload); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		assertGolden(t, "query/dql-metadata-filtered-json", buf.String())
	})
}

func TestGolden_QueryDQL_Metadata_YAML(t *testing.T) {
	records := dqlRecordsFixture()
	meta := metadataFixture()

	t.Run("all", func(t *testing.T) {
		payload := map[string]interface{}{
			"records":  records,
			"metadata": MetadataToMap(meta, []string{"all"}),
		}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("yaml", &buf)
		if err := printer.Print(payload); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		assertGolden(t, "query/dql-metadata-yaml", buf.String())
	})

	t.Run("filtered", func(t *testing.T) {
		fields := []string{"executionTimeMilliseconds", "scannedRecords", "scannedDataPoints", "sampled", "queryId"}
		payload := map[string]interface{}{
			"records":  records,
			"metadata": MetadataToMap(meta, fields),
		}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("yaml", &buf)
		if err := printer.Print(payload); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		assertGolden(t, "query/dql-metadata-filtered-yaml", buf.String())
	})
}

func TestGolden_QueryDQL_Metadata_Table(t *testing.T) {
	records := dqlRecordsFixture()
	meta := metadataFixture()

	// Disable color for deterministic output
	ResetColorCache()
	SetPlainMode(true)
	defer ResetColorCache()

	t.Run("all", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("table", &buf)
		if err := printer.PrintList(records); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		// Append metadata footer (same as printResults does)
		footer := FormatMetadataFooter(meta, nil)
		assertGolden(t, "query/dql-metadata-table", buf.String()+footer)
	})

	t.Run("filtered", func(t *testing.T) {
		fields := []string{"executionTimeMilliseconds", "scannedRecords", "queryId"}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("table", &buf)
		if err := printer.PrintList(records); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		footer := FormatMetadataFooter(meta, fields)
		assertGolden(t, "query/dql-metadata-filtered-table", buf.String()+footer)
	})
}

func TestGolden_QueryDQL_Metadata_Wide(t *testing.T) {
	records := dqlRecordsFixture()
	meta := metadataFixture()

	// Disable color for deterministic output
	ResetColorCache()
	SetPlainMode(true)
	defer ResetColorCache()

	t.Run("all", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("wide", &buf)
		if err := printer.PrintList(records); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		// Append metadata footer (same as printResults does for wide format)
		footer := FormatMetadataFooter(meta, nil)
		assertGolden(t, "query/dql-metadata-wide", buf.String()+footer)
	})

	t.Run("filtered", func(t *testing.T) {
		fields := []string{"executionTimeMilliseconds", "scannedRecords", "queryId"}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("wide", &buf)
		if err := printer.PrintList(records); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		footer := FormatMetadataFooter(meta, fields)
		assertGolden(t, "query/dql-metadata-filtered-wide", buf.String()+footer)
	})
}

func TestGolden_QueryDQL_Metadata_CSV(t *testing.T) {
	records := dqlRecordsFixture()
	meta := metadataFixture()

	t.Run("all", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("csv", &buf)
		if err := printer.PrintList(records); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		// Prepend metadata comments (same as printResults does)
		comments := FormatMetadataCSVComments(meta, nil)
		assertGolden(t, "query/dql-metadata-csv", comments+buf.String())
	})

	t.Run("filtered", func(t *testing.T) {
		fields := []string{"executionTimeMilliseconds", "scannedRecords", "queryId"}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("csv", &buf)
		if err := printer.PrintList(records); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		comments := FormatMetadataCSVComments(meta, fields)
		assertGolden(t, "query/dql-metadata-filtered-csv", comments+buf.String())
	})
}

func TestGolden_QueryDQL_Metadata_TOON(t *testing.T) {
	records := dqlRecordsFixture()
	meta := metadataFixture()

	t.Run("all", func(t *testing.T) {
		payload := map[string]interface{}{
			"records":  records,
			"metadata": MetadataToMap(meta, []string{"all"}),
		}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("toon", &buf)
		if err := printer.Print(payload); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		assertGolden(t, "query/dql-metadata-toon", buf.String())
	})

	t.Run("filtered", func(t *testing.T) {
		fields := []string{"executionTimeMilliseconds", "scannedRecords", "scannedDataPoints", "sampled", "queryId"}
		payload := map[string]interface{}{
			"records":  records,
			"metadata": MetadataToMap(meta, fields),
		}
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("toon", &buf)
		if err := printer.Print(payload); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		assertGolden(t, "query/dql-metadata-filtered-toon", buf.String())
	})
}

// ---------------------------------------------------------------------------
// Golden tests: empty results
// ---------------------------------------------------------------------------

func TestGolden_EmptyResults(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("table", &buf)
		if err := printer.PrintList([]workflow.Workflow{}); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		assertGolden(t, "empty/workflows-table", buf.String())
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("json", &buf)
		if err := printer.PrintList([]workflow.Workflow{}); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		assertGolden(t, "empty/workflows-json", buf.String())
	})
}

// ---------------------------------------------------------------------------
// Golden tests: visual formats (chart, sparkline, barchart, braille)
// Use fixed dimensions for deterministic output.
// ---------------------------------------------------------------------------

func TestGolden_QueryDQL_Chart(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "chart",
		Writer: &buf,
		Width:  80,
		Height: 15,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	// Strip ANSI for deterministic comparison
	assertGolden(t, "query/dql-chart", stripANSI(buf.String()))
}

func TestGolden_QueryDQL_Sparkline(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "sparkline",
		Writer: &buf,
		Width:  60,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	assertGolden(t, "query/dql-sparkline", stripANSI(buf.String()))
}

func TestGolden_QueryDQL_BarChart(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "barchart",
		Writer: &buf,
		Width:  60,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	assertGolden(t, "query/dql-barchart", stripANSI(buf.String()))
}

func TestGolden_QueryDQL_Braille(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "braille",
		Writer: &buf,
		Width:  40,
		Height: 10,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	assertGolden(t, "query/dql-braille", stripANSI(buf.String()))
}

// ---------------------------------------------------------------------------
// Golden tests: error output
// ---------------------------------------------------------------------------

func TestGolden_ErrorNotFound(t *testing.T) {
	// Simulate the error message a user would see
	errMsg := "Error: workflow \"my-workflow\" not found\n"
	assertGolden(t, "errors/not-found", errMsg)
}

func TestGolden_ErrorAuth(t *testing.T) {
	errMsg := "Error: authentication failed: token expired or invalid\n"
	assertGolden(t, "errors/auth-error", errMsg)
}

func TestGolden_ErrorPermission(t *testing.T) {
	errMsg := "Error: insufficient permissions: requires scope \"automation:workflows:read\"\n"
	assertGolden(t, "errors/permission-error", errMsg)
}

// ---------------------------------------------------------------------------
// Golden tests: agent mode error output (JSON envelope with ok: false)
// ---------------------------------------------------------------------------

func TestGolden_AgentErrorAuth(t *testing.T) {
	var buf bytes.Buffer
	detail := &ErrorDetail{
		Code:       "auth_required",
		Message:    "authentication failed",
		Operation:  "get workflows",
		StatusCode: 401,
		RequestID:  "req-abc-123",
		Suggestions: []string{
			"Run 'dtctl auth login' to refresh your token",
			"Verify your token is correct in the current context configuration",
		},
	}
	if err := PrintError(&buf, detail); err != nil {
		t.Fatalf("PrintError failed: %v", err)
	}
	assertGolden(t, "errors/auth-error-agent", buf.String())
}

func TestGolden_AgentErrorNotFound(t *testing.T) {
	var buf bytes.Buffer
	detail := &ErrorDetail{
		Code:       "not_found",
		Message:    "workflow not found",
		Operation:  "get workflows",
		StatusCode: 404,
		Suggestions: []string{
			"Verify the resource name or ID is correct",
			"List available resources: 'dtctl get workflows'",
		},
	}
	if err := PrintError(&buf, detail); err != nil {
		t.Fatalf("PrintError failed: %v", err)
	}
	assertGolden(t, "errors/not-found-agent", buf.String())
}

func TestGolden_AgentErrorSafetyBlocked(t *testing.T) {
	var buf bytes.Buffer
	detail := &ErrorDetail{
		Code:    "safety_blocked",
		Message: "Context 'production' (readonly) does not allow delete operations",
		Suggestions: []string{
			"Switch to a context with write permissions",
		},
	}
	if err := PrintError(&buf, detail); err != nil {
		t.Fatalf("PrintError failed: %v", err)
	}
	assertGolden(t, "errors/safety-blocked-agent", buf.String())
}

func TestGolden_AgentErrorGeneric(t *testing.T) {
	var buf bytes.Buffer
	detail := &ErrorDetail{
		Code:    "error",
		Message: "something unexpected happened",
	}
	if err := PrintError(&buf, detail); err != nil {
		t.Fatalf("PrintError failed: %v", err)
	}
	assertGolden(t, "errors/generic-error-agent", buf.String())
}

// ---------------------------------------------------------------------------
// Golden tests: agent mode output
// ---------------------------------------------------------------------------

func TestGolden_AgentMode(t *testing.T) {
	workflows := workflowFixtures()

	t.Run("list", func(t *testing.T) {
		var buf bytes.Buffer
		ctx := &ResponseContext{
			Verb:     "get",
			Resource: "workflow",
		}
		printer := NewAgentPrinter(&buf, ctx)
		printer.SetTotal(len(workflows))
		printer.SetSuggestions([]string{
			"Run 'dtctl describe workflow <id>' for details",
			"Run 'dtctl exec workflow <id>' to trigger a workflow",
		})
		if err := printer.PrintList(workflows); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		assertGolden(t, "get/workflows-agent", buf.String())
	})
}

// ---------------------------------------------------------------------------
// Golden tests: watch output (change prefixes)
// ---------------------------------------------------------------------------

func TestGolden_WatchChanges(t *testing.T) {
	wfs := workflowFixtures()

	changes := []Change{
		{Type: ChangeTypeAdded, Resource: wfs[0]},
		{Type: ChangeTypeModified, Resource: wfs[1]},
		{Type: ChangeTypeDeleted, Resource: wfs[2]},
	}

	var buf bytes.Buffer
	basePrinter := NewPrinterWithWriter("table", &buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, &buf, false) // no color for deterministic output

	if err := watchPrinter.PrintChanges(changes); err != nil {
		t.Fatalf("PrintChanges failed: %v", err)
	}

	assertGolden(t, "get/workflows-watch-changes", buf.String())
}

// ---------------------------------------------------------------------------
// Hub fixtures
// ---------------------------------------------------------------------------

func hubExtensionFixtures() []hub.HubExtension {
	return []hub.HubExtension{
		{
			ID:          "com.dynatrace.extension.host-monitoring",
			Name:        "Host Monitoring",
			Type:        "EXTENSION_2",
			Description: "Comprehensive host performance monitoring",
		},
		{
			ID:          "com.dynatrace.extension.jmx",
			Name:        "JMX Extension",
			Type:        "EXTENSION_2",
			Description: "Java management extensions monitoring",
		},
		{
			ID:   "custom:my-custom-extension",
			Name: "Custom Metrics Collector",
			Type: "EXTENSION_2",
		},
	}
}

// ---------------------------------------------------------------------------
// Golden tests: Hub extensions
// ---------------------------------------------------------------------------

func TestGolden_GetHubExtensions(t *testing.T) {
	exts := hubExtensionFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(exts); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/hub-extensions-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeHubExtension(t *testing.T) {
	ext := hubExtensionFixtures()[0]

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(ext); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/hub-extension-"+name, buf.String())
		})
	}
}

func hubExtensionReleaseFixtures() []hub.HubExtensionRelease {
	return []hub.HubExtensionRelease{
		{Version: "3.1.0", ReleaseDate: "2025-03-01"},
		{Version: "3.0.2", ReleaseDate: "2025-01-15"},
		{Version: "3.0.0", ReleaseDate: "2024-11-20"},
	}
}

func TestGolden_GetHubExtensionReleases(t *testing.T) {
	releases := hubExtensionReleaseFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(releases); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/hub-extension-releases-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: create extension (single ExtensionVersion)
// ---------------------------------------------------------------------------

func TestGolden_DescribeExtensionVersion(t *testing.T) {
	v := extensionVersionFixtures()[0]

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(v); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/extension-version-"+name, buf.String())
		})
	}
}

func activeGateGroupFixtures() []extension.ActiveGateGroupItem {
	return []extension.ActiveGateGroupItem{
		{
			GroupName:            "esx-linux-ag",
			AvailableActiveGates: 2,
			ActiveGates: []extension.ActiveGateEntry{
				{ID: 187309619},
				{ID: 1981204261},
			},
		},
		{
			GroupName:            "windows-prod",
			AvailableActiveGates: 1,
			ActiveGates: []extension.ActiveGateEntry{
				{ID: 305812744},
			},
		},
	}
}

func TestGolden_GetActiveGateGroups(t *testing.T) {
	groups := activeGateGroupFixtures()

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(groups); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/extension-active-gate-groups-"+name, buf.String())
		})
	}
}

// monitoringConfigSchemaFixture returns a synthetic JSON Schema document that
// mirrors the structure returned by the Extensions 2.0 schema endpoint.
// It intentionally includes documentation, customMessage, and displayName fields
// so that the --no-fluff stripping can be verified.
func monitoringConfigSchemaFixture() interface{} {
	return map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"displayName": "PostgreSQL Monitoring Configuration",
		"type":        "object",
		"properties": map[string]interface{}{
			"enabled": map[string]interface{}{
				"type":          "boolean",
				"displayName":   "Enable monitoring",
				"documentation": "When set to false, monitoring is suspended.",
				"customMessage": "Disabling will stop all metric collection.",
				"default":       true,
			},
			"host": map[string]interface{}{
				"type":        "string",
				"displayName": "Hostname",
				"description": "PostgreSQL server hostname or IP address.",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"displayName": "Port",
				"description": "PostgreSQL server port.",
				"default":     5432,
			},
			"credentials": map[string]interface{}{
				"type":          "object",
				"displayName":   "Credentials",
				"documentation": "Authentication settings for the PostgreSQL server.",
				"properties": map[string]interface{}{
					"username": map[string]interface{}{
						"type":          "string",
						"displayName":   "Username",
						"customMessage": "Use a read-only user for security.",
					},
					"password": map[string]interface{}{
						"type":        "string",
						"displayName": "Password",
					},
				},
			},
		},
	}
}

func TestGolden_DescribeExtensionSchema(t *testing.T) {
	schema := monitoringConfigSchemaFixture()

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(schema); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/extension-schema-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeExtensionSchemaNoFluff(t *testing.T) {
	schema := monitoringConfigSchemaFixture()
	stripped := extension.StripSchemaFluff(schema)

	formats := map[string]string{
		"json": "json",
		"yaml": "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(stripped); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/extension-schema-no-fluff-"+name, buf.String())
		})
	}
}
