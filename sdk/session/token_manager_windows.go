//go:build windows

package session

import (
	"errors"
	"syscall"
)

// isWindowsAdminKeyringErr reports whether err is ERROR_NO_SUCH_LOGON_SESSION
// (0x520, decimal 1312). Windows Credential Manager returns this when the
// process runs under an elevated (Admin) security token that has no associated
// logon session for the credential store.
//
// Using errors.Is rather than string matching avoids locale-dependent failures:
// FormatMessage falls back to the user's locale when English MUI resources are
// absent, so the English string cannot be relied on.
func isWindowsAdminKeyringErr(err error) bool {
	const errNoSuchLogonSession = syscall.Errno(1312)
	return errors.Is(err, errNoSuchLogonSession)
}
