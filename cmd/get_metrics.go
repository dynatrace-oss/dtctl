package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/exec"
)

// smartscapeIDPattern matches Dynatrace smartscape entity IDs (e.g. SERVICE-440469AFB753DD1A).
// These are auto-detected so users don't need to quote them.
var smartscapeIDPattern = regexp.MustCompile(`^[A-Z][A-Z_]*-[0-9A-F]{16}$`)

var getMetricsCmd = &cobra.Command{
	Use:     "metrics",
	Aliases: []string{"metric"},
	Short:   "List available metric keys with unit and kind",
	Long: `List available metric keys, optionally scoped to a dimension filter,
with unit and kind joined from the catalog.

Each record represents one unique metric key. Use --dimension or
--string-dimension to scope results to a specific service, host, or other
dimension — the filter applies before deduplication, so only metric keys
actually emitted by that entity are returned.

Use -o wide to also include the metric display name and description.

Two dimension filter flags are available:

  --string-dimension key=value
    Always treated as a string match (~ operator). No inner quoting needed.
    Preferred for entity IDs and plain strings (agent-friendly).

  --dimension key=value
    Typed: booleans and integers unquoted, strings must be double-quoted.
    Smartscape entity IDs are auto-detected and need no quotes.

Examples:
  # All metric keys in the environment
  dtctl get metrics

  # Metric keys available for a specific service
  dtctl get metrics --string-dimension dt.smartscape.service=SERVICE-440469AFB753DD1A

  # Metric keys for a process group
  dtctl get metrics --string-dimension dt.process_group.detected_name=com.example.MyService

  # Narrow to specific keys
  dtctl get metrics --metric-keys dt.service.request.count,dt.service.request.response_time

  # Wide: adds display name and description
  dtctl get metrics --string-dimension dt.smartscape.service=SERVICE-440469AFB753DD1A -o wide

  # JSON for agent consumption
  dtctl get metrics --string-dimension dt.smartscape.service=SERVICE-440469AFB753DD1A -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		metricKeys, _ := cmd.Flags().GetStringSlice("metric-keys")
		dimensions, _ := cmd.Flags().GetStringArray("dimension")
		stringDimensions, _ := cmd.Flags().GetStringArray("string-dimension")

		query, err := buildMetricsQuery(metricKeys, dimensions, stringDimensions, outputFormat == "wide")
		if err != nil {
			return err
		}

		executor := NewDQLExecutorFromConfig(cfg, c)
		opts := exec.DQLExecuteOptions{
			OutputFormat: outputFormat,
			JQFilter:     jqFilter,
			AgentMode:    agentMode,
		}
		return executor.ExecuteWithContext(context.Background(), query, opts)
	},
}

func buildMetricsQuery(metricKeys []string, dimensions []string, stringDimensions []string, wide bool) (string, error) {
	var sb strings.Builder
	sb.WriteString("metrics")

	if len(metricKeys) > 0 {
		sb.WriteString("\n| filter in(metric.key, {")
		for i, k := range metricKeys {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q", k)
		}
		sb.WriteString("})")
	}

	var filters []string
	for _, dim := range dimensions {
		f, err := dimensionFilter(dim)
		if err != nil {
			return "", err
		}
		filters = append(filters, f)
	}
	for _, dim := range stringDimensions {
		f, err := stringDimensionFilter(dim)
		if err != nil {
			return "", err
		}
		filters = append(filters, f)
	}
	if len(filters) > 0 {
		sb.WriteString("\n| filter ")
		sb.WriteString(strings.Join(filters, " and "))
	}

	sb.WriteString("\n| dedup metric.key")
	sb.WriteString("\n| fields metric.key")

	if wide {
		sb.WriteString("\n| join [ load \"/dt/platform/metrics.metadata\" | fields metric.key, name, unit, kind, description ], on: { metric.key }, prefix: \"metadata.\", kind: leftOuter")
	} else {
		sb.WriteString("\n| join [ load \"/dt/platform/metrics.metadata\" | fields metric.key, unit, kind ], on: { metric.key }, prefix: \"metadata.\", kind: leftOuter")
	}
	sb.WriteString("\n| fieldsRemove `metadata.metric.key`")

	return sb.String(), nil
}

// stringDimensionFilter converts a key=value flag into a DQL ~ filter.
// The value is always treated as a string — no inner quoting required.
func stringDimensionFilter(dim string) (string, error) {
	eqIdx := strings.Index(dim, "=")
	if eqIdx < 0 {
		return "", fmt.Errorf("invalid --string-dimension %q: expected key=value", dim)
	}
	key := strings.TrimSpace(dim[:eqIdx])
	val := strings.TrimSpace(dim[eqIdx+1:])
	if key == "" {
		return "", fmt.Errorf("invalid --string-dimension %q: key must not be empty", dim)
	}
	return fmt.Sprintf("%s ~ %q", key, val), nil
}

// dimensionFilter converts a key=value flag into a DQL filter expression.
//
// Operator selection:
//   - Quoted strings and smartscape entity IDs use ~ (entity/phrase match)
//   - Booleans and integers use == (exact)
//   - Anything else unquoted is rejected — wrap strings in double quotes
func dimensionFilter(dim string) (string, error) {
	eqIdx := strings.Index(dim, "=")
	if eqIdx < 0 {
		return "", fmt.Errorf("invalid --dimension %q: expected key=value", dim)
	}
	key := strings.TrimSpace(dim[:eqIdx])
	val := strings.TrimSpace(dim[eqIdx+1:])
	if key == "" {
		return "", fmt.Errorf("invalid --dimension %q: key must not be empty", dim)
	}

	// Quoted string → ~
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		return fmt.Sprintf("%s ~ %q", key, val[1:len(val)-1]), nil
	}
	// Boolean → ==
	if val == "true" || val == "false" {
		return fmt.Sprintf("%s == %s", key, val), nil
	}
	// Smartscape entity ID (auto-detected, no quotes required) → ~
	if smartscapeIDPattern.MatchString(val) {
		return fmt.Sprintf("%s ~ %q", key, val), nil
	}
	// Integer → ==
	if _, err := strconv.ParseInt(val, 10, 64); err == nil {
		return fmt.Sprintf("%s == %s", key, val), nil
	}
	return "", fmt.Errorf("--dimension value %q is ambiguous: wrap string values in double quotes, e.g. --dimension '%s=\"%s\"'", val, key, val)
}

func init() {
	getMetricsCmd.Flags().StringSlice("metric-keys", nil, "filter by metric keys (comma-separated)")
	getMetricsCmd.Flags().StringArray("dimension", nil, `filter by dimension (key=value, repeatable); strings must be double-quoted: key="value"; booleans/integers unquoted`)
	getMetricsCmd.Flags().StringArray("string-dimension", nil, "filter by string dimension (key=value, repeatable); value always treated as string, no quoting needed")
}
