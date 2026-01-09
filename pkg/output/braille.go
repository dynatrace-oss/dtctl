package output

import (
	"fmt"
	"io"
	"math"
)

// BrailleChartPrinter prints timeseries data as high-resolution braille graphs
type BrailleChartPrinter struct {
	writer io.Writer
	width  int
	height int
}

// NewBrailleChartPrinter creates a new braille chart printer
func NewBrailleChartPrinter(writer io.Writer) *BrailleChartPrinter {
	// Auto-detect terminal dimensions
	width, height := GetTerminalSize()
	// Leave margin for y-axis labels and borders
	width = width - 15
	height = (height - 10) / 4 // Braille is 4 dots per row
	if width < 40 {
		width = 40
	}
	if height < 5 {
		height = 5
	}
	return &BrailleChartPrinter{
		writer: writer,
		width:  width,
		height: height,
	}
}

// NewBrailleChartPrinterWithSize creates a new braille chart printer with custom dimensions
func NewBrailleChartPrinterWithSize(writer io.Writer, width, height int) *BrailleChartPrinter {
	if width <= 0 {
		width = DefaultChartWidth
	}
	if height <= 0 {
		height = DefaultChartHeight / 2
	}
	return &BrailleChartPrinter{
		writer: writer,
		width:  width,
		height: height,
	}
}

// Print prints the data as braille charts if it contains timeseries
func (p *BrailleChartPrinter) Print(obj interface{}) error {
	chartPrinter := &ChartPrinter{writer: p.writer, height: DefaultChartHeight}
	ts, err := chartPrinter.extractTimeseries(obj)
	if err != nil {
		fmt.Fprintf(p.writer, "Warning: %v. Falling back to JSON output.\n\n", err)
		return (&JSONPrinter{writer: p.writer}).Print(obj)
	}

	return p.renderBrailleChart(ts)
}

// PrintList prints a list of records as braille charts
func (p *BrailleChartPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}

// renderBrailleChart renders the timeseries data as braille graphs
func (p *BrailleChartPrinter) renderBrailleChart(ts *TimeseriesData) error {
	// Print styled header with timeframe (use \r\n for raw terminal mode)
	timeFormat := "2006-01-02 15:04"
	fmt.Fprintf(p.writer, "%s%s%s %s─%s %s%s%s\r\n\r\n",
		BrightCyan, ts.Start.UTC().Format(timeFormat), Reset,
		Dim, Reset,
		BrightCyan, ts.End.UTC().Format(timeFormat), Reset)

	// Render each series
	for i, s := range ts.Series {
		label := s.Label
		if label == "" {
			label = s.Name
		}

		// Calculate stats
		minVal, maxVal, avgVal := calculateStats(s.Values)

		// Print series header (use \r\n for raw terminal mode)
		color := getSeriesColor(i)
		fmt.Fprintf(p.writer, "%s%s● %s%s%s\r\n",
			color, Bold, label, Reset, Reset)

		// Create braille graph
		bg := NewBrailleGraph(p.width, p.height)
		bg.PlotFilled(s.Values, minVal, maxVal)

		// Render with color gradient
		graph := bg.RenderColored()

		// Print Y-axis labels and graph
		p.printWithYAxis(graph, minVal, maxVal)

		// Print stats line (use \r\n for raw terminal mode)
		fmt.Fprintf(p.writer, "  %smin:%s %.2f  %smax:%s %.2f  %savg:%s %.2f\r\n\r\n",
			Dim, BrightGreen, minVal,
			Dim, BrightRed, maxVal,
			Dim, BrightCyan, avgVal)
	}

	return nil
}

// printWithYAxis prints the graph with Y-axis labels
func (p *BrailleChartPrinter) printWithYAxis(graph string, minVal, maxVal float64) {
	lines := splitLines(graph)

	for i, line := range lines {
		// Calculate Y value for this row
		progress := float64(i) / float64(len(lines)-1)
		yVal := maxVal - progress*(maxVal-minVal)

		// Print Y-axis label (use \r\n for raw terminal mode)
		fmt.Fprintf(p.writer, "%s%6.1f%s %s│%s %s\r\n",
			Dim, yVal, Reset,
			BrightBlue, Reset,
			line)
	}

	// Print X-axis (use \r\n for raw terminal mode)
	fmt.Fprintf(p.writer, "%s       %s└%s%s\r\n",
		Reset,
		BrightBlue,
		repeatString(BoxHorizontal, p.width),
		Reset)
}

// splitLines splits a string into lines
func splitLines(s string) []string {
	var lines []string
	var current string
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// repeatString repeats a string n times
func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// getSeriesColor returns a color for a series index
func getSeriesColor(idx int) string {
	colors := []string{
		BrightCyan,
		BrightGreen,
		BrightYellow,
		BrightMagenta,
		BrightBlue,
		BrightRed,
	}
	return colors[idx%len(colors)]
}

// MiniGraph renders a small inline braille graph for embedding in tables
func MiniGraph(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// Find min/max
	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	for _, v := range values {
		if !math.IsNaN(v) {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
	}

	if minVal == math.MaxFloat64 {
		return ""
	}

	// Create a single-row braille graph
	bg := NewBrailleGraph(width, 1)
	bg.PlotLine(values, minVal, maxVal)

	return bg.Render()
}

// MiniGraphColored renders a colored mini graph
func MiniGraphColored(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// Find min/max
	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	for _, v := range values {
		if !math.IsNaN(v) {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
	}

	if minVal == math.MaxFloat64 {
		return ""
	}

	// Create a single-row braille graph
	bg := NewBrailleGraph(width, 1)
	bg.PlotLine(values, minVal, maxVal)

	return bg.RenderColored()
}
