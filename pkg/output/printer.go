package output

import (
	"encoding/json"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Printer interface for output formatting
type Printer interface {
	Print(interface{}) error
	PrintList(interface{}) error
}

// PrinterOptions configures printer behavior
type PrinterOptions struct {
	Format     string
	Writer     io.Writer
	PlainMode  bool
	Width      int  // Chart width (0 = default)
	Height     int  // Chart height (0 = default)
	Fullscreen bool // Use terminal dimensions
}

// NewPrinter creates a new printer based on the format
func NewPrinter(format string) Printer {
	return NewPrinterWithWriter(format, os.Stdout)
}

// NewPrinterWithWriter creates a new printer with a specific writer
func NewPrinterWithWriter(format string, writer io.Writer) Printer {
	return NewPrinterWithOpts(PrinterOptions{
		Format: format,
		Writer: writer,
	})
}

// NewPrinterWithOptions creates a new printer with specific options
func NewPrinterWithOptions(format string, writer io.Writer, plainMode bool) Printer {
	return NewPrinterWithOpts(PrinterOptions{
		Format:    format,
		Writer:    writer,
		PlainMode: plainMode,
	})
}

// NewPrinterWithOpts creates a new printer with full options
func NewPrinterWithOpts(opts PrinterOptions) Printer {
	format := opts.Format
	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}

	// In plain mode, force JSON output instead of table for machine readability
	if opts.PlainMode && (format == "table" || format == "wide") {
		format = "json"
	}

	// Determine dimensions
	width, height := opts.Width, opts.Height
	termWidth, _ := GetTerminalSize()
	if opts.Fullscreen {
		width, height = GetFullscreenDimensions()
	}

	switch format {
	case "json":
		return &JSONPrinter{writer: writer}
	case "yaml", "yml":
		return &YAMLPrinter{writer: writer}
	case "csv":
		return &CSVPrinter{writer: writer}
	case "chart":
		if width > 0 || height > 0 {
			return NewChartPrinterWithSize(writer, width, height)
		}
		return NewChartPrinter(writer)
	case "sparkline", "spark":
		// Sparkline needs terminal width for layout, not chart area width
		if opts.Fullscreen {
			return NewSparklinePrinterWithSize(writer, termWidth-2)
		}
		if width > 0 {
			return NewSparklinePrinterWithSize(writer, width)
		}
		return NewSparklinePrinter(writer)
	case "barchart", "bar":
		// Bar chart needs terminal width for layout
		if opts.Fullscreen || width > 0 {
			w := width
			if opts.Fullscreen {
				w, _ = GetTerminalSize()
			}
			return NewBarChartPrinterWithSize(writer, w)
		}
		return NewBarChartPrinter(writer)
	case "braille", "br":
		if width > 0 || height > 0 {
			return NewBrailleChartPrinterWithSize(writer, width, height)
		}
		return NewBrailleChartPrinter(writer)
	case "table", "wide":
		return &TablePrinter{writer: writer, wide: format == "wide"}
	default:
		return &TablePrinter{writer: writer}
	}
}

// JSONPrinter prints output as JSON
type JSONPrinter struct {
	writer io.Writer
}

// Print prints a single object as JSON
func (p *JSONPrinter) Print(obj interface{}) error {
	encoder := json.NewEncoder(p.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(obj)
}

// PrintList prints a list of objects as JSON
func (p *JSONPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}

// YAMLPrinter prints output as YAML
type YAMLPrinter struct {
	writer io.Writer
}

// Print prints a single object as YAML
func (p *YAMLPrinter) Print(obj interface{}) error {
	encoder := yaml.NewEncoder(p.writer)
	encoder.SetIndent(2)
	return encoder.Encode(obj)
}

// PrintList prints a list of objects as YAML
func (p *YAMLPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}
