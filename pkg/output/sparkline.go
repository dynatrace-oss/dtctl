package output

import (
	"fmt"
	"io"
	"math"
	"strings"
)

// Sparkline characters from lowest to highest
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// SparklinePrinter prints timeseries data as compact sparklines
type SparklinePrinter struct {
	writer io.Writer
	width  int
}

// NewSparklinePrinter creates a new sparkline printer
func NewSparklinePrinter(writer io.Writer) *SparklinePrinter {
	// Auto-detect terminal width
	width, _ := GetTerminalSize()
	return &SparklinePrinter{writer: writer, width: width}
}

// NewSparklinePrinterWithSize creates a new sparkline printer with custom width
func NewSparklinePrinterWithSize(writer io.Writer, width int) *SparklinePrinter {
	if width <= 0 {
		width = DefaultChartWidth
	}
	return &SparklinePrinter{writer: writer, width: width}
}

// Print prints the data as sparklines if it contains timeseries, otherwise falls back to JSON
func (p *SparklinePrinter) Print(obj interface{}) error {
	// Reuse chart printer's extraction logic
	chartPrinter := &ChartPrinter{writer: p.writer, height: DefaultChartHeight}
	ts, err := chartPrinter.extractTimeseries(obj)
	if err != nil {
		fmt.Fprintf(p.writer, "Warning: %v. Falling back to JSON output.\n\n", err)
		return (&JSONPrinter{writer: p.writer}).Print(obj)
	}

	return p.renderSparklines(ts)
}

// PrintList prints a list of records as sparklines
func (p *SparklinePrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}

// renderSparklines renders the timeseries data as sparklines with btop-inspired styling
func (p *SparklinePrinter) renderSparklines(ts *TimeseriesData) error {
	// Print styled header with timeframe (use \r\n for raw terminal mode)
	timeFormat := "2006-01-02 15:04"
	fmt.Fprintf(p.writer, "%s%s%s %s─%s %s%s%s\r\n\r\n",
		BrightCyan, ts.Start.UTC().Format(timeFormat), Reset,
		Dim, Reset,
		BrightCyan, ts.End.UTC().Format(timeFormat), Reset)

	// Find the longest label for alignment
	maxLabelLen := 0
	for _, s := range ts.Series {
		label := s.Label
		if label == "" {
			label = s.Name
		}
		if len(label) > maxLabelLen {
			maxLabelLen = len(label)
		}
	}

	// Cap label length based on available width
	// Layout: label(maxLabelLen) + " │ " (3) + sparkline + " │ " (3) + stats (~30)
	// Stats format: "min:XX.X max:XX.X avg:XX.X" ≈ 30 chars
	const statsWidth = 30
	const separatorWidth = 6 // " │ " twice
	maxAllowedLabelLen := 25
	if maxLabelLen > maxAllowedLabelLen {
		maxLabelLen = maxAllowedLabelLen
	}

	// Calculate actual sparkline width
	sparkWidth := p.width - maxLabelLen - separatorWidth - statsWidth
	if sparkWidth < 20 {
		sparkWidth = 20
	}

	// Render each series as a colored sparkline
	for _, s := range ts.Series {
		label := s.Label
		if label == "" {
			label = s.Name
		}

		// Truncate long labels
		if len(label) > maxLabelLen {
			label = label[:maxLabelLen-3] + "..."
		}

		// Generate colored sparkline with calculated width
		spark := RenderColoredSparkline(s.Values, sparkWidth)

		// Calculate stats
		min, max, avg := calculateStats(s.Values)

		// Print with btop-style formatting: label │ sparkline │ stats
		// Use \r\n for raw terminal mode compatibility
		fmt.Fprintf(p.writer, "%s%-*s%s %s│%s %s %s│%s %s%.1f%s/%s%.1f%s/%s%.1f%s\r\n",
			BrightWhite, maxLabelLen, label, Reset,
			Dim, Reset,
			spark,
			Dim, Reset,
			BrightGreen, min, Reset,
			BrightRed, max, Reset,
			BrightCyan, avg, Reset)
	}

	return nil
}

// generateSparkline converts a slice of float64 values to a sparkline string with fixed width
func generateSparkline(values []float64) string {
	return generateSparklineWithWidth(values, DefaultChartWidth)
}

// generateSparklineWithWidth converts a slice of float64 values to a sparkline string with specified width
func generateSparklineWithWidth(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// Resample values to fit the target width
	if len(values) != width {
		values = resampleValues(values, width)
	}

	// Find min and max (excluding NaN)
	min, max := math.MaxFloat64, -math.MaxFloat64
	validCount := 0
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		validCount++
	}

	if validCount == 0 {
		return strings.Repeat("?", len(values))
	}

	// Handle case where all values are the same
	valueRange := max - min
	if valueRange == 0 {
		// All values are the same, use middle character
		return strings.Repeat(string(sparkChars[len(sparkChars)/2]), len(values))
	}

	// Build sparkline
	var sb strings.Builder
	for _, v := range values {
		if math.IsNaN(v) {
			sb.WriteRune(' ') // Gap for missing data
			continue
		}

		// Normalize to 0-1 range
		normalized := (v - min) / valueRange

		// Map to character index (0 to len-1)
		idx := int(normalized * float64(len(sparkChars)-1))
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		if idx < 0 {
			idx = 0
		}

		sb.WriteRune(sparkChars[idx])
	}

	return sb.String()
}

// calculateStats calculates min, max, and average of values (excluding NaN)
func calculateStats(values []float64) (min, max, avg float64) {
	min = math.MaxFloat64
	max = -math.MaxFloat64
	sum := 0.0
	count := 0

	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
		count++
	}

	if count == 0 {
		return 0, 0, 0
	}

	avg = sum / float64(count)
	return min, max, avg
}

// resampleValues resamples a slice of values to a target length using linear interpolation
func resampleValues(values []float64, targetLen int) []float64 {
	if len(values) == 0 || targetLen <= 0 {
		return values
	}

	if len(values) == targetLen {
		return values
	}

	result := make([]float64, targetLen)
	ratio := float64(len(values)-1) / float64(targetLen-1)

	for i := 0; i < targetLen; i++ {
		srcIdx := float64(i) * ratio
		lowIdx := int(srcIdx)
		highIdx := lowIdx + 1

		if highIdx >= len(values) {
			result[i] = values[len(values)-1]
		} else {
			// Linear interpolation
			frac := srcIdx - float64(lowIdx)
			lowVal := values[lowIdx]
			highVal := values[highIdx]

			// Handle NaN values
			if math.IsNaN(lowVal) {
				result[i] = highVal
			} else if math.IsNaN(highVal) {
				result[i] = lowVal
			} else {
				result[i] = lowVal + frac*(highVal-lowVal)
			}
		}
	}

	return result
}
