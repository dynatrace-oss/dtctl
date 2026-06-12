package output

import (
	"fmt"

	"github.com/itchyny/gojq"
)

// IsStructuredOutputFormat reports whether a format can represent arbitrary
// JSON values emitted by jq.
func IsStructuredOutputFormat(format string) bool {
	switch format {
	case "json", "yaml", "yml", "toon":
		return true
	default:
		return false
	}
}

// NormalizeJQOutputFormat promotes non-structured formats to json when --jq is used.
func NormalizeJQOutputFormat(format string) string {
	if IsStructuredOutputFormat(format) {
		return format
	}
	return "json"
}

// ApplyJQ transforms input using the provided jq filter.
// If filter is empty, input is returned unchanged.
func ApplyJQ(filter string, input interface{}) (interface{}, error) {
	if filter == "" {
		return input, nil
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}

	generic, err := toGeneric(input)
	if err != nil {
		return nil, fmt.Errorf("failed to apply --jq filter: %w", err)
	}

	iter := code.Run(generic)
	results := make([]interface{}, 0, 1)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if runErr, ok := v.(error); ok {
			return nil, fmt.Errorf("failed to apply --jq filter: %w", runErr)
		}
		results = append(results, v)
	}

	switch len(results) {
	case 0:
		return nil, nil
	case 1:
		return results[0], nil
	default:
		return results, nil
	}
}
