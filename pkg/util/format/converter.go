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
