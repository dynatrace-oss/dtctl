//go:build !windows

package cmd

// platformRawCommandLine returns "" because only Windows passes a single
// command-line string to the process for the C runtime to split -- POSIX
// exec hands over an argv array, so nothing can mangle quotes in transit.
func platformRawCommandLine() string {
	return ""
}
