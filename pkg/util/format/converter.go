package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents the data format
type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// DetectFormat auto-detects whether the data is JSON or YAML
func DetectFormat(data []byte) (Format, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty data")
	}

	// Trim whitespace for detection
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("empty data")
	}

	// JSON typically starts with { or [
	firstChar := trimmed[0]
	if firstChar == '{' || firstChar == '[' {
		// Try to parse as JSON to confirm
		var js interface{}
		if err := json.Unmarshal(trimmed, &js); err == nil {
			return FormatJSON, nil
		}
	}

	// Try parsing as YAML (YAML is a superset of JSON, so also validates JSON)
	var y interface{}
	if err := yaml.Unmarshal(trimmed, &y); err == nil {
		// It's valid YAML (could also be JSON)
		// If we got here and it's not JSON, it's YAML
		return FormatYAML, nil
	}

	return "", fmt.Errorf("could not detect format: invalid JSON or YAML")
}

// YAMLToJSON converts YAML data to JSON
func YAMLToJSON(yamlData []byte) ([]byte, error) {
	// Parse YAML
	var data interface{}
	if err := yaml.Unmarshal(yamlData, &data); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	// Convert to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON: %w", err)
	}

	return jsonData, nil
}

// JSONToYAML converts JSON data to YAML
func JSONToYAML(jsonData []byte) ([]byte, error) {
	// Parse JSON
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Convert to YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(data); err != nil {
		return nil, fmt.Errorf("failed to convert to YAML: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to close YAML encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// YAMLNodeFromJSON renders a value through its JSON representation so that
// yaml.Marshal produces output structurally identical to json.Marshal.
//
// It is the canonical implementation for a type's MarshalYAML when the type
// relies on JSON struct tags for its wire shape (json:"-" to hide display-only
// fields, omitempty, camelCase keys) and/or embeds []byte or json.RawMessage
// fields. Without it, yaml.v3 falls back to reflection: struct tags are ignored
// (keys are lowercased, omitempty is not honored, json:"-" fields leak in) and
// []byte/json.RawMessage fields are emitted as a sequence of raw byte values.
//
// Numbers keep their JSON token: decoding them into float64 would round an
// integer above 2^53, and a document printed with `-o yaml` and applied back
// must carry the value the server sent.
//
// json.Marshal never invokes MarshalYAML, so calling this from a MarshalYAML
// method does not recurse.
func YAMLNodeFromJSON(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return exactNumbers(out), nil
}

// exactNumbers replaces every json.Number in a decoded JSON value with a YAML
// scalar node carrying the original token. Maps and slices stay plain Go
// values, so yaml.v3 orders and lays them out exactly as before.
func exactNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = exactNumbers(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = exactNumbers(child)
		}
		return t
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(string(t), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: string(t)}
	default:
		return v
	}
}

// ValidateAndConvert validates input data and converts it to JSON
// It auto-detects the format and converts YAML to JSON if needed
func ValidateAndConvert(data []byte) ([]byte, error) {
	format, err := DetectFormat(data)
	if err != nil {
		return nil, err
	}

	switch format {
	case FormatJSON:
		// Already JSON, just validate and return
		var js interface{}
		if err := json.Unmarshal(data, &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return data, nil

	case FormatYAML:
		// Convert YAML to JSON
		jsonData, err := YAMLToJSON(data)
		if err != nil {
			return nil, err
		}
		return jsonData, nil

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// PrettyJSON formats JSON with indentation
func PrettyJSON(jsonData []byte) ([]byte, error) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, jsonData, "", "  "); err != nil {
		return nil, fmt.Errorf("failed to format JSON: %w", err)
	}
	return prettyJSON.Bytes(), nil
}

// PrettyYAML formats YAML with proper indentation
func PrettyYAML(yamlData []byte) ([]byte, error) {
	// Parse YAML
	var data interface{}
	if err := yaml.Unmarshal(yamlData, &data); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	// Re-encode with proper formatting
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(data); err != nil {
		return nil, fmt.Errorf("failed to format YAML: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to close YAML encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// GetExtension returns the file extension for the format
func GetExtension(format string) string {
	switch strings.ToLower(format) {
	case "yaml", "yml":
		return ".yaml"
	case "json":
		return ".json"
	default:
		return ".txt"
	}
}
