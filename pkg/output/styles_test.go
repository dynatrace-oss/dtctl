package output

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"testing"
)

func TestColorEnabled_NOCOLORDisablesColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	result := ColorEnabled()
	if result {
		t.Error("ColorEnabled() should return false when NO_COLOR is set (even if empty)")
	}
}

func TestColorEnabled_NOCOLORTakesPrecedence(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "1")
	result := ColorEnabled()
	if result {
		t.Error("ColorEnabled() should return false when NO_COLOR is set, even with FORCE_COLOR=1")
	}
}

func TestColorEnabled_FORCECOLOREnablesColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	result := ColorEnabled()
	if !result {
		t.Error("ColorEnabled() should return true when FORCE_COLOR=1 and NO_COLOR is not set")
	}
}

func TestColorEnabled_FORCECOLOROnlyWithValue1(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "true")
	// Without a TTY (test environment), and FORCE_COLOR != "1", should be false
	result := ColorEnabled()
	if result {
		t.Error("ColorEnabled() should return false when FORCE_COLOR is not '1' and stdout is not a TTY")
	}
}

func TestColorEnabled_Caching(t *testing.T) {
	ResetColorCache()
	t.Setenv("FORCE_COLOR", "1")
	os.Unsetenv("NO_COLOR")
	first := ColorEnabled()

	// Change env - should still return cached value
	t.Setenv("NO_COLOR", "1")
	second := ColorEnabled()

	if first != second {
		t.Error("ColorEnabled() should return cached result on subsequent calls")
	}
}

func TestResetColorCache(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	first := ColorEnabled()
	if !first {
		t.Fatal("expected ColorEnabled() = true with FORCE_COLOR=1")
	}

	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	second := ColorEnabled()
	if second {
		t.Error("After ResetColorCache(), ColorEnabled() should re-evaluate and return false with NO_COLOR set")
	}
}

func TestColorize_WithColorEnabled(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")

	result := Colorize(BrightCyan, "hello")
	expected := BrightCyan + "hello" + Reset
	if result != expected {
		t.Errorf("Colorize() = %q, want %q", result, expected)
	}
}

func TestColorize_WithColorDisabled(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "")

	result := Colorize(BrightCyan, "hello")
	if result != "hello" {
		t.Errorf("Colorize() with NO_COLOR = %q, want %q", result, "hello")
	}
}

func TestColorCode_WithColorEnabled(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")

	result := ColorCode(BrightCyan)
	if result != BrightCyan {
		t.Errorf("ColorCode() = %q, want %q", result, BrightCyan)
	}
}

func TestColorCode_WithColorDisabled(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "")

	result := ColorCode(BrightCyan)
	if result != "" {
		t.Errorf("ColorCode() with NO_COLOR = %q, want %q", result, "")
	}
}

func TestColorEnabled_PlainModeDisablesColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")

	SetPlainMode(true)
	result := ColorEnabled()
	if result {
		t.Error("ColorEnabled() should return false when plain mode is enabled, even with FORCE_COLOR=1")
	}
}

func TestColorEnabled_PlainModeTakesPrecedenceOverFORCECOLOR(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")

	SetPlainMode(true)
	result := ColorEnabled()
	if result {
		t.Error("ColorEnabled() should return false when plain mode is set, even with FORCE_COLOR=1")
	}

	// After reset, plain mode should be cleared and FORCE_COLOR should work
	ResetColorCache()
	result = ColorEnabled()
	if !result {
		t.Error("After ResetColorCache(), plain mode should be cleared and FORCE_COLOR=1 should enable color")
	}
}

func TestRenderGradientBar(t *testing.T) {
	// Test various fill levels
	tests := []struct {
		value    float64
		maxValue float64
		width    int
	}{
		{0, 100, 20},
		{25, 100, 20},
		{50, 100, 20},
		{75, 100, 20},
		{100, 100, 20},
	}

	for _, tt := range tests {
		bar := RenderGradientBar(tt.value, tt.maxValue, tt.width)
		if bar == "" {
			t.Errorf("RenderGradientBar(%v, %v, %v) returned empty string", tt.value, tt.maxValue, tt.width)
		}
	}
}

func TestRenderColoredSparkline(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 4, 3, 2, 1, 2, 3, 4, 5}
	spark := RenderColoredSparkline(values, 20)
	if spark == "" {
		t.Error("RenderColoredSparkline returned empty string")
	}
}

func TestBrailleGraph(t *testing.T) {
	bg := NewBrailleGraph(40, 5)
	values := make([]float64, 100)
	for i := range values {
		values[i] = math.Sin(float64(i) * 0.1)
	}
	bg.PlotLine(values, -1, 1)
	result := bg.Render()
	if result == "" {
		t.Error("BrailleGraph.Render returned empty string")
	}
}

func TestBrailleGraphColored(t *testing.T) {
	bg := NewBrailleGraph(40, 5)
	values := make([]float64, 100)
	for i := range values {
		values[i] = math.Sin(float64(i) * 0.1)
	}
	bg.PlotFilled(values, -1, 1)
	result := bg.RenderColored()
	if result == "" {
		t.Error("BrailleGraph.RenderColored returned empty string")
	}
}

func TestDrawBox(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3"
	box := DrawBox("Test Title", content, 40)
	if box == "" {
		t.Error("DrawBox returned empty string")
	}
}

func TestVisibleLength(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{BrightCyan + "hello" + Reset, 5},
		{Bold + BrightRed + "test" + Reset, 4},
		{"", 0},
	}

	for _, tt := range tests {
		got := visibleLength(tt.input)
		if got != tt.expected {
			t.Errorf("visibleLength(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

// Demo function to show the visualizations (not a real test, just for visual inspection)
func TestDemoVisualizations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping demo in short mode")
	}

	var buf bytes.Buffer

	fmt.Fprint(&buf, "\n"+Bold+BrightCyan+"=== btop-inspired Visualization Demo ==="+Reset+"\n\n")

	// Gradient bars
	fmt.Fprintln(&buf, Bold+"Gradient Bars:"+Reset)
	for i := 0; i <= 10; i++ {
		pct := float64(i) * 10
		bar := RenderProgressBar(pct, 100, 30, true)
		fmt.Fprintf(&buf, "  %s\n", bar)
	}

	// Colored sparklines
	fmt.Fprintln(&buf, "\n"+Bold+"Colored Sparklines:"+Reset)
	values1 := []float64{1, 3, 5, 7, 9, 8, 6, 4, 2, 3, 5, 7, 9, 10, 8, 6, 4, 2, 1, 3}
	values2 := []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 9}
	fmt.Fprintf(&buf, "  CPU Usage:  %s\n", RenderColoredSparkline(values1, 40))
	fmt.Fprintf(&buf, "  Memory:     %s\n", RenderColoredSparkline(values2, 40))

	// Braille graph
	fmt.Fprintln(&buf, "\n"+Bold+"Braille Graph (high resolution):"+Reset)
	bg := NewBrailleGraph(50, 6)
	sineWave := make([]float64, 200)
	for i := range sineWave {
		sineWave[i] = math.Sin(float64(i)*0.05)*50 + 50
	}
	bg.PlotFilled(sineWave, 0, 100)
	fmt.Fprintln(&buf, bg.RenderColored())

	// Box drawing
	fmt.Fprintln(&buf, "\n"+Bold+"Box Drawing:"+Reset)
	boxContent := fmt.Sprintf("CPU: %s 45.2%%\nMem: %s 67.8%%\nDisk: %s 23.1%%",
		RenderGradientBar(45.2, 100, 15),
		RenderGradientBar(67.8, 100, 15),
		RenderGradientBar(23.1, 100, 15))
	fmt.Fprintln(&buf, DrawBox("System Stats", boxContent, 50))

	// Print to test output
	t.Log(buf.String())
}
