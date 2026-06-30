package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// extensionManifest is the parsed representation of extension.yaml,
// limited to the fields needed for asset inspection.
type extensionManifest struct {
	Alerts       []string          `yaml:"alerts"`
	Openpipeline *openpipelineConf `yaml:"openpipeline"`
}

type openpipelineConf struct {
	Pipelines []string `yaml:"pipelines"`
}

// AlertTemplate holds a parsed alert template from an extension package.
type AlertTemplate struct {
	Name      string          `json:"name" yaml:"name" table:"NAME"`
	EventType string          `json:"eventType" yaml:"eventType" table:"EVENT_TYPE"`
	Enabled   *bool           `json:"enabled" yaml:"enabled" table:"ENABLED"`
	Content   json.RawMessage `json:"content,omitempty" yaml:"content,omitempty" table:"-"`
}

// SmartscapeNode holds a deduplicated smartscape node type extracted from pipeline files.
type SmartscapeNode struct {
	NodeType string          `json:"nodeType" yaml:"nodeType" table:"NODE_TYPE"`
	Content  json.RawMessage `json:"content,omitempty" yaml:"content,omitempty" table:"-"`
}

// SmartscapeEdge holds a deduplicated smartscape edge type extracted from pipeline files.
type SmartscapeEdge struct {
	SourceType string `json:"sourceType" yaml:"sourceType" table:"SOURCE_TYPE"`
	EdgeType   string `json:"edgeType" yaml:"edgeType" table:"EDGE_TYPE"`
	TargetType string `json:"targetType" yaml:"targetType" table:"TARGET_TYPE"`
}

// SmartscapeAssets groups nodes and edges extracted from pipeline files.
type SmartscapeAssets struct {
	Nodes []SmartscapeNode `json:"nodes" yaml:"nodes"`
	Edges []SmartscapeEdge `json:"edges" yaml:"edges"`
}

// ExtensionAssets holds the inspected asset types from an extension package.
type ExtensionAssets struct {
	AlertTemplates []AlertTemplate   `json:"alert_templates,omitempty" yaml:"alert_templates,omitempty"`
	Smartscape     *SmartscapeAssets `json:"smartscape,omitempty" yaml:"smartscape,omitempty"`
}

// zipFS is a simple in-memory virtual file system over a zip archive.
type zipFS struct {
	files map[string][]byte
}

// openZipFS opens a zip archive from raw bytes and returns a zipFS.
func openZipFS(data []byte) (*zipFS, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	fs := &zipFS{files: make(map[string][]byte)}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read zip entry %q: %w", f.Name, err)
		}
		fs.files[f.Name] = content
	}
	return fs, nil
}

// get returns the raw bytes of a file in the zip, or (nil, false) if not found.
func (fs *zipFS) get(name string) ([]byte, bool) {
	// Exact match first.
	if b, ok := fs.files[name]; ok {
		return b, true
	}
	// Suffix match for paths that may have a leading directory component.
	suffix := "/" + name
	for k, v := range fs.files {
		if k == name || strings.HasSuffix(k, suffix) {
			return v, true
		}
	}
	return nil, false
}

// resolveFS opens the effective inner zip from an outer zip.
// Dynatrace extension packages are outer zips that contain an inner
// extension.zip with the actual files.  If the inner zip is not present
// (flat layout) the outer zip is used directly.
func resolveFS(outerData []byte) (*zipFS, error) {
	outer, err := openZipFS(outerData)
	if err != nil {
		return nil, fmt.Errorf("open extension package: %w", err)
	}
	if innerData, ok := outer.get("extension.zip"); ok {
		inner, err := openZipFS(innerData)
		if err != nil {
			return nil, fmt.Errorf("open inner extension.zip: %w", err)
		}
		return inner, nil
	}
	return outer, nil
}

// InspectAssets parses an extension package zip (raw bytes) and returns the
// requested asset types.  assetTypes is a slice of type names such as
// "alert_templates" or "smartscape".  When full is true, raw file content is
// attached to each item.
func InspectAssets(packageData []byte, assetTypes []string, full bool) (*ExtensionAssets, error) {
	fs, err := resolveFS(packageData)
	if err != nil {
		return nil, err
	}

	// Parse extension.yaml
	manifestData, ok := fs.get("extension.yaml")
	if !ok {
		return nil, fmt.Errorf("extension.yaml not found in package")
	}
	var manifest extensionManifest
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse extension.yaml: %w", err)
	}

	result := &ExtensionAssets{}

	for _, assetType := range assetTypes {
		switch strings.ToLower(strings.TrimSpace(assetType)) {
		case "alert_templates":
			templates, err := extractAlertTemplates(fs, manifest.Alerts, full)
			if err != nil {
				return nil, fmt.Errorf("extract alert_templates: %w", err)
			}
			result.AlertTemplates = templates

		case "smartscape":
			var pipelinePaths []string
			if manifest.Openpipeline != nil {
				pipelinePaths = manifest.Openpipeline.Pipelines
			}
			sc, err := extractSmartscape(fs, pipelinePaths, full)
			if err != nil {
				return nil, fmt.Errorf("extract smartscape: %w", err)
			}
			result.Smartscape = sc

		default:
			return nil, fmt.Errorf("unknown asset type %q (supported: alert_templates, smartscape)", assetType)
		}
	}

	return result, nil
}

// extractAlertTemplates reads alert template files listed in the manifest.
func extractAlertTemplates(fs *zipFS, paths []string, full bool) ([]AlertTemplate, error) {
	var templates []AlertTemplate
	for _, path := range paths {
		data, ok := fs.get(path)
		if !ok {
			return nil, fmt.Errorf("alert template file %q not found in package", path)
		}

		// Parse the alert template JSON for the summary fields.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse alert template %q: %w", path, err)
		}

		t := AlertTemplate{}

		if v, ok := raw["name"]; ok {
			_ = json.Unmarshal(v, &t.Name)
		}
		if v, ok := raw["eventType"]; ok {
			_ = json.Unmarshal(v, &t.EventType)
		}
		if v, ok := raw["enabled"]; ok {
			var b bool
			if err := json.Unmarshal(v, &b); err == nil {
				t.Enabled = &b
			}
		}
		if full {
			t.Content = json.RawMessage(data)
		}

		templates = append(templates, t)
	}
	return templates, nil
}

// smartscapeProcessor is the minimal schema of a processor entry used for
// both node and edge extraction.
type smartscapeProcessor struct {
	ExtractNode bool   `json:"extractNode"`
	NodeType    string `json:"nodeType"`
	SourceType  string `json:"sourceType"`
	EdgeType    string `json:"edgeType"`
	TargetType  string `json:"targetType"`
}

// extractSmartscape reads pipeline files and returns deduplicated nodes and edges.
func extractSmartscape(fs *zipFS, pipelinePaths []string, full bool) (*SmartscapeAssets, error) {
	nodesSeen := map[string]bool{}
	edgesSeen := map[string]bool{}
	var nodes []SmartscapeNode
	var edges []SmartscapeEdge

	for _, path := range pipelinePaths {
		data, ok := fs.get(path)
		if !ok {
			// Warn-and-continue: a missing pipeline file is not fatal.
			continue
		}

		// Parse the pipeline JSON to extract smartscapeNodeExtraction.processors.
		var pipeline struct {
			SmartscapeNodeExtraction *struct {
				Processors []json.RawMessage `json:"processors"`
			} `json:"smartscapeNodeExtraction"`
		}
		if err := json.Unmarshal(data, &pipeline); err != nil {
			return nil, fmt.Errorf("parse pipeline file %q: %w", path, err)
		}
		if pipeline.SmartscapeNodeExtraction == nil {
			continue
		}

		for _, rawProc := range pipeline.SmartscapeNodeExtraction.Processors {
			var proc smartscapeProcessor
			if err := json.Unmarshal(rawProc, &proc); err != nil {
				continue
			}

			// Nodes: processors with extractNode: true, deduplicated by nodeType.
			if proc.ExtractNode && proc.NodeType != "" {
				if !nodesSeen[proc.NodeType] {
					nodesSeen[proc.NodeType] = true
					n := SmartscapeNode{NodeType: proc.NodeType}
					if full {
						n.Content = rawProc
					}
					nodes = append(nodes, n)
				}
			}

			// Edges: processors that carry sourceType/targetType, deduplicated by
			// (sourceType, edgeType, targetType).
			if proc.SourceType != "" && proc.TargetType != "" {
				key := proc.SourceType + "|" + proc.EdgeType + "|" + proc.TargetType
				if !edgesSeen[key] {
					edgesSeen[key] = true
					edges = append(edges, SmartscapeEdge{
						SourceType: proc.SourceType,
						EdgeType:   proc.EdgeType,
						TargetType: proc.TargetType,
					})
				}
			}
		}
	}

	return &SmartscapeAssets{
		Nodes: nodes,
		Edges: edges,
	}, nil
}
