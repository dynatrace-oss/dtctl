package exec

import (
	"strings"
	"testing"
)

func TestFilterValueCondition(t *testing.T) {
	cases := []struct{ query, field, value string }{
		{`fetch logs | filter service.name == "checkout"`, "service.name", "checkout"},
		{`fetch logs | filter contains(k8s.workload.name, "Cart", caseSensitive: true)`, "k8s.workload.name", "Cart"},
		{"fetch logs | filter `odd name` == \"x\"", "`odd name`", "x"},
		{`fetch logs | filter status >= 500 and matchesValue(dt.entity.name, "pay*")`, "dt.entity.name", "pay*"},
		{`fetch logs | sort timestamp desc | filter a == "1" or contains(b, "2")`, "a", "1"},
		{`fetch logs | filter x > 3`, "", ""},
		{`fetch logs | fieldsAdd y = "a" | filter y == "a"`, "", ""},
		{`timeseries avg(dt.host.cpu.usage), filter: host.name == "h"`, "", ""},
	}
	for _, tc := range cases {
		f, v := filterValueCondition(tc.query)
		if f != tc.field || v != tc.value {
			t.Errorf("filterValueCondition(%q) = %q, %q; want %q, %q", tc.query, f, v, tc.field, tc.value)
		}
	}
}

// valueProbe answers the field sample with logSample and the value listing
// with groups.
func valueProbe(groups ...map[string]interface{}) *fakeProbe {
	return &fakeProbe{respond: func(q string) (*DQLQueryResponse, error) {
		if strings.Contains(q, "summarize n = count(), by:{v = ") {
			return recordsResponse(groups...), nil
		}
		return logSample(), nil
	}}
}

func TestEmptyResultAdvice_ValueNearMatch(t *testing.T) {
	p := valueProbe(
		map[string]interface{}{"v": "frontend", "n": "900"},
		map[string]interface{}{"v": "CheckoutService", "n": "120"},
		map[string]interface{}{"v": nil, "n": "40"},
	)
	e := &DQLExecutor{probe: p.run}
	reason, sugg := e.emptyResultAdvice(`fetch logs, from:now()-1d | filter service.name == "checkout"`, recordsResponse(), nil, DQLExecuteOptions{})
	if len(p.calls) != 2 || !strings.HasPrefix(p.calls[1], `fetch logs, from:now()-1d | summarize n = count(), by:{v = service.name}`) {
		t.Fatalf("probes = %q", p.calls)
	}
	if reason == nil || reason.Code != "value_not_found" || reason.Field != "service.name" || len(reason.DidYouMean) != 1 || reason.DidYouMean[0] != "CheckoutService" {
		t.Fatalf("reason = %+v", reason)
	}
	if !hasSuggestion(sugg, `but it has "CheckoutService"`) || hasSuggestion(sugg, "DEFAULT query window") {
		t.Errorf("suggestions = %q", sugg)
	}
}

func TestEmptyResultAdvice_ValueListingWithoutNearMatch(t *testing.T) {
	p := valueProbe(
		map[string]interface{}{"v": "frontend", "n": int64(900)},
		map[string]interface{}{"v": nil, "n": int64(40)},
	)
	e := &DQLExecutor{probe: p.run}
	reason, sugg := e.emptyResultAdvice(`fetch logs | filter service.name == "billing"`, recordsResponse(), nil, DQLExecuteOptions{})
	if reason != nil {
		t.Fatalf("no near match is no structured finding, got %+v", reason)
	}
	if !hasSuggestion(sugg, `its most frequent values are "frontend" (900), null (40)`) || !hasSuggestion(sugg, "DEFAULT query window") {
		t.Errorf("suggestions = %q", sugg)
	}
}

func TestEmptyResultAdvice_ValueFieldAlwaysNull(t *testing.T) {
	p := valueProbe(map[string]interface{}{"v": nil, "n": "100"})
	e := &DQLExecutor{probe: p.run}
	_, sugg := e.emptyResultAdvice(`fetch logs | filter loglevel == "ERROR"`, recordsResponse(), nil, DQLExecuteOptions{})
	if !hasSuggestion(sugg, "is empty (null) on every `logs` record") {
		t.Errorf("suggestions = %q", sugg)
	}
}

func TestEmptyResultAdvice_NoValueProbeAfterFieldFinding(t *testing.T) {
	p := valueProbe()
	e := &DQLExecutor{probe: p.run}
	e.emptyResultAdvice(`fetch logs | filter servce.name == "x"`, recordsResponse(), nil, DQLExecuteOptions{})
	if len(p.calls) != 1 {
		t.Errorf("a field finding must not be followed by a value probe; probes = %q", p.calls)
	}
}

func TestSampleAdvice(t *testing.T) {
	cases := []struct {
		query string
		rows  int
		want  bool
	}{
		{`fetch dt.synthetic.events, from:now()-24h | limit 20`, 20, true},
		{`fetch logs | filter loglevel == "ERROR" | fields content | limit 10`, 10, true},
		{`fetch logs | limit 20`, 7, false},
		{`fetch spans | sort duration desc | limit 5`, 5, false},
		{`fetch logs | summarize count(), by:{x} | limit 5`, 5, false},
		{`smartscapeNodes "SERVICE" | limit 10`, 10, false},
	}
	for _, tc := range cases {
		if got := len(sampleAdvice(tc.query, tc.rows)) > 0; got != tc.want {
			t.Errorf("sampleAdvice(%q, %d) = %v, want %v", tc.query, tc.rows, got, tc.want)
		}
	}
}
