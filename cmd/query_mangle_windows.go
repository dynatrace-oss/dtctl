//go:build windows

package cmd

import "golang.org/x/sys/windows"

// platformRawCommandLine returns the unparsed command line the process was
// started with, which is where quote mangling is visible -- os.Args has already
// had the quotes removed.
func platformRawCommandLine() string {
	return windows.UTF16PtrToString(windows.GetCommandLine())
}
