package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func buildZip(t *testing.T, files map[string]interface{}) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		var data []byte
		var err error
		switch v := content.(type) {
		case string:
			data = []byte(v)
		default:
			data, err = json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal %s: %v", name, err)
			}
		}
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// buildNestedZip wraps an inner zip inside an outer zip as "extension.zip",
// matching the real Dynatrace extension package format.
func buildNestedZip(t *testing.T, innerFiles map[string]interface{}) []byte {
	t.Helper()
	inner := buildZip(t, innerFiles)

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("extension.zip")
	if err != nil {
		t.Fatalf("create outer zip entry: %v", err)
	}
	if _, err := f.Write(inner); err != nil {
		t.Fatalf("write inner zip: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close outer zip: %v", err)
	}
	return buf.Bytes()
}

func TestParseAssets_UnknownType(t *testing.T) {
	zipData := buildNestedZip(t, map[string]interface{}{})
	_, err := ParseAssets(zipData, []string{"dashboards"}, false)
	if err == nil {
		t.Fatal("expected error for unknown asset type")
	}
	if !strings.Contains(err.Error(), "unknown asset type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseAssets_AlertTemplates(t *testing.T) {
	zipData := buildNestedZip(t, map[string]interface{}{
		"extension.yaml": "alerts:\n  - path: alerts/my-alert.json\n",
		"alerts/my-alert.json": map[string]interface{}{
			"name":      "Test Alert",
			"eventType": "AVAILABILITY",
			"enabled":   true,
		},
	})

	result, err := ParseAssets(zipData, []string{"alert_templates"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AlertTemplates) != 1 {
		t.Fatalf("expected 1 alert template, got %d", len(result.AlertTemplates))
	}
	a := result.AlertTemplates[0]
	if a.Name != "Test Alert" || a.EventType != "AVAILABILITY" || a.Enabled == nil || !*a.Enabled {
		t.Errorf("unexpected alert: %+v", a)
	}
}

func TestParseAssets_Smartscape(t *testing.T) {
	pipeline := `{
		"smartscapeNodeExtraction": {"processors": [
			{"id": "node_a", "type": "smartscapeNode", "description": "Create NODE_A",
			 "smartscapeNode": {"nodeType": "NODE_A", "nodeIdFieldName": "dt.smartscape.node_a", "extractNode": true,
			   "staticEdgesToExtract": [{"edgeType": "runs_on", "targetType": "NODE_B", "targetIdFieldName": "dt.smartscape.node_b"}]}},
			{"id": "enrich_a", "type": "smartscapeNode", "description": "Enrich NODE_A",
			 "smartscapeNode": {"nodeType": "NODE_A", "nodeIdFieldName": "dt.smartscape.node_a", "extractNode": false,
			   "staticEdgesToExtract": []}}
		]}
	}`
	zipData := buildNestedZip(t, map[string]interface{}{
		"extension.yaml":      "openpipeline:\n  pipelines:\n    - pipelinePath: openpipeline/p.json\n      displayName: Test\n",
		"openpipeline/p.json": pipeline,
	})
	result, err := ParseAssets(zipData, []string{"smartscape"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Smartscape == nil {
		t.Fatal("expected non-nil Smartscape result")
	}
	if len(result.Smartscape.Nodes) != 1 {
		t.Fatalf("expected 1 node (extractNode=true only), got %d", len(result.Smartscape.Nodes))
	}
	if result.Smartscape.Nodes[0].NodeType != "NODE_A" {
		t.Errorf("unexpected node type: %s", result.Smartscape.Nodes[0].NodeType)
	}
	if len(result.Smartscape.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(result.Smartscape.Edges))
	}
	e := result.Smartscape.Edges[0]
	if e.SourceType != "NODE_A" || e.EdgeType != "runs_on" || e.TargetType != "NODE_B" {
		t.Errorf("unexpected edge: %+v", e)
	}
}

func TestParseAssets_BothTypes(t *testing.T) {
	pipeline := `{"smartscapeNodeExtraction": {"processors": [
		{"id": "n", "type": "smartscapeNode", "description": "Create NODE_X",
		 "smartscapeNode": {"nodeType": "NODE_X", "nodeIdFieldName": "dt.node_x", "extractNode": true, "staticEdgesToExtract": []}}
	]}}`
	zipData := buildNestedZip(t, map[string]interface{}{
		"extension.yaml":      "alerts:\n  - path: alerts/alert.json\nopenpipeline:\n  pipelines:\n    - pipelinePath: openpipeline/p.json\n      displayName: Test\n",
		"alerts/alert.json":   map[string]interface{}{"name": "Alert One", "eventType": "CUSTOM_ALERT"},
		"openpipeline/p.json": pipeline,
	})
	result, err := ParseAssets(zipData, []string{"alert_templates", "smartscape"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AlertTemplates) != 1 {
		t.Errorf("expected 1 alert template, got %d", len(result.AlertTemplates))
	}
	if result.Smartscape == nil || len(result.Smartscape.Nodes) != 1 {
		t.Errorf("expected 1 smartscape node, got %v", result.Smartscape)
	}
}

func TestParseAssets_EmptyTypeReturnsZeroNotError(t *testing.T) {
	// Extension with no openpipeline section — smartscape should return empty result, not error.
	zipData := buildNestedZip(t, map[string]interface{}{
		"extension.yaml": "alerts: []\n",
	})
	result, err := ParseAssets(zipData, []string{"smartscape"}, false)
	if err != nil {
		t.Fatalf("unexpected error for missing type: %v", err)
	}
	if result.Smartscape == nil {
		t.Fatal("expected non-nil Smartscape result")
	}
	if len(result.Smartscape.Nodes) != 0 || len(result.Smartscape.Edges) != 0 {
		t.Errorf("expected empty smartscape, got %+v", result.Smartscape)
	}
}

func TestParseAssets_FullPopulatesContent(t *testing.T) {
	zipData := buildNestedZip(t, map[string]interface{}{
		"extension.yaml": "alerts:\n  - path: alerts/a.json\n",
		"alerts/a.json":  map[string]interface{}{"name": "A", "eventType": "CUSTOM_ALERT", "threshold": 42},
	})
	result, err := ParseAssets(zipData, []string{"alert_templates"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AlertTemplates) != 1 {
		t.Fatalf("expected 1 alert template, got %d", len(result.AlertTemplates))
	}
	if result.AlertTemplates[0].Content == nil {
		t.Error("expected Content to be populated with --full")
	}
	// Content should contain the full JSON including fields not in the summary struct.
	if !strings.Contains(string(result.AlertTemplates[0].Content), `"threshold"`) {
		t.Errorf("Content missing extra fields: %s", result.AlertTemplates[0].Content)
	}
}

func TestParseAssets_NoFullLeavesContentNil(t *testing.T) {
	zipData := buildNestedZip(t, map[string]interface{}{
		"extension.yaml": "alerts:\n  - path: alerts/a.json\n",
		"alerts/a.json":  map[string]interface{}{"name": "A", "eventType": "CUSTOM_ALERT"},
	})
	result, err := ParseAssets(zipData, []string{"alert_templates"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlertTemplates[0].Content != nil {
		t.Error("expected Content to be nil without --full")
	}
}

func TestParseAssets_FlatZip(t *testing.T) {
	zipData := buildZip(t, map[string]interface{}{
		"extension.yaml":   "alerts:\n  - path: alerts/flat.json\n",
		"alerts/flat.json": map[string]interface{}{"name": "Flat Alert", "eventType": "AVAILABILITY"},
	})
	result, err := ParseAssets(zipData, []string{"alert_templates"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AlertTemplates) != 1 {
		t.Errorf("expected 1 alert template in flat zip, got %d", len(result.AlertTemplates))
	}
}
