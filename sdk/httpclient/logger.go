package httpclient

// Logger is the interface accepted by the HTTP client for debug output.
// Implementations must be safe for concurrent use.
type Logger interface {
	Debugf(format string, args ...interface{})
}

// noopLogger discards all log output.
type noopLogger struct{}

func (noopLogger) Debugf(string, ...interface{}) {}
