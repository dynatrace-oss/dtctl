package watch

import (
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// Re-export output types for use within watch package
type ChangeType = output.ChangeType
type Change = output.Change

const (
	ChangeTypeAdded    = output.ChangeTypeAdded
	ChangeTypeModified = output.ChangeTypeModified
	ChangeTypeDeleted  = output.ChangeTypeDeleted
)

type ResourceFetcher func() (interface{}, error)

type WatcherOptions struct {
	Interval    time.Duration
	Client      *client.Client
	Fetcher     ResourceFetcher
	Printer     output.Printer
	ShowInitial bool
}

type Watcher struct {
	interval    time.Duration
	client      *client.Client
	fetcher     ResourceFetcher
	differ      *Differ
	printer     output.Printer
	stopCh      chan struct{}
	showInitial bool
}
