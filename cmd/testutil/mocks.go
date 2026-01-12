package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockServer represents a test HTTP server with request tracking
type MockServer struct {
	*httptest.Server
	RequestCount int
	LastRequest  *http.Request
}

// NewMockServer creates a new mock server with custom handlers
func NewMockServer(t *testing.T, handlers map[string]http.HandlerFunc) *MockServer {
	t.Helper()

	ms := &MockServer{}

	mux := http.NewServeMux()
	for path, handler := range handlers {
		p := path // capture loop variable
		h := handler
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			ms.RequestCount++
			ms.LastRequest = r
			h(w, r)
		})
	}

	ms.Server = httptest.NewServer(mux)
	return ms
}

// WorkflowListResponse creates a standard workflow list JSON response
func WorkflowListResponse() []byte {
	response := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"id":    "wf-1",
				"title": "Test Workflow 1",
				"owner": "test-user",
			},
			{
				"id":    "wf-2",
				"title": "Test Workflow 2",
				"owner": "test-user",
			},
		},
		"count": 2,
	}
	data, _ := json.Marshal(response)
	return data
}

// WorkflowGetResponse creates a standard workflow get JSON response
func WorkflowGetResponse(id string) []byte {
	response := map[string]interface{}{
		"id":    id,
		"title": "Test Workflow",
		"owner": "test-user",
		"tasks": []map[string]interface{}{
			{"name": "task1", "action": "dynatrace.automations:run-javascript"},
		},
	}
	data, _ := json.Marshal(response)
	return data
}

// DQLSuccessResponse creates a standard DQL success JSON response
func DQLSuccessResponse() []byte {
	response := map[string]interface{}{
		"state": "SUCCEEDED",
		"result": map[string]interface{}{
			"records": []map[string]interface{}{
				{"timestamp": "2024-01-01T00:00:00Z", "content": "test log"},
			},
		},
	}
	data, _ := json.Marshal(response)
	return data
}

// DQLSuccessResponseWithRecords creates a DQL response with specified number of records
func DQLSuccessResponseWithRecords(count int) []byte {
	records := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		records[i] = map[string]interface{}{
			"id":    i,
			"value": "test-value",
		}
	}

	response := map[string]interface{}{
		"state": "SUCCEEDED",
		"result": map[string]interface{}{
			"records": records,
		},
	}
	data, _ := json.Marshal(response)
	return data
}

// ErrorResponse creates a standard error JSON response
func ErrorResponse(code int, message string) []byte {
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(response)
	return data
}

// CurrentUserResponse creates a standard current user response
func CurrentUserResponse() []byte {
	response := map[string]interface{}{
		"userId": "test-user-id",
		"email":  "test@example.com",
		"name":   "Test User",
	}
	data, _ := json.Marshal(response)
	return data
}

// DocumentListResponse creates a standard document list response
func DocumentListResponse(docType string) []byte {
	response := map[string]interface{}{
		"documents": []map[string]interface{}{
			{
				"id":   "doc-1",
				"name": "Test " + docType + " 1",
				"type": docType,
			},
			{
				"id":   "doc-2",
				"name": "Test " + docType + " 2",
				"type": docType,
			},
		},
	}
	data, _ := json.Marshal(response)
	return data
}

// ExecutionResponse creates a workflow execution response
func ExecutionResponse(executionID string, state string) []byte {
	response := map[string]interface{}{
		"id":    executionID,
		"state": state,
	}
	data, _ := json.Marshal(response)
	return data
}
