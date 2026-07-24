package metrics

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const enableEnv = "DTCTL_ENABLE_METRICS"

var defaultCollector = NewFromEnv()

// Collector stores dtctl runtime metrics in memory and, when enabled, also emits
// OpenTelemetry metric instruments for the current process.
type Collector struct {
	enabled bool

	mu                 sync.Mutex
	commands           map[string]*commandStat
	apiErrors          map[string]*apiErrorStat
	totalRuns          int64
	totalCommandErrors int64
	totalAPIErrors     int64

	meter          metric.Meter
	commandLatency metric.Float64Histogram
	commandCount   metric.Int64Counter
	apiErrorCount  metric.Int64Counter
}

type commandStat struct {
	Count       int64
	Total       time.Duration
	Last        time.Duration
	LastSuccess bool
}

type apiErrorStat struct {
	Count       int64
	LastStatus  int
	LastMessage string
}

// CommandSnapshot is a stable, serialisable view of a command metric.
type CommandSnapshot struct {
	Command     string        `json:"command" table:"COMMAND"`
	Count       int64         `json:"count" table:"COUNT"`
	Total       time.Duration `json:"total" table:"TOTAL"`
	Average     time.Duration `json:"average" table:"AVERAGE"`
	Last        time.Duration `json:"last" table:"LAST"`
	LastSuccess bool          `json:"lastSuccess" table:"LAST SUCCESS"`
}

// APIErrorSnapshot is a stable, serialisable view of an API error metric.
type APIErrorSnapshot struct {
	Operation   string `json:"operation" table:"OPERATION"`
	Count       int64  `json:"count" table:"COUNT"`
	LastStatus  int    `json:"lastStatus" table:"LAST STATUS"`
	LastMessage string `json:"lastMessage" table:"LAST MESSAGE,wide"`
}

// Snapshot is the exported collector state.
type Snapshot struct {
	Enabled            bool               `json:"enabled" table:"ENABLED"`
	TotalCommands      int64              `json:"totalCommands" table:"TOTAL COMMANDS"`
	TotalCommandErrors int64              `json:"totalCommandErrors" table:"TOTAL COMMAND ERRORS"`
	TotalAPIErrors     int64              `json:"totalApiErrors" table:"TOTAL API ERRORS"`
	Commands           []CommandSnapshot  `json:"commands" table:"-"`
	APIErrors          []APIErrorSnapshot `json:"apiErrors" table:"-"`
}

// NewFromEnv builds a collector from the DTCTL_ENABLE_METRICS toggle.
func NewFromEnv() *Collector {
	return New(parseBoolEnv(os.Getenv(enableEnv)))
}

// New creates a collector with the given enabled state.
func New(enabled bool) *Collector {
	c := &Collector{
		enabled:   enabled,
		commands:  make(map[string]*commandStat),
		apiErrors: make(map[string]*apiErrorStat),
	}

	if enabled {
		c.meter = otel.Meter("github.com/dynatrace-oss/dtctl/pkg/metrics")
		c.commandLatency, _ = c.meter.Float64Histogram(
			"dtctl.command.duration",
			metric.WithUnit("s"),
			metric.WithDescription("dtctl command duration in seconds"),
		)
		c.commandCount, _ = c.meter.Int64Counter(
			"dtctl.command.count",
			metric.WithDescription("dtctl command invocation count"),
		)
		c.apiErrorCount, _ = c.meter.Int64Counter(
			"dtctl.api.errors",
			metric.WithDescription("dtctl API error count"),
		)
	}

	return c
}

// Default returns the process-wide collector.
func Default() *Collector { return defaultCollector }

// SetDefault replaces the process-wide collector. It is primarily useful in tests.
func SetDefault(c *Collector) {
	if c == nil {
		c = New(false)
	}
	defaultCollector = c
}

// ResetDefault refreshes the process-wide collector from the environment.
func ResetDefault() {
	defaultCollector = NewFromEnv()
}

// Enabled reports whether the collector is active.
func (c *Collector) Enabled() bool {
	return c != nil && c.enabled
}

// RecordCommand records a completed command invocation.
func (c *Collector) RecordCommand(name string, duration time.Duration, err error) {
	if c == nil {
		return
	}

	c.mu.Lock()
	stat := c.commands[name]
	if stat == nil {
		stat = &commandStat{}
		c.commands[name] = stat
	}
	stat.Count++
	stat.Total += duration
	stat.Last = duration
	stat.LastSuccess = err == nil
	c.totalRuns++
	if err != nil {
		c.totalCommandErrors++
	}
	c.mu.Unlock()

	if !c.enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("command", name),
		attribute.Bool("success", err == nil),
	}
	ctx := context.Background()
	c.commandLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	c.commandCount.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordAPIError records an API error. The status code and message are retained
// for export, while the OTel counter receives a low-cardinality operation label.
func (c *Collector) RecordAPIError(operation string, statusCode int, message string) {
	if c == nil {
		return
	}
	if operation == "" {
		operation = "http"
	}

	c.mu.Lock()
	stat := c.apiErrors[operation]
	if stat == nil {
		stat = &apiErrorStat{}
		c.apiErrors[operation] = stat
	}
	stat.Count++
	stat.LastStatus = statusCode
	stat.LastMessage = message
	c.totalAPIErrors++
	c.mu.Unlock()

	if !c.enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.String("status_class", statusClass(statusCode)),
	}
	if c.apiErrorCount != nil {
		c.apiErrorCount.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}

// Snapshot returns a point-in-time export of the collector state.
func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := Snapshot{
		Enabled:            c.enabled,
		TotalCommands:      c.totalRuns,
		TotalCommandErrors: c.totalCommandErrors,
		TotalAPIErrors:     c.totalAPIErrors,
	}

	if len(c.commands) > 0 {
		snapshot.Commands = make([]CommandSnapshot, 0, len(c.commands))
		for name, stat := range c.commands {
			avg := time.Duration(0)
			if stat.Count > 0 {
				avg = time.Duration(int64(stat.Total) / stat.Count)
			}
			snapshot.Commands = append(snapshot.Commands, CommandSnapshot{
				Command:     name,
				Count:       stat.Count,
				Total:       stat.Total,
				Average:     avg,
				Last:        stat.Last,
				LastSuccess: stat.LastSuccess,
			})
		}
		sort.Slice(snapshot.Commands, func(i, j int) bool {
			return snapshot.Commands[i].Command < snapshot.Commands[j].Command
		})
	}

	if len(c.apiErrors) > 0 {
		snapshot.APIErrors = make([]APIErrorSnapshot, 0, len(c.apiErrors))
		for op, stat := range c.apiErrors {
			snapshot.APIErrors = append(snapshot.APIErrors, APIErrorSnapshot{
				Operation:   op,
				Count:       stat.Count,
				LastStatus:  stat.LastStatus,
				LastMessage: stat.LastMessage,
			})
		}
		sort.Slice(snapshot.APIErrors, func(i, j int) bool {
			return snapshot.APIErrors[i].Operation < snapshot.APIErrors[j].Operation
		})
	}

	return snapshot
}

func parseBoolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func statusClass(statusCode int) string {
	if statusCode <= 0 {
		return "unknown"
	}
	return strconv.Itoa(statusCode/100) + "xx"
}
