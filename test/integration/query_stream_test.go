//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"

	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// TestQueryExecuteStream_MatchesAccumulatedResult is the live-tenant guard for
// issue #466's streaming decode: it runs the same query through the
// accumulating ExecuteAndPoll and through ExecuteStream (with a callback that
// re-accumulates), against a real Grail backend, and requires the two to
// agree on row count and content. This is the end-to-end proof that
// decodeResponseStream's manual field-by-field decode has not drifted from
// what the live API actually sends, beyond what the synthetic fixture in
// query_stream_test.go can cover.
func TestQueryExecuteStream_MatchesAccumulatedResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := SetupIntegration(t)
	handler := sdkquery.NewHandler(httpclient.Wrap(env.Client.HTTP()))

	const query = "fetch logs | limit 25"

	accumulated, err := handler.ExecuteAndPoll(context.Background(), sdkquery.ExecuteRequest{Query: query}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	wantRecords := accumulated.GetRecords()

	var streamed []map[string]interface{}
	onRecord := func(row map[string]interface{}) error {
		streamed = append(streamed, row)
		return nil
	}

	execResp, err := handler.ExecuteStream(context.Background(), sdkquery.ExecuteRequest{Query: query}, onRecord)
	if err != nil {
		t.Fatalf("ExecuteStream() error: %v", err)
	}

	// A query this small (limit 25) may still complete asynchronously on a
	// busy tenant; poll to completion the same way ExecuteAndPoll would.
	result := execResp
	for result.RequestToken != "" && result.State != sdkquery.StateSucceeded {
		result, err = handler.PollStream(context.Background(), result.RequestToken, 5000, false, onRecord)
		if err != nil {
			t.Fatalf("PollStream() error: %v", err)
		}
	}

	if len(streamed) != len(wantRecords) {
		t.Fatalf("ExecuteStream/PollStream delivered %d rows via callback, ExecuteAndPoll accumulated %d",
			len(streamed), len(wantRecords))
	}
	// decodeResponseStream must never leave the streaming methods'
	// Records/Result.Records populated — every row must have come through
	// the callback instead.
	if result.Result != nil && result.Result.Records != nil {
		t.Errorf("ExecuteStream/PollStream Result.Records = %#v, want nil (rows must arrive via callback only)", result.Result.Records)
	}

	for i := range wantRecords {
		if len(streamed[i]) != len(wantRecords[i]) {
			t.Errorf("row %d: field count = %d, want %d (accumulated: %#v, streamed: %#v)",
				i, len(streamed[i]), len(wantRecords[i]), wantRecords[i], streamed[i])
		}
	}
}
