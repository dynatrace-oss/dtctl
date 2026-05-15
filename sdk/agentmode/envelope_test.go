package agentmode

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{400, "bad_request"},
		{401, "auth_required"},
		{403, "permission_denied"},
		{404, "not_found"},
		{409, "conflict"},
		{429, "rate_limited"},
		{500, "server_error"},
		{503, "server_error"},
		{418, "error"},
	}
	for _, tt := range tests {
		if got := ClassifyHTTPError(tt.code); got != tt.want {
			t.Errorf("ClassifyHTTPError(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestWriteSuccess(t *testing.T) {
	var buf bytes.Buffer
	total := 5
	ctx := &ResponseContext{Total: &total, Verb: "get", Resource: "workflow"}
	err := WriteSuccess(&buf, []string{"a", "b"}, ctx)
	if err != nil {
		t.Fatal(err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Error("expected ok=true")
	}
	if resp.Context.Verb != "get" {
		t.Errorf("verb = %q, want get", resp.Context.Verb)
	}
}

func TestWriteError(t *testing.T) {
	var buf bytes.Buffer
	err := WriteError(&buf, &ErrorDetail{
		Code:    "not_found",
		Message: "workflow not found",
	})
	if err != nil {
		t.Fatal(err)
	}

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Error("expected ok=false")
	}
	if resp.Error.Code != "not_found" {
		t.Errorf("error code = %q", resp.Error.Code)
	}
}
