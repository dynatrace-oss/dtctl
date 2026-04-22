//go:build windows

package output

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVTProcessing enables ANSI/VT escape sequence processing on the
// Windows console for both stdout and stderr. Returns true if VT sequences
// are supported on stdout, false otherwise.
//
// On Windows 10+, the console supports VT sequences but they must be
// explicitly enabled via SetConsoleMode with ENABLE_VIRTUAL_TERMINAL_PROCESSING.
// See: https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences
func enableVTProcessing() bool {
	if !enableVTOnHandle(os.Stdout.Fd()) {
		return false
	}

	// Best-effort: also enable on stderr so colored diagnostics render correctly.
	// Failure here is non-fatal — stderr may be redirected to a file/pipe.
	_ = enableVTOnHandle(os.Stderr.Fd())

	return true
}

// enableVTOnHandle enables ENABLE_VIRTUAL_TERMINAL_PROCESSING on a single
// console handle. Returns false if the handle is not a console or the mode
// cannot be set.
func enableVTOnHandle(fd uintptr) bool {
	handle := windows.Handle(fd)

	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}

	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true // already enabled
	}

	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return false
	}

	return true
}
