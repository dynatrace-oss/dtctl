package exec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
