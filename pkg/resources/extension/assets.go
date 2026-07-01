package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

// AlertAsset is the display model for an alert template bundled in an extension.
type AlertAsset struct {
	File      string          `json:"file" table:"FILE"`
	Name      string          `json:"name" table:"NAME"`
	EventType string          `json:"eventType" table:"EVENT_TYPE"`
	Enabled   *bool           `json:"enabled" table:"ENABLED"`
	Content   json.RawMessage `json:"content,omitempty" table:"-"`
}

// SmartscapeNode is the display model for a node type extracted from an extension's pipelines.
type SmartscapeNode struct {
	NodeType        string          `json:"nodeType" table:"NODE_TYPE"`
	NodeIDFieldName string          `json:"nodeIdFieldName" table:"ID_FIELD"`
	Description     string          `json:"description,omitempty" table:"DESCRIPTION"`
	Pipeline        string          `json:"pipeline" table:"PIPELINE"`
	Content         json.RawMessage `json:"content,omitempty" table:"-"`
}

// SmartscapeEdge is the display model for a static edge defined in an extension's pipelines.
type SmartscapeEdge struct {
	SourceType string `json:"sourceType" table:"FROM"`
	EdgeType   string `json:"edgeType" table:"EDGE_TYPE"`
	TargetType string `json:"targetType" table:"TO"`
}

// SmartscapeAssetResult groups nodes and edges for the smartscape asset type.
type SmartscapeAssetResult struct {
	Nodes []SmartscapeNode `json:"nodes,omitempty"`
	Edges []SmartscapeEdge `json:"edges,omitempty"`
}

// assetTypeDirectories is kept for validation; both types now use extension.yaml as their source.
var assetTypeDirectories = map[string]struct{}{
	"alert_templates": {},
	"smartscape":      {},
}

// extensionManifest is the relevant subset of extension.yaml.
type extensionManifest struct {
	Alerts []struct {
		Path string `yaml:"path"`
	} `yaml:"alerts"`
	Openpipeline struct {
		Pipelines []struct {
			PipelinePath string `yaml:"pipelinePath"`
			DisplayName  string `yaml:"displayName"`
		} `yaml:"pipelines"`
	} `yaml:"openpipeline"`
}

// readManifest parses extension.yaml from the zip and returns the manifest.
func readManifest(zr *zip.Reader) (*extensionManifest, error) {
	for _, f := range zr.File {
		if f.Name != "extension.yaml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open extension.yaml: %w", err)
		}
		defer rc.Close()
		var m extensionManifest
		if err := yaml.NewDecoder(rc).Decode(&m); err != nil {
			return nil, fmt.Errorf("decode extension.yaml: %w", err)
		}
		return &m, nil
	}
	return nil, fmt.Errorf("extension.yaml not found in package")
}

const supportedAssetTypeNames = "alert_templates, smartscape"

// AssetResult holds all parsed assets grouped by type.
type AssetResult struct {
	AlertTemplates []AlertAsset           `json:"alert_templates,omitempty"`
	Smartscape     *SmartscapeAssetResult `json:"smartscape,omitempty"`
}

// ParseAssets parses the requested asset types from a (possibly nested) extension zip
// and returns the structured result. When full is true, each asset includes its
// complete file content.
func ParseAssets(zipData []byte, types []string, full bool) (*AssetResult, error) {
	for _, t := range types {
		if _, ok := assetTypeDirectories[strings.ToLower(t)]; !ok {
			return nil, fmt.Errorf("unknown asset type %q — supported: %s", t, supportedAssetTypeNames)
		}
	}

	inner, err := extractInnerZip(zipData)
	if err != nil {
		return nil, err
	}

	result := &AssetResult{}

	for _, t := range types {
		switch strings.ToLower(t) {
		case "alert_templates":
			if err := parseAlertTemplates(inner, result, full); err != nil {
				return nil, err
			}
		case "smartscape":
			if err := parseSmartscape(inner, result, full); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// extractInnerZip opens the outer zip and returns a zip.Reader for the nested
// extension.zip. If no nesting exists, the outer zip is returned as-is.
func extractInnerZip(data []byte) (*zip.Reader, error) {
	outer, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open extension zip: %w", err)
	}

	for _, f := range outer.File {
		if f.Name == "extension.zip" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open inner extension.zip: %w", err)
			}

			var buf bytes.Buffer
			if _, err := buf.ReadFrom(rc); err != nil {
				rc.Close()
				return nil, fmt.Errorf("failed to read inner extension.zip: %w", err)
			}
			rc.Close()
			inner := buf.Bytes()
			return zip.NewReader(bytes.NewReader(inner), int64(len(inner)))
		}
	}

	return outer, nil
}

// zipLookup returns the zip entry for path, trying exact match then suffix match.
// Suffix matching handles cases where paths inside extension.yaml include a
// directory prefix that differs from the zip entry name.
func zipLookup(fileByName map[string]*zip.File, path string) (*zip.File, bool) {
	if f, ok := fileByName[path]; ok {
		return f, true
	}
	suffix := "/" + path
	for name, f := range fileByName {
		if strings.HasSuffix(name, suffix) {
			return f, true
		}
	}
	return nil, false
}

func readRawJSON(f *zip.File) (json.RawMessage, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer rc.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(rc).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode %s: %w", f.Name, err)
	}
	return raw, nil
}

func parseAlertTemplates(zr *zip.Reader, result *AssetResult, full bool) error {
	manifest, err := readManifest(zr)
	if err != nil {
		return err
	}

	fileByName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		fileByName[f.Name] = f
	}

	for _, entry := range manifest.Alerts {
		f, ok := zipLookup(fileByName, entry.Path)
		if !ok {
			return fmt.Errorf("alert file listed in extension.yaml not found in package: %s", entry.Path)
		}
		raw, err := readRawJSON(f)
		if err != nil {
			return err
		}
		asset := AlertAsset{File: entry.Path}
		_ = json.Unmarshal(raw, &asset)
		if full {
			asset.Content = raw
		}
		result.AlertTemplates = append(result.AlertTemplates, asset)
	}
	return nil
}

// pipelineProcessor is the relevant subset of a smartscapeNodeExtraction processor.
type pipelineProcessor struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	SmartscapeNode struct {
		NodeType        string `json:"nodeType"`
		NodeIDFieldName string `json:"nodeIdFieldName"`
		ExtractNode     bool   `json:"extractNode"`
		StaticEdges     []struct {
			EdgeType          string `json:"edgeType"`
			TargetType        string `json:"targetType"`
			TargetIDFieldName string `json:"targetIdFieldName"`
		} `json:"staticEdgesToExtract"`
	} `json:"smartscapeNode"`
	RawContent json.RawMessage `json:"-"`
}

func parseSmartscape(zr *zip.Reader, result *AssetResult, full bool) error {
	manifest, err := readManifest(zr)
	if err != nil {
		return err
	}

	fileByName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		fileByName[f.Name] = f
	}

	sc := &SmartscapeAssetResult{}
	seenNodes := map[string]bool{}
	seenEdges := map[string]bool{}

	for _, pipeline := range manifest.Openpipeline.Pipelines {
		f, ok := zipLookup(fileByName, pipeline.PipelinePath)
		if !ok {
			slog.Warn("pipeline file listed in extension.yaml not found in package, skipping", "path", pipeline.PipelinePath)
			continue
		}

		var doc struct {
			SmartscapeNodeExtraction struct {
				Processors []json.RawMessage `json:"processors"`
			} `json:"smartscapeNodeExtraction"`
		}
		raw, err := readRawJSON(f)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("parse pipeline %s: %w", pipeline.PipelinePath, err)
		}

		for _, procRaw := range doc.SmartscapeNodeExtraction.Processors {
			var proc pipelineProcessor
			if err := json.Unmarshal(procRaw, &proc); err != nil {
				continue
			}
			if proc.Type != "smartscapeNode" || !proc.SmartscapeNode.ExtractNode {
				continue
			}
			proc.RawContent = procRaw

			nodeType := proc.SmartscapeNode.NodeType
			if !seenNodes[nodeType] {
				seenNodes[nodeType] = true
				node := SmartscapeNode{
					NodeType:        nodeType,
					NodeIDFieldName: proc.SmartscapeNode.NodeIDFieldName,
					Description:     proc.Description,
					Pipeline:        pipeline.PipelinePath,
				}
				if full {
					node.Content = procRaw
				}
				sc.Nodes = append(sc.Nodes, node)
			}

			for _, e := range proc.SmartscapeNode.StaticEdges {
				key := nodeType + "|" + e.EdgeType + "|" + e.TargetType
				if !seenEdges[key] {
					seenEdges[key] = true
					sc.Edges = append(sc.Edges, SmartscapeEdge{
						SourceType: nodeType,
						EdgeType:   e.EdgeType,
						TargetType: e.TargetType,
					})
				}
			}
		}
	}

	result.Smartscape = sc
	return nil
}
