package output

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
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

func TestGolden_GetExecutions(t *testing.T) {
	executions := executionFixtures()

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
			if err := printer.PrintList(executions); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/executions-"+name, buf.String())
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
// Golden tests: DQL query results (map-based records)
// ---------------------------------------------------------------------------

func TestGolden_QueryDQL(t *testing.T) {
	records := dqlRecordsFixture()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"csv":   "csv",
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
