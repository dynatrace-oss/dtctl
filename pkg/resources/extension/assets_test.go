package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"testing"
)

// buildTestZip creates an in-memory zip archive from a map of filename→content.
func buildTestZip(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(content); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildNestedZip builds the outer zip that contains an inner extension.zip.
func buildNestedZip(innerFiles map[string][]byte) ([]byte, error) {
	inner, err := buildTestZip(innerFiles)
	if err != nil {
		return nil, err
	}
	return buildTestZip(map[string][]byte{"extension.zip": inner})
}

var alertYAML = []byte(`
name: com.example.test
version: "1.0.0"
alerts:
  - alerts/my_alert.json
`)

var alertJSON = []byte(`{
  "name": "My Alert",
  "eventType": "AVAILABILITY",
  "enabled": true
}`)

var pipelineYAML = []byte(`
name: com.example.test
version: "1.0.0"
openpipeline:
  pipelines:
    - openpipeline/logs.json
`)

var pipelineJSON = []byte(`{
  "smartscapeNodeExtraction": {
    "processors": [
      {
        "extractNode": true,
        "nodeType": "HOST",
        "sourceType": "HOST",
        "edgeType": "RUNS",
        "targetType": "PROCESS_GROUP_INSTANCE"
      },
      {
        "extractNode": false,
        "nodeType": "SERVICE"
      }
    ]
  }
}`)

func TestInspectAssets_AlertTemplates_FlatLayout(t *testing.T) {
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml":     alertYAML,
		"alerts/my_alert.json": alertJSON,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"alert_templates"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.AlertTemplates) != 1 {
		t.Fatalf("expected 1 alert template, got %d", len(assets.AlertTemplates))
	}
	tpl := assets.AlertTemplates[0]
	if tpl.Name != "My Alert" {
		t.Errorf("expected Name=%q, got %q", "My Alert", tpl.Name)
	}
	if tpl.EventType != "AVAILABILITY" {
		t.Errorf("expected EventType=%q, got %q", "AVAILABILITY", tpl.EventType)
	}
	if tpl.Enabled == nil || !*tpl.Enabled {
		t.Error("expected Enabled=true")
	}
	if tpl.Content != nil {
		t.Error("expected Content=nil when full=false")
	}
}

func TestInspectAssets_AlertTemplates_NestedLayout(t *testing.T) {
	zipData, err := buildNestedZip(map[string][]byte{
		"extension.yaml":     alertYAML,
		"alerts/my_alert.json": alertJSON,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"alert_templates"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.AlertTemplates) != 1 {
		t.Fatalf("expected 1 alert template, got %d", len(assets.AlertTemplates))
	}
}

func TestInspectAssets_AlertTemplates_Full(t *testing.T) {
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml":     alertYAML,
		"alerts/my_alert.json": alertJSON,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"alert_templates"}, true)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.AlertTemplates) != 1 {
		t.Fatalf("expected 1 alert template, got %d", len(assets.AlertTemplates))
	}
	tpl := assets.AlertTemplates[0]
	if tpl.Content == nil {
		t.Fatal("expected Content to be set when full=true")
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(tpl.Content, &raw); err != nil {
		t.Fatalf("Content is not valid JSON: %v", err)
	}
	if raw["name"] != "My Alert" {
		t.Errorf("expected Content.name=%q, got %v", "My Alert", raw["name"])
	}
}

func TestInspectAssets_Smartscape(t *testing.T) {
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml":         pipelineYAML,
		"openpipeline/logs.json": pipelineJSON,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"smartscape"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	sc := assets.Smartscape
	if sc == nil {
		t.Fatal("expected Smartscape to be non-nil")
	}
	// Only the processor with extractNode=true should be a node
	if len(sc.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(sc.Nodes))
	}
	if sc.Nodes[0].NodeType != "HOST" {
		t.Errorf("expected node NodeType=%q, got %q", "HOST", sc.Nodes[0].NodeType)
	}
	if sc.Nodes[0].Content != nil {
		t.Error("expected node Content=nil when full=false")
	}
	// The first processor also has sourceType/targetType → one edge
	if len(sc.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(sc.Edges))
	}
	e := sc.Edges[0]
	if e.SourceType != "HOST" || e.EdgeType != "RUNS" || e.TargetType != "PROCESS_GROUP_INSTANCE" {
		t.Errorf("unexpected edge: %+v", e)
	}
}

func TestInspectAssets_Smartscape_Dedup(t *testing.T) {
	dupPipeline := []byte(`{
  "smartscapeNodeExtraction": {
    "processors": [
      {"extractNode": true, "nodeType": "HOST"},
      {"extractNode": true, "nodeType": "HOST"},
      {"extractNode": true, "nodeType": "SERVICE"},
      {"sourceType": "A", "edgeType": "CALLS", "targetType": "B"},
      {"sourceType": "A", "edgeType": "CALLS", "targetType": "B"}
    ]
  }
}`)

	yaml := []byte(`
name: com.example.test
version: "1.0.0"
openpipeline:
  pipelines:
    - p.json
`)
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml": yaml,
		"p.json":         dupPipeline,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"smartscape"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.Smartscape.Nodes) != 2 {
		t.Errorf("expected 2 deduplicated nodes, got %d", len(assets.Smartscape.Nodes))
	}
	if len(assets.Smartscape.Edges) != 1 {
		t.Errorf("expected 1 deduplicated edge, got %d", len(assets.Smartscape.Edges))
	}
}

func TestInspectAssets_Smartscape_Full(t *testing.T) {
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml":         pipelineYAML,
		"openpipeline/logs.json": pipelineJSON,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"smartscape"}, true)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.Smartscape.Nodes) == 0 {
		t.Fatal("expected nodes")
	}
	n := assets.Smartscape.Nodes[0]
	if n.Content == nil {
		t.Fatal("expected Content to be set when full=true")
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(n.Content, &raw); err != nil {
		t.Fatalf("Node.Content is not valid JSON: %v", err)
	}
}

func TestInspectAssets_MultipleTypes(t *testing.T) {
	manifestYAML := []byte(`
name: com.example.test
version: "1.0.0"
alerts:
  - alerts/my_alert.json
openpipeline:
  pipelines:
    - openpipeline/logs.json
`)
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml":           manifestYAML,
		"alerts/my_alert.json":     alertJSON,
		"openpipeline/logs.json":   pipelineJSON,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}

	assets, err := InspectAssets(zipData, []string{"alert_templates", "smartscape"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.AlertTemplates) != 1 {
		t.Errorf("expected 1 alert template, got %d", len(assets.AlertTemplates))
	}
	if assets.Smartscape == nil {
		t.Error("expected Smartscape to be non-nil")
	}
}

func TestInspectAssets_UnknownType(t *testing.T) {
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml": alertYAML,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}
	_, err = InspectAssets(zipData, []string{"unknown_type"}, false)
	if err == nil {
		t.Error("expected error for unknown asset type")
	}
}

func TestInspectAssets_MissingManifest(t *testing.T) {
	zipData, err := buildTestZip(map[string][]byte{
		"some_file.txt": []byte("data"),
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}
	_, err = InspectAssets(zipData, []string{"alert_templates"}, false)
	if err == nil {
		t.Error("expected error for missing extension.yaml")
	}
}

func TestInspectAssets_NoAlerts(t *testing.T) {
	yaml := []byte(`name: com.example.test
version: "1.0.0"
`)
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml": yaml,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}
	assets, err := InspectAssets(zipData, []string{"alert_templates"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if len(assets.AlertTemplates) != 0 {
		t.Errorf("expected 0 alert templates, got %d", len(assets.AlertTemplates))
	}
}

func TestInspectAssets_NoOpenpipeline(t *testing.T) {
	yaml := []byte(`name: com.example.test
version: "1.0.0"
`)
	zipData, err := buildTestZip(map[string][]byte{
		"extension.yaml": yaml,
	})
	if err != nil {
		t.Fatalf("failed to build test zip: %v", err)
	}
	assets, err := InspectAssets(zipData, []string{"smartscape"}, false)
	if err != nil {
		t.Fatalf("InspectAssets returned error: %v", err)
	}
	if assets.Smartscape == nil {
		t.Fatal("expected Smartscape struct (empty)")
	}
	if len(assets.Smartscape.Nodes) != 0 || len(assets.Smartscape.Edges) != 0 {
		t.Error("expected empty nodes and edges when no openpipeline configured")
	}
}
