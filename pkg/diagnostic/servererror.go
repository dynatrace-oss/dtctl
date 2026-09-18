package diagnostic

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// maxBodyExcerpt caps how much of an unparseable body reaches the user. A proxy
// or WAF can answer a 403 with a multi-KB HTML page, which would otherwise
// become the whole error message — and, in agent mode, the whole "message"
// field of the envelope. Mirrors the 1 KB cap in sdk/httpclient.CheckResponse.
const maxBodyExcerpt = 1024

// ServerErrorMessage extracts the human-readable reason from a platform error
// body, falling back to a truncated excerpt of the raw body so the server's
// explanation is never silently dropped.
//
// Platform errors arrive in two shapes — {"error":{"message":…,"details":{…}}}
// and a bare {"message":…} — and "details" of the first is where a 403 that
// really is a scope problem names the scopes it wanted. Dropping it would send
// the caller after the IAM policy gate when the token was at fault, so it is
// appended to the message.
func ServerErrorMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	var envelope struct {
		Error *struct {
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err == nil {
		switch {
		case envelope.Error != nil && envelope.Error.Message != "":
			msg := envelope.Error.Message
			if details := errorDetails(envelope.Error.Details); details != "" {
				msg += " (" + details + ")"
			}
			return msg
		case envelope.Message != "":
			return envelope.Message
		}
	}

	return truncateExcerpt(body)
}

// errorDetails renders the "details" of a platform error envelope. The
// missingScopes shape settles the scope-vs-policy question outright, so it is
// named explicitly; a plain string and any other shape are passed through so
// nothing is hidden.
func errorDetails(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return truncateExcerpt(strings.TrimSpace(s))
	}

	var obj struct {
		MissingScopes []string `json:"missingScopes"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && len(obj.MissingScopes) > 0 {
		return "missing scopes: " + strings.Join(obj.MissingScopes, ", ")
	}

	return truncateExcerpt(strings.TrimSpace(string(raw)))
}

// truncateExcerpt caps s at maxBodyExcerpt bytes without splitting a rune.
func truncateExcerpt(s string) string {
	if len(s) <= maxBodyExcerpt {
		return s
	}
	cut := s[:maxBodyExcerpt]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "... (truncated)"
}
