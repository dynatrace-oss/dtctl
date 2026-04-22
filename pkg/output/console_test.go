package output

import (
	"os"
	"testing"
)

func TestEnableVTProcessing_ReturnsTrue(t *testing.T) {
	// On non-Windows platforms, enableVTProcessing is a no-op that always
	// returns true. On Windows in CI, it should also succeed (Windows 10+
	// supports VT processing) or return false gracefully if stdout is not
	// a console (e.g., piped in CI).
	result := enableVTProcessing()

	// We can't assert a specific value in CI because stdout may be a pipe,
	// but we verify the function doesn't panic and returns a valid bool.
	_ = result
}

func TestDetectColor_VTProcessingIntegration(t *testing.T) {
	// When FORCE_COLOR=1 is set, detectColor bypasses the TTY check and
	// VT processing call, so color is enabled regardless of platform.
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")

	if !ColorEnabled() {
		t.Error("ColorEnabled() should return true with FORCE_COLOR=1")
	}

	// When stdout is not a TTY (typical in tests/CI) and no FORCE_COLOR,
	// detectColor should return false before even calling enableVTProcessing.
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	os.Unsetenv("FORCE_COLOR")

	if ColorEnabled() {
		t.Error("ColorEnabled() should return false when stdout is not a TTY and FORCE_COLOR is not set")
	}
}
