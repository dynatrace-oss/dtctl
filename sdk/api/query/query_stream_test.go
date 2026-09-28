package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

// --- decodeResponseStream: row-by-row streaming behavior ---

func TestDecodeResponseStream_StreamsResultRecordsInOrder(t *testing.T) {
	body := `{
		"state": "SUCCEEDED",
		"result": {
			"records": [{"n": 1}, {"n": 2}, {"n": 3}]
		}
	}`

	var got []map[string]interface{}
	resp, err := decodeResponseStream(bytes.NewBufferString(body), recordSink{
		onResult: func(row map[string]interface{}) error {
			got = append(got, row)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("decodeResponseStream() error: %v", err)
	}
	if resp.State != "SUCCEEDED" {
		t.Errorf("State = %q, want SUCCEEDED", resp.State)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows via callback, want 3", len(got))
	}
	for i, want := range []float64{1, 2, 3} {
		if got[i]["n"] != want {
			t.Errorf("row %d: n = %v, want %v", i, got[i]["n"], want)
		}
	}
	// decodeResponseStream never accumulates rows itself — only the presence marker.
	if resp.Result.Records == nil || len(resp.Result.Records) != 0 {
		t.Errorf("Result.Records = %#v, want a non-nil empty presence marker", resp.Result.Records)
	}
}

func TestDecodeResponseStream_StreamsTopLevelRecords(t *testing.T) {
	body := `{"state": "SUCCEEDED", "records": [{"a": 1}, {"a": 2}]}`

	var got []map[string]interface{}
	resp, err := decodeResponseStream(bytes.NewBufferString(body), recordSink{
		onTop: func(row map[string]interface{}) error {
			got = append(got, row)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("decodeResponseStream() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows via callback, want 2", len(got))
	}
	if resp.Records == nil || len(resp.Records) != 0 {
		t.Errorf("Records = %#v, want a non-nil empty presence marker", resp.Records)
	}
}

func TestDecodeResponseStream_CallbackErrorAborts(t *testing.T) {
	body := `{"result": {"records": [{"n": 1}, {"n": 2}, {"n": 3}]}}`
	sentinel := errors.New("boom")

	seen := 0
	_, err := decodeResponseStream(bytes.NewBufferString(body), recordSink{
		onResult: func(row map[string]interface{}) error {
			seen++
			if seen == 2 {
				return sentinel
			}
			return nil
		},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want wrapping %v", err, sentinel)
	}
	if seen != 2 {
		t.Errorf("callback invoked %d times, want exactly 2 (abort on the 2nd)", seen)
	}
}

func TestDecodeResponseStream_RejectsTrailingData(t *testing.T) {
	body := `{"state": "SUCCEEDED"} trailing garbage`
	if _, err := decodeResponseStream(bytes.NewBufferString(body), recordSink{}); err == nil {
		t.Fatal("expected an error for trailing data after the response object")
	}
}

func TestDecodeResponseStream_AllowsTrailingWhitespace(t *testing.T) {
	body := "{\"state\": \"SUCCEEDED\"}   \n"
	if _, err := decodeResponseStream(bytes.NewBufferString(body), recordSink{}); err != nil {
		t.Fatalf("decodeResponseStream() error: %v, want nil for trailing whitespace only", err)
	}
}

func TestDecodeResponseStream_PresenceFidelity(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantTop    bool // Records != nil after decode
		wantResult bool // Result != nil
		wantResRec bool // Result.Records != nil after decode
	}{
		{name: "no result, no top records", body: `{"state":"SUCCEEDED"}`},
		{name: "result absent records key", body: `{"result":{"types":[]}}`, wantResult: true},
		{name: "result null", body: `{"result":null}`},
		{name: "result records null", body: `{"result":{"records":null}}`, wantResult: true},
		{name: "result records empty", body: `{"result":{"records":[]}}`, wantResult: true, wantResRec: true},
		{name: "result records populated", body: `{"result":{"records":[{"a":1}]}}`, wantResult: true, wantResRec: true},
		{name: "top records null", body: `{"records":null}`},
		{name: "top records empty", body: `{"records":[]}`, wantTop: true},
		{name: "top records populated", body: `{"records":[{"a":1}]}`, wantTop: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := decodeResponseStream(bytes.NewBufferString(tt.body), recordSink{})
			if err != nil {
				t.Fatalf("decodeResponseStream() error: %v", err)
			}
			if got := resp.Records != nil; got != tt.wantTop {
				t.Errorf("Records != nil = %v, want %v", got, tt.wantTop)
			}
			if got := resp.Result != nil; got != tt.wantResult {
				t.Errorf("Result != nil = %v, want %v", got, tt.wantResult)
			}
			if resp.Result != nil {
				if got := resp.Result.Records != nil; got != tt.wantResRec {
					t.Errorf("Result.Records != nil = %v, want %v", got, tt.wantResRec)
				}
			}
		})
	}
}

// --- ExecuteStream / PollStream: public, non-accumulating API ---

func TestExecuteStream_InvokesCallbackAndLeavesRecordsNil(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			State:  "SUCCEEDED",
			Result: &Result{Records: []map[string]interface{}{{"n": 1}, {"n": 2}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	var got []map[string]interface{}
	resp, err := h.ExecuteStream(context.Background(), ExecuteRequest{Query: "fetch logs"}, func(row map[string]interface{}) error {
		got = append(got, row)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteStream() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows via callback, want 2", len(got))
	}
	if resp.Result.Records != nil {
		t.Errorf("Result.Records = %#v, want nil — ExecuteStream must never accumulate", resp.Result.Records)
	}
	if resp.Records != nil {
		t.Errorf("Records = %#v, want nil", resp.Records)
	}
}

func TestExecuteStream_AsyncPollsWithStreamingDecode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(Response{State: "RUNNING", RequestToken: "tok-stream"})
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("request-token") != "tok-stream" {
			t.Errorf("unexpected request-token: %s", r.URL.Query().Get("request-token"))
		}
		resp := Response{State: "SUCCEEDED", Result: &Result{Records: []map[string]interface{}{{"n": 1}}}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	resp, err := h.ExecuteStream(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)
	if err != nil {
		t.Fatalf("ExecuteStream() error: %v", err)
	}
	if resp.State != "RUNNING" || resp.RequestToken != "tok-stream" {
		t.Fatalf("resp = %+v, want RUNNING/tok-stream", resp)
	}

	var got []map[string]interface{}
	polled, err := h.PollStream(context.Background(), resp.RequestToken, 5000, false, func(row map[string]interface{}) error {
		got = append(got, row)
		return nil
	})
	if err != nil {
		t.Fatalf("PollStream() error: %v", err)
	}
	if polled.State != "SUCCEEDED" {
		t.Fatalf("polled.State = %q, want SUCCEEDED", polled.State)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows via callback, want 1", len(got))
	}
}

func TestPollStream_StreamsRowsAndReportsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"jwt expired"}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.PollStream(context.Background(), "tok-abc", 5000, false, nil)
	if err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
}

func TestPollStream_StreamsResultRecords(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			State:  "SUCCEEDED",
			Result: &Result{Records: []map[string]interface{}{{"row": "one"}, {"row": "two"}, {"row": "three"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	var got []map[string]interface{}
	resp, err := h.PollStream(context.Background(), "tok-abc", 5000, false, func(row map[string]interface{}) error {
		got = append(got, row)
		return nil
	})
	if err != nil {
		t.Fatalf("PollStream() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows via callback, want 3", len(got))
	}
	if resp.Result.Records != nil {
		t.Errorf("Result.Records = %#v, want nil", resp.Result.Records)
	}
}

// --- Execute/Poll: presence fidelity after the streaming rewrite ---

func TestExecute_RecordsPresenceFidelityMatchesUnmarshal(t *testing.T) {
	tests := []struct {
		name           string
		serverBody     string
		wantTopNil     bool
		wantResultNil  bool
		wantResRecsNil bool
		wantResRecsLen int
	}{
		{
			name:          "no result at all",
			serverBody:    `{"state":"SUCCEEDED"}`,
			wantTopNil:    true,
			wantResultNil: true,
		},
		{
			name:           "result with empty records",
			serverBody:     `{"state":"SUCCEEDED","result":{"records":[]}}`,
			wantTopNil:     true,
			wantResRecsNil: false,
			wantResRecsLen: 0,
		},
		{
			name:           "result with two records",
			serverBody:     `{"state":"SUCCEEDED","result":{"records":[{"a":1},{"a":2}]}}`,
			wantTopNil:     true,
			wantResRecsNil: false,
			wantResRecsLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.serverBody))
			})

			h := NewHandler(newTestClient(t, mux))
			resp, err := h.Execute(context.Background(), ExecuteRequest{Query: "fetch logs"})
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}

			if got := resp.Records == nil; got != tt.wantTopNil {
				t.Errorf("Records == nil = %v, want %v", got, tt.wantTopNil)
			}
			if tt.wantResultNil {
				if resp.Result != nil {
					t.Fatalf("Result = %+v, want nil", resp.Result)
				}
				return
			}
			if resp.Result == nil {
				t.Fatal("Result = nil, want non-nil")
			}
			if got := resp.Result.Records == nil; got != tt.wantResRecsNil {
				t.Errorf("Result.Records == nil = %v, want %v", got, tt.wantResRecsNil)
			}
			if len(resp.Result.Records) != tt.wantResRecsLen {
				t.Errorf("len(Result.Records) = %d, want %d", len(resp.Result.Records), tt.wantResRecsLen)
			}
		})
	}
}

// Regression guard: decodes a fully-populated Response with both encoding/json and
// decodeResponseStream and requires identical results, catching a field added without
// a matching case in decodeResponseStream/decodeResultStream.
func TestDecodeResponseStream_AllFields(t *testing.T) {
	fixture := Response{
		State:        "SUCCEEDED",
		RequestToken: "tok-fixture",
		Progress:     42,
		Records:      []map[string]interface{}{{"top": "row"}},
		Metadata: &Metadata{
			Metrics: []MetricInfo{{
				MetricKey:   "dt.host.cpu.usage",
				FieldName:   "cpu",
				Aggregation: "avg",
				DisplayName: "CPU Usage",
				Description: "desc",
				Unit:        "Percent",
			}},
			Grail: &GrailMetadata{
				Query:                     "fetch logs",
				CanonicalQuery:            "fetch logs",
				QueryID:                   "q-1",
				DQLVersion:                "1.0",
				Timezone:                  "UTC",
				Locale:                    "en_US",
				ExecutionTimeMilliseconds: 123,
				ScannedRecords:            456,
				ScannedBytes:              789,
				ScannedDataPoints:         10,
				Sampled:                   true,
				Notifications: []Notification{{
					Severity:         "WARNING",
					NotificationType: "SCAN_LIMIT",
					Message:          "scan limit reached",
					MessageFormat:    "plain",
					Arguments:        []string{"arg1", "arg2"},
				}},
				AnalysisTimeframe: &AnalysisTimeframe{Start: "2026-01-01T00:00:00Z", End: "2026-01-01T01:00:00Z"},
				Contributions: &Contributions{Buckets: []BucketContribution{{
					Name:                "bucket-1",
					Table:               "logs",
					ScannedBytes:        321,
					MatchedRecordsRatio: 0.5,
				}}},
			},
		},
		Result: &Result{
			Records: []map[string]interface{}{{"result": "row1"}, {"result": "row2"}},
			Types: []ColumnTypes{{
				IndexRange: []int{0, 1},
				Mappings:   map[string]ColumnType{"col1": {Type: "string"}},
			}},
			Metadata: &Metadata{
				Metrics: []MetricInfo{{MetricKey: "dt.host.mem.usage", Unit: "Byte"}},
				Grail:   &GrailMetadata{Query: "nested", Sampled: false},
			},
		},
	}

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("json.Marshal(fixture): %v", err)
	}

	var want Response
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("json.Unmarshal (ground truth): %v", err)
	}

	var topRecords, resultRecords []map[string]interface{}
	got, err := decodeResponseStream(bytes.NewReader(raw), recordSink{
		onTop:    func(row map[string]interface{}) error { topRecords = append(topRecords, row); return nil },
		onResult: func(row map[string]interface{}) error { resultRecords = append(resultRecords, row); return nil },
	})
	if err != nil {
		t.Fatalf("decodeResponseStream: %v", err)
	}
	// Mirror Execute's own reattachment step exactly.
	if got.Records != nil {
		got.Records = orEmpty(topRecords)
	}
	if got.Result != nil && got.Result.Records != nil {
		got.Result.Records = orEmpty(resultRecords)
	}

	if !reflect.DeepEqual(want, *got) {
		t.Errorf("decodeResponseStream produced a different Response than json.Unmarshal.\nwant: %#v\ngot:  %#v", want, *got)
	}
}
