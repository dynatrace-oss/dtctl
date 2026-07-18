package cmd

import (
	"bytes"
	"strings"
	"testing"

	cmdtestutil "github.com/dynatrace-oss/dtctl/cmd/testutil"
	"github.com/dynatrace-oss/dtctl/pkg/inventory"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

func inventoryFixture() *inventory.Inventory {
	return &inventory.Inventory{
		Context:      "example",
		GeneratedAt:  "2026-01-02T03:04:05Z",
		Capabilities: []string{"hosts", "logs", "spans"},
		Absent: []inventory.CapabilityStatus{
			{Name: "rum", Evidence: "no user.events in the data-object catalog"},
		},
		Unknown: []inventory.CapabilityStatus{
			{Name: "genai", Evidence: "probe failed: scan limit exceeded"},
		},
		EntityTypes: map[string]int64{"HOST": 12, "SERVICE": 40, "K8S_POD": 200},
		DataObjects: []string{"dt.entity.host", "dt.entity.service", "logs", "spans"},
		Unfetchable: []string{"metrics"},
		Buckets:     []string{"default_logs", "default_spans"},
		Segments:    []inventory.SegmentInfo{{UID: "seg-1", Name: "prod", Description: "production workloads"}},
		Notes:       []string{"catalog objects without fetch support: metrics"},
		Discovery:   &inventory.Report{Queries: 4, Seconds: 2.5},
	}
}

func TestTopCensusTypes(t *testing.T) {
	census := map[string]int64{"HOST": 12, "SERVICE": 40, "K8S_POD": 40, "AWS_LAMBDA": 1}
	// Ordered by count desc, ties broken by name.
	if got, want := topCensusTypes(census, 12), "K8S_POD:40 SERVICE:40 HOST:12 AWS_LAMBDA:1"; got != want {
		t.Errorf("topCensusTypes = %q, want %q", got, want)
	}
	// Truncation reports how many types were dropped.
	if got, want := topCensusTypes(census, 2), "K8S_POD:40 SERVICE:40 (+2 more types)"; got != want {
		t.Errorf("topCensusTypes(n=2) = %q, want %q", got, want)
	}
}

func TestPrintInventoryHumanGolden(t *testing.T) {
	originalPlainMode := plainMode
	defer func() {
		plainMode = originalPlainMode
		output.ResetColorCache()
	}()
	plainMode = true
	output.ResetColorCache()

	got := captureStdout(t, func() {
		printInventoryHuman(inventoryFixture())
	})
	// The dt.entity.* views must be collapsed to a count, not enumerated.
	if strings.Contains(got, "dt.entity.host") {
		t.Errorf("dt.entity.* views must be collapsed in human output:\n%s", got)
	}
	cmdtestutil.AssertGolden(t, "inventory/human", cmdtestutil.StripANSI(got))
}

func TestInventoryOutputGolden(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			printer := output.NewPrinterWithOptions(format, &buf, false)
			if err := printer.Print(inventoryFixture()); err != nil {
				t.Fatalf("print failed: %v", err)
			}
			cmdtestutil.AssertGolden(t, "inventory/"+format, buf.String())
		})
	}
}

func TestInventoryOutputGoldenAgent(t *testing.T) {
	var buf bytes.Buffer
	printer := output.NewAgentPrinter(&buf, &output.ResponseContext{Verb: "inventory"})
	if err := printer.Print(inventoryFixture()); err != nil {
		t.Fatalf("print failed: %v", err)
	}
	cmdtestutil.AssertGolden(t, "inventory/agent", buf.String())
}
