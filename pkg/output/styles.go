package output

import (
	"fmt"
	"strings"
)

// ANSI color codes inspired by btop's color scheme
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"

	// Foreground colors
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// Bright foreground colors
	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"
)

// Box-drawing characters (single line - btop style)
const (
	BoxHorizontal     = "─"
	BoxVertical       = "│"
	BoxTopLeft        = "┌"
	BoxTopRight       = "┐"
	BoxBottomLeft     = "└"
	BoxBottomRight    = "┘"
	BoxVerticalRight  = "├"
	BoxVerticalLeft   = "┤"
	BoxHorizontalDown = "┬"
	BoxHorizontalUp   = "┴"
	BoxCross          = "┼"
)

// Double-line box characters for emphasis
const (
	BoxDoubleHorizontal  = "═"
	BoxDoubleVertical    = "║"
	BoxDoubleTopLeft     = "╔"
	BoxDoubleTopRight    = "╗"
	BoxDoubleBottomLeft  = "╚"
	BoxDoubleBottomRight = "╝"
)

// Braille patterns for high-resolution graphs (2x4 dot matrix per character)
// Each braille character represents a 2-column x 4-row grid of dots
// Dots are numbered: 1 4
//
//	2 5
//	3 6
//	7 8
var brailleBase = '\u2800' // Empty braille pattern

// BrailleGraph renders a high-resolution graph using braille characters
type BrailleGraph struct {
	width  int
	height int // in braille rows (each row = 4 pixels)
	data   [][]bool
}

// NewBrailleGraph creates a new braille graph canvas
func NewBrailleGraph(width, heightRows int) *BrailleGraph {
	// Each braille char is 2 dots wide, 4 dots tall
	pixelWidth := width * 2
	pixelHeight := heightRows * 4

	data := make([][]bool, pixelHeight)
	for i := range data {
		data[i] = make([]bool, pixelWidth)
	}

	return &BrailleGraph{
		width:  width,
		height: heightRows,
		data:   data,
	}
}

// SetPixel sets a pixel in the braille canvas
func (bg *BrailleGraph) SetPixel(x, y int) {
	if y >= 0 && y < len(bg.data) && x >= 0 && x < len(bg.data[0]) {
		bg.data[y][x] = true
	}
}

// PlotLine plots a series of values as a line graph
func (bg *BrailleGraph) PlotLine(values []float64, minVal, maxVal float64) {
	if len(values) == 0 {
		return
	}

	pixelWidth := bg.width * 2
	pixelHeight := bg.height * 4

	// Resample values to fit pixel width
	resampled := resampleValues(values, pixelWidth)

	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	// Plot each point and connect with lines
	var prevY int
	for x, v := range resampled {
		// Normalize to pixel height (inverted Y axis)
		normalized := (v - minVal) / valRange
		y := pixelHeight - 1 - int(normalized*float64(pixelHeight-1))
		if y < 0 {
			y = 0
		}
		if y >= pixelHeight {
			y = pixelHeight - 1
		}

		bg.SetPixel(x, y)

		// Connect to previous point with vertical line if needed
		if x > 0 {
			if prevY < y {
				for py := prevY; py <= y; py++ {
					bg.SetPixel(x, py)
				}
			} else {
				for py := y; py <= prevY; py++ {
					bg.SetPixel(x, py)
				}
			}
		}
		prevY = y
	}
}

// PlotFilled plots values as a filled area graph
func (bg *BrailleGraph) PlotFilled(values []float64, minVal, maxVal float64) {
	if len(values) == 0 {
		return
	}

	pixelWidth := bg.width * 2
	pixelHeight := bg.height * 4

	// Resample values to fit pixel width
	resampled := resampleValues(values, pixelWidth)

	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	// Plot each column filled from bottom
	for x, v := range resampled {
		normalized := (v - minVal) / valRange
		topY := pixelHeight - 1 - int(normalized*float64(pixelHeight-1))
		if topY < 0 {
			topY = 0
		}
		if topY >= pixelHeight {
			topY = pixelHeight - 1
		}

		// Fill from topY to bottom
		for y := topY; y < pixelHeight; y++ {
			bg.SetPixel(x, y)
		}
	}
}

// Render converts the canvas to a string of braille characters
func (bg *BrailleGraph) Render() string {
	var sb strings.Builder

	for row := 0; row < bg.height; row++ {
		for col := 0; col < bg.width; col++ {
			char := bg.getBrailleChar(col, row)
			sb.WriteRune(char)
		}
		if row < bg.height-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

// RenderColored renders with color gradient based on row position
func (bg *BrailleGraph) RenderColored() string {
	var sb strings.Builder

	for row := 0; row < bg.height; row++ {
		// Color gradient: green at top, yellow in middle, red at bottom
		color := getGradientColor(float64(row) / float64(bg.height-1))
		sb.WriteString(color)

		for col := 0; col < bg.width; col++ {
			char := bg.getBrailleChar(col, row)
			sb.WriteRune(char)
		}
		sb.WriteString(Reset)
		if row < bg.height-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

// getBrailleChar returns the braille character for a 2x4 cell
func (bg *BrailleGraph) getBrailleChar(col, row int) rune {
	// Braille dot positions:
	// 0 3  (dots 1,4)
	// 1 4  (dots 2,5)
	// 2 5  (dots 3,6)
	// 3 6  (dots 7,8)
	dotOffsets := []int{0x01, 0x02, 0x04, 0x40, 0x08, 0x10, 0x20, 0x80}

	px := col * 2
	py := row * 4

	var pattern int
	for dy := 0; dy < 4; dy++ {
		for dx := 0; dx < 2; dx++ {
			y := py + dy
			x := px + dx
			if y < len(bg.data) && x < len(bg.data[0]) && bg.data[y][x] {
				dotIdx := dy + dx*4
				if dotIdx < len(dotOffsets) {
					pattern |= dotOffsets[dotIdx]
				}
			}
		}
	}

	return brailleBase + rune(pattern)
}

// Gradient color functions
func getGradientColor(position float64) string {
	// position: 0.0 = top (green/good), 1.0 = bottom (red/bad)
	if position < 0.33 {
		return BrightGreen
	} else if position < 0.66 {
		return BrightYellow
	}
	return BrightRed
}

// getValueGradientColor returns color based on value (0.0 = low/green, 1.0 = high/red)
func getValueGradientColor(normalized float64) string {
	if normalized < 0.5 {
		return BrightGreen
	} else if normalized < 0.75 {
		return BrightYellow
	}
	return BrightRed
}

// getInverseGradientColor returns color (0.0 = low/red, 1.0 = high/green)
func getInverseGradientColor(normalized float64) string {
	if normalized < 0.25 {
		return BrightRed
	} else if normalized < 0.5 {
		return BrightYellow
	}
	return BrightGreen
}

// Bar rendering
var barChars = []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

// RenderGradientBar renders a simple horizontal bar (white filled, dim empty)
func RenderGradientBar(value, maxValue float64, width int) string {
	return RenderGradientBarWithScheme(value, maxValue, width, 0)
}

// RenderGradientBarWithScheme renders a simple bar (schemeIndex ignored for simplicity)
func RenderGradientBarWithScheme(value, maxValue float64, width int, schemeIndex int) string {
	if maxValue == 0 {
		maxValue = 1
	}
	normalized := value / maxValue
	if normalized > 1 {
		normalized = 1
	}
	if normalized < 0 {
		normalized = 0
	}

	fullBlocks := int(normalized * float64(width))
	emptyBlocks := width - fullBlocks

	var sb strings.Builder

	// Filled portion - white blocks
	if fullBlocks > 0 {
		sb.WriteString(BrightWhite)
		sb.WriteString(strings.Repeat("█", fullBlocks))
	}

	// Empty portion - dim
	if emptyBlocks > 0 {
		sb.WriteString(Dim)
		sb.WriteString(strings.Repeat("░", emptyBlocks))
	}
	sb.WriteString(Reset)

	return sb.String()
}

// RenderProgressBar renders a progress bar with percentage
func RenderProgressBar(value, maxValue float64, width int, showPercent bool) string {
	bar := RenderGradientBar(value, maxValue, width)
	if showPercent {
		pct := (value / maxValue) * 100
		if pct > 100 {
			pct = 100
		}
		return fmt.Sprintf("%s %s%.1f%%%s", bar, Bold, pct, Reset)
	}
	return bar
}

// Box drawing helpers
func DrawBox(title string, content string, width int) string {
	var sb strings.Builder

	// Ensure minimum width
	if width < len(title)+4 {
		width = len(title) + 4
	}

	// Top border with title
	sb.WriteString(BrightBlue)
	sb.WriteString(BoxTopLeft)
	if title != "" {
		sb.WriteString(BoxHorizontal)
		sb.WriteString(Reset)
		sb.WriteString(Bold)
		sb.WriteString(BrightCyan)
		sb.WriteString(title)
		sb.WriteString(Reset)
		sb.WriteString(BrightBlue)
		remaining := width - len(title) - 3
		sb.WriteString(strings.Repeat(BoxHorizontal, remaining))
	} else {
		sb.WriteString(strings.Repeat(BoxHorizontal, width-2))
	}
	sb.WriteString(BoxTopRight)
	sb.WriteString(Reset)
	sb.WriteString("\n")

	// Content lines
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		sb.WriteString(BrightBlue)
		sb.WriteString(BoxVertical)
		sb.WriteString(Reset)

		// Pad line to width
		lineLen := visibleLength(line)
		padding := width - 2 - lineLen
		if padding < 0 {
			padding = 0
		}
		sb.WriteString(line)
		sb.WriteString(strings.Repeat(" ", padding))

		sb.WriteString(BrightBlue)
		sb.WriteString(BoxVertical)
		sb.WriteString(Reset)
		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(BrightBlue)
	sb.WriteString(BoxBottomLeft)
	sb.WriteString(strings.Repeat(BoxHorizontal, width-2))
	sb.WriteString(BoxBottomRight)
	sb.WriteString(Reset)

	return sb.String()
}

// visibleLength returns the visible length of a string (excluding ANSI codes)
func visibleLength(s string) int {
	inEscape := false
	length := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

// DrawHeader renders a styled header line
func DrawHeader(title string, width int) string {
	var sb strings.Builder

	sb.WriteString(BrightBlue)
	sb.WriteString(BoxVerticalRight)
	sb.WriteString(Reset)
	sb.WriteString(Bold)
	sb.WriteString(BrightCyan)
	sb.WriteString(title)
	sb.WriteString(Reset)
	sb.WriteString(BrightBlue)

	remaining := width - len(title) - 2
	if remaining > 0 {
		sb.WriteString(strings.Repeat(BoxHorizontal, remaining))
	}
	sb.WriteString(BoxVerticalLeft)
	sb.WriteString(Reset)

	return sb.String()
}

// DrawSeparator renders a horizontal separator
func DrawSeparator(width int) string {
	return BrightBlue + BoxVerticalRight + strings.Repeat(BoxHorizontal, width-2) + BoxVerticalLeft + Reset
}

// Sparkline characters with different styles
var (
	SparkCharsBlock  = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	SparkCharsDots   = []rune{'⣀', '⣤', '⣶', '⣿'}
	SparkCharsSmooth = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
)

// RenderColoredSparkline renders a simple white sparkline
func RenderColoredSparkline(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// Resample if needed
	if len(values) != width {
		values = resampleValues(values, width)
	}

	// Find min/max
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	valRange := max - min
	if valRange == 0 {
		valRange = 1
	}

	var sb strings.Builder
	chars := SparkCharsBlock

	sb.WriteString(BrightWhite)
	for _, v := range values {
		normalized := (v - min) / valRange
		idx := int(normalized * float64(len(chars)-1))
		if idx >= len(chars) {
			idx = len(chars) - 1
		}
		if idx < 0 {
			idx = 0
		}
		sb.WriteRune(chars[idx])
	}
	sb.WriteString(Reset)

	return sb.String()
}

// StatsDisplay formats statistics in btop style
func StatsDisplay(label string, value float64, unit string, labelWidth int) string {
	return fmt.Sprintf("%s%-*s%s %s%.2f%s %s%s%s",
		Dim, labelWidth, label+":", Reset,
		Bold, value, Reset,
		Dim, unit, Reset)
}
