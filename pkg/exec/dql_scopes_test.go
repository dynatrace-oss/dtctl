package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequiredStorageScopes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []ScopeNeed
	}{
		{
			name:  "fetch logs",
			query: "fetch logs | limit 10",
			want:  []ScopeNeed{{Scope: "storage:logs:read", Because: []string{"fetch logs"}}},
		},
		{
			name:  "fetch with parameters after the data object",
			query: `fetch spans, from:now()-1h | limit 1`,
			want:  []ScopeNeed{{Scope: "storage:spans:read", Because: []string{"fetch spans"}}},
		},
		{
			name:  "dotted data object",
			query: "fetch security.events | limit 1",
			want:  []ScopeNeed{{Scope: "storage:security.events:read", Because: []string{"fetch security.events"}}},
		},
		{
			name:  "entity views share one scope",
			query: "fetch dt.entity.service | fields entity.name",
			want:  []ScopeNeed{{Scope: "storage:entities:read", Because: []string{"fetch dt.entity.service"}}},
		},
		{
			name:  "smartscape function on a log query (the observed fan-out)",
			query: "fetch logs | fieldsAdd svc = getNodeName(dt.smartscape.service) | limit 5",
			want: []ScopeNeed{
				{Scope: "storage:logs:read", Because: []string{"fetch logs"}},
				{Scope: "storage:smartscape:read", Because: []string{"getNodeName()"}},
			},
		},
		{
			name:  "smartscape commands",
			query: `smartscapeNodes "HOST" | limit 10`,
			want:  []ScopeNeed{{Scope: "storage:smartscape:read", Because: []string{"smartscapeNodes"}}},
		},
		{
			name:  "reasons for one scope are merged and de-duplicated",
			query: `smartscapeEdges "runs_on" | fieldsAdd a = getNodeName(source_id), b = getNodeName(target_id), c = getNodeField(target_id, "name")`,
			want: []ScopeNeed{{Scope: "storage:smartscape:read", Because: []string{
				"getNodeField()", "getNodeName()", "smartscapeEdges",
			}}},
		},
		{
			name:  "timeseries",
			query: "timeseries avg(dt.host.cpu.usage)",
			want:  []ScopeNeed{{Scope: "storage:metrics:read", Because: []string{"timeseries"}}},
		},
		{
			name:  "subquery in lookup",
			query: "fetch logs | lookup [fetch bizevents | fields id], sourceField:id, lookupField:id",
			want: []ScopeNeed{
				{Scope: "storage:bizevents:read", Because: []string{"fetch bizevents"}},
				{Scope: "storage:logs:read", Because: []string{"fetch logs"}},
			},
		},
		{
			name:  "leading whitespace and newlines",
			query: "\n  fetch events\n  | limit 1",
			want:  []ScopeNeed{{Scope: "storage:events:read", Because: []string{"fetch events"}}},
		},

		// Everything below must yield nothing: a requirement the analysis is not
		// certain of would turn a working query into a blocked one.
		{name: "empty", query: ""},
		{name: "unknown data object", query: "fetch dt.system.data_objects | fields name"},
		{name: "unknown custom data object", query: "fetch my.custom.table"},
		{name: "keyword inside a string literal", query: `fetch dt.system.events | filter content == "fetch logs | getNodeName(x)"`},
		{name: "keyword inside an escaped string literal", query: `fetch dt.system.events | filter content == "say \"fetch logs\" twice"`},
		{name: "keyword inside a triple-quoted string", query: "fetch dt.system.events | filter content == \"\"\"\nfetch logs\n\"\"\""},
		{name: "keyword inside a line comment", query: "// fetch logs\nfetch dt.system.events"},
		{name: "keyword inside a block comment", query: "/* smartscapeNodes \"HOST\" */ fetch dt.system.events"},
		{name: "single quote inside a double-quoted string is plain text", query: `fetch dt.system.events | filter content == "it's"`},
		{name: "keyword inside a backtick identifier", query: "fetch dt.system.events | fieldsAdd `getNodeName(x)` = 1"},
		{name: "fetch not at command position", query: "fetch dt.system.events | filter fetch logs"},
		{name: "makeTimeseries is not timeseries", query: "fetch dt.system.events | makeTimeseries count()"},
		{name: "field named like a function without a call", query: "fetch dt.system.events | fields getNodeName"},
		{name: "smartscape field reference needs no smartscape scope", query: "fetch dt.system.events | fields dt.smartscape.service"},
		{name: "case matters in DQL", query: "FETCH logs"},
		{name: "unterminated string abstains", query: `fetch logs | filter content == "oops`},
		{name: "unterminated block comment abstains", query: "fetch logs /* oops"},
		// Grail rejects single-quoted strings (PARSE_ERROR_SINGLE_QUOTES), and
		// dtctl turns that into a quoting hint. The precheck must not preempt the
		// hint with a scope error read out of the quoted text.
		{name: "pipe and fetch inside a single-quoted string", query: "fetch dt.system.events | filter content == '| fetch logs'"},
		{name: "smartscape function inside a single-quoted string", query: "fetch dt.system.events | filter content == 'getNodeName(x)'"},
		{name: "escaped quote inside a single-quoted string", query: `fetch dt.system.events | filter content == 'it\'s | fetch logs'`},
		{name: "single quote anywhere outside a literal abstains", query: "fetch logs | filter content == 'ERROR'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RequiredStorageScopes(tt.query))
		})
	}
}

func TestStorageScopeForDataObject(t *testing.T) {
	for obj, want := range map[string]string{
		"logs":            "storage:logs:read",
		"spans":           "storage:spans:read",
		"events":          "storage:events:read",
		"bizevents":       "storage:bizevents:read",
		"security.events": "storage:security.events:read",
		"user.events":     "storage:user.events:read",
		"user.sessions":   "storage:user.sessions:read",
		"dt.entity.host":  "storage:entities:read",
	} {
		got, ok := StorageScopeForDataObject(obj)
		require.True(t, ok, obj)
		require.Equal(t, want, got, obj)
	}
	for _, obj := range []string{"", "dt.system.events", "dt.entity.", "logs2", "my.table"} {
		_, ok := StorageScopeForDataObject(obj)
		require.False(t, ok, obj)
	}
}

func TestStorageScopesInText(t *testing.T) {
	require.Equal(t,
		[]string{"storage:logs:read", "storage:smartscape:read"},
		StorageScopesInText("missing storage:smartscape:read and storage:logs:read (storage:logs:read)"))
	require.Empty(t, StorageScopesInText("NOT_AUTHORIZED_FOR_TABLE"))
}
