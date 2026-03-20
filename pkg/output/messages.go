package output

import (
	"fmt"
	"io"
	"os"
)

// PrintSuccess prints a success message with a green "OK" prefix.
// Output goes to stderr so it doesn't interfere with structured stdout.
func PrintSuccess(format string, args ...interface{}) {
	FprintSuccess(os.Stderr, format, args...)
}

// FprintSuccess prints a success message to the given writer.
func FprintSuccess(w io.Writer, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	prefix := Colorize(Bold+Green, "OK")
	fmt.Fprintf(w, "%s %s\n", prefix, msg)
}

// PrintWarning prints a warning message with a yellow prefix.
// Output goes to stderr so it doesn't interfere with structured stdout.
func PrintWarning(format string, args ...interface{}) {
	FprintWarning(os.Stderr, format, args...)
}

// FprintWarning prints a warning message to the given writer.
func FprintWarning(w io.Writer, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	prefix := Colorize(Bold+Yellow, "Warning:")
	fmt.Fprintf(w, "%s %s\n", prefix, msg)
}

// PrintHumanError prints a human-readable error message to stderr.
// Uses bold red for the "Error:" prefix. For agent/plain mode, use PrintError instead.
func PrintHumanError(format string, args ...interface{}) {
	FprintHumanError(os.Stderr, format, args...)
}

// FprintHumanError prints a human-readable error message to the given writer.
func FprintHumanError(w io.Writer, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	prefix := Colorize(Bold+Red, "Error:")
	fmt.Fprintf(w, "%s %s\n", prefix, msg)
}

// PrintHint prints a hint message to stderr, indented with a cyan label.
// Use this for actionable suggestions that follow an error or warning.
func PrintHint(format string, args ...interface{}) {
	FprintHint(os.Stderr, format, args...)
}

// FprintHint prints a hint message to the given writer.
func FprintHint(w io.Writer, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	label := Colorize(Cyan, "Hint:")
	fmt.Fprintf(w, "  %s %s\n", label, msg)
}

// PrintInfo prints an informational message to stderr.
// No prefix or color is applied — use this for supplementary detail lines
// (e.g., IDs, URLs) that accompany a PrintSuccess message.
// Output goes to stderr so it doesn't interfere with structured stdout.
func PrintInfo(format string, args ...interface{}) {
	FprintInfo(os.Stderr, format, args...)
}

// FprintInfo prints an informational message to the given writer.
func FprintInfo(w io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(w, format+"\n", args...)
}

// doctorTagWidth is the fixed visual width for doctor status tags.
// All tags are padded to this width so columns align regardless of tag length.
const doctorTagWidth = 6

// DoctorOK returns a styled "[OK]" tag for doctor check output, padded to doctorTagWidth.
func DoctorOK() string {
	return Colorize(Bold+Green, fmt.Sprintf("%-*s", doctorTagWidth, "[OK]"))
}

// DoctorWarn returns a styled "[WARN]" tag for doctor check output, padded to doctorTagWidth.
func DoctorWarn() string {
	return Colorize(Bold+Yellow, fmt.Sprintf("%-*s", doctorTagWidth, "[WARN]"))
}

// DoctorFail returns a styled "[FAIL]" tag for doctor check output, padded to doctorTagWidth.
func DoctorFail() string {
	return Colorize(Bold+Red, fmt.Sprintf("%-*s", doctorTagWidth, "[FAIL]"))
}

// DescribeSection prints a bold section header (e.g., "Tasks:", "Criteria:").
// The header is printed on its own line with no trailing content.
func DescribeSection(header string) {
	FprintDescribeSection(os.Stdout, header)
}

// FprintDescribeSection prints a bold section header to the given writer.
func FprintDescribeSection(w io.Writer, header string) {
	fmt.Fprintf(w, "%s\n", Colorize(Bold, header))
}

// DescribeKV prints a key-value line with a bold label, right-padded to width.
// width is the total width of the label column (including the colon and padding).
// The value is formatted with the given format string and args.
//
// Example: DescribeKV("ID:", 13, "%s", "abc-123")
// Output:  ID:          abc-123    (where "ID:" is bold)
func DescribeKV(label string, width int, format string, args ...interface{}) {
	FprintDescribeKV(os.Stdout, label, width, format, args...)
}

// FprintDescribeKV prints a key-value line with a bold label to the given writer.
func FprintDescribeKV(w io.Writer, label string, width int, format string, args ...interface{}) {
	value := fmt.Sprintf(format, args...)
	boldLabel := Colorize(Bold, label)
	// Pad with spaces after the (non-ANSI) label to reach the desired width
	padding := width - len(label)
	if padding < 1 {
		padding = 1
	}
	fmt.Fprintf(w, "%s%*s%s\n", boldLabel, padding, "", value)
}
