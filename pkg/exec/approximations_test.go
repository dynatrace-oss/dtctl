package exec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// Synthetic advisory text; the shape mirrors what the query API returns (#415).
const (
	approxText = "~ on content: substring match, not token match"
	infoNotice = "an informational advisory"
)

// approximateResponse is a successful result that Grail qualified: one
// approximation and one INFO notification, neither of which truncates it.
func approximateResponse() *DQLQueryResponse {
	return &DQLQueryResponse{
		State: "SUCCEEDED",
		Result: &DQLResult{
			Records: []map[string]interface{}{{"n": "1"}},
			Metadata: &DQLMetadata{Grail: &GrailMetadata{
				ExecutionTimeMilliseconds: 9,
				ScannedRecords:            1,
				Sampled:                   true,
				Approximations:            []string{approxText},
				Notifications: []QueryNotification{{
					Severity:         "INFO",
					NotificationType: "EXAMPLE_NOTICE",
					Message:          infoNotice,
				}},
			}},
		},
	}
}

func TestExtractQueryMetadata_CarriesApproximationsAndNotifications(t *testing.T) {
	meta := extractQueryMetadata(approximateResponse())
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if len(meta.Approximations) != 1 || meta.Approximations[0] != approxText {
		t.Errorf("Approximations = %q, want [%q]", meta.Approximations, approxText)
	}
	if !meta.Sampled {
		t.Error("Sampled = false, want true")
	}
	want := output.MetadataNotice{Severity: "INFO", NotificationType: "EXAMPLE_NOTICE", Message: infoNotice}
	if len(meta.Notifications) != 1 || meta.Notifications[0].Severity != want.Severity ||
		meta.Notifications[0].NotificationType != want.NotificationType || meta.Notifications[0].Message != want.Message {
		t.Errorf("Notifications = %+v, want [%+v] (INFO must not be filtered out of the metadata)", meta.Notifications, want)
	}

	// An exact, notice-free response leaves both lists empty so the encoded
	// metadata block is unchanged.
	plain := extractQueryMetadata(resultWithScanStatsOnly())
	if plain == nil || plain.Approximations != nil || plain.Notifications != nil {
		t.Errorf("exact response: got %+v, want no approximations or notifications", plain)
	}
}

func resultWithScanStatsOnly() *DQLQueryResponse {
	resp, _ := resultWithScanStats()
	return resp
}

// -o json -M=all outside agent mode: the metadata block carries both lists.
func TestPrintResults_JSONMetadataCarriesApproximations(t *testing.T) {
	e := &DQLExecutor{}
	var stdout []byte
	_ = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := e.printResults("fetch logs", approximateResponse(), DQLExecuteOptions{
				OutputFormat:   "json",
				MetadataFields: []string{"all"},
			}); err != nil {
				t.Errorf("printResults: %v", err)
			}
		})
	})
	var parsed struct {
		Metadata struct {
			Approximations []string                `json:"approximations"`
			Notifications  []output.MetadataNotice `json:"notifications"`
			Sampled        bool                    `json:"sampled"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(stdout, &parsed); err != nil {
		t.Fatalf("unmarshal %q: %v", stdout, err)
	}
	if len(parsed.Metadata.Approximations) != 1 || parsed.Metadata.Approximations[0] != approxText {
		t.Errorf("metadata.approximations = %q, want [%q]", parsed.Metadata.Approximations, approxText)
	}
	if len(parsed.Metadata.Notifications) != 1 || parsed.Metadata.Notifications[0].Message != infoNotice {
		t.Errorf("metadata.notifications = %+v, want the INFO notice", parsed.Metadata.Notifications)
	}
	if !parsed.Metadata.Sampled {
		t.Error("metadata.sampled = false, want true")
	}
}

// Human output: the approximation goes to stderr as a warning and stdout is
// exactly what it was before — no metadata was asked for, so none is printed.
func TestPrintResults_ApproximationsOnStderrWithoutEnvelope(t *testing.T) {
	tests := []struct {
		name string
		opts DQLExecuteOptions
	}{
		{"human table", DQLExecuteOptions{OutputFormat: "table"}},
		{"human json", DQLExecuteOptions{OutputFormat: "json"}},
		{"agent csv", DQLExecuteOptions{OutputFormat: "csv", AgentMode: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &DQLExecutor{}
			var stdout, baseline []byte
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					if err := e.printResults("fetch logs", approximateResponse(), tt.opts); err != nil {
						t.Errorf("printResults: %v", err)
					}
				})
			})
			if !strings.Contains(string(stderr), approximationPrefix+approxText) {
				t.Errorf("stderr = %q, want the approximation warning", stderr)
			}
			if strings.Contains(string(stdout), approxText) {
				t.Errorf("stdout = %q, must not carry the approximation without --metadata", stdout)
			}

			// Same rows, no advisories: stdout must be byte-identical.
			exact := approximateResponse()
			exact.Result.Metadata.Grail.Approximations = nil
			exact.Result.Metadata.Grail.Notifications = nil
			_ = captureStderr(t, func() {
				baseline = captureStdout(t, func() {
					_ = e.printResults("fetch logs", exact, tt.opts)
				})
			})
			if string(stdout) != string(baseline) {
				t.Errorf("stdout changed by an approximation:\n got: %q\nwant: %q", stdout, baseline)
			}
		})
	}
}

// Agent envelope: the approximation is a context.warnings entry (not stderr),
// and the default minimal metadata keeps it because it marks the result as
// approximate.
func TestPrintResults_AgentEnvelopeCarriesApproximations(t *testing.T) {
	tests := []struct {
		name string
		opts DQLExecuteOptions
	}{
		{"inline records", DQLExecuteOptions{OutputFormat: "json", AgentMode: true}},
		{"jq", DQLExecuteOptions{OutputFormat: "json", AgentMode: true, JQFilter: ".records"}},
		{"spilled", DQLExecuteOptions{OutputFormat: "json", AgentMode: true,
			Spill: SpillOptions{Mode: SpillAlways, ToPath: "result.jsonl"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.Spill.ToPath != "" {
				tt.opts.Spill.ToPath = t.TempDir() + "/" + tt.opts.Spill.ToPath
			}
			tt.opts.MetadataFields = []string{output.MetadataMinimal}
			tt.opts.MetadataDefaulted = true

			e := &DQLExecutor{}
			var stdout []byte
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() {
					if err := e.printResults("fetch logs, from:now()-1h", approximateResponse(), tt.opts); err != nil {
						t.Errorf("printResults: %v", err)
					}
				})
			})
			if strings.Contains(string(stderr), approxText) {
				t.Errorf("stderr = %q, want nothing about the approximation: the envelope carries it", stderr)
			}
			var resp struct {
				Context  *output.ResponseContext `json:"context"`
				Metadata struct {
					Approximations []string `json:"approximations"`
					Sampled        bool     `json:"sampled"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(stdout, &resp); err != nil {
				t.Fatalf("stdout is not an envelope: %v\n%s", err, stdout)
			}
			if resp.Context == nil || !contains(resp.Context.Warnings, approximationPrefix+approxText) {
				t.Errorf("context.warnings = %+v, want %q", resp.Context, approximationPrefix+approxText)
			}
			if len(resp.Metadata.Approximations) != 1 || resp.Metadata.Approximations[0] != approxText {
				t.Errorf("metadata.approximations = %q, want [%q]", resp.Metadata.Approximations, approxText)
			}
			if !resp.Metadata.Sampled {
				t.Error("metadata.sampled = false, want true under minimal")
			}
		})
	}
}

func TestApproximationWarnings(t *testing.T) {
	if got := approximationWarnings(nil); got != nil {
		t.Errorf("nil response: got %q, want nil", got)
	}
	if got := approximationWarnings(resultWithScanStatsOnly()); got != nil {
		t.Errorf("exact response: got %q, want nil", got)
	}
	got := approximationWarnings(approximateResponse())
	if len(got) != 1 || got[0] != approximationPrefix+approxText {
		t.Errorf("got %q, want [%q]", got, approximationPrefix+approxText)
	}
}
