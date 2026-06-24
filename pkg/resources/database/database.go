package database

import (
	"context"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkdatabase "github.com/dynatrace-oss/dtctl/sdk/api/database"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Database represents a database instance for CLI display.
type Database struct {
	ID     string `json:"id"     table:"ID"`
	Name   string `json:"name"   table:"NAME"`
	Vendor string `json:"vendor" table:"VENDOR"`
	Type   string `json:"type"   table:"TYPE,wide"`
	Host   string `json:"host"   table:"HOST,wide"`
	Port   string `json:"port"   table:"PORT,wide"`
}

// DatabaseList holds a list of databases.
type DatabaseList struct {
	Databases  []Database `json:"databases"`
	TotalCount int        `json:"totalCount"`
}

// ListOptions configures the List call.
type ListOptions struct {
	Vendor string
}

// Handler wraps the SDK database handler for the CLI layer.
type Handler struct {
	sdk *sdkdatabase.Handler
}

// NewHandler creates a new CLI database handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkdatabase.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// List returns all database instances, optionally filtered by vendor.
func (h *Handler) List(opts ListOptions) (*DatabaseList, error) {
	sdkList, err := h.sdk.List(context.Background(), sdkdatabase.ListOptions{
		Vendor: opts.Vendor,
	})
	if err != nil {
		return nil, err
	}

	dbs := make([]Database, len(sdkList.Databases))
	for i, d := range sdkList.Databases {
		dbs[i] = fromSDK(d)
	}
	return &DatabaseList{
		Databases:  dbs,
		TotalCount: sdkList.TotalCount,
	}, nil
}

func fromSDK(d sdkdatabase.Database) Database {
	return Database{
		ID:     d.ID,
		Name:   d.Name,
		Vendor: d.Vendor,
		Type:   d.Type,
		Host:   d.Host,
		Port:   d.Port,
	}
}
