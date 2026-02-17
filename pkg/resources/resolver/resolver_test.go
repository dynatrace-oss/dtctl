package resolver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
)

func TestNewResolver(t *testing.T) {
	c := &client.Client{}
	r := NewResolver(c)

	if r == nil {
		t.Fatal("NewResolver returned nil")
	}

	if r.client != c {
		t.Error("NewResolver did not set client correctly")
	}
}

func TestLooksLikeID(t *testing.T) {
	tests := []struct {
		name         string
		identifier   string
		resourceType ResourceType
		want         bool
	}{
		{
			name:         "workflow UUID with dashes",
			identifier:   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			resourceType: TypeWorkflow,
			want:         true,
		},
		{
			name:         "dashboard UUID with dashes",
			identifier:   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			resourceType: TypeDashboard,
			want:         true,
		},
		{
			name:         "notebook UUID with dashes",
			identifier:   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			resourceType: TypeNotebook,
			want:         true,
		},
		{
			name:         "workflow name without dashes",
			identifier:   "my-workflow",
			resourceType: TypeWorkflow,
			want:         false, // Too short
		},
		{
			name:         "dashboard name without UUID format",
			identifier:   "MyDashboard",
			resourceType: TypeDashboard,
			want:         false,
		},
		{
			name:         "short string with dash",
			identifier:   "abc-def",
			resourceType: TypeWorkflow,
			want:         false, // Not long enough
		},
		{
			name:         "long string without dashes",
			identifier:   "abcdefghijklmnopqrstuvwxyz",
			resourceType: TypeDashboard,
			want:         false, // No dashes
		},
		{
			name:         "empty string",
			identifier:   "",
			resourceType: TypeWorkflow,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Resolver{}
			got := r.looksLikeID(tt.identifier, tt.resourceType)

			if got != tt.want {
				t.Errorf("looksLikeID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveID_AlreadyID(t *testing.T) {
	// Mock server should not be called when identifier looks like an ID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Server should not be called when identifier looks like an ID")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	tests := []struct {
		name         string
		resourceType ResourceType
		identifier   string
	}{
		{
			name:         "workflow with UUID",
			resourceType: TypeWorkflow,
			identifier:   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		},
		{
			name:         "dashboard with UUID",
			resourceType: TypeDashboard,
			identifier:   "12345678-1234-1234-1234-123456789012",
		},
		{
			name:         "notebook with UUID",
			resourceType: TypeNotebook,
			identifier:   "notebook-id-with-dashes-long-enough",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := resolver.ResolveID(tt.resourceType, tt.identifier)

			if err != nil {
				t.Errorf("ResolveID() error = %v, want nil", err)
			}

			if id != tt.identifier {
				t.Errorf("ResolveID() = %v, want %v", id, tt.identifier)
			}
		})
	}
}

func TestResolveID_WorkflowByName_SingleMatch(t *testing.T) {
	workflowList := workflow.WorkflowList{
		Count: 1,
		Results: []workflow.Workflow{
			{
				ID:    "workflow-id-1",
				Title: "My Test Workflow",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform/automation/v1/workflows" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workflowList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(TypeWorkflow, "Test")

	if err != nil {
		t.Errorf("ResolveID() error = %v, want nil", err)
	}

	if id != "workflow-id-1" {
		t.Errorf("ResolveID() = %v, want workflow-id-1", id)
	}
}

func TestResolveID_WorkflowByName_MultipleMatches(t *testing.T) {
	workflowList := workflow.WorkflowList{
		Count: 2,
		Results: []workflow.Workflow{
			{
				ID:    "workflow-id-1",
				Title: "Test Workflow 1",
			},
			{
				ID:    "workflow-id-2",
				Title: "Test Workflow 2",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workflowList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(TypeWorkflow, "Test")

	if err == nil {
		t.Error("ResolveID() should return error for ambiguous name")
	}

	if id != "" {
		t.Errorf("ResolveID() should return empty string on error, got %v", id)
	}

	// Check error message contains both workflow IDs
	errMsg := err.Error()
	if !strings.Contains(errMsg, "ambiguous") {
		t.Errorf("Error should mention 'ambiguous', got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "workflow-id-1") {
		t.Errorf("Error should list workflow-id-1, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "workflow-id-2") {
		t.Errorf("Error should list workflow-id-2, got: %v", errMsg)
	}
}

func TestResolveID_WorkflowByName_NoMatches(t *testing.T) {
	workflowList := workflow.WorkflowList{
		Count:   0,
		Results: []workflow.Workflow{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workflowList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(TypeWorkflow, "NonExistent")

	if err == nil {
		t.Error("ResolveID() should return error when no matches found")
	}

	if id != "" {
		t.Errorf("ResolveID() should return empty string on error, got %v", id)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "no workflow found") {
		t.Errorf("Error should mention no workflow found, got: %v", errMsg)
	}
}

func TestResolveID_DashboardByName_SingleMatch(t *testing.T) {
	docList := document.DocumentList{
		TotalCount: 1,
		Documents: []document.DocumentMetadata{
			{
				ID:   "dashboard-id-1",
				Name: "My Test Dashboard",
				Type: "dashboard",
				ModificationInfo: document.ModificationInfo{
					CreatedTime:      time.Now(),
					LastModifiedTime: time.Now(),
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform/document/v1/documents" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Check filter parameter
		filter := r.URL.Query().Get("filter")
		if !strings.Contains(filter, "type=='dashboard'") {
			t.Errorf("Expected filter to contain type=='dashboard', got: %s", filter)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(docList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(TypeDashboard, "Test")

	if err != nil {
		t.Errorf("ResolveID() error = %v, want nil", err)
	}

	if id != "dashboard-id-1" {
		t.Errorf("ResolveID() = %v, want dashboard-id-1", id)
	}
}

func TestResolveID_NotebookByName_SingleMatch(t *testing.T) {
	docList := document.DocumentList{
		TotalCount: 1,
		Documents: []document.DocumentMetadata{
			{
				ID:   "notebook-id-1",
				Name: "My Test Notebook",
				Type: "notebook",
				ModificationInfo: document.ModificationInfo{
					CreatedTime:      time.Now(),
					LastModifiedTime: time.Now(),
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform/document/v1/documents" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Check filter parameter
		filter := r.URL.Query().Get("filter")
		if !strings.Contains(filter, "type=='notebook'") {
			t.Errorf("Expected filter to contain type=='notebook', got: %s", filter)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(docList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(TypeNotebook, "Test")

	if err != nil {
		t.Errorf("ResolveID() error = %v, want nil", err)
	}

	if id != "notebook-id-1" {
		t.Errorf("ResolveID() = %v, want notebook-id-1", id)
	}
}

func TestResolveID_UnsupportedResourceType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Server should not be called for unsupported resource type")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(ResourceType("unsupported"), "test")

	if err == nil {
		t.Error("ResolveID() should return error for unsupported resource type")
	}

	if id != "" {
		t.Errorf("ResolveID() should return empty string on error, got %v", id)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "unsupported resource type") {
		t.Errorf("Error should mention unsupported resource type, got: %v", errMsg)
	}
}

func TestResolveID_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	id, err := resolver.ResolveID(TypeWorkflow, "test")

	if err == nil {
		t.Error("ResolveID() should return error on API error")
	}

	if id != "" {
		t.Errorf("ResolveID() should return empty string on error, got %v", id)
	}
}

func TestSearchWorkflows_CaseInsensitiveMatch(t *testing.T) {
	workflowList := workflow.WorkflowList{
		Count: 3,
		Results: []workflow.Workflow{
			{
				ID:    "workflow-1",
				Title: "Production Workflow",
			},
			{
				ID:    "workflow-2",
				Title: "Test Workflow",
			},
			{
				ID:    "workflow-3",
				Title: "Development Workflow",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workflowList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	// Search with lowercase - should match "Production Workflow"
	matches, err := resolver.searchWorkflows("production")

	if err != nil {
		t.Errorf("searchWorkflows() error = %v, want nil", err)
	}

	if len(matches) != 1 {
		t.Fatalf("searchWorkflows() returned %d matches, want 1", len(matches))
	}

	if matches[0].ID != "workflow-1" {
		t.Errorf("searchWorkflows() returned ID %v, want workflow-1", matches[0].ID)
	}

	if matches[0].Name != "Production Workflow" {
		t.Errorf("searchWorkflows() returned Name %v, want Production Workflow", matches[0].Name)
	}

	if matches[0].Type != TypeWorkflow {
		t.Errorf("searchWorkflows() returned Type %v, want %v", matches[0].Type, TypeWorkflow)
	}
}

func TestSearchWorkflows_PartialMatch(t *testing.T) {
	workflowList := workflow.WorkflowList{
		Count: 2,
		Results: []workflow.Workflow{
			{
				ID:    "workflow-1",
				Title: "Deploy to Production",
			},
			{
				ID:    "workflow-2",
				Title: "Deploy to Staging",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workflowList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	// Search for "deploy" should match both
	matches, err := resolver.searchWorkflows("deploy")

	if err != nil {
		t.Errorf("searchWorkflows() error = %v, want nil", err)
	}

	if len(matches) != 2 {
		t.Fatalf("searchWorkflows() returned %d matches, want 2", len(matches))
	}
}

func TestSearchDocuments_Dashboard(t *testing.T) {
	docList := document.DocumentList{
		TotalCount: 1,
		Documents: []document.DocumentMetadata{
			{
				ID:   "dash-1",
				Name: "Production Dashboard",
				Type: "dashboard",
				ModificationInfo: document.ModificationInfo{
					CreatedTime:      time.Now(),
					LastModifiedTime: time.Now(),
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if !strings.Contains(filter, "type=='dashboard'") {
			t.Errorf("Expected filter to contain type=='dashboard', got: %s", filter)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(docList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	matches, err := resolver.searchDocuments("production", "dashboard")

	if err != nil {
		t.Errorf("searchDocuments() error = %v, want nil", err)
	}

	if len(matches) != 1 {
		t.Fatalf("searchDocuments() returned %d matches, want 1", len(matches))
	}

	if matches[0].Type != TypeDashboard {
		t.Errorf("searchDocuments() returned Type %v, want %v", matches[0].Type, TypeDashboard)
	}
}

func TestSearchDocuments_Notebook(t *testing.T) {
	docList := document.DocumentList{
		TotalCount: 1,
		Documents: []document.DocumentMetadata{
			{
				ID:   "note-1",
				Name: "Analysis Notebook",
				Type: "notebook",
				ModificationInfo: document.ModificationInfo{
					CreatedTime:      time.Now(),
					LastModifiedTime: time.Now(),
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if !strings.Contains(filter, "type=='notebook'") {
			t.Errorf("Expected filter to contain type=='notebook', got: %s", filter)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(docList)
	}))
	defer server.Close()

	c, err := client.New(server.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	resolver := NewResolver(c)

	matches, err := resolver.searchDocuments("analysis", "notebook")

	if err != nil {
		t.Errorf("searchDocuments() error = %v, want nil", err)
	}

	if len(matches) != 1 {
		t.Fatalf("searchDocuments() returned %d matches, want 1", len(matches))
	}

	if matches[0].Type != TypeNotebook {
		t.Errorf("searchDocuments() returned Type %v, want %v", matches[0].Type, TypeNotebook)
	}
}

func TestAmbiguousNameError(t *testing.T) {
	resolver := &Resolver{}

	matches := []Resource{
		{
			ID:   "id-1",
			Name: "Resource One",
			Type: TypeWorkflow,
		},
		{
			ID:   "id-2",
			Name: "Resource Two",
			Type: TypeWorkflow,
		},
		{
			ID:   "id-3",
			Name: "Resource Three",
			Type: TypeWorkflow,
		},
	}

	err := resolver.ambiguousNameError(TypeWorkflow, "Resource", matches)

	if err == nil {
		t.Fatal("ambiguousNameError() should return an error")
	}

	errMsg := err.Error()

	// Check error message contains key information
	if !strings.Contains(errMsg, "ambiguous") {
		t.Errorf("Error should contain 'ambiguous', got: %v", errMsg)
	}

	if !strings.Contains(errMsg, "workflow") {
		t.Errorf("Error should contain resource type 'workflow', got: %v", errMsg)
	}

	if !strings.Contains(errMsg, "Resource") {
		t.Errorf("Error should contain the name 'Resource', got: %v", errMsg)
	}

	// Check all three matches are listed
	for _, match := range matches {
		if !strings.Contains(errMsg, match.ID) {
			t.Errorf("Error should contain ID %s, got: %v", match.ID, errMsg)
		}
		if !strings.Contains(errMsg, match.Name) {
			t.Errorf("Error should contain Name %s, got: %v", match.Name, errMsg)
		}
	}

	// Check helpful message
	if !strings.Contains(errMsg, "use the exact ID") {
		t.Errorf("Error should contain helpful message about using exact ID, got: %v", errMsg)
	}
}
