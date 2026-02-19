package exec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func TestDQLExecutor_ExecuteQueryWithOptions_CustomHeaders(t *testing.T) {
	tests := []struct {
		name                    string
		maxResultRecords        int64
		maxReturnedRecords      int64
		expectMaxResultHeader   bool
		expectMaxReturnedHeader bool
	}{
		{
			name:                    "no custom headers",
			maxResultRecords:        0,
			maxReturnedRecords:      0,
			expectMaxResultHeader:   false,
			expectMaxReturnedHeader: false,
		},
		{
			name:                    "max-result-records only",
			maxResultRecords:        5000,
			maxReturnedRecords:      0,
			expectMaxResultHeader:   true,
			expectMaxReturnedHeader: false,
		},
		{
			name:                    "max-returned-records only",
			maxResultRecords:        0,
			maxReturnedRecords:      10000,
			expectMaxResultHeader:   false,
			expectMaxReturnedHeader: true,
		},
		{
			name:                    "both custom headers",
			maxResultRecords:        5000,
			maxReturnedRecords:      10000,
			expectMaxResultHeader:   true,
			expectMaxReturnedHeader: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			var receivedRequest DQLQueryRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Decode the request body to check parameters
				_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

				// Return a successful query response
				response := DQLQueryResponse{
					State: "SUCCEEDED",
					Result: &DQLResult{
						Records: []map[string]interface{}{
							{"test": "value"},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create client pointing to test server
			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			executor := NewDQLExecutor(c)

			// Execute query with options
			opts := DQLExecuteOptions{
				MaxResultRecords: tt.maxResultRecords,
				MaxResultBytes:   tt.maxReturnedRecords,
			}

			_, err = executor.ExecuteQueryWithOptions("fetch logs", opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify request body parameters
			if tt.expectMaxResultHeader {
				if receivedRequest.MaxResultRecords != 5000 {
					t.Errorf("expected MaxResultRecords to be 5000, got %d", receivedRequest.MaxResultRecords)
				}
			} else {
				if receivedRequest.MaxResultRecords != 0 {
					t.Errorf("expected MaxResultRecords to be 0, got %d", receivedRequest.MaxResultRecords)
				}
			}

			if tt.expectMaxReturnedHeader {
				if receivedRequest.MaxResultBytes != 10000 {
					t.Errorf("expected MaxResultBytes to be 10000, got %d", receivedRequest.MaxResultBytes)
				}
			} else {
				if receivedRequest.MaxResultBytes != 0 {
					t.Errorf("expected MaxResultBytes to be 0, got %d", receivedRequest.MaxResultBytes)
				}
			}
		})
	}
}

func TestDQLExecutor_ExecuteQuery_BackwardCompatibility(t *testing.T) {
	// Test that the ExecuteQuery method still works for backward compatibility
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DQLQueryResponse{
			State: "SUCCEEDED",
			Result: &DQLResult{
				Records: []map[string]interface{}{
					{"test": "value"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	result, err := executor.ExecuteQuery("fetch logs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.State != "SUCCEEDED" {
		t.Errorf("expected state SUCCEEDED, got %s", result.State)
	}

	if len(result.Result.Records) != 1 {
		t.Errorf("expected 1 record, got %d", len(result.Result.Records))
	}
}

func TestDQLExecutor_ExecuteQueryWithOptions_PollingWithBodyParams(t *testing.T) {
	// Test that body parameters are sent in the initial request and polling works
	callCount := 0
	var receivedRequest DQLQueryRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if r.URL.Path == "/platform/storage/query/v1/query:execute" {
			// Capture the initial request body
			_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

			// First call - return RUNNING state
			response := DQLQueryResponse{
				State:        "RUNNING",
				RequestToken: "test-token-123",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/platform/storage/query/v1/query:poll" {
			// Poll call - just return success
			response := DQLQueryResponse{
				State: "SUCCEEDED",
				Result: &DQLResult{
					Records: []map[string]interface{}{
						{"test": "value"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	opts := DQLExecuteOptions{
		MaxResultRecords: 5000,
		MaxResultBytes:   10000,
	}

	_, err = executor.ExecuteQueryWithOptions("fetch logs", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify we made both calls (execute and poll)
	if callCount != 2 {
		t.Errorf("expected 2 API calls (execute + poll), got %d", callCount)
	}

	// Verify body parameters were sent in the initial request
	if receivedRequest.MaxResultRecords != 5000 {
		t.Errorf("expected MaxResultRecords to be 5000 in initial request, got %d",
			receivedRequest.MaxResultRecords)
	}

	if receivedRequest.MaxResultBytes != 10000 {
		t.Errorf("expected MaxResultBytes to be 10000 in initial request, got %d",
			receivedRequest.MaxResultBytes)
	}
}

func TestDQLExecutor_ExecuteQueryWithOptions_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "bad request",
			statusCode:     http.StatusBadRequest,
			responseBody:   "Invalid query",
			expectedErrMsg: "query failed with status 400",
		},
		{
			name:           "unauthorized",
			statusCode:     http.StatusUnauthorized,
			responseBody:   "Unauthorized",
			expectedErrMsg: "query failed with status 401",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "Internal error",
			expectedErrMsg: "query failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			executor := NewDQLExecutor(c)

			_, err = executor.ExecuteQueryWithOptions("fetch logs", DQLExecuteOptions{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.Error()[:len(tt.expectedErrMsg)] != tt.expectedErrMsg {
				t.Errorf("expected error to start with '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

func TestDQLQueryResponse_GetNotifications(t *testing.T) {
	tests := []struct {
		name             string
		response         DQLQueryResponse
		expectedCount    int
		expectedSeverity string
		expectedMessage  string
	}{
		{
			name: "notifications in top-level metadata",
			response: DQLQueryResponse{
				State: "SUCCEEDED",
				Metadata: &DQLMetadata{
					Grail: &GrailMetadata{
						Notifications: []QueryNotification{
							{
								Severity: "WARNING",
								Message:  "Scan limit reached",
							},
						},
					},
				},
			},
			expectedCount:    1,
			expectedSeverity: "WARNING",
			expectedMessage:  "Scan limit reached",
		},
		{
			name: "notifications in result metadata",
			response: DQLQueryResponse{
				State: "SUCCEEDED",
				Result: &DQLResult{
					Records: []map[string]interface{}{{"test": "value"}},
					Metadata: &DQLMetadata{
						Grail: &GrailMetadata{
							Notifications: []QueryNotification{
								{
									Severity: "WARNING",
									Message:  "Result truncated",
								},
							},
						},
					},
				},
			},
			expectedCount:    1,
			expectedSeverity: "WARNING",
			expectedMessage:  "Result truncated",
		},
		{
			name: "no notifications",
			response: DQLQueryResponse{
				State: "SUCCEEDED",
				Result: &DQLResult{
					Records: []map[string]interface{}{{"test": "value"}},
				},
			},
			expectedCount: 0,
		},
		{
			name: "empty metadata",
			response: DQLQueryResponse{
				State:    "SUCCEEDED",
				Metadata: &DQLMetadata{},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifications := tt.response.GetNotifications()
			if len(notifications) != tt.expectedCount {
				t.Errorf("expected %d notifications, got %d", tt.expectedCount, len(notifications))
			}
			if tt.expectedCount > 0 {
				if notifications[0].Severity != tt.expectedSeverity {
					t.Errorf("expected severity %s, got %s", tt.expectedSeverity, notifications[0].Severity)
				}
				if notifications[0].Message != tt.expectedMessage {
					t.Errorf("expected message %s, got %s", tt.expectedMessage, notifications[0].Message)
				}
			}
		})
	}
}

func TestDQLQueryResponse_ParseNotificationsFromJSON(t *testing.T) {
	// Test parsing the actual JSON format from the API response
	jsonResponse := `{
		"state": "SUCCEEDED",
		"progress": 100,
		"result": {
			"records": [{"count()": "194414758"}],
			"types": [{"indexRange": [0, 0], "mappings": {"count()": {"type": "long"}}}]
		},
		"metadata": {
			"grail": {
				"canonicalQuery": "fetch spans, from:-10d\n| summarize count()",
				"timezone": "Z",
				"query": "fetch spans, from: -10d | summarize count()",
				"scannedRecords": 268132936,
				"dqlVersion": "V1_0",
				"scannedBytes": 500000000000,
				"scannedDataPoints": 0,
				"executionTimeMilliseconds": 1676,
				"notifications": [
					{
						"severity": "WARNING",
						"messageFormat": "Your execution was stopped after %1$s gigabytes of data were scanned.",
						"arguments": ["500"],
						"notificationType": "SCAN_LIMIT_GBYTES",
						"message": "Your execution was stopped after 500 gigabytes of data were scanned."
					}
				],
				"queryId": "fe5b87f0-dfe8-457d-899b-50eabf9ab55d",
				"sampled": false
			}
		}
	}`

	var response DQLQueryResponse
	if err := json.Unmarshal([]byte(jsonResponse), &response); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	notifications := response.GetNotifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}

	n := notifications[0]
	if n.Severity != "WARNING" {
		t.Errorf("expected severity WARNING, got %s", n.Severity)
	}
	if n.NotificationType != "SCAN_LIMIT_GBYTES" {
		t.Errorf("expected notificationType SCAN_LIMIT_GBYTES, got %s", n.NotificationType)
	}
	if n.Message == "" {
		t.Error("expected non-empty message")
	}
	if len(n.Arguments) != 1 || n.Arguments[0] != "500" {
		t.Errorf("expected arguments [500], got %v", n.Arguments)
	}

	// Also verify metadata fields were parsed
	if response.Metadata.Grail.ScannedBytes != 500000000000 {
		t.Errorf("expected scannedBytes 500000000000, got %d", response.Metadata.Grail.ScannedBytes)
	}
	if response.Metadata.Grail.ExecutionTimeMilliseconds != 1676 {
		t.Errorf("expected executionTimeMilliseconds 1676, got %d", response.Metadata.Grail.ExecutionTimeMilliseconds)
	}
}

func TestGetHintForNotification(t *testing.T) {
	tests := []struct {
		name             string
		notificationType string
		message          string
		wantHint         bool
		wantContains     string
	}{
		{
			name:             "scan limit by type",
			notificationType: "SCAN_LIMIT_GBYTES",
			message:          "",
			wantHint:         true,
			wantContains:     "--default-scan-limit-gbytes",
		},
		{
			name:             "result limit records by type",
			notificationType: "RESULT_LIMIT_RECORDS",
			message:          "",
			wantHint:         true,
			wantContains:     "--max-result-records",
		},
		{
			name:             "result limit bytes by type",
			notificationType: "RESULT_LIMIT_BYTES",
			message:          "",
			wantHint:         true,
			wantContains:     "--max-result-bytes",
		},
		{
			name:             "fetch timeout by type",
			notificationType: "FETCH_TIMEOUT",
			message:          "",
			wantHint:         true,
			wantContains:     "--fetch-timeout-seconds",
		},
		{
			name:             "sampling applied by type",
			notificationType: "SAMPLING_APPLIED",
			message:          "",
			wantHint:         true,
			wantContains:     "--default-sampling-ratio",
		},
		{
			name:             "consumption limit by type",
			notificationType: "QUERY_CONSUMPTION_LIMIT",
			message:          "",
			wantHint:         true,
			wantContains:     "--enforce-query-consumption-limit",
		},
		{
			name:             "result limited by message pattern",
			notificationType: "",
			message:          "Result has been limited to 1000 records",
			wantHint:         true,
			wantContains:     "--max-result-records",
		},
		{
			name:             "scan gigabyte by message pattern",
			notificationType: "",
			message:          "Scan stopped after 500 gigabytes were processed",
			wantHint:         true,
			wantContains:     "--default-scan-limit-gbytes",
		},
		{
			name:             "unknown notification type",
			notificationType: "UNKNOWN_TYPE",
			message:          "",
			wantHint:         false,
		},
		{
			name:             "empty notification",
			notificationType: "",
			message:          "",
			wantHint:         false,
		},
		{
			name:             "unrelated message",
			notificationType: "",
			message:          "Query completed successfully",
			wantHint:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := getHintForNotification(tt.notificationType, tt.message)

			if tt.wantHint {
				if hint == "" {
					t.Error("expected a hint, got empty string")
				}
				if tt.wantContains != "" && !strings.Contains(hint, tt.wantContains) {
					t.Errorf("hint %q should contain %q", hint, tt.wantContains)
				}
			} else {
				if hint != "" {
					t.Errorf("expected no hint, got %q", hint)
				}
			}
		})
	}
}

func TestNewDQLExecutor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)
	if executor == nil {
		t.Fatal("NewDQLExecutor returned nil")
	}
	if executor.client != c {
		t.Error("executor client not set correctly")
	}
}

func TestDQLExecutor_ExecuteQueryWithOptions_RunningNoToken(t *testing.T) {
	// Test error when query is running but no request token is returned
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DQLQueryResponse{
			State:        "RUNNING",
			RequestToken: "", // Missing token
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)
	_, err = executor.ExecuteQueryWithOptions("fetch logs", DQLExecuteOptions{})

	if err == nil {
		t.Fatal("expected error for missing request token")
	}
	if !strings.Contains(err.Error(), "no request token") {
		t.Errorf("expected 'no request token' error, got: %v", err)
	}
}

func TestDQLExecutor_ExecuteQueryWithOptions_PollFailed(t *testing.T) {
	// Test handling of FAILED state during polling
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if r.URL.Path == "/platform/storage/query/v1/query:execute" {
			response := DQLQueryResponse{
				State:        "RUNNING",
				RequestToken: "test-token-123",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/platform/storage/query/v1/query:poll" {
			response := DQLQueryResponse{
				State: "FAILED",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)
	_, err = executor.ExecuteQueryWithOptions("fetch logs", DQLExecuteOptions{})

	if err == nil {
		t.Fatal("expected error for failed query")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected 'failed' in error, got: %v", err)
	}
}

func TestDQLExecutor_ExecuteQueryWithOptions_AllParameters(t *testing.T) {
	// Test that all DQL API parameters are correctly sent in the request body
	var receivedRequest DQLQueryRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the request body
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		response := DQLQueryResponse{
			State: "SUCCEEDED",
			Result: &DQLResult{
				Records: []map[string]interface{}{
					{"test": "value"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	// Test with all parameters set
	opts := DQLExecuteOptions{
		MaxResultRecords:             5000,
		MaxResultBytes:               10000000,
		DefaultScanLimitGbytes:       5.0,
		DefaultSamplingRatio:         10.0,
		FetchTimeoutSeconds:          120,
		EnablePreview:                true,
		EnforceQueryConsumptionLimit: true,
		IncludeTypes:                 true,
		DefaultTimeframeStart:        "2022-04-20T12:10:04.123Z",
		DefaultTimeframeEnd:          "2022-04-20T13:10:04.123Z",
		Locale:                       "en_US",
		Timezone:                     "Europe/Paris",
	}

	_, err = executor.ExecuteQueryWithOptions("fetch logs", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all parameters were sent correctly
	if receivedRequest.MaxResultRecords != 5000 {
		t.Errorf("expected MaxResultRecords to be 5000, got %d", receivedRequest.MaxResultRecords)
	}
	if receivedRequest.MaxResultBytes != 10000000 {
		t.Errorf("expected MaxResultBytes to be 10000000, got %d", receivedRequest.MaxResultBytes)
	}
	if receivedRequest.DefaultScanLimitGbytes != 5.0 {
		t.Errorf("expected DefaultScanLimitGbytes to be 5.0, got %f", receivedRequest.DefaultScanLimitGbytes)
	}
	if receivedRequest.DefaultSamplingRatio != 10.0 {
		t.Errorf("expected DefaultSamplingRatio to be 10.0, got %f", receivedRequest.DefaultSamplingRatio)
	}
	if receivedRequest.FetchTimeoutSeconds != 120 {
		t.Errorf("expected FetchTimeoutSeconds to be 120, got %d", receivedRequest.FetchTimeoutSeconds)
	}
	if receivedRequest.EnablePreview != true {
		t.Errorf("expected EnablePreview to be true, got %v", receivedRequest.EnablePreview)
	}
	if receivedRequest.EnforceQueryConsumptionLimit != true {
		t.Errorf("expected EnforceQueryConsumptionLimit to be true, got %v", receivedRequest.EnforceQueryConsumptionLimit)
	}
	if receivedRequest.IncludeTypes == nil || *receivedRequest.IncludeTypes != true {
		t.Errorf("expected IncludeTypes to be true, got %v", receivedRequest.IncludeTypes)
	}
	if receivedRequest.DefaultTimeframeStart != "2022-04-20T12:10:04.123Z" {
		t.Errorf("expected DefaultTimeframeStart to be '2022-04-20T12:10:04.123Z', got %s", receivedRequest.DefaultTimeframeStart)
	}
	if receivedRequest.DefaultTimeframeEnd != "2022-04-20T13:10:04.123Z" {
		t.Errorf("expected DefaultTimeframeEnd to be '2022-04-20T13:10:04.123Z', got %s", receivedRequest.DefaultTimeframeEnd)
	}
	if receivedRequest.Locale != "en_US" {
		t.Errorf("expected Locale to be 'en_US', got %s", receivedRequest.Locale)
	}
	if receivedRequest.Timezone != "Europe/Paris" {
		t.Errorf("expected Timezone to be 'Europe/Paris', got %s", receivedRequest.Timezone)
	}
}

// TestVerifyQuery_Valid tests verification of a valid query
func TestVerifyQuery_Valid(t *testing.T) {
	var receivedRequest DQLVerifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify correct endpoint
		if r.URL.Path != "/platform/storage/query/v1/query:verify" {
			t.Errorf("expected path /platform/storage/query/v1/query:verify, got %s", r.URL.Path)
		}

		// Capture request body
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		// Return valid response
		response := DQLVerifyResponse{
			Valid: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	result, err := executor.VerifyQuery("fetch logs", DQLVerifyOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected valid query")
	}

	if receivedRequest.Query != "fetch logs" {
		t.Errorf("expected query 'fetch logs', got %s", receivedRequest.Query)
	}
}

// TestVerifyQuery_Invalid tests verification of an invalid query
func TestVerifyQuery_Invalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid response with syntax error notification
		response := DQLVerifyResponse{
			Valid: false,
			Notifications: []MetadataNotification{
				{
					Severity:         "ERROR",
					NotificationType: "SYNTAX_ERROR",
					Message:          "Expected command, got 'invalid'",
					SyntaxPosition: &SyntaxPosition{
						Start: &Position{Line: 1, Column: 1},
						End:   &Position{Line: 1, Column: 8},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	result, err := executor.VerifyQuery("invalid query", DQLVerifyOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid query")
	}

	if len(result.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result.Notifications))
	}

	n := result.Notifications[0]
	if n.Severity != "ERROR" {
		t.Errorf("expected severity ERROR, got %s", n.Severity)
	}
	if n.NotificationType != "SYNTAX_ERROR" {
		t.Errorf("expected notificationType SYNTAX_ERROR, got %s", n.NotificationType)
	}
	if n.Message == "" {
		t.Error("expected non-empty error message")
	}
	if n.SyntaxPosition == nil {
		t.Error("expected syntax position for syntax error")
	} else {
		if n.SyntaxPosition.Start == nil || n.SyntaxPosition.Start.Line != 1 {
			t.Error("expected syntax position with line 1")
		}
	}
}

// TestVerifyQuery_WithCanonical tests verification with canonical query generation
func TestVerifyQuery_WithCanonical(t *testing.T) {
	var receivedRequest DQLVerifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		response := DQLVerifyResponse{
			Valid:          true,
			CanonicalQuery: "fetch logs\n| limit 100",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	opts := DQLVerifyOptions{
		GenerateCanonicalQuery: true,
	}

	result, err := executor.VerifyQuery("fetch logs | limit 100", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected valid query")
	}

	if result.CanonicalQuery == "" {
		t.Error("expected canonical query to be returned")
	}

	if receivedRequest.GenerateCanonicalQuery != true {
		t.Error("expected GenerateCanonicalQuery to be true in request")
	}

	expectedCanonical := "fetch logs\n| limit 100"
	if result.CanonicalQuery != expectedCanonical {
		t.Errorf("expected canonical query %q, got %q", expectedCanonical, result.CanonicalQuery)
	}
}

// TestVerifyQuery_WithTimezone tests verification with timezone option
func TestVerifyQuery_WithTimezone(t *testing.T) {
	var receivedRequest DQLVerifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		response := DQLVerifyResponse{
			Valid: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	opts := DQLVerifyOptions{
		Timezone: "Europe/Paris",
	}

	result, err := executor.VerifyQuery("fetch logs", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected valid query")
	}

	if receivedRequest.Timezone != "Europe/Paris" {
		t.Errorf("expected timezone 'Europe/Paris', got %s", receivedRequest.Timezone)
	}
}

// TestVerifyQuery_WithLocale tests verification with locale option
func TestVerifyQuery_WithLocale(t *testing.T) {
	var receivedRequest DQLVerifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		response := DQLVerifyResponse{
			Valid: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	opts := DQLVerifyOptions{
		Locale: "en_US",
	}

	result, err := executor.VerifyQuery("fetch logs", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected valid query")
	}

	if receivedRequest.Locale != "en_US" {
		t.Errorf("expected locale 'en_US', got %s", receivedRequest.Locale)
	}
}

// TestVerifyQuery_WithAllOptions tests verification with all options
func TestVerifyQuery_WithAllOptions(t *testing.T) {
	var receivedRequest DQLVerifyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		response := DQLVerifyResponse{
			Valid:          true,
			CanonicalQuery: "fetch logs\n| limit 100",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	opts := DQLVerifyOptions{
		GenerateCanonicalQuery: true,
		Timezone:               "America/New_York",
		Locale:                 "de_DE",
	}

	result, err := executor.VerifyQuery("fetch logs | limit 100", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("expected valid query")
	}

	// Verify all request parameters
	if receivedRequest.Query != "fetch logs | limit 100" {
		t.Errorf("expected query 'fetch logs | limit 100', got %s", receivedRequest.Query)
	}
	if receivedRequest.GenerateCanonicalQuery != true {
		t.Error("expected GenerateCanonicalQuery to be true")
	}
	if receivedRequest.Timezone != "America/New_York" {
		t.Errorf("expected timezone 'America/New_York', got %s", receivedRequest.Timezone)
	}
	if receivedRequest.Locale != "de_DE" {
		t.Errorf("expected locale 'de_DE', got %s", receivedRequest.Locale)
	}

	// Verify canonical query in response
	if result.CanonicalQuery == "" {
		t.Error("expected canonical query to be returned")
	}
}

// TestVerifyQuery_NetworkError tests handling of network errors
func TestVerifyQuery_NetworkError(t *testing.T) {
	// Create client pointing to non-existent server
	c, err := client.New("http://localhost:0", "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	_, err = executor.VerifyQuery("fetch logs", DQLVerifyOptions{})
	if err == nil {
		t.Fatal("expected network error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to verify query") {
		t.Errorf("expected 'failed to verify query' error, got: %v", err)
	}
}

// TestVerifyQuery_AuthError tests handling of authentication errors
func TestVerifyQuery_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 401 Unauthorized
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": {"code": 401, "message": "Unauthorized"}}`))
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	executor := NewDQLExecutor(c)

	_, err = executor.VerifyQuery("fetch logs", DQLVerifyOptions{})
	if err == nil {
		t.Fatal("expected authentication error, got nil")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected '401' in error, got: %v", err)
	}
}

// TestVerifyQuery_ServerError tests handling of server errors
func TestVerifyQuery_ServerError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedErrMsg string
	}{
		{
			name:           "bad request",
			statusCode:     http.StatusBadRequest,
			expectedErrMsg: "query verification failed with status 400",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			expectedErrMsg: "query verification failed with status 500",
		},
		{
			name:           "service unavailable",
			statusCode:     http.StatusServiceUnavailable,
			expectedErrMsg: "query verification failed with status 503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte("Error"))
			}))
			defer server.Close()

			c, err := client.New(server.URL, "test-token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			executor := NewDQLExecutor(c)

			_, err = executor.VerifyQuery("fetch logs", DQLVerifyOptions{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestVerifyTypes_JSONMarshaling tests JSON marshaling/unmarshaling of verify types
func TestVerifyTypes_JSONMarshaling(t *testing.T) {
	t.Run("DQLVerifyRequest", func(t *testing.T) {
		req := DQLVerifyRequest{
			Query:                  "fetch logs",
			GenerateCanonicalQuery: true,
			Timezone:               "UTC",
			Locale:                 "en_US",
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded DQLVerifyRequest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Query != req.Query {
			t.Errorf("expected query %s, got %s", req.Query, decoded.Query)
		}
		if decoded.GenerateCanonicalQuery != req.GenerateCanonicalQuery {
			t.Errorf("expected GenerateCanonicalQuery %v, got %v", req.GenerateCanonicalQuery, decoded.GenerateCanonicalQuery)
		}
		if decoded.Timezone != req.Timezone {
			t.Errorf("expected timezone %s, got %s", req.Timezone, decoded.Timezone)
		}
		if decoded.Locale != req.Locale {
			t.Errorf("expected locale %s, got %s", req.Locale, decoded.Locale)
		}
	})

	t.Run("DQLVerifyResponse", func(t *testing.T) {
		resp := DQLVerifyResponse{
			Valid:          false,
			CanonicalQuery: "fetch logs\n| limit 100",
			Notifications: []MetadataNotification{
				{
					Severity:         "ERROR",
					NotificationType: "SYNTAX_ERROR",
					Message:          "Test error",
					SyntaxPosition: &SyntaxPosition{
						Start: &Position{Line: 1, Column: 5},
						End:   &Position{Line: 1, Column: 10},
					},
				},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded DQLVerifyResponse
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Valid != resp.Valid {
			t.Errorf("expected valid %v, got %v", resp.Valid, decoded.Valid)
		}
		if decoded.CanonicalQuery != resp.CanonicalQuery {
			t.Errorf("expected canonical query %s, got %s", resp.CanonicalQuery, decoded.CanonicalQuery)
		}
		if len(decoded.Notifications) != len(resp.Notifications) {
			t.Fatalf("expected %d notifications, got %d", len(resp.Notifications), len(decoded.Notifications))
		}

		n := decoded.Notifications[0]
		if n.Severity != "ERROR" {
			t.Errorf("expected severity ERROR, got %s", n.Severity)
		}
		if n.NotificationType != "SYNTAX_ERROR" {
			t.Errorf("expected notificationType SYNTAX_ERROR, got %s", n.NotificationType)
		}
		if n.SyntaxPosition == nil {
			t.Fatal("expected syntax position")
		}
		if n.SyntaxPosition.Start.Line != 1 || n.SyntaxPosition.Start.Column != 5 {
			t.Errorf("expected start position (1,5), got (%d,%d)", n.SyntaxPosition.Start.Line, n.SyntaxPosition.Start.Column)
		}
	})

	t.Run("MetadataNotification", func(t *testing.T) {
		notif := MetadataNotification{
			Severity:         "WARNING",
			NotificationType: "DEPRECATION",
			Message:          "This feature is deprecated",
		}

		data, err := json.Marshal(notif)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded MetadataNotification
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Severity != notif.Severity {
			t.Errorf("expected severity %s, got %s", notif.Severity, decoded.Severity)
		}
		if decoded.NotificationType != notif.NotificationType {
			t.Errorf("expected notificationType %s, got %s", notif.NotificationType, decoded.NotificationType)
		}
		if decoded.Message != notif.Message {
			t.Errorf("expected message %s, got %s", notif.Message, decoded.Message)
		}
	})

	t.Run("SyntaxPosition", func(t *testing.T) {
		pos := SyntaxPosition{
			Start: &Position{Line: 2, Column: 10},
			End:   &Position{Line: 2, Column: 20},
		}

		data, err := json.Marshal(pos)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		var decoded SyntaxPosition
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Start == nil || decoded.End == nil {
			t.Fatal("expected start and end positions")
		}
		if decoded.Start.Line != 2 || decoded.Start.Column != 10 {
			t.Errorf("expected start (2,10), got (%d,%d)", decoded.Start.Line, decoded.Start.Column)
		}
		if decoded.End.Line != 2 || decoded.End.Column != 20 {
			t.Errorf("expected end (2,20), got (%d,%d)", decoded.End.Line, decoded.End.Column)
		}
	})
}

// TestVerifyQuery_ParseActualAPIResponse tests parsing real API response format
func TestVerifyQuery_ParseActualAPIResponse(t *testing.T) {
	// Test with actual JSON format from the verify API
	jsonResponse := `{
		"valid": false,
		"notifications": [
			{
				"severity": "ERROR",
				"notificationType": "SYNTAX_ERROR",
				"message": "Expected command, got 'invalid'",
				"syntaxPosition": {
					"start": {"line": 1, "column": 1},
					"end": {"line": 1, "column": 8}
				}
			},
			{
				"severity": "WARNING",
				"notificationType": "DEPRECATION_WARNING",
				"message": "This syntax is deprecated"
			}
		],
		"canonicalQuery": "fetch logs"
	}`

	var response DQLVerifyResponse
	if err := json.Unmarshal([]byte(jsonResponse), &response); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if response.Valid {
		t.Error("expected valid to be false")
	}

	if len(response.Notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(response.Notifications))
	}

	// Check first notification (ERROR with syntax position)
	n1 := response.Notifications[0]
	if n1.Severity != "ERROR" {
		t.Errorf("expected severity ERROR, got %s", n1.Severity)
	}
	if n1.NotificationType != "SYNTAX_ERROR" {
		t.Errorf("expected notificationType SYNTAX_ERROR, got %s", n1.NotificationType)
	}
	if n1.SyntaxPosition == nil {
		t.Fatal("expected syntax position for first notification")
	}
	if n1.SyntaxPosition.Start.Line != 1 || n1.SyntaxPosition.Start.Column != 1 {
		t.Errorf("expected start position (1,1), got (%d,%d)", n1.SyntaxPosition.Start.Line, n1.SyntaxPosition.Start.Column)
	}

	// Check second notification (WARNING without syntax position)
	n2 := response.Notifications[1]
	if n2.Severity != "WARNING" {
		t.Errorf("expected severity WARNING, got %s", n2.Severity)
	}
	if n2.NotificationType != "DEPRECATION_WARNING" {
		t.Errorf("expected notificationType DEPRECATION_WARNING, got %s", n2.NotificationType)
	}
	if n2.SyntaxPosition != nil {
		t.Error("expected no syntax position for second notification")
	}

	if response.CanonicalQuery != "fetch logs" {
		t.Errorf("expected canonical query 'fetch logs', got %s", response.CanonicalQuery)
	}
}
