// Package database provides a typed client for listing database instances
// monitored by Dynatrace via Smartscape nodes. It queries multiple Smartscape
// node types (DB_INSTANCE_POSTGRES, DB_INSTANCE_MYSQL, DB_INSTANCE_MSSQL,
// DB_INSTANCE_MARIADB) using DQL and returns a unified list.
package database

import (
	"context"
	"fmt"
	"strings"

	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// nodeType is a Smartscape node type identifier for a database instance.
type nodeType struct {
	id     string
	vendor string
}

// supportedNodes lists all Smartscape node types queried for the databases resource.
var supportedNodes = []nodeType{
	{id: "DB_INSTANCE_POSTGRES", vendor: "PostgreSQL"},
	{id: "DB_INSTANCE_MYSQL", vendor: "MySQL"},
	{id: "DB_INSTANCE_MSSQL", vendor: "MSSQL"},
	{id: "DB_INSTANCE_MARIADB", vendor: "MariaDB"},
}

// Handler queries Smartscape for database instances via DQL.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new database handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// Database represents a single database instance from Smartscape.
type Database struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Type   string `json:"type"`
	Host   string `json:"host,omitempty"`
	Port   string `json:"port,omitempty"`
}

// DatabaseList holds the result of a List call.
type DatabaseList struct {
	Databases  []Database `json:"databases"`
	TotalCount int        `json:"totalCount"`
}

// ListOptions configures which databases to retrieve.
type ListOptions struct {
	// Vendor filters results to a specific database vendor (e.g. "PostgreSQL",
	// "MySQL", "MSSQL", "MariaDB"). Empty means all vendors.
	Vendor string
}

// List returns all database instances visible in Smartscape, optionally
// filtered by vendor. Results are sorted by name ascending.
func (h *Handler) List(ctx context.Context, opts ListOptions) (*DatabaseList, error) {
	nodes := filterNodes(opts.Vendor)
	if len(nodes) == 0 {
		return &DatabaseList{}, nil
	}

	query := buildQuery(nodes)

	qh := sdkquery.NewHandler(h.client)
	resp, err := qh.ExecuteAndPoll(ctx, sdkquery.ExecuteRequest{Query: query}, nil)
	if err != nil {
		return nil, fmt.Errorf("database smartscape query failed: %w", err)
	}

	records := resp.GetRecords()
	dbs := make([]Database, 0, len(records))
	for _, rec := range records {
		dbs = append(dbs, recordToDatabase(rec))
	}

	return &DatabaseList{
		Databases:  dbs,
		TotalCount: len(dbs),
	}, nil
}

// filterNodes returns the supported node types that match the given vendor
// filter (case-insensitive prefix or full match). Empty vendor returns all.
func filterNodes(vendor string) []nodeType {
	if vendor == "" {
		return supportedNodes
	}
	lower := strings.ToLower(vendor)
	var matched []nodeType
	for _, n := range supportedNodes {
		if strings.HasPrefix(strings.ToLower(n.vendor), lower) {
			matched = append(matched, n)
		}
	}
	return matched
}

// buildQuery constructs a DQL query that unions all requested node types via
// append, adds a vendor label, and sorts by name.
func buildQuery(nodes []nodeType) string {
	// Each per-type fragment selects the same projected fields so append works.
	const fieldProjection = `| fields id, name, vendor, type = nodeType, host = db.connection_details.hostname, port = db.connection_details.port`

	parts := make([]string, len(nodes))
	for i, n := range nodes {
		parts[i] = fmt.Sprintf(
			"smartscapeNodes \"%s\"\n| fieldsAdd vendor = \"%s\", nodeType = \"%s\"\n%s",
			n.id, n.vendor, n.id, fieldProjection,
		)
	}

	query := parts[0]
	for _, part := range parts[1:] {
		query += "\n| append [\n" + part + "\n]"
	}
	query += "\n| sort name asc"
	return query
}

func recordToDatabase(rec map[string]interface{}) Database {
	db := Database{}
	if v, ok := rec["id"].(string); ok {
		db.ID = v
	}
	if v, ok := rec["name"].(string); ok {
		db.Name = v
	}
	if v, ok := rec["vendor"].(string); ok {
		db.Vendor = v
	}
	if v, ok := rec["type"].(string); ok {
		db.Type = v
	}
	if v, ok := rec["host"].(string); ok {
		db.Host = v
	}
	if v, ok := rec["port"].(string); ok {
		db.Port = v
	}
	return db
}
