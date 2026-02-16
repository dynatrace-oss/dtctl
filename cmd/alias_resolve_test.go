package cmd

import (
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestResolveAlias(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		aliases   map[string]string
		wantArgs  []string
		wantShell bool
		wantErr   string
	}{
		{
			name:     "simple expansion",
			args:     []string{"wf"},
			aliases:  map[string]string{"wf": "get workflows"},
			wantArgs: []string{"get", "workflows"},
		},
		{
			name:     "positional params",
			args:     []string{"pw", "my-id"},
			aliases:  map[string]string{"pw": "get workflow $1"},
			wantArgs: []string{"get", "workflow", "my-id"},
		},
		{
			name:     "multiple positional params",
			args:     []string{"deploy", "workflows.yaml", "production"},
			aliases:  map[string]string{"deploy": "apply -f $1 --context=$2"},
			wantArgs: []string{"apply", "-f", "workflows.yaml", "--context=production"},
		},
		{
			name:     "extra args appended",
			args:     []string{"wf", "--context=prod"},
			aliases:  map[string]string{"wf": "get workflows"},
			wantArgs: []string{"get", "workflows", "--context=prod"},
		},
		{
			name:     "extra args appended after params",
			args:     []string{"pw", "my-id", "--output=json"},
			aliases:  map[string]string{"pw": "get workflow $1"},
			wantArgs: []string{"get", "workflow", "my-id", "--output=json"},
		},
		{
			name:      "shell alias",
			args:      []string{"count"},
			aliases:   map[string]string{"count": "!dtctl get workflows -o json | jq length"},
			wantArgs:  []string{"dtctl get workflows -o json | jq length"},
			wantShell: true,
		},
		{
			name:      "shell alias with args",
			args:      []string{"count", "extra"},
			aliases:   map[string]string{"count": "!dtctl get workflows -o json | jq length"},
			wantArgs:  []string{"dtctl get workflows -o json | jq length extra"},
			wantShell: true,
		},
		{
			name:    "missing required arg",
			args:    []string{"pw"},
			aliases: map[string]string{"pw": "get workflow $1"},
			wantErr: "requires at least 1 argument",
		},
		{
			name:    "missing multiple required args",
			args:    []string{"deploy", "workflows.yaml"},
			aliases: map[string]string{"deploy": "apply -f $1 --context=$2"},
			wantErr: "requires at least 2 argument",
		},
		{
			name:     "no match returns nil",
			args:     []string{"unknown"},
			aliases:  map[string]string{"wf": "get workflows"},
			wantArgs: nil,
		},
		{
			name:     "flag as first arg skips lookup",
			args:     []string{"--help"},
			aliases:  map[string]string{"--help": "bad"},
			wantArgs: nil,
		},
		{
			name:     "empty args",
			args:     []string{},
			aliases:  map[string]string{"wf": "get workflows"},
			wantArgs: nil,
		},
		{
			name:     "nil config",
			args:     []string{"wf"},
			aliases:  nil,
			wantArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			if tt.aliases != nil {
				cfg.Aliases = tt.aliases
			}

			gotArgs, gotShell, err := resolveAlias(tt.args, cfg)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantArgs, gotArgs)
			require.Equal(t, tt.wantShell, gotShell)
		})
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple command",
			input: "get workflows",
			want:  []string{"get", "workflows"},
		},
		{
			name:  "double quoted string",
			input: `query "fetch logs | limit 10"`,
			want:  []string{"query", "fetch logs | limit 10"},
		},
		{
			name:  "single quoted string",
			input: `get workflow 'my workflow'`,
			want:  []string{"get", "workflow", "my workflow"},
		},
		{
			name:  "mixed quotes",
			input: `query "fetch logs" --filter='status == "ERROR"'`,
			want:  []string{"query", "fetch logs", "--filter=status == \"ERROR\""},
		},
		{
			name:  "flags",
			input: "get workflows --context=production -o json",
			want:  []string{"get", "workflows", "--context=production", "-o", "json"},
		},
		{
			name:  "multiple spaces",
			input: "get    workflows   --context=prod",
			want:  []string{"get", "workflows", "--context=prod"},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "only spaces",
			input: "   ",
			want:  []string{},
		},
		{
			name:  "trailing spaces",
			input: "get workflows  ",
			want:  []string{"get", "workflows"},
		},
		{
			name:  "leading spaces",
			input: "  get workflows",
			want:  []string{"get", "workflows"},
		},
		{
			name:  "positional params",
			input: "get workflow $1 --context=$2",
			want:  []string{"get", "workflow", "$1", "--context=$2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCommand(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSubstituteParams(t *testing.T) {
	tests := []struct {
		name        string
		s           string
		args        []string
		wantResult  string
		wantMaxUsed int
	}{
		{
			name:        "single param",
			s:           "get workflow $1",
			args:        []string{"my-id"},
			wantResult:  "get workflow my-id",
			wantMaxUsed: 1,
		},
		{
			name:        "multiple params",
			s:           "apply -f $1 --context=$2",
			args:        []string{"workflows.yaml", "production"},
			wantResult:  "apply -f workflows.yaml --context=production",
			wantMaxUsed: 2,
		},
		{
			name:        "param not replaced when missing",
			s:           "get workflow $1 --context=$2",
			args:        []string{"my-id"},
			wantResult:  "get workflow my-id --context=$2",
			wantMaxUsed: 2,
		},
		{
			name:        "no params",
			s:           "get workflows",
			args:        []string{"extra"},
			wantResult:  "get workflows",
			wantMaxUsed: 0,
		},
		{
			name:        "param $9",
			s:           "$1 $2 $3 $4 $5 $6 $7 $8 $9",
			args:        []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
			wantResult:  "a b c d e f g h i",
			wantMaxUsed: 9,
		},
		{
			name:        "repeated param",
			s:           "get workflow $1 --id=$1",
			args:        []string{"my-id"},
			wantResult:  "get workflow my-id --id=my-id",
			wantMaxUsed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxUsed := 0
			got := substituteParams(tt.s, tt.args, &maxUsed)
			require.Equal(t, tt.wantResult, got)
			require.Equal(t, tt.wantMaxUsed, maxUsed)
		})
	}
}
