package dqlcost

import (
	"strings"
	"testing"
)

func TestRewriteDocument_RewritesTileQueries(t *testing.T) {
	in := []byte(`name: Test
type: dashboard
content:
  tiles:
    "1":
      title: Errors
      type: data
      query: |
        fetch logs | filter loglevel == "ERROR"
`)
	out, changed, changes, err := RewriteDocument(in)
	if err != nil {
		t.Fatalf("RewriteDocument: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one change")
	}
	s := string(out)
	if !strings.Contains(s, "from:now()-2h") {
		t.Errorf("default from not injected:\n%s", s)
	}
	if !strings.Contains(s, "scanLimitGBytes:500") {
		t.Errorf("scan limit not injected:\n%s", s)
	}
}

func TestRewriteDocument_SkipsCleanDocument(t *testing.T) {
	in := []byte(`name: Test
type: dashboard
content:
  tiles:
    "1":
      type: data
      query: |
        fetch logs, from:now()-1h, scanLimitGBytes:50 | filter loglevel == "ERROR"
`)
	out, changed, _, err := RewriteDocument(in)
	if err != nil {
		t.Fatalf("RewriteDocument: %v", err)
	}
	if changed {
		t.Errorf("clean doc should not be changed; got:\n%s", string(out))
	}
}

func TestRewriteDocument_IgnoresNonQueryKeys(t *testing.T) {
	in := []byte(`name: Test
description: "something about query"
content:
  tiles:
    "1":
      type: markdown
      content: "fetch logs | limit 10"
`)
	_, changed, _, err := RewriteDocument(in)
	if err != nil {
		t.Fatalf("RewriteDocument: %v", err)
	}
	if changed {
		t.Error("non-query keys should not be rewritten")
	}
}
