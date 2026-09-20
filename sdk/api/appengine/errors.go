package appengine

import "fmt"

// App Engine non-standard HTTP status codes. The platform answers with these
// when the request itself was fine and the submitted code failed.
const (
	statusJSError     = 540
	statusSyntaxError = 541
)

// ExecutionError reports a failure inside the submitted function code.
//
// It is deliberately not an httpclient.APIError. The request reached App Engine
// and App Engine answered; what failed is the caller's own code. Carrying it as
// a 5xx would classify a deterministic bug as a Dynatrace-side outage, which
// tells an agent to retry something that can never succeed unchanged.
type ExecutionError struct {
	// StatusCode is the non-standard status App Engine used to report the failure.
	StatusCode int
	// Message is the platform's description of the failure.
	Message string
	// Body is the raw response body, which holds the stack trace when there is one.
	Body string
}

func (e *ExecutionError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Body)
	}
	return e.Message
}

// newExecutionError builds the error for a non-standard App Engine status, or
// returns nil when the status is not one of them.
func newExecutionError(statusCode int, body string) *ExecutionError {
	switch statusCode {
	case statusJSError:
		return &ExecutionError{StatusCode: statusJSError, Message: "JavaScript error occurred", Body: body}
	case statusSyntaxError:
		return &ExecutionError{StatusCode: statusSyntaxError, Message: "runtime error occurred", Body: body}
	}
	return nil
}
