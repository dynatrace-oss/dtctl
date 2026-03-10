package output

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	rookout "github.com/dynatrace-oss/dtctl/pkg/proto/rookout"
	"google.golang.org/protobuf/proto"
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
	stringsCache := []map[string]uint32{
		{"": 0},
		{"root": 1},
		{"java.lang.String": 2},
		{"hello": 3},
	}

	rootValue := &rookout.Variant2{
		VariantTypeMaxDepth:      uint32(rookout.Variant_VARIANT_STRING) << 1,
		OriginalTypeIndexInCache: 2,
		BytesIndexInCache:        3,
		OriginalSize:             5,
	}
	rootNamespace := &rookout.Variant2{
		VariantTypeMaxDepth:   uint32(rookout.Variant_VARIANT_NAMESPACE) << 1,
		AttributeNamesInCache: []uint32{1},
		AttributeValues:       []*rookout.Variant2{rootValue},
	}
	aug := &rookout.AugReportMessage{Arguments2: rootNamespace}
	payload, err := proto.Marshal(aug)
	if err != nil {
		t.Fatalf("marshal aug report: %v", err)
	}

	encoded := toBase64(payload)
	stringMapRaw, err := json.Marshal(stringsCache)
	if err != nil {
		t.Fatalf("marshal string map: %v", err)
	}

	obj := map[string]interface{}{
		"records": []map[string]interface{}{
			{
				"snapshot.data":       encoded,
				"snapshot.string_map": string(stringMapRaw),
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
	root, ok := parsedSnapshot["root"].(map[string]interface{})
	if !ok {
		t.Fatalf("parsed_snapshot.root missing: %#v", parsedSnapshot)
	}
	if root["@value"] != "hello" {
		t.Fatalf("parsed_snapshot.root.@value = %#v, want hello", root["@value"])
	}
	if root["@CT"] == nil {
		t.Fatalf("parsed_snapshot.root.@CT missing: %#v", root)
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

func TestSnapshotPrinter_HandlesVariant2EdgeCases(t *testing.T) {
	stringMap := []map[string]uint32{
		{"": 0},
		{"msg": 1},
		{"formatted": 2},
		{"9223372036854775807123": 3},
		{"java.util.Set": 4},
		{"item-a": 5},
		{"item-b": 6},
	}

	formatted := &rookout.Variant2{
		VariantTypeMaxDepth:      uint32(rookout.Variant_VARIANT_FORMATTED_MESSAGE) << 1,
		BytesIndexInCache:        2,
		OriginalTypeIndexInCache: 1,
	}
	largeInt := &rookout.Variant2{
		VariantTypeMaxDepth:      uint32(rookout.Variant_VARIANT_LARGE_INT) << 1,
		BytesIndexInCache:        3,
		OriginalTypeIndexInCache: 1,
	}
	setList := &rookout.Variant2{
		VariantTypeMaxDepth:      uint32(rookout.Variant_VARIANT_SET) << 1,
		OriginalTypeIndexInCache: 4,
		CollectionValues: []*rookout.Variant2{
			{VariantTypeMaxDepth: uint32(rookout.Variant_VARIANT_STRING) << 1, BytesIndexInCache: 5, OriginalTypeIndexInCache: 1},
			{VariantTypeMaxDepth: uint32(rookout.Variant_VARIANT_STRING) << 1, BytesIndexInCache: 6, OriginalTypeIndexInCache: 1},
		},
	}
	errorVariant := &rookout.Variant2{
		VariantTypeMaxDepth: uint32(rookout.Variant_VARIANT_ERROR) << 1,
		ErrorValue: &rookout.Error2{
			Message:    "boom",
			Parameters: formatted,
			Exc:        largeInt,
		},
	}
	timeVariant := &rookout.Variant2{
		VariantTypeMaxDepth:      uint32(rookout.Variant_VARIANT_TIME) << 1,
		OriginalTypeIndexInCache: 1,
		TimeValue:                &rookout.Timestamp{Seconds: 1700000000, Nanos: 123000000},
	}

	root := &rookout.Variant2{
		VariantTypeMaxDepth:   uint32(rookout.Variant_VARIANT_NAMESPACE) << 1,
		AttributeNamesInCache: []uint32{1, 2, 3, 4, 5},
		AttributeValues:       []*rookout.Variant2{formatted, largeInt, setList, errorVariant, timeVariant},
	}

	aug := &rookout.AugReportMessage{Arguments2: root, ReverseListOrder: true}
	payload, err := proto.Marshal(aug)
	if err != nil {
		t.Fatalf("marshal aug report: %v", err)
	}

	stringMapRaw, err := json.Marshal(stringMap)
	if err != nil {
		t.Fatalf("marshal string map: %v", err)
	}

	obj := map[string]interface{}{
		"records": []map[string]interface{}{{
			"snapshot.data":       base64.StdEncoding.EncodeToString(payload),
			"snapshot.string_map": string(stringMapRaw),
		}},
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

	record := got["records"].([]interface{})[0].(map[string]interface{})
	parsed := record["parsed_snapshot"].(map[string]interface{})

	formattedOut := parsed["msg"].(map[string]interface{})
	if formattedOut["@value"] != "formatted" {
		t.Fatalf("formatted value mismatch: %#v", formattedOut)
	}

	largeIntOut := parsed["formatted"].(map[string]interface{})
	if largeIntOut["@value"] != "9223372036854775807123" {
		t.Fatalf("large int value mismatch: %#v", largeIntOut)
	}

	setOut := parsed["9223372036854775807123"].(map[string]interface{})
	if setOut["@CT"].(float64) != setType {
		t.Fatalf("expected set common type, got %#v", setOut)
	}
	setValues := setOut["@value"].([]interface{})
	first := setValues[0].(map[string]interface{})
	if first["@value"] != "item-b" {
		t.Fatalf("expected reverse list order to apply to set values, got %#v", setValues)
	}

	errorOut := parsed["java.util.Set"].(map[string]interface{})
	if errorOut["@CT"].(float64) != namespaceType {
		t.Fatalf("expected error namespace output, got %#v", errorOut)
	}
	errorValue := errorOut["@value"].(map[string]interface{})
	if errorValue["message"] != "boom" {
		t.Fatalf("error message mismatch: %#v", errorValue)
	}

	timeOut := parsed["item-a"].(map[string]interface{})
	if timeOut["@value"] != "2023-11-14T22:13:20.123000Z" {
		t.Fatalf("timestamp format mismatch: %#v", timeOut)
	}
}

func toBase64(b []byte) string {
	const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	if len(b) == 0 {
		return ""
	}

	result := make([]byte, 0, ((len(b)+2)/3)*4)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		remain := len(b) - i
		n |= uint32(b[i]) << 16
		if remain > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if remain > 2 {
			n |= uint32(b[i+2])
		}

		result = append(result,
			encodeStd[(n>>18)&63],
			encodeStd[(n>>12)&63],
		)
		if remain > 1 {
			result = append(result, encodeStd[(n>>6)&63])
		} else {
			result = append(result, '=')
		}
		if remain > 2 {
			result = append(result, encodeStd[n&63])
		} else {
			result = append(result, '=')
		}
	}

	return string(result)
}
