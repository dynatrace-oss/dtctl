package output

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestParseSnapshotStringMap_ArrayFormat(t *testing.T) {
	raw := `[{"":0},{"metadata":1},{"rule_id":2}]`
	result, err := parseSnapshotStringMap(raw)
	if err != nil {
		t.Fatalf("parseSnapshotStringMap() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}
	if result[0] != "" || result[1] != "metadata" || result[2] != "rule_id" {
		t.Fatalf("unexpected parsed cache: %#v", result)
	}
}

func TestParseSnapshotStringMap_MapFormat(t *testing.T) {
	raw := `{"":0,"metadata":1,"rule_id":2}`
	result, err := parseSnapshotStringMap(raw)
	if err != nil {
		t.Fatalf("parseSnapshotStringMap() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}
	if result[1] != "metadata" || result[2] != "rule_id" {
		t.Fatalf("unexpected parsed cache: %#v", result)
	}
}

func TestSnapshotPrinter_EnrichesRecord(t *testing.T) {
	payload := make([]byte, 0)
	payload = protowire.AppendTag(payload, 1, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 1) // resolves to "metadata"
	payload = protowire.AppendTag(payload, 2, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 123)

	encoded := base64.StdEncoding.EncodeToString(payload)

	obj := map[string]interface{}{
		"records": []map[string]interface{}{
			{
				"snapshot.data":       encoded,
				"snapshot.string_map": `[{"":0},{"metadata":1}]`,
				"snapshot.id":         "abc",
			},
		},
	}

	var out bytes.Buffer
	printer := &SnapshotPrinter{writer: &out}
	if err := printer.Print(obj); err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	records, ok := got["records"].([]interface{})
	if !ok || len(records) != 1 {
		t.Fatalf("records missing or invalid: %#v", got["records"])
	}
	record, ok := records[0].(map[string]interface{})
	if !ok {
		t.Fatalf("record invalid: %#v", records[0])
	}

	parsedSnapshot, ok := record["parsed_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("parsed_snapshot missing: %#v", record)
	}
	if parsedSnapshot["viewName"] == "" {
		t.Fatalf("parsed_snapshot.viewName missing: %#v", parsedSnapshot)
	}

	if _, ok := parsedSnapshot["view"]; !ok {
		t.Fatalf("parsed_snapshot.view missing: %#v", parsedSnapshot)
	}

	if _, exists := record["snapshot.parsed"]; exists {
		t.Fatalf("snapshot.parsed should not be present: %#v", record)
	}
	if _, exists := record["snapshot.message_namespace"]; exists {
		t.Fatalf("snapshot.message_namespace should not be present: %#v", record)
	}
	if _, exists := record["snapshot.namespace_view"]; exists {
		t.Fatalf("snapshot.namespace_view should not be present: %#v", record)
	}
	if _, exists := record["snapshot.message_view"]; exists {
		t.Fatalf("snapshot.message_view should not be present: %#v", record)
	}
}

func TestInferLocalDetails(t *testing.T) {
	cache := []string{
		"locals",
		"accountId", "java.lang.String", "acc-123",
		"debugOn", "java.lang.Boolean", "true",
		"connectTimeout", "java.lang.Integer", "5000",
		"traceback",
	}

	locals := extractLocalNames(cache)
	details := inferLocalDetails(locals, cache)

	if details["accountId"]["originalType"] != "java.lang.String" {
		t.Fatalf("accountId originalType = %v", details["accountId"]["originalType"])
	}
	if details["accountId"]["value"] != "acc-123" {
		t.Fatalf("accountId value = %v", details["accountId"]["value"])
	}

	if details["debugOn"]["originalType"] != "java.lang.Boolean" {
		t.Fatalf("debugOn originalType = %v", details["debugOn"]["originalType"])
	}
	if details["debugOn"]["value"] != "true" {
		t.Fatalf("debugOn value = %v", details["debugOn"]["value"])
	}

	if details["connectTimeout"]["originalType"] != "java.lang.Integer" {
		t.Fatalf("connectTimeout originalType = %v", details["connectTimeout"]["originalType"])
	}
}

func TestExtractLocalNames_UsesLocalsSectionOnly(t *testing.T) {
	cache := []string{
		"metadata",
		"variables",
		"firstElement",
		"java.lang.Long",
		"threading",
		"locals",
		"count",
		"lastElement",
		"step",
		"threading",
	}

	names := extractLocalNames(cache)
	has := map[string]bool{}
	for _, n := range names {
		has[n] = true
	}

	if has["firstElement"] {
		t.Fatalf("did not expect firstElement from variables section, got %#v", names)
	}
	if !has["count"] {
		t.Fatalf("expected count in extracted names, got %#v", names)
	}
	if !has["lastElement"] {
		t.Fatalf("expected lastElement in extracted names, got %#v", names)
	}
}

func TestNormalizeLocalsHierarchy_NestsLinkedObjectOnly(t *testing.T) {
	locals := map[string]interface{}{
		"this": map[string]interface{}{
			"@common_type":  "object",
			"@original_type": "MyClass",
			"@value":        "dbHelper",
		},
		"dbHelper": map[string]interface{}{
			"@common_type":  "object",
			"@original_type": "DatabaseHelper",
		},
		"CC_MANUFACTURE_DETAILS_QUERY": map[string]interface{}{"@value": "SELECT 1"},
		"COUNT_ORDER_BY_ACCOUNT_ID_QUERY": map[string]interface{}{"@value": "SELECT COUNT(*)"},
		"count": map[string]interface{}{"@value": 27},
	}

	normalized := normalizeLocalsHierarchy(locals)

	thisObj, ok := normalized["this"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected normalized this object, got %#v", normalized["this"])
	}
	dbHelperObj, ok := thisObj["dbHelper"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected this.dbHelper object, got %#v", thisObj["dbHelper"])
	}
	if dbHelperObj["@common_type"] == "" {
		t.Fatalf("expected this.dbHelper @common_type to exist, got %#v", dbHelperObj["@common_type"])
	}
	if dbHelperObj["@original_type"] == "" {
		t.Fatalf("expected this.dbHelper @original_type to exist, got %#v", dbHelperObj["@original_type"])
	}
	if _, ok := normalized["CC_MANUFACTURE_DETAILS_QUERY"]; !ok {
		t.Fatalf("expected unrelated local to remain top-level, got %#v", normalized)
	}
	if _, ok := normalized["COUNT_ORDER_BY_ACCOUNT_ID_QUERY"]; !ok {
		t.Fatalf("expected unrelated local to remain top-level, got %#v", normalized)
	}
	if _, ok := normalized["count"]; !ok {
		t.Fatalf("expected non-linked local to remain top-level, got %#v", normalized)
	}
}
