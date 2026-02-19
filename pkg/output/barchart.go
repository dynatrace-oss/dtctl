package output

import (
	"fmt"
	"io"
	"math"
)

// BarChartPrinter prints timeseries data as horizontal bar charts (aggregated)
type BarChartPrinter struct {
	writer   io.Writer
	barWidth int
}

// NewBarChartPrinter creates a new bar chart printer
func NewBarChartPrinter(writer io.Writer) *BarChartPrinter {
	// Auto-detect terminal width
	width, _ := GetTerminalSize()
	return &BarChartPrinter{
		writer:   writer,
		barWidth: width,
	}
}

// NewBarChartPrinterWithSize creates a new bar chart printer with custom width
func NewBarChartPrinterWithSize(writer io.Writer, width int) *BarChartPrinter {
	if width <= 0 {
		width = DefaultChartWidth
	}
	return &BarChartPrinter{
		writer:   writer,
		barWidth: width,
	}
}

// Print prints the data as bar charts if it contains timeseries, otherwise falls back to JSON
func (p *BarChartPrinter) Print(obj interface{}) error {
	// Reuse chart printer's extraction logic
	chartPrinter := &ChartPrinter{writer: p.writer, height: DefaultChartHeight}
	ts, err := chartPrinter.extractTimeseries(obj)
	if err != nil {
		_, _ = fmt.Fprintf(p.writer, "Warning: %v. Falling back to JSON output.\n\n", err)
		return (&JSONPrinter{writer: p.writer}).Print(obj)
	}

	return p.renderBarChart(ts)
}

// PrintList prints a list of records as bar charts
func (p *BarChartPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}

// renderBarChart renders the timeseries data as a horizontal bar chart with btop-inspired styling
func (p *BarChartPrinter) renderBarChart(ts *TimeseriesData) error {
	// Print styled header with timeframe (use \r\n for raw terminal mode)
	timeFormat := "2006-01-02 15:04"
	_, _ = fmt.Fprintf(p.writer, "%s%s%s %s─%s %s%s%s\r\n\r\n",
		BrightCyan, ts.Start.UTC().Format(timeFormat), Reset,
		Dim, Reset,
		BrightCyan, ts.End.UTC().Format(timeFormat), Reset)

	// For bar charts, we show the average value for each series
	// Find global min/max for scaling
	globalMin, globalMax := math.MaxFloat64, -math.MaxFloat64
	type seriesStats struct {
		label string
		avg   float64
		min   float64
		max   float64
	}
	stats := make([]seriesStats, 0, len(ts.Series))

	for _, s := range ts.Series {
		min, max, avg := calculateStats(s.Values)
		if avg < globalMin {
			globalMin = avg
		}
		if avg > globalMax {
			globalMax = avg
		}

		label := s.Label
		if label == "" {
			label = s.Name
		}
		stats = append(stats, seriesStats{
			label: label,
			avg:   avg,
			min:   min,
			max:   max,
		})
	}

	// Find the longest label for alignment
	maxLabelLen := 0
	for _, s := range stats {
		if len(s.label) > maxLabelLen {
			maxLabelLen = len(s.label)
		}
	}

	// Cap label length based on available width
	// Layout: label(maxLabelLen) + " │ " (3) + bar + " │ " (3) + value (~10)
	const valueWidth = 10
	const separatorWidth = 6 // " │ " twice
	maxAllowedLabelLen := 25
	if maxLabelLen > maxAllowedLabelLen {
		maxLabelLen = maxAllowedLabelLen
	}

	// Calculate actual bar width
	barWidth := p.barWidth - maxLabelLen - separatorWidth - valueWidth
	if barWidth < 20 {
		barWidth = 20
	}

	// Determine scale - use 0 as minimum if all values are positive
	scaleMin := globalMin
	if globalMin >= 0 {
		scaleMin = 0
	}
	scaleMax := globalMax
	scaleRange := scaleMax - scaleMin
	if scaleRange == 0 {
		scaleRange = 1 // Avoid division by zero
	}

	// Render each series as a gradient bar with different color scheme
	for i, s := range stats {
		label := s.label
		if len(label) > maxLabelLen {
			label = label[:maxLabelLen-3] + "..."
		}

		// Calculate normalized value for gradient bar
		normalized := (s.avg - scaleMin) / scaleRange

		// Build gradient bar with calculated width and unique color scheme
		bar := RenderGradientBarWithScheme(normalized, 1.0, barWidth, i)

		// Print with btop-style formatting: label │ bar │ value
		// Use \r\n for raw terminal mode compatibility
		_, _ = fmt.Fprintf(p.writer, "%s%-*s%s %s│%s %s %s│%s %s%7.2f%s\r\n",
			BrightWhite, maxLabelLen, label, Reset,
			Dim, Reset,
			bar,
			Dim, Reset,
			BrightCyan, s.avg, Reset)
	}

	return nil
}
