package output

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplyJQ_SingleResult(t *testing.T) {
	in := map[string]interface{}{"name": "alpha", "id": 42}
	out, err := ApplyJQ(".name", in)
	if err != nil {
		t.Fatalf("ApplyJQ failed: %v", err)
	}

	if out != "alpha" {
		t.Fatalf("expected filtered value 'alpha', got: %#v", out)
	}
}

func TestApplyJQ_MultiResult(t *testing.T) {
	in := []map[string]interface{}{
		{"name": "alpha", "id": "1"},
		{"name": "beta", "id": "2"},
	}
	out, err := ApplyJQ(".[] | {name: .name}", in)
	if err != nil {
		t.Fatalf("ApplyJQ failed: %v", err)
	}

	want := []interface{}{
		map[string]interface{}{"name": "alpha"},
		map[string]interface{}{"name": "beta"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("unexpected result:\nwant: %#v\ngot:  %#v", want, out)
	}
}

func TestApplyJQ_InvalidFilter(t *testing.T) {
	errInput := map[string]interface{}{"name": "alpha"}
	_, err := ApplyJQ(".[", errInput)
	if err == nil {
		t.Fatal("expected invalid jq filter error")
	}
	if !strings.Contains(err.Error(), "invalid --jq filter") {
		t.Fatalf("expected invalid filter error, got: %v", err)
	}
}

func TestNormalizeJQOutputFormat(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "json", want: "json"},
		{in: "yaml", want: "yaml"},
		{in: "yml", want: "yml"},
		{in: "toon", want: "toon"},
		{in: "table", want: "json"},
		{in: "csv", want: "json"},
		{in: "", want: "json"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizeJQOutputFormat(tt.in); got != tt.want {
				t.Fatalf("NormalizeJQOutputFormat(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
