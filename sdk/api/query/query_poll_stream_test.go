package query

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
)

// A preview is a whole snapshot, not a delta, so a still-running poll's rows
// must never reach the caller — the final response repeats them.
func TestExecuteAndPollStream_DropsPreviewRows(t *testing.T) {
	var polls int32
	mux := http.NewServeMux()
	mux.HandleFunc(basePath+":execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"state":"RUNNING","requestToken":"tok","progress":10}`)
	})
	mux.HandleFunc(basePath+":poll", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&polls, 1) == 1 {
			// A preview snapshot of the first two rows.
			fmt.Fprint(w, `{"state":"RUNNING","progress":60,"result":{"records":[{"n":1},{"n":2}]}}`)
			return
		}
		fmt.Fprint(w, `{"state":"SUCCEEDED","result":{"records":[{"n":1},{"n":2},{"n":3}]}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	var got []map[string]interface{}
	resp, err := h.ExecuteAndPollStream(context.Background(), ExecuteRequest{Query: "fetch logs", EnablePreview: true},
		ExecuteAndPollOptions{}, func(row map[string]interface{}) error {
			got = append(got, row)
			return nil
		})
	if err != nil {
		t.Fatalf("ExecuteAndPollStream: %v", err)
	}
	if resp.State != StateSucceeded {
		t.Errorf("State = %q, want SUCCEEDED", resp.State)
	}

	want := []map[string]interface{}{{"n": float64(1)}, {"n": float64(2)}, {"n": float64(3)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("streamed rows = %#v, want %#v (preview rows must not be delivered)", got, want)
	}
}

// Not forwarding a preview's rows must not hide it from OnUpdate: a caller
// that set EnablePreview gets the same snapshot ExecuteAndPollWithOptions
// reports, while the row callback still sees only the final rows.
func TestExecuteAndPollStream_ReportsPreviewToOnUpdate(t *testing.T) {
	var polls int32
	mux := http.NewServeMux()
	mux.HandleFunc(basePath+":execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"state":"RUNNING","requestToken":"tok","progress":10}`)
	})
	mux.HandleFunc(basePath+":poll", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&polls, 1) == 1 {
			// Rows ahead of the state, so the gate has to hold them first.
			fmt.Fprint(w, `{"result":{"records":[{"n":1},{"n":2}]},"progress":60,"state":"RUNNING"}`)
			return
		}
		fmt.Fprint(w, `{"state":"SUCCEEDED","result":{"records":[{"n":1},{"n":2},{"n":3}]}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	var previews [][]map[string]interface{}
	var got []map[string]interface{}
	_, err := h.ExecuteAndPollStream(context.Background(), ExecuteRequest{Query: "fetch logs", EnablePreview: true},
		ExecuteAndPollOptions{OnUpdate: func(u PollUpdate) {
			if u.Preview != nil {
				previews = append(previews, u.Preview.Records)
			}
		}}, func(row map[string]interface{}) error {
			got = append(got, row)
			return nil
		})
	if err != nil {
		t.Fatalf("ExecuteAndPollStream: %v", err)
	}

	wantPreview := [][]map[string]interface{}{{{"n": float64(1)}, {"n": float64(2)}}}
	if !reflect.DeepEqual(previews, wantPreview) {
		t.Errorf("previews = %#v, want %#v", previews, wantPreview)
	}
	if len(got) != 3 {
		t.Errorf("streamed %d rows, want the 3 final ones", len(got))
	}
}

// The streamed response must not carry the rows as well, or the caller holds
// the very result the streaming was meant to avoid.
func TestExecuteAndPollStream_ResponseCarriesNoRows(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(basePath+":execute", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"state":"SUCCEEDED","result":{"records":[{"a":1},{"a":2}]},"records":[{"a":1}]}`)
	})

	h := NewHandler(newTestClient(t, mux))
	count := 0
	resp, err := h.ExecuteAndPollStream(context.Background(), ExecuteRequest{Query: "fetch logs"},
		ExecuteAndPollOptions{}, func(map[string]interface{}) error { count++; return nil })
	if err != nil {
		t.Fatalf("ExecuteAndPollStream: %v", err)
	}
	if count != 3 {
		t.Errorf("streamed %d rows, want 3 (both the top-level and the nested array)", count)
	}
	if len(resp.Records) != 0 {
		t.Errorf("Response.Records = %#v, want empty", resp.Records)
	}
	if resp.Result != nil && len(resp.Result.Records) != 0 {
		t.Errorf("Response.Result.Records = %#v, want empty", resp.Result.Records)
	}
}

// A response that puts its rows before its state cannot be classified as the
// rows arrive; the gate must hold them and settle correctly once state shows up.
func TestExecuteAndPollStream_RowsBeforeState(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "succeeded",
			body: `{"result":{"records":[{"a":1},{"a":2}]},"state":"SUCCEEDED"}`,
			want: 2,
		},
		{
			name: "still running",
			body: `{"result":{"records":[{"a":1},{"a":2}]},"state":"RUNNING","requestToken":"tok"}`,
			want: 0,
		},
		{
			name: "no state at all (pre-state synchronous shape)",
			body: `{"records":[{"a":1},{"a":2}]}`,
			want: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(basePath+":execute", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			})
			// A RUNNING execute is polled; answer once and finish with no rows so
			// the count below only reflects what the execute response delivered.
			mux.HandleFunc(basePath+":poll", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"state":"SUCCEEDED","result":{"records":[]}}`)
			})

			h := NewHandler(newTestClient(t, mux))
			count := 0
			if _, err := h.ExecuteAndPollStream(context.Background(), ExecuteRequest{Query: "fetch logs"},
				ExecuteAndPollOptions{}, func(map[string]interface{}) error { count++; return nil }); err != nil {
				t.Fatalf("ExecuteAndPollStream: %v", err)
			}
			if count != tc.want {
				t.Errorf("streamed %d rows, want %d", count, tc.want)
			}
		})
	}
}

// ExecuteAndPollStream must agree with ExecuteAndPoll on everything but where
// the rows end up.
func TestExecuteAndPollStream_MatchesExecuteAndPoll(t *testing.T) {
	const body = `{"state":"SUCCEEDED","result":{"records":[{"a":1},{"b":"x"}],"types":[{"indexRange":[0,1],"mappings":{"a":{"type":"long"}}}]},"metadata":{"grail":{"scannedBytes":42,"queryId":"q-1"}}}`
	mux := http.NewServeMux()
	mux.HandleFunc(basePath+":execute", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	})
	h := NewHandler(newTestClient(t, mux))

	buffered, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll: %v", err)
	}
	var streamedRows []map[string]interface{}
	streamed, err := h.ExecuteAndPollStream(context.Background(), ExecuteRequest{Query: "fetch logs"},
		ExecuteAndPollOptions{}, func(row map[string]interface{}) error {
			streamedRows = append(streamedRows, row)
			return nil
		})
	if err != nil {
		t.Fatalf("ExecuteAndPollStream: %v", err)
	}

	if !reflect.DeepEqual(streamedRows, buffered.GetRecords()) {
		t.Errorf("rows:\n streamed %#v\n buffered %#v", streamedRows, buffered.GetRecords())
	}
	if !reflect.DeepEqual(streamed.GetTypes(), buffered.GetTypes()) {
		t.Errorf("types: streamed %#v, buffered %#v", streamed.GetTypes(), buffered.GetTypes())
	}
	if !reflect.DeepEqual(streamed.GetMetadata(), buffered.GetMetadata()) {
		t.Errorf("metadata: streamed %#v, buffered %#v", streamed.GetMetadata(), buffered.GetMetadata())
	}
}
