package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/appengine"
)

// saveGlobals saves and restores the printer-related globals around a test.
func saveGlobals(t *testing.T) {
	t.Helper()
	origFmt := outputFormat
	origAgent := agentMode
	origPlain := plainMode
	t.Cleanup(func() {
		outputFormat = origFmt
		agentMode = origAgent
		plainMode = origPlain
	})
}

// TestPrintBodyOnly_InvokeResponse_EnvelopeWithJSONBody verifies that when the
// envelope contains a JSON-encoded body string, only the parsed inner value is
// printed (no statusCode or other envelope fields leak through).
func TestPrintBodyOnly_InvokeResponse_EnvelopeWithJSONBody(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		Body: `{"statusCode":200,"body":"{\"result\":\"ok\"}"}`,
		RawBody: map[string]interface{}{
			"statusCode": float64(200),
			"body":       `{"result":"ok"}`,
		},
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	// Inner value must be present
	if !strings.Contains(output, `"result"`) {
		t.Errorf("expected 'result' key in output, got: %s", output)
	}
	// Envelope field must NOT bleed through
	if strings.Contains(output, "statusCode") {
		t.Errorf("envelope field 'statusCode' leaked into output: %s", output)
	}
	// Output must be valid JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Errorf("output is not valid JSON: %v\noutput: %s", err, output)
	}
}

// TestPrintBodyOnly_InvokeResponse_EnvelopeWithNonJSONBody verifies that a
// non-JSON body string is printed verbatim (no parse attempt, no mangling).
func TestPrintBodyOnly_InvokeResponse_EnvelopeWithNonJSONBody(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		RawBody: map[string]interface{}{
			"statusCode": float64(200),
			"body":       "plain text result",
		},
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if output != "plain text result\n" {
		t.Errorf("expected 'plain text result\\n', got: %q", output)
	}
}

// TestPrintBodyOnly_InvokeResponse_NoEnvelope verifies that when RawBody is nil
// (no JSON was parsed), the fallback prints the raw Body text as-is.
func TestPrintBodyOnly_InvokeResponse_NoEnvelope(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		RawBody: nil,
		Body:    "raw response text",
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if output != "raw response text\n" {
		t.Errorf("expected 'raw response text\\n', got: %q", output)
	}
}

// TestPrintBodyOnly_ExecutorResponse_WithResult verifies that the parsed Result
// from an ad-hoc execution is pretty-printed as JSON without the surrounding
// Logs field.
func TestPrintBodyOnly_ExecutorResponse_WithResult(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionExecutorResponse{
		Result: map[string]interface{}{"answer": float64(42)},
		Logs:   "some log output",
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	// Result field must appear
	if !strings.Contains(output, `"answer"`) {
		t.Errorf("expected 'answer' key in output, got: %s", output)
	}
	// Logs must NOT appear — body-only means only the return value
	if strings.Contains(output, "logs") {
		t.Errorf("'logs' field leaked into output: %s", output)
	}
	// Output must be valid JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Errorf("output is not valid JSON: %v\noutput: %s", err, output)
	}
}

// TestPrintBodyOnly_ExecutorResponse_NilResult verifies that a nil Result is
// printed as the JSON null literal, not as an empty string or Go "<nil>".
func TestPrintBodyOnly_ExecutorResponse_NilResult(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionExecutorResponse{
		Result: nil,
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if output != "null\n" {
		t.Errorf("expected 'null\\n', got: %q", output)
	}
}

// TestPrintBodyOnly_InvokeResponse_NonEnvelopeJSON verifies that when RawBody is
// a plain JSON object without a "body" key (i.e. no envelope), the full parsed
// response is pretty-printed rather than the raw compact string.
func TestPrintBodyOnly_InvokeResponse_NonEnvelopeJSON(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		Body:    `{"answer":42}`,
		RawBody: map[string]interface{}{"answer": float64(42)},
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(output, `"answer"`) {
		t.Errorf("expected 'answer' key in output, got: %s", output)
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Errorf("output is not valid JSON: %v\noutput: %s", err, output)
	}
}

// TestPrintBodyOnly_InvokeResponse_EnvelopeWithObjectBody verifies that when the
// envelope's "body" is already a JSON object (not a JSON-encoded string), it is
// marshaled directly rather than triggering the string-parse path.
func TestPrintBodyOnly_InvokeResponse_EnvelopeWithObjectBody(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		Body: `{"statusCode":200,"body":{"key":"value"}}`,
		RawBody: map[string]interface{}{
			"statusCode": float64(200),
			"body":       map[string]interface{}{"key": "value"},
		},
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(output, `"key"`) {
		t.Errorf("expected 'key' in output, got: %s", output)
	}
	if strings.Contains(output, "statusCode") {
		t.Errorf("envelope field 'statusCode' leaked into output: %s", output)
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Errorf("output is not valid JSON: %v\noutput: %s", err, output)
	}
}

// TestPrintBodyOnly_DeferredResponse verifies that a DeferredExecutionResponse
// (which has no meaningful body) does not cause an error and produces output
// containing the execution ID via the standard printer fallback.
func TestPrintBodyOnly_DeferredResponse(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.DeferredExecutionResponse{ID: "exec-123"}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Errorf("printBodyOnly returned unexpected error for deferred response: %v", err)
			}
		})
	})

	if !strings.Contains(output, "exec-123") {
		t.Errorf("expected execution ID in output, got: %s", output)
	}
}

// TestPrintBodyOnly_InvokeResponse_ZeroStatusNoStderr verifies that a zero
// StatusCode (unset) produces no stderr output — avoids spurious "Status: 0".
func TestPrintBodyOnly_InvokeResponse_ZeroStatusNoStderr(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		StatusCode: 0,
		Body:       "hello",
		RawBody:    nil,
	}

	var stderrOut string
	captureStdout(t, func() {
		stderrOut = captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if stderrOut != "" {
		t.Errorf("expected no stderr for zero StatusCode, got: %q", stderrOut)
	}
}

// TestPrintBodyOnly_InvokeResponse_EnvelopeStatusCodePreferred verifies that
// the envelope's own statusCode is used for the stderr metadata rather than
// the outer HTTP status (which is often always 200).
func TestPrintBodyOnly_InvokeResponse_EnvelopeStatusCodePreferred(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		StatusCode: 200, // outer HTTP status
		RawBody: map[string]interface{}{
			"statusCode": float64(404), // function's own status
			"body":       `{"error":"not found"}`,
		},
	}

	var stderrOut string
	captureStdout(t, func() {
		stderrOut = captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(stderrOut, "Status: 404") {
		t.Errorf("expected envelope statusCode 404 on stderr, got: %q", stderrOut)
	}
	if strings.Contains(stderrOut, "200") {
		t.Errorf("outer HTTP status 200 leaked into stderr, expected 404: %q", stderrOut)
	}
}

// TestPrintBodyOnly_InvokeResponse_StatusOnStderr verifies that the HTTP status
// code is written to stderr (not stdout) when --body-only is active.
func TestPrintBodyOnly_InvokeResponse_StatusOnStderr(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		StatusCode: 200,
		RawBody: map[string]interface{}{
			"statusCode": float64(200),
			"body":       `{"result":"ok"}`,
		},
	}

	var stderrOut string
	captureStdout(t, func() {
		stderrOut = captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(stderrOut, "Status: 200") {
		t.Errorf("expected 'Status: 200' on stderr, got: %q", stderrOut)
	}
}

// TestPrintBodyOnly_ExecutorResponse_LogsOnStderr verifies that execution logs
// go to stderr and are absent from stdout when --body-only is active.
func TestPrintBodyOnly_ExecutorResponse_LogsOnStderr(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionExecutorResponse{
		Result: map[string]interface{}{"x": float64(1)},
		Logs:   "console.log output here",
	}

	var stderrOut, stdoutOut string
	stdoutOut = captureStdout(t, func() {
		stderrOut = captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(stderrOut, "console.log output here") {
		t.Errorf("expected logs on stderr, got: %q", stderrOut)
	}
	if strings.Contains(stdoutOut, "console.log output here") {
		t.Errorf("logs leaked into stdout: %q", stdoutOut)
	}
}

// TestPrintBodyOnly_ExecutorResponse_EmptyLogsNoStderr verifies that an empty
// Logs field produces no output on stderr.
func TestPrintBodyOnly_ExecutorResponse_EmptyLogsNoStderr(t *testing.T) {
	saveGlobals(t)
	outputFormat = "json"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionExecutorResponse{
		Result: map[string]interface{}{"x": float64(1)},
		Logs:   "",
	}

	var stderrOut string
	captureStdout(t, func() {
		stderrOut = captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if stderrOut != "" {
		t.Errorf("expected no stderr output for empty logs, got: %q", stderrOut)
	}
}

// TestPrintBodyOnly_InvokeResponse_YAMLFormat verifies that -o yaml formats
// the extracted body as YAML rather than JSON.
func TestPrintBodyOnly_InvokeResponse_YAMLFormat(t *testing.T) {
	saveGlobals(t)
	outputFormat = "yaml"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionInvokeResponse{
		StatusCode: 200,
		RawBody: map[string]interface{}{
			"statusCode": float64(200),
			"body":       `{"answer":42}`,
		},
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	// YAML format uses "key: value" syntax, not JSON braces
	if !strings.Contains(output, "answer:") {
		t.Errorf("expected YAML key 'answer:' in output, got: %q", output)
	}
	if strings.Contains(output, `"answer"`) {
		t.Errorf("JSON-style quoted key leaked into YAML output: %q", output)
	}
}

// TestPrintBodyOnly_ExecutorResponse_YAMLFormat verifies that -o yaml formats
// the Result from an ad-hoc execution as YAML.
func TestPrintBodyOnly_ExecutorResponse_YAMLFormat(t *testing.T) {
	saveGlobals(t)
	outputFormat = "yaml"
	agentMode = false
	plainMode = false

	input := &appengine.FunctionExecutorResponse{
		Result: map[string]interface{}{"greeting": "hello"},
		Logs:   "",
	}

	output := captureStdout(t, func() {
		captureStderr(t, func() {
			if err := printBodyOnly(input); err != nil {
				t.Fatalf("printBodyOnly returned unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(output, "greeting:") {
		t.Errorf("expected YAML key 'greeting:' in output, got: %q", output)
	}
	if strings.Contains(output, `"greeting"`) {
		t.Errorf("JSON-style quoted key leaked into YAML output: %q", output)
	}
}
