package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func newMetadataFlagCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "x"}
	c.Flags().StringP("metadata", "M", "", "")
	c.Flags().Lookup("metadata").NoOptDefVal = "all"
	if err := c.Flags().Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return c
}

// Agent mode is token-optimal by default: without -M it gets the minimal
// metadata set; any explicit -M wins, and outside agent mode nothing changes.
func TestResolveMetadataFlag(t *testing.T) {
	tests := []struct {
		name          string
		agent         bool
		args          []string
		wantFields    []string
		wantDefaulted bool
	}{
		{"agent default is minimal", true, nil, []string{"minimal"}, true},
		{"non-agent default is off", false, nil, nil, false},
		{"agent -M=all restores full", true, []string{"-M=all"}, []string{"all"}, false},
		{"agent bare -M is full", true, []string{"-M"}, []string{"all"}, false},
		{"agent explicit minimal is not a default", true, []string{"--metadata=minimal"}, []string{"minimal"}, false},
		{"agent --metadata= turns it off", true, []string{"--metadata="}, nil, false},
		{"non-agent -M=minimal", false, []string{"-M=minimal"}, []string{"minimal"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, defaulted, err := resolveMetadataFlag(newMetadataFlagCmd(t, tt.args...), tt.agent)
			if err != nil {
				t.Fatalf("resolveMetadataFlag: %v", err)
			}
			if !reflect.DeepEqual(fields, tt.wantFields) || defaulted != tt.wantDefaulted {
				t.Errorf("got (%v, %v), want (%v, %v)", fields, defaulted, tt.wantFields, tt.wantDefaulted)
			}
		})
	}
}

func TestResolveMetadataFlag_InvalidField(t *testing.T) {
	if _, _, err := resolveMetadataFlag(newMetadataFlagCmd(t, "-M=nope"), true); err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}
