package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/guptarohit/asciigraph"
)

const (
	// MaxSeriesCount is the maximum number of series to display in a chart
	MaxSeriesCount = 10
	// DefaultChartHeight is the default height of the chart
	DefaultChartHeight = 15
	// DefaultChartWidth is the default width for all chart visualizations
	DefaultChartWidth = 100
)

// ChartPrinter prints timeseries data as ASCII charts
type ChartPrinter struct {
	writer io.Writer
	height int
	width  int
}

// NewChartPrinter creates a new chart printer
func NewChartPrinter(writer io.Writer) *ChartPrinter {
	// Auto-detect terminal dimensions
	width, height := GetTerminalSize()
	// Leave margin for y-axis labels and headers
	// Headers: live mode (2 lines) + timeframe (2 lines) + legend (~3 lines) = 7 lines
	width = width - 15
	height = height - 12
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}
	return &ChartPrinter{
		writer: writer,
		height: height,
		width:  width,
	}
}

// NewChartPrinterWithSize creates a new chart printer with custom dimensions
func NewChartPrinterWithSize(writer io.Writer, width, height int) *ChartPrinter {
	if width <= 0 {
		width = DefaultChartWidth
	}
	if height <= 0 {
		height = DefaultChartHeight
	}
	return &ChartPrinter{
		writer: writer,
		height: height,
		width:  width,
	}
}

// Series represents a single timeseries
type Series struct {
	Name   string
	Label  string
	Values []float64
}

// TimeseriesData represents extracted timeseries data
type TimeseriesData struct {
	Start    time.Time
	End      time.Time
	Interval time.Duration
	Series   []Series
}

// Print prints the data as a chart if it contains timeseries, otherwise falls back to JSON
func (p *ChartPrinter) Print(obj interface{}) error {
	ts, err := p.extractTimeseries(obj)
	if err != nil {
		fmt.Fprintf(p.writer, "Warning: %v. Falling back to JSON output.\n\n", err)
		return (&JSONPrinter{writer: p.writer}).Print(obj)
	}

	return p.renderChart(ts)
}

// PrintList prints a list of records as charts
func (p *ChartPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}

// extractTimeseries attempts to extract timeseries data from the input
func (p *ChartPrinter) extractTimeseries(obj interface{}) (*TimeseriesData, error) {
	// Handle different input types
	switch v := obj.(type) {
	case map[string]interface{}:
		// Check if this is a DQL response with records ([]interface{})
		if records, ok := v["records"].([]interface{}); ok {
			return p.extractFromRecords(records)
		}
		// Check if this is a DQL response with records ([]map[string]interface{})
		if records, ok := v["records"].([]map[string]interface{}); ok {
			ifaces := make([]interface{}, len(records))
			for i, r := range records {
				ifaces[i] = r
			}
			return p.extractFromRecords(ifaces)
		}
		// Check if this is an analyzer result
		if result, ok := v["result"].(map[string]interface{}); ok {
			return p.extractFromAnalyzerResult(result)
		}
		// Try as a single record
		return p.extractFromRecord(v)

	case []map[string]interface{}:
		records := make([]interface{}, len(v))
		for i, r := range v {
			records[i] = r
		}
		return p.extractFromRecords(records)

	case []interface{}:
		return p.extractFromRecords(v)

	default:
		// Use reflection for struct types
		return p.extractFromReflect(obj)
	}
}

// extractFromRecords extracts timeseries from a slice of records
func (p *ChartPrinter) extractFromRecords(records []interface{}) (*TimeseriesData, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records found")
	}

	// Each record may be a separate timeseries (e.g., grouped by dimension)
	var allSeries []Series
	var start, end time.Time
	var interval time.Duration
	var lastErr error

	for _, rec := range records {
		record, ok := rec.(map[string]interface{})
		if !ok {
			lastErr = fmt.Errorf("record is not map[string]interface{}, got %T", rec)
			continue
		}

		ts, err := p.extractFromRecord(record)
		if err != nil {
			lastErr = err
			continue
		}

		// Use timeframe from first valid record
		if start.IsZero() {
			start = ts.Start
			end = ts.End
			interval = ts.Interval
		}

		allSeries = append(allSeries, ts.Series...)
	}

	if len(allSeries) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("no timeseries data found in records: %w", lastErr)
		}
		return nil, fmt.Errorf("no timeseries data found in records")
	}

	// Limit series count
	if len(allSeries) > MaxSeriesCount {
		fmt.Fprintf(p.writer, "Warning: Found %d series, showing first %d. Use filters to reduce.\n\n",
			len(allSeries), MaxSeriesCount)
		allSeries = allSeries[:MaxSeriesCount]
	}

	return &TimeseriesData{
		Start:    start,
		End:      end,
		Interval: interval,
		Series:   allSeries,
	}, nil
}

// extractFromRecord extracts timeseries from a single record (DQL timeseries result)
func (p *ChartPrinter) extractFromRecord(record map[string]interface{}) (*TimeseriesData, error) {
	// Check for timeframe and interval (DQL timeseries format)
	timeframe, hasTimeframe := record["timeframe"].(map[string]interface{})
	intervalStr, hasInterval := record["interval"].(string)

	if !hasTimeframe || !hasInterval {
		return nil, fmt.Errorf("record does not contain timeseries data (missing timeframe or interval)")
	}

	// Parse timeframe
	start, end, err := parseTimeframe(timeframe)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timeframe: %w", err)
	}

	// Parse interval (nanoseconds as string)
	var intervalNs int64
	_, _ = fmt.Sscanf(intervalStr, "%d", &intervalNs)
	interval := time.Duration(intervalNs)

	// Find metric columns (arrays of numbers)
	var series []Series
	dimensionLabel := extractDimensionLabel(record)

	for key, value := range record {
		if key == "timeframe" || key == "interval" {
			continue
		}

		// Check if it's an array of numbers
		values, ok := extractFloatArray(value)
		if !ok || len(values) == 0 {
			continue
		}

		s := Series{
			Name:   key,
			Label:  dimensionLabel,
			Values: values,
		}
		if dimensionLabel != "" {
			s.Label = dimensionLabel
		} else {
			s.Label = key
		}
		series = append(series, s)
	}

	if len(series) == 0 {
		return nil, fmt.Errorf("no numeric timeseries columns found")
	}

	// Sort series by name for consistent output
	sort.Slice(series, func(i, j int) bool {
		return series[i].Name < series[j].Name
	})

	return &TimeseriesData{
		Start:    start,
		End:      end,
		Interval: interval,
		Series:   series,
	}, nil
}

// extractFromAnalyzerResult extracts timeseries from analyzer output
func (p *ChartPrinter) extractFromAnalyzerResult(result map[string]interface{}) (*TimeseriesData, error) {
	// Analyzer results have output array
	output, ok := result["output"].([]interface{})
	if !ok {
		// Try data array
		output, ok = result["data"].([]interface{})
	}
	if !ok || len(output) == 0 {
		return nil, fmt.Errorf("analyzer result does not contain output data")
	}

	// Try to extract timeseries from the nested structure
	var allRecords []interface{}

	for _, item := range output {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Try direct records (simple case)
		if _, hasTimeframe := itemMap["timeframe"]; hasTimeframe {
			allRecords = append(allRecords, item)
			continue
		}

		// Helper to extract records from a query structure
		extractRecordsFromQuery := func(query map[string]interface{}) []interface{} {
			// Try expression.records path
			if expr, ok := query["expression"].(map[string]interface{}); ok {
				if records, ok := expr["records"].([]interface{}); ok {
					return records
				}
			}
			// Try direct records path
			if records, ok := query["records"].([]interface{}); ok {
				return records
			}
			return nil
		}

		// Try various query paths used by different analyzers
		queryPaths := []string{
			"analyzedTimeSeriesQuery",
			"forecastedTimeSeriesQuery",
			"timeSeriesDataWithPredictions",
		}

		for _, path := range queryPaths {
			if query, ok := itemMap[path].(map[string]interface{}); ok {
				if records := extractRecordsFromQuery(query); records != nil {
					allRecords = append(allRecords, records...)
				}
			}
		}
	}

	if len(allRecords) == 0 {
		return nil, fmt.Errorf("no timeseries data found in analyzer output")
	}

	return p.extractFromRecords(allRecords)
}

// extractFromReflect uses reflection to extract timeseries from struct types
func (p *ChartPrinter) extractFromReflect(obj interface{}) (*TimeseriesData, error) {
	// Convert struct to map via JSON to get consistent types for type assertions
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %w", err)
	}

	// Check if this is an analyzer result with nested "result" field
	if result, ok := data["result"].(map[string]interface{}); ok {
		return p.extractFromAnalyzerResult(result)
	}

	// Try to extract directly (but avoid infinite recursion)
	// Check for records or other timeseries indicators
	if _, hasRecords := data["records"]; hasRecords {
		return p.extractTimeseries(data)
	}

	return nil, fmt.Errorf("could not extract timeseries from struct: no result or records field found")
}

// parseTimeframe parses start and end times from a timeframe map
func parseTimeframe(tf map[string]interface{}) (time.Time, time.Time, error) {
	startStr, ok := tf["start"].(string)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("missing start time")
	}
	endStr, ok := tf["end"].(string)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("missing end time")
	}

	// Try multiple time formats
	timeFormats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000000Z",
		"2006-01-02T15:04Z", // Short format used by some analyzers
		"2006-01-02T15:04:05Z",
	}

	var start time.Time
	var err error
	for _, format := range timeFormats {
		start, err = time.Parse(format, startStr)
		if err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse start time %q: %w", startStr, err)
	}

	var end time.Time
	for _, format := range timeFormats {
		end, err = time.Parse(format, endStr)
		if err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse end time %q: %w", endStr, err)
	}

	return start, end, nil
}

// extractDimensionLabel extracts a label from dimension columns (e.g., dt.entity.host)
func extractDimensionLabel(record map[string]interface{}) string {
	// Common dimension keys
	dimensionKeys := []string{
		"dt.entity.host",
		"dt.entity.service",
		"dt.entity.process_group_instance",
		"dt.entity.process_group",
		"host.name",
		"service.name",
	}

	for _, key := range dimensionKeys {
		if val, ok := record[key].(string); ok && val != "" {
			return val
		}
	}

	// Look for any string field that might be a dimension
	for key, val := range record {
		if key == "timeframe" || key == "interval" {
			continue
		}
		if str, ok := val.(string); ok && str != "" {
			return str
		}
	}

	return ""
}

// extractFloatArray extracts a float64 array from an interface value
func extractFloatArray(value interface{}) ([]float64, bool) {
	switch v := value.(type) {
	case []float64:
		return v, true
	case []interface{}:
		result := make([]float64, len(v))
		for i, item := range v {
			switch n := item.(type) {
			case float64:
				result[i] = n
			case int:
				result[i] = float64(n)
			case int64:
				result[i] = float64(n)
			case nil:
				result[i] = math.NaN()
			default:
				return nil, false
			}
		}
		return result, true
	default:
		return nil, false
	}
}

// renderChart renders the timeseries data as an ASCII chart with btop-inspired styling
func (p *ChartPrinter) renderChart(ts *TimeseriesData) error {
	// Print styled header with timeframe (use \r\n for raw terminal mode)
	timeFormat := "2006-01-02 15:04"
	fmt.Fprintf(p.writer, "%s%s%s %s─%s %s%s%s\r\n\r\n",
		BrightCyan, ts.Start.UTC().Format(timeFormat), Reset,
		Dim, Reset,
		BrightCyan, ts.End.UTC().Format(timeFormat), Reset)

	var graph string
	if len(ts.Series) == 1 {
		// Single series
		s := ts.Series[0]
		graph = asciigraph.Plot(s.Values,
			asciigraph.Height(p.height),
			asciigraph.Width(p.width),
			asciigraph.Caption(s.Name),
		)
	} else {
		// Multiple series
		data := make([][]float64, len(ts.Series))
		legends := make([]string, len(ts.Series))
		colors := getSeriesColors(len(ts.Series))

		for i, s := range ts.Series {
			data[i] = s.Values
			legends[i] = s.Label
		}

		graph = asciigraph.PlotMany(data,
			asciigraph.Height(p.height),
			asciigraph.Width(p.width),
			asciigraph.SeriesColors(colors...),
			asciigraph.SeriesLegends(legends...),
		)
	}

	// Replace \n with \r\n for raw terminal mode compatibility
	graph = strings.ReplaceAll(graph, "\n", "\r\n")
	fmt.Fprint(p.writer, graph)
	fmt.Fprint(p.writer, "\r\n")

	return nil
}

// getSeriesColors returns a slice of colors for the given number of series
func getSeriesColors(count int) []asciigraph.AnsiColor {
	palette := []asciigraph.AnsiColor{
		asciigraph.Blue,
		asciigraph.Green,
		asciigraph.Red,
		asciigraph.Yellow,
		asciigraph.Cyan,
		asciigraph.Magenta,
		asciigraph.White,
		asciigraph.Coral,
		asciigraph.Aquamarine,
		asciigraph.Chocolate,
	}

	colors := make([]asciigraph.AnsiColor, count)
	for i := 0; i < count; i++ {
		colors[i] = palette[i%len(palette)]
	}
	return colors
}

// formatDuration formats a duration for display
//
//nolint:unused // Reserved for future chart features
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// truncateString truncates a string to maxLen characters
//
//nolint:unused // Reserved for future chart features
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// containsTimeseries checks if the data likely contains timeseries
//
//nolint:unused // Reserved for future chart features
func containsTimeseries(obj interface{}) bool {
	switch v := obj.(type) {
	case map[string]interface{}:
		_, hasTimeframe := v["timeframe"]
		_, hasInterval := v["interval"]
		if hasTimeframe && hasInterval {
			return true
		}
		if records, ok := v["records"].([]interface{}); ok && len(records) > 0 {
			if rec, ok := records[0].(map[string]interface{}); ok {
				_, hasTimeframe := rec["timeframe"]
				_, hasInterval := rec["interval"]
				return hasTimeframe && hasInterval
			}
		}
	case []interface{}:
		if len(v) > 0 {
			if rec, ok := v[0].(map[string]interface{}); ok {
				_, hasTimeframe := rec["timeframe"]
				_, hasInterval := rec["interval"]
				return hasTimeframe && hasInterval
			}
		}
	}
	return false
}

// containsAny checks if string contains any of the substrings
//
//nolint:unused // Reserved for future chart features
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
