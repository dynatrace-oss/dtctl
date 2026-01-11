package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestGlobalFlags validates all global persistent flags
func TestGlobalFlags(t *testing.T) {
	tests := []struct {
		flagName     string
		defaultValue string
	}{
		{"config", ""},
		{"context", ""},
		{"output", "table"},
		{"verbose", "0"},
		{"dry-run", "false"},
		{"plain", "false"},
		{"chunk-size", "500"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Global flag --%s not found", tt.flagName)
			}

			if flag.DefValue != tt.defaultValue {
				t.Errorf("Flag --%s default = %q, want %q", tt.flagName, flag.DefValue, tt.defaultValue)
			}
		})
	}
}

// TestQueryFlags validates query command flags
func TestQueryFlags(t *testing.T) {
	tests := []struct {
		flagName     string
		defaultValue string
	}{
		{"file", ""},
		{"live", "false"},
		{"interval", "1m0s"},
		{"width", "0"},
		{"height", "0"},
		{"fullscreen", "false"},
		{"max-result-records", "0"},
		{"max-result-bytes", "0"},
		{"default-scan-limit-gbytes", "0"},
		{"default-sampling-ratio", "0"},
		{"fetch-timeout-seconds", "0"},
		{"enable-preview", "false"},
		{"enforce-query-consumption-limit", "false"},
		{"include-types", "false"},
		{"default-timeframe-start", ""},
		{"default-timeframe-end", ""},
		{"locale", ""},
		{"timezone", ""},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := queryCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Query flag --%s not found", tt.flagName)
			}

			if flag.DefValue != tt.defaultValue {
				t.Errorf("Flag --%s default = %q, want %q", tt.flagName, flag.DefValue, tt.defaultValue)
			}
		})
	}
}

// TestExecFlags validates exec command flags
func TestExecFlags(t *testing.T) {
	// Test flags on execWorkflowCmd
	workflowFlags := []struct {
		flagName     string
		defaultValue string
	}{
		{"params", "[]"},
		{"wait", "false"},
		{"timeout", "30m0s"},
	}

	for _, tt := range workflowFlags {
		t.Run("workflow_"+tt.flagName, func(t *testing.T) {
			flag := execWorkflowCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Exec workflow flag --%s not found", tt.flagName)
			}

			if flag.DefValue != tt.defaultValue {
				t.Errorf("Flag --%s default = %q, want %q", tt.flagName, flag.DefValue, tt.defaultValue)
			}
		})
	}

	// Test flags on execFunctionCmd
	functionFlags := []struct {
		flagName     string
		defaultValue string
	}{
		{"method", "GET"},
		{"payload", ""},
		{"data", ""},
		{"code", ""},
		{"file", ""},
		{"defer", "false"},
	}

	for _, tt := range functionFlags {
		t.Run("function_"+tt.flagName, func(t *testing.T) {
			flag := execFunctionCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Exec function flag --%s not found", tt.flagName)
			}

			if flag.DefValue != tt.defaultValue {
				t.Errorf("Flag --%s default = %q, want %q", tt.flagName, flag.DefValue, tt.defaultValue)
			}
		})
	}

	// Test flags on execDQLCmd
	dqlFlags := []string{"file"}
	for _, flagName := range dqlFlags {
		t.Run("dql_"+flagName, func(t *testing.T) {
			flag := execDQLCmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Fatalf("Exec DQL flag --%s not found", flagName)
			}
		})
	}

	// Test flags on execAnalyzerCmd
	analyzerFlags := []string{"file"}
	for _, flagName := range analyzerFlags {
		t.Run("analyzer_"+flagName, func(t *testing.T) {
			flag := execAnalyzerCmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Fatalf("Exec analyzer flag --%s not found", flagName)
			}
		})
	}
}

// TestGetFlags validates get command flags
func TestGetFlags(t *testing.T) {
	// Test a sample of get subcommand flags
	tests := []struct {
		subcmd   string
		flagName string
	}{
		{"dashboards", "name"},
		{"dashboards", "mine"},
		{"notebooks", "name"},
		{"notebooks", "mine"},
		{"settings", "schema"},
		{"settings", "scope"},
		{"slos", "filter"},
		{"notifications", "type"},
	}

	for _, tt := range tests {
		t.Run(tt.subcmd+"_"+tt.flagName, func(t *testing.T) {
			// Find the subcommand
			var subcmd *cobra.Command
			for _, cmd := range getCmd.Commands() {
				if cmd.Name() == tt.subcmd {
					subcmd = cmd
					break
				}
			}

			if subcmd == nil {
				t.Skipf("Subcommand %s not found", tt.subcmd)
				return
			}

			flag := subcmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Get %s flag --%s not found", tt.subcmd, tt.flagName)
			}
		})
	}
}

// TestCreateFlags validates create command flags
func TestCreateFlags(t *testing.T) {
	// Test common create flags
	tests := []struct {
		subcmd   string
		flagName string
	}{
		{"workflow", "file"},
		{"workflow", "set"},
		{"notebook", "file"},
		{"notebook", "name"},
		{"notebook", "description"},
		{"dashboard", "file"},
		{"dashboard", "name"},
		{"settings", "file"},
		{"settings", "schema"},
		{"settings", "scope"},
	}

	for _, tt := range tests {
		t.Run(tt.subcmd+"_"+tt.flagName, func(t *testing.T) {
			// Find the subcommand
			var subcmd *cobra.Command
			for _, cmd := range createCmd.Commands() {
				if cmd.Name() == tt.subcmd {
					subcmd = cmd
					break
				}
			}

			if subcmd == nil {
				t.Skipf("Create subcommand %s not found", tt.subcmd)
				return
			}

			flag := subcmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Create %s flag --%s not found", tt.subcmd, tt.flagName)
			}
		})
	}
}

// TestWaitFlags validates wait command flags
func TestWaitFlags(t *testing.T) {
	queryFlags := []struct {
		flagName     string
		defaultValue string
	}{
		{"for", ""},
		{"file", ""},
		{"max-attempts", "0"},
		{"quiet", "false"},
	}

	for _, tt := range queryFlags {
		t.Run("query_"+tt.flagName, func(t *testing.T) {
			flag := waitQueryCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Wait query flag --%s not found", tt.flagName)
			}

			if flag.DefValue != tt.defaultValue {
				t.Errorf("Flag --%s default = %q, want %q", tt.flagName, flag.DefValue, tt.defaultValue)
			}
		})
	}
}

// TestApplyFlags validates apply command flags
func TestApplyFlags(t *testing.T) {
	tests := []struct {
		flagName     string
		defaultValue string
	}{
		{"file", ""},
		{"show-diff", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := applyCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Apply flag --%s not found", tt.flagName)
			}

			if flag.DefValue != tt.defaultValue {
				t.Errorf("Flag --%s default = %q, want %q", tt.flagName, flag.DefValue, tt.defaultValue)
			}
		})
	}
}

// TestFlagParsing tests that flags can be parsed correctly
func TestFlagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flagName string
		wantVal  string
	}{
		{
			name:     "output flag short form",
			args:     []string{"-o", "json"},
			flagName: "output",
			wantVal:  "json",
		},
		{
			name:     "output flag long form",
			args:     []string{"--output", "yaml"},
			flagName: "output",
			wantVal:  "yaml",
		},
		{
			name:     "verbose flag single",
			args:     []string{"-v"},
			flagName: "verbose",
			wantVal:  "1",
		},
		{
			name:     "dry-run flag",
			args:     []string{"--dry-run"},
			flagName: "dry-run",
			wantVal:  "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd.ParseFlags(tt.args)

			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("Flag %s not found", tt.flagName)
			}

			if flag.Value.String() != tt.wantVal {
				t.Errorf("Flag %s value = %q, want %q", tt.flagName, flag.Value.String(), tt.wantVal)
			}

			// Reset for next test
			flag.Value.Set(flag.DefValue)
			flag.Changed = false
		})
	}
}

// TestAllCommandsHaveFlags ensures major commands have at least some flags
func TestAllCommandsHaveFlags(t *testing.T) {
	commands := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"query", queryCmd},
		{"exec", execCmd},
		{"get", getCmd},
		{"create", createCmd},
		{"apply", applyCmd},
		{"wait", waitCmd},
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			flagCount := 0
			tc.cmd.Flags().VisitAll(func(f *pflag.Flag) {
				flagCount++
			})

			// Each command should have at least one flag or subcommands with flags
			if flagCount == 0 && len(tc.cmd.Commands()) == 0 {
				t.Errorf("Command %s has no flags and no subcommands", tc.name)
			}
		})
	}
}
