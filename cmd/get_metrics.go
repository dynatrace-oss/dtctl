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
	Short:   "List available metric timeseries with metadata",
	Long: `List available metric timeseries, enriched with metadata (name, unit,
description, kind) joined from the metrics catalog.

Each record represents a unique timeseries tuple (metric key + dimension
combination). Metadata fields are prefixed with "metadata.".

Value types for --dimension:
  strings must be double-quoted:  key="value"
  booleans and integers unquoted: failed=false  http.response.status_code=200
  smartscape entity IDs detected: dt.smartscape.service=SERVICE-HEXID16

Examples:
  # List all metric timeseries with metadata
  dtctl get metrics

  # Filter by metric keys
  dtctl get metrics --metric-keys dt.service.request.count,dt.service.messaging.process.count

  # Filter by dimensions
  dtctl get metrics --dimension dt.smartscape.service=SERVICE-440469AFB753DD1A
  dtctl get metrics --dimension 'dt.process_group.detected_name="com.example.MyService"'
  dtctl get metrics --dimension failed=false
  dtctl get metrics --dimension http.response.status_code=200

  # Combine filters
  dtctl get metrics \
    --metric-keys dt.service.request.count,dt.service.messaging.process.count \
    --dimension dt.smartscape.service=SERVICE-440469AFB753DD1A \
    --dimension 'dt.process_group.detected_name="com.example.MyService"'

  # Output as JSON
  dtctl get metrics -o json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, c, err := SetupClient()
		if err != nil {
			return err
		}

		metricKeys, _ := cmd.Flags().GetStringSlice("metric-keys")
		dimensions, _ := cmd.Flags().GetStringArray("dimension")

		query, err := buildMetricsQuery(metricKeys, dimensions)
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

func buildMetricsQuery(metricKeys []string, dimensions []string) (string, error) {
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

	if len(dimensions) > 0 {
		filters := make([]string, 0, len(dimensions))
		for _, dim := range dimensions {
			f, err := dimensionFilter(dim)
			if err != nil {
				return "", err
			}
			filters = append(filters, f)
		}
		sb.WriteString("\n| filter ")
		sb.WriteString(strings.Join(filters, " and "))
	}

	sb.WriteString("\n| join [ load \"/dt/platform/metrics.metadata\" ], on: { metric.key }, prefix: \"metadata.\", kind: leftOuter")

	return sb.String(), nil
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
	return "", fmt.Errorf("--dimension value %q is ambiguous: wrap strings in double quotes (e.g. key=%q)", val, val)
}

func init() {
	getMetricsCmd.Flags().StringSlice("metric-keys", nil, "filter by metric keys (comma-separated)")
	getMetricsCmd.Flags().StringArray("dimension", nil, `filter by dimension (key=value, repeatable)
strings must be quoted: key="value"; booleans/integers unquoted: failed=false`)
}
