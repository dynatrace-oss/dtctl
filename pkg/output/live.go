package output

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	// DefaultLiveInterval is the default refresh interval for live mode
	DefaultLiveInterval = 60 * time.Second
	// MinLiveInterval is the minimum allowed refresh interval
	MinLiveInterval = 1 * time.Second
)

// LivePrinter wraps a Printer and provides live update functionality
type LivePrinter struct {
	printer     Printer
	interval    time.Duration
	writer      io.Writer
	fullscreen  bool
	printerOpts PrinterOptions
	resizeCh    chan os.Signal
}

// DataFetcher is a function that fetches fresh data for live updates
type DataFetcher func(ctx context.Context) (interface{}, error)

// NewLivePrinter creates a new live printer that wraps an existing printer
func NewLivePrinter(printer Printer, interval time.Duration, writer io.Writer) *LivePrinter {
	return NewLivePrinterWithOpts(printer, interval, writer, PrinterOptions{})
}

// NewLivePrinterWithOpts creates a new live printer with options for resize support
func NewLivePrinterWithOpts(printer Printer, interval time.Duration, writer io.Writer, opts PrinterOptions) *LivePrinter {
	if interval < MinLiveInterval {
		interval = MinLiveInterval
	}
	if writer == nil {
		writer = os.Stdout
	}
	return &LivePrinter{
		printer:     printer,
		interval:    interval,
		writer:      writer,
		fullscreen:  opts.Fullscreen,
		printerOpts: opts,
	}
}

// RunLive starts the live update loop, calling fetcher periodically
// It handles Ctrl+C gracefully and clears the screen between updates
func (p *LivePrinter) RunLive(ctx context.Context, fetcher DataFetcher) error {
	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Set up resize signal handling for fullscreen mode
	if p.fullscreen {
		p.resizeCh = make(chan os.Signal, 1)
		signal.Notify(p.resizeCh, syscall.SIGWINCH)
		defer signal.Stop(p.resizeCh)
	}

	// Set up keyboard input handling for 'q' to quit
	keyCh := make(chan rune, 1)
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		// Save terminal state and set raw mode
		oldState, err := term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, oldState)
			// Read keyboard input in background
			go func() {
				buf := make([]byte, 1)
				for {
					n, err := os.Stdin.Read(buf)
					if err != nil || n == 0 {
						return
					}
					select {
					case keyCh <- rune(buf[0]):
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}

	go func() {
		<-sigCh
		cancel()
	}()

	// Initial fetch and display
	if err := p.fetchAndPrint(ctx, fetcher); err != nil {
		return err
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Clear status line and exit cleanly
			fmt.Fprintln(p.writer, "\nLive mode stopped.")
			return nil
		case key := <-keyCh:
			// Handle 'q' or 'Q' to quit
			if key == 'q' || key == 'Q' {
				fmt.Fprintln(p.writer, "\nLive mode stopped.")
				return nil
			}
		case <-ticker.C:
			if err := p.fetchAndPrint(ctx, fetcher); err != nil {
				// Print error but continue trying
				fmt.Fprintf(p.writer, "\nError fetching data: %v\n", err)
			}
		case <-p.resizeCh:
			// Terminal resized - update printer dimensions and redraw
			p.updatePrinterForResize()
			if err := p.fetchAndPrint(ctx, fetcher); err != nil {
				fmt.Fprintf(p.writer, "\nError fetching data: %v\n", err)
			}
		}
	}
}

// updatePrinterForResize recreates the printer with new terminal dimensions
func (p *LivePrinter) updatePrinterForResize() {
	if !p.fullscreen {
		return
	}
	// Get new dimensions and recreate printer
	opts := p.printerOpts
	opts.Fullscreen = true // Force recalculation
	p.printer = NewPrinterWithOpts(opts)
}

// fetchAndPrint fetches data and prints it, clearing the screen first
func (p *LivePrinter) fetchAndPrint(ctx context.Context, fetcher DataFetcher) error {
	data, err := fetcher(ctx)
	if err != nil {
		return err
	}

	// Clear screen and move cursor to top-left
	p.clearScreen()

	// Print timestamp header (use \r\n for raw terminal mode)
	fmt.Fprintf(p.writer, "%sLive mode%s (refresh: %s) - Press 'q' or Ctrl+C to stop\r\n",
		BrightCyan, Reset, p.interval)
	fmt.Fprintf(p.writer, "%sLast update:%s %s\r\n\r\n",
		Dim, Reset, time.Now().Format("2006-01-02 15:04:05"))

	// Recreate printer with current terminal dimensions for fullscreen mode
	if p.fullscreen {
		p.updatePrinterForResize()
	}

	// Print the data using the wrapped printer
	return p.printer.Print(data)
}

// clearScreen clears the terminal screen using ANSI escape codes
func (p *LivePrinter) clearScreen() {
	// ANSI escape codes: clear screen and move cursor to home position
	fmt.Fprint(p.writer, "\033[2J\033[H")
}

// GetTerminalSize returns the current terminal width and height
// Returns default values if terminal size cannot be determined
func GetTerminalSize() (width, height int) {
	width, height = DefaultChartWidth, DefaultChartHeight

	// Try to get terminal size from stdout
	if fd := int(os.Stdout.Fd()); term.IsTerminal(fd) {
		if w, h, err := term.GetSize(fd); err == nil {
			width = w
			height = h
		}
	}

	return width, height
}

// GetFullscreenDimensions returns chart dimensions suitable for fullscreen display
// It leaves some margin for headers and borders
func GetFullscreenDimensions() (width, height int) {
	termWidth, termHeight := GetTerminalSize()

	// Leave margin for headers, labels, and borders
	// Width: subtract ~15 for y-axis labels and margins
	// Height: subtract for live mode headers (4), chart timeframe header (2), legend (3), margins (3) = 12
	width = termWidth - 15
	height = termHeight - 12

	// Ensure minimum dimensions
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	return width, height
}
