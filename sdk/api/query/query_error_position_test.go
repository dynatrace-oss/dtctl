package query

import (
	"context"
	"net/http"
	"testing"
)

// TestExecute_ErrorResponseCarriesPositionAndQuery pins the fields a caller
// needs to point at the offending part of a query: Grail reports the query it
// parsed (details.queryString) and the offending span
// (details.syntaxErrorPosition, 1-based line/column, end inclusive).
func TestExecute_ErrorResponseCarriesPositionAndQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"PARSE_ERROR","details":{"exceptionType":"DQL-ERROR-PARSING",` +
			`"errorType":"PARSE_ERROR","errorMessage":"` + "`by`" + ` isn't allowed here.","arguments":["` + "`by`" + `"],` +
			`"queryString":"fetch logs | summarize count() by service.name",` +
			`"syntaxErrorPosition":{"start":{"column":32,"index":31,"line":1},"end":{"column":33,"index":32,"line":1}}},"code":400}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Execute(context.Background(), ExecuteRequest{Query: "fetch logs | summarize count() by service.name"})

	var qErr *QueryError
	if !errorAsQueryError(err, &qErr) {
		t.Fatalf("expected *QueryError, got %T: %v", err, err)
	}
	if qErr.Query != "fetch logs | summarize count() by service.name" {
		t.Errorf("Query = %q", qErr.Query)
	}
	p := qErr.Position
	if p == nil || p.Start == nil || p.End == nil {
		t.Fatalf("Position = %+v, want start and end", p)
	}
	if *p.Start != (Position{Line: 1, Column: 32}) || *p.End != (Position{Line: 1, Column: 33}) {
		t.Errorf("Position = %+v..%+v, want 1:32..1:33", *p.Start, *p.End)
	}
}

// TestExecute_ErrorResponseWithoutPosition keeps errors that carry no
// position (auth, quota, older backends) free of a fabricated one.
func TestExecute_ErrorResponseWithoutPosition(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid query","details":{"errorType":"SYNTAX_ERROR","errorMessage":"parse error"}}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Execute(context.Background(), ExecuteRequest{Query: "bad"})

	var qErr *QueryError
	if !errorAsQueryError(err, &qErr) {
		t.Fatalf("expected *QueryError, got %T: %v", err, err)
	}
	if qErr.Position != nil || qErr.Query != "" {
		t.Errorf("Position = %+v, Query = %q; want both empty", qErr.Position, qErr.Query)
	}
}
