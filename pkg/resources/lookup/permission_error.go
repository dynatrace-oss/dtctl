package lookup

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dynatrace-oss/dtctl/pkg/diagnostic"
)

// maxBodyExcerpt caps how much of an unparseable body reaches the user. A proxy
// or WAF can answer a 403 with a multi-KB HTML page, which would otherwise
// become the whole error message — and, in agent mode, the whole "message"
// field of the envelope. Mirrors the 1 KB cap in sdk/httpclient.CheckResponse.
const maxBodyExcerpt = 1024

// forbiddenError builds the 403 error for a Resource Store file operation.
//
// A Grail file operation passes two independent gates that share a name: the
// OAuth scope in the token (e.g. storage:files:delete) and the IAM policy
// permission of the same name, which is additionally conditioned on
// storage:file-path. A token minted at --safety-level dangerously-unrestricted
// carries every storage:files:* scope, so a 403 here almost always means the
// policy gate, not the scope gate — but the old message ("access denied to
// delete file") read like a token problem and dropped the response body that
// said otherwise, which is how github.com/dynatrace-oss/dtctl#344 reached
// support as a missing-scope bug.
//
// verb is the user-facing operation ("delete"/"write"), permission the scope
// and IAM permission name, and cmdHint the command that verifies the scope gate.
func forbiddenError(verb, permission, cmdHint, path, body string) error {
	// Operation carries the "what", Message the server's "why" — diagnostic.Error
	// renders them as "Failed to <operation> (HTTP 403): <message>", so repeating
	// the path or verb in both would read twice.
	msg := serverErrorMessage(body)
	if msg == "" {
		msg = "access denied, and the server returned no reason"
	}

	return &diagnostic.Error{
		Operation:  fmt.Sprintf("%s file %q", verb, path),
		StatusCode: 403,
		Message:    msg,
		Suggestions: []string{
			fmt.Sprintf("The OAuth scope and the Grail IAM permission are separate gates, both named %q — a granted scope alone is not enough", permission),
			fmt.Sprintf("Check the scope gate first: %s", cmdHint),
			fmt.Sprintf("If that reports \"ok\", or \"unknown\" because the token is not introspectable, the tenant's IAM policy is missing the permission. Add: ALLOW %s WHERE storage:file-path startsWith \"/lookups/\";", permission),
			"Policy permissions can be path-scoped, so access to one /lookups/ subtree does not imply access to another",
		},
	}
}

// serverErrorMessage extracts the human-readable reason from a Resource Store
// error body, falling back to a truncated excerpt of the raw body so the
// server's explanation is never silently dropped.
//
// Platform errors arrive in two shapes — {"error":{"message":…,"details":{…}}}
// and a bare {"message":…} — and "details" of the first is where a 403 that
// really is a scope problem names the scopes it wanted. Dropping it would send
// the caller after the IAM policy gate when the token was at fault, so it is
// appended to the message.
func serverErrorMessage(body string) string {
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
