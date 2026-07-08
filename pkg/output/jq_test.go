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

func TestCompileJQ_RunRecord(t *testing.T) {
	prog, err := CompileJQ(`select(.status == 500)`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Matching record passes through.
	out, err := prog.RunRecord(map[string]interface{}{"host": "web-02", "status": 500})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 emitted value, got %d (%#v)", len(out), out)
	}

	// Non-matching record is dropped (empty output is how filtering works).
	out, err = prog.RunRecord(map[string]interface{}{"host": "web-01", "status": 200})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want 0 emitted values for a non-match, got %#v", out)
	}
}

func TestCompileJQ_Invalid(t *testing.T) {
	if _, err := CompileJQ(`select(`); err == nil {
		t.Fatal("expected a compile error for an invalid program")
	} else if !strings.Contains(err.Error(), "invalid --jq filter") {
		t.Fatalf("err = %v, want invalid --jq filter", err)
	}
}

func TestCompileJQ_RunRecord_Reusable(t *testing.T) {
	// A compiled program must be safe to run over many records in sequence.
	prog, err := CompileJQ(`{h: .host}`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, host := range []string{"a", "b", "c"} {
		out, err := prog.RunRecord(map[string]interface{}{"host": host})
		if err != nil {
			t.Fatalf("run %s: %v", host, err)
		}
		if len(out) != 1 {
			t.Fatalf("host %s: want 1 value, got %#v", host, out)
		}
		obj, ok := out[0].(map[string]interface{})
		if !ok || obj["h"] != host {
			t.Fatalf("host %s: got %#v", host, out[0])
		}
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
