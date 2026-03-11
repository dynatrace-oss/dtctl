package resolver

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
)

// Resolver resolves resource names to IDs
type Resolver struct {
	client *client.Client
}

// NewResolver creates a new resolver
func NewResolver(c *client.Client) *Resolver {
	return &Resolver{client: c}
}

// ResourceType represents the type of resource to resolve
type ResourceType string

const (
	TypeWorkflow  ResourceType = "workflow"
	TypeDashboard ResourceType = "dashboard"
	TypeNotebook  ResourceType = "notebook"
	TypeDocument  ResourceType = "document" // generic, searches all document types
)

// ResolveID resolves a name or ID to a resource ID
// If identifier looks like an ID, returns it directly
// If it's a name, searches for matching resources
// Returns error if multiple matches found (ambiguous)
func (r *Resolver) ResolveID(resourceType ResourceType, identifier string) (string, error) {
	// If identifier looks like an ID, return it directly
	if r.looksLikeID(identifier, resourceType) {
		return identifier, nil
	}

	// Search for resources by name
	matches, err := r.searchByName(resourceType, identifier)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no %s found with name %q", resourceType, identifier)
	}

	if len(matches) == 1 {
		return matches[0].ID, nil
	}

	// Multiple matches - return error with suggestions
	return "", r.ambiguousNameError(resourceType, identifier, matches)
}

// looksLikeID checks if a string looks like a resource ID
func (r *Resolver) looksLikeID(str string, resourceType ResourceType) bool {
	// All supported resource types use UUIDs (with dashes)
	if resourceType == TypeDashboard || resourceType == TypeNotebook ||
		resourceType == TypeWorkflow || resourceType == TypeDocument {
		// Simple heuristic: contains dashes and is long enough
		return strings.Contains(str, "-") && len(str) > 20
	}

	return false
}

// Resource represents a found resource
type Resource struct {
	ID   string
	Name string
	Type ResourceType
}

// searchByName searches for resources by name
func (r *Resolver) searchByName(resourceType ResourceType, name string) ([]Resource, error) {
	switch resourceType {
	case TypeWorkflow:
		return r.searchWorkflows(name)
	case TypeDashboard:
		return r.searchDashboards(name)
	case TypeNotebook:
		return r.searchNotebooks(name)
	case TypeDocument:
		return r.searchAllDocuments(name)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// searchWorkflows searches for workflows by name
func (r *Resolver) searchWorkflows(name string) ([]Resource, error) {
	handler := workflow.NewHandler(r.client)
	list, err := handler.List(workflow.WorkflowFilters{})
	if err != nil {
		return nil, err
	}

	var matches []Resource
	nameLower := strings.ToLower(name)

	for _, wf := range list.Results {
		if strings.Contains(strings.ToLower(wf.Title), nameLower) {
			matches = append(matches, Resource{
				ID:   wf.ID,
				Name: wf.Title,
				Type: TypeWorkflow,
			})
		}
	}

	return matches, nil
}

// searchDashboards searches for dashboards by name
func (r *Resolver) searchDashboards(name string) ([]Resource, error) {
	return r.searchDocuments(name, "dashboard")
}

// searchNotebooks searches for notebooks by name
func (r *Resolver) searchNotebooks(name string) ([]Resource, error) {
	return r.searchDocuments(name, "notebook")
}

// searchDocuments searches for documents by name and type
func (r *Resolver) searchDocuments(name, docType string) ([]Resource, error) {
	handler := document.NewHandler(r.client)

	filters := document.DocumentFilters{
		Type: docType,
		Name: name,
	}

	list, err := handler.List(filters)
	if err != nil {
		return nil, err
	}

	var matches []Resource
	nameLower := strings.ToLower(name)

	for _, doc := range list.Documents {
		if strings.Contains(strings.ToLower(doc.Name), nameLower) {
			resourceType := TypeDashboard
			if docType == "notebook" {
				resourceType = TypeNotebook
			}

			matches = append(matches, Resource{
				ID:   doc.ID,
				Name: doc.Name,
				Type: resourceType,
			})
		}
	}

	return matches, nil
}

// searchAllDocuments searches for documents across all types by name
func (r *Resolver) searchAllDocuments(name string) ([]Resource, error) {
	handler := document.NewHandler(r.client)

	// No type filter - search all document types
	filters := document.DocumentFilters{
		Name: name,
	}

	list, err := handler.List(filters)
	if err != nil {
		return nil, err
	}

	var matches []Resource
	nameLower := strings.ToLower(name)

	for _, doc := range list.Documents {
		if strings.Contains(strings.ToLower(doc.Name), nameLower) {
			// Store the actual document type (e.g. "dashboard", "launchpad") so the
			// disambiguation prompt can show it. We wrap it as a ResourceType string.
			matches = append(matches, Resource{
				ID:   doc.ID,
				Name: doc.Name,
				Type: ResourceType(doc.Type),
			})
		}
	}

	return matches, nil
}

// ambiguousNameError creates an error message for ambiguous names
func (r *Resolver) ambiguousNameError(resourceType ResourceType, name string, matches []Resource) error {
	msg := fmt.Sprintf("ambiguous %s name %q - multiple matches found:\n", resourceType, name)

	for i, match := range matches {
		if resourceType == TypeDocument {
			// For generic documents, show the actual document type to help users distinguish
			msg += fmt.Sprintf("  %d. %s (type: %s, ID: %s)\n", i+1, match.Name, match.Type, match.ID)
		} else {
			msg += fmt.Sprintf("  %d. %s (ID: %s)\n", i+1, match.Name, match.ID)
		}
	}

	msg += "\nPlease use the exact ID to specify which resource you want."

	return fmt.Errorf("%s", msg)
}
