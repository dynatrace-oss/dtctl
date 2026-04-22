//go:build !windows

package output

// enableVTProcessing is a no-op on non-Windows platforms where ANSI/VT
// escape sequences are natively supported by terminals.
func enableVTProcessing() bool {
	return true
}
