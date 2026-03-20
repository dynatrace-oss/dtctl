package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestFprintSuccess_NoColor(t *testing.T) {
	// Ensure color is disabled for predictable output
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintSuccess(&buf, "Workflow %q created", "my-wf")

	got := buf.String()
	if !strings.Contains(got, "OK") {
		t.Errorf("expected 'OK' prefix, got: %s", got)
	}
	if !strings.Contains(got, `Workflow "my-wf" created`) {
		t.Errorf("expected formatted message, got: %s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected trailing newline")
	}
}

func TestFprintSuccess_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintSuccess(&buf, "created resource")

	got := buf.String()
	// Should contain ANSI bold+green escape for "OK"
	if !strings.Contains(got, Bold+Green) {
		t.Errorf("expected bold+green ANSI code in output, got: %s", got)
	}
	if !strings.Contains(got, "OK") {
		t.Errorf("expected 'OK' prefix, got: %s", got)
	}
	if !strings.Contains(got, "created resource") {
		t.Errorf("expected message text, got: %s", got)
	}
}

func TestFprintWarning_NoColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintWarning(&buf, "resource %s is deprecated", "old-res")

	got := buf.String()
	if !strings.Contains(got, "Warning:") {
		t.Errorf("expected 'Warning:' prefix, got: %s", got)
	}
	if !strings.Contains(got, `resource old-res is deprecated`) {
		t.Errorf("expected formatted message, got: %s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected trailing newline")
	}
}

func TestFprintWarning_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintWarning(&buf, "dry-run mode")

	got := buf.String()
	// Should contain ANSI bold+yellow escape for "Warning:"
	if !strings.Contains(got, Bold+Yellow) {
		t.Errorf("expected bold+yellow ANSI code in output, got: %s", got)
	}
	if !strings.Contains(got, "Warning:") {
		t.Errorf("expected 'Warning:' prefix, got: %s", got)
	}
	if !strings.Contains(got, "dry-run mode") {
		t.Errorf("expected message text, got: %s", got)
	}
}

func TestFprintSuccess_FormatArgs(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintSuccess(&buf, "%d resources created in %s", 5, "production")

	got := buf.String()
	if !strings.Contains(got, "5 resources created in production") {
		t.Errorf("expected formatted message with multiple args, got: %s", got)
	}
}

func TestFprintWarning_FormatArgs(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintWarning(&buf, "skipping %d of %d items", 3, 10)

	got := buf.String()
	if !strings.Contains(got, "skipping 3 of 10 items") {
		t.Errorf("expected formatted message with multiple args, got: %s", got)
	}
}

func TestFprintInfo(t *testing.T) {
	var buf bytes.Buffer
	FprintInfo(&buf, "  ID:   %s", "abc-123")

	got := buf.String()
	expected := "  ID:   abc-123\n"
	if got != expected {
		t.Errorf("FprintInfo output = %q, want %q", got, expected)
	}
}

func TestFprintInfo_NoFormatArgs(t *testing.T) {
	var buf bytes.Buffer
	FprintInfo(&buf, "Note: Bucket creation can take up to 1 minute")

	got := buf.String()
	expected := "Note: Bucket creation can take up to 1 minute\n"
	if got != expected {
		t.Errorf("FprintInfo output = %q, want %q", got, expected)
	}
}

func TestFprintHumanError_NoColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintHumanError(&buf, "failed to connect to %s", "example.invalid")

	got := buf.String()
	if !strings.Contains(got, "Error:") {
		t.Errorf("expected 'Error:' prefix, got: %s", got)
	}
	if !strings.Contains(got, "failed to connect to example.invalid") {
		t.Errorf("expected formatted message, got: %s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected trailing newline")
	}
}

func TestFprintHumanError_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintHumanError(&buf, "resource not found")

	got := buf.String()
	if !strings.Contains(got, Bold+Red) {
		t.Errorf("expected bold+red ANSI code in output, got: %s", got)
	}
	if !strings.Contains(got, "Error:") {
		t.Errorf("expected 'Error:' prefix, got: %s", got)
	}
	if !strings.Contains(got, "resource not found") {
		t.Errorf("expected message text, got: %s", got)
	}
}

func TestFprintHint_NoColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintHint(&buf, "Did you mean %s?", "https://example.invalid/")

	got := buf.String()
	if !strings.Contains(got, "Hint:") {
		t.Errorf("expected 'Hint:' prefix, got: %s", got)
	}
	if !strings.Contains(got, "Did you mean https://example.invalid/?") {
		t.Errorf("expected formatted message, got: %s", got)
	}
	// Should be indented
	if !strings.HasPrefix(got, "  ") {
		t.Errorf("expected indented output, got: %s", got)
	}
}

func TestFprintHint_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintHint(&buf, "use --verbose for details")

	got := buf.String()
	if !strings.Contains(got, Cyan) {
		t.Errorf("expected cyan ANSI code in output, got: %s", got)
	}
	if !strings.Contains(got, "Hint:") {
		t.Errorf("expected 'Hint:' prefix, got: %s", got)
	}
	if !strings.Contains(got, "use --verbose for details") {
		t.Errorf("expected message text, got: %s", got)
	}
}

func TestDoctorTags_NoColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	if got := DoctorOK(); got != "[OK]  " {
		t.Errorf("DoctorOK() = %q, want %q", got, "[OK]  ")
	}
	if got := DoctorWarn(); got != "[WARN]" {
		t.Errorf("DoctorWarn() = %q, want %q", got, "[WARN]")
	}
	if got := DoctorFail(); got != "[FAIL]" {
		t.Errorf("DoctorFail() = %q, want %q", got, "[FAIL]")
	}
}

func TestDoctorTags_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	got := DoctorOK()
	if !strings.Contains(got, Bold+Green) {
		t.Errorf("DoctorOK() should contain bold+green ANSI, got: %q", got)
	}
	if !strings.Contains(got, "[OK]") {
		t.Errorf("DoctorOK() should contain [OK], got: %q", got)
	}

	got = DoctorWarn()
	if !strings.Contains(got, Bold+Yellow) {
		t.Errorf("DoctorWarn() should contain bold+yellow ANSI, got: %q", got)
	}
	if !strings.Contains(got, "[WARN]") {
		t.Errorf("DoctorWarn() should contain [WARN], got: %q", got)
	}

	got = DoctorFail()
	if !strings.Contains(got, Bold+Red) {
		t.Errorf("DoctorFail() should contain bold+red ANSI, got: %q", got)
	}
	if !strings.Contains(got, "[FAIL]") {
		t.Errorf("DoctorFail() should contain [FAIL], got: %q", got)
	}
}

func TestFprintDescribeSection_NoColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintDescribeSection(&buf, "Tasks:")

	got := buf.String()
	if got != "Tasks:\n" {
		t.Errorf("FprintDescribeSection() = %q, want %q", got, "Tasks:\n")
	}
}

func TestFprintDescribeSection_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintDescribeSection(&buf, "Criteria:")

	got := buf.String()
	if !strings.Contains(got, Bold) {
		t.Errorf("expected bold ANSI code, got: %q", got)
	}
	if !strings.Contains(got, "Criteria:") {
		t.Errorf("expected section header text, got: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected trailing newline")
	}
}

func TestFprintDescribeKV_NoColor(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintDescribeKV(&buf, "ID:", 13, "%s", "abc-123")

	got := buf.String()
	// Without color, should produce: "ID:          abc-123\n"
	// "ID:" is 3 chars, padding = 13 - 3 = 10 spaces
	expected := "ID:          abc-123\n"
	if got != expected {
		t.Errorf("FprintDescribeKV() = %q, want %q", got, expected)
	}
}

func TestFprintDescribeKV_WithColor(t *testing.T) {
	ResetColorCache()
	os.Unsetenv("NO_COLOR")
	t.Setenv("FORCE_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintDescribeKV(&buf, "Name:", 13, "%s", "my-resource")

	got := buf.String()
	if !strings.Contains(got, Bold) {
		t.Errorf("expected bold ANSI code for label, got: %q", got)
	}
	if !strings.Contains(got, "Name:") {
		t.Errorf("expected label text, got: %q", got)
	}
	if !strings.Contains(got, "my-resource") {
		t.Errorf("expected value text, got: %q", got)
	}
}

func TestFprintDescribeKV_Alignment(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	// Test that different labels align to the same column
	var buf bytes.Buffer
	FprintDescribeKV(&buf, "ID:", 13, "%s", "abc")
	FprintDescribeKV(&buf, "Description:", 13, "%s", "xyz")

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Find the position of the value in each line
	idx1 := strings.Index(lines[0], "abc")
	idx2 := strings.Index(lines[1], "xyz")
	if idx1 != idx2 {
		t.Errorf("values not aligned: 'abc' at %d, 'xyz' at %d\nline1: %q\nline2: %q",
			idx1, idx2, lines[0], lines[1])
	}
}

func TestFprintDescribeKV_FormatArgs(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	var buf bytes.Buffer
	FprintDescribeKV(&buf, "Retention:", 13, "%d days", 90)

	got := buf.String()
	if !strings.Contains(got, "90 days") {
		t.Errorf("expected formatted value, got: %q", got)
	}
}

func TestFprintDescribeKV_MinimumPadding(t *testing.T) {
	ResetColorCache()
	t.Setenv("NO_COLOR", "1")
	defer ResetColorCache()

	// When label is longer than width, should still have at least 1 space
	var buf bytes.Buffer
	FprintDescribeKV(&buf, "Very Long Label:", 5, "%s", "val")

	got := buf.String()
	// Should have at least one space between label and value
	if !strings.Contains(got, "Very Long Label: val") {
		t.Errorf("expected minimum 1 space padding, got: %q", got)
	}
}
