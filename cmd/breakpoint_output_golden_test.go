package cmd

import (
	"bytes"
	"testing"

	cmdtestutil "github.com/dynatrace-oss/dtctl/cmd/testutil"
	"github.com/dynatrace-oss/dtctl/pkg/output"
)

func TestBreakpointMessageOutputGolden(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{name: "table", format: "table"},
		{name: "json", format: "json"},
		{name: "yaml", format: "yaml"},
	}

	result := breakpointCommandOutput{Message: "Created breakpoint at OrderController.java:306"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := output.NewPrinterWithOptions(tt.format, &buf, false)
			if err := printer.Print(result); err != nil {
				t.Fatalf("print failed: %v", err)
			}
			cmdtestutil.AssertGolden(t, "breakpoint-output/"+tt.name, buf.String())
		})
	}
}

func TestBreakpointMessageOutputGoldenAgent(t *testing.T) {
	var buf bytes.Buffer
	printer := output.NewAgentPrinter(&buf, &output.ResponseContext{Verb: "create", Resource: "breakpoint"})

	result := breakpointCommandOutput{Message: "Created breakpoint at OrderController.java:306"}
	if err := printer.Print(result); err != nil {
		t.Fatalf("print failed: %v", err)
	}

	cmdtestutil.AssertGolden(t, "breakpoint-output/agent", buf.String())
}
