package output

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// SnapshotPrinter prints snapshot query records with decoded snapshot.data payload.
type SnapshotPrinter struct {
	writer io.Writer
}

// Print prints an object as JSON with snapshot records enriched by decoded payload fields.
func (p *SnapshotPrinter) Print(obj interface{}) error {
	transformed := transformSnapshotObject(obj)
	encoder := json.NewEncoder(p.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(transformed)
}

// PrintList prints a list object as snapshot output.
func (p *SnapshotPrinter) PrintList(obj interface{}) error {
	return p.Print(obj)
}

func transformSnapshotObject(obj interface{}) interface{} {
	root, ok := obj.(map[string]interface{})
	if !ok {
		return obj
	}

	records, ok := root["records"].([]map[string]interface{})
	if ok {
		transformed := make([]map[string]interface{}, 0, len(records))
		for _, rec := range records {
			transformed = append(transformed, enrichSnapshotRecord(rec))
		}
		out := make(map[string]interface{}, len(root))
		for k, v := range root {
			out[k] = v
		}
		out["records"] = transformed
		return out
	}

	recordsIfc, ok := root["records"].([]interface{})
	if !ok {
		return obj
	}

	transformedIfc := make([]interface{}, 0, len(recordsIfc))
	for _, recIfc := range recordsIfc {
		rec, ok := recIfc.(map[string]interface{})
		if !ok {
			transformedIfc = append(transformedIfc, recIfc)
			continue
		}
		transformedIfc = append(transformedIfc, enrichSnapshotRecord(rec))
	}

	out := make(map[string]interface{}, len(root))
	for k, v := range root {
		out[k] = v
	}
	out["records"] = transformedIfc
	return out
}

func enrichSnapshotRecord(record map[string]interface{}) map[string]interface{} {
	data, okData := record["snapshot.data"].(string)
	if !okData || data == "" {
		return record
	}

	out := make(map[string]interface{}, len(record)+1)
	for k, v := range record {
		out[k] = v
	}

	var indexToString []string
	stringMapRaw, hasStringMap := record["snapshot.string_map"].(string)
	if hasStringMap && stringMapRaw != "" {
		parsedStrings, err := parseSnapshotStringMap(stringMapRaw)
		if err != nil {
			out["snapshot.decode_error"] = fmt.Sprintf("failed to parse snapshot.string_map: %v", err)
			return out
		}
		indexToString = parsedStrings
	}

	decoded, err := decodeSnapshotDataToGeneric(data, indexToString)
	if err != nil {
		out["snapshot.decode_error"] = err.Error()
		return out
	}

	namespace := buildSerializerCompatibleNamespace(record, decoded, indexToString)
	out["parsed_snapshot"] = buildNamespaceViewEquivalent(namespace)
	return out
}

func parseSnapshotStringMap(raw string) ([]string, error) {
	var asArray []map[string]uint32
	if err := json.Unmarshal([]byte(raw), &asArray); err == nil {
		maxIndex := 0
		for _, item := range asArray {
			for _, idx := range item {
				if int(idx) > maxIndex {
					maxIndex = int(idx)
				}
				break
			}
		}

		result := make([]string, maxIndex+1)
		for _, item := range asArray {
			for value, idx := range item {
				result[idx] = value
				break
			}
		}
		return result, nil
	}

	var asMap map[string]uint32
	if err := json.Unmarshal([]byte(raw), &asMap); err == nil {
		maxIndex := 0
		for _, idx := range asMap {
			if int(idx) > maxIndex {
				maxIndex = int(idx)
			}
		}
		result := make([]string, maxIndex+1)
		for value, idx := range asMap {
			result[idx] = value
		}
		return result, nil
	}

	return nil, fmt.Errorf("invalid JSON format for snapshot.string_map")
}

func decodeSnapshotDataToGeneric(data string, stringCache []string) (interface{}, error) {
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode snapshot.data base64: %w", err)
	}

	fields, err := decodeProtobufMessageWithDepth(decodedData, stringCache, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to decode snapshot.data protobuf: %w", err)
	}

	referenced := collectReferencedStrings(fields)
	if len(referenced) > 80 {
		referenced = referenced[:80]
	}

	return map[string]interface{}{
		"fields":            fields,
		"referenced_strings": referenced,
		"string_cache_size": len(stringCache),
	}, nil
}

func decodeProtobufMessageWithDepth(data []byte, stringCache []string, depth int) ([]interface{}, error) {
	if len(data) == 0 {
		return []interface{}{}, nil
	}
	if depth > 3 {
		return []interface{}{}, nil
	}

	fields := make([]interface{}, 0)
	decodedCount := 0
	for len(data) > 0 {
		if decodedCount >= 200 {
			fields = append(fields, map[string]interface{}{
				"truncated": true,
				"reason":    "too many fields",
			})
			break
		}

		fieldNum, wireType, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, fmt.Errorf("invalid protobuf tag")
		}
		data = data[n:]

		field := map[string]interface{}{
			"field": fieldNum,
			"wire":  wireTypeName(wireType),
		}

		switch wireType {
		case protowire.VarintType:
			value, m := protowire.ConsumeVarint(data)
			if m < 0 {
				return nil, fmt.Errorf("invalid varint value")
			}
			data = data[m:]
			field["value"] = resolveStringIndex(value, stringCache)
		case protowire.Fixed32Type:
			value, m := protowire.ConsumeFixed32(data)
			if m < 0 {
				return nil, fmt.Errorf("invalid fixed32 value")
			}
			data = data[m:]
			field["value"] = value
		case protowire.Fixed64Type:
			value, m := protowire.ConsumeFixed64(data)
			if m < 0 {
				return nil, fmt.Errorf("invalid fixed64 value")
			}
			data = data[m:]
			field["value"] = value
		case protowire.BytesType:
			value, m := protowire.ConsumeBytes(data)
			if m < 0 {
				return nil, fmt.Errorf("invalid bytes value")
			}
			data = data[m:]

			if nested, ok := tryDecodeNestedMessage(value, stringCache, depth); ok {
				field["value"] = map[string]interface{}{"message": nested}
			} else if utf8.Valid(value) {
				asStr := string(value)
				if len(asStr) > 160 {
					asStr = asStr[:160] + "..."
				}
				field["value"] = asStr
			} else {
				field["value"] = map[string]interface{}{
					"bytes_len": len(value),
					"base64":    base64.StdEncoding.EncodeToString(value[:min(len(value), 40)]),
				}
			}
		default:
			return nil, fmt.Errorf("unsupported wire type: %v", wireType)
		}

		fields = append(fields, field)
		decodedCount++
	}

	return fields, nil
}

func tryDecodeNestedMessage(data []byte, stringCache []string, depth int) ([]interface{}, bool) {
	if len(data) == 0 {
		return nil, false
	}
	if depth >= 2 {
		return nil, false
	}

	fields, err := decodeProtobufMessageWithDepth(data, stringCache, depth+1)
	if err != nil || len(fields) == 0 {
		return nil, false
	}
	return fields, true
}

func resolveStringIndex(value uint64, stringCache []string) interface{} {
	if int(value) < len(stringCache) && stringCache[value] != "" {
		return map[string]interface{}{
			"raw":   value,
			"string": stringCache[value],
		}
	}
	return value
}

func wireTypeName(t protowire.Type) string {
	switch t {
	case protowire.VarintType:
		return "varint"
	case protowire.Fixed32Type:
		return "fixed32"
	case protowire.Fixed64Type:
		return "fixed64"
	case protowire.BytesType:
		return "bytes"
	case protowire.StartGroupType:
		return "start_group"
	case protowire.EndGroupType:
		return "end_group"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func collectReferencedStrings(fields []interface{}) []string {
	seen := make(map[string]struct{})
	var out []string

	var walkValue func(v interface{})
	walkValue = func(v interface{}) {
		switch typed := v.(type) {
		case map[string]interface{}:
			if s, ok := typed["string"].(string); ok && s != "" {
				if _, exists := seen[s]; !exists {
					seen[s] = struct{}{}
					out = append(out, s)
				}
			}
			for _, vv := range typed {
				walkValue(vv)
			}
		case []interface{}:
			for _, vv := range typed {
				walkValue(vv)
			}
		}
	}

	walkValue(fields)
	return out
}

func buildMessageView(record map[string]interface{}, decoded interface{}, stringCache []string) map[string]interface{} {
	snapshotMsg := getString(record, "snapshot.message")
	traceID := getString(record, "trace.id")
	spanID := getString(record, "span.id")
	threadName := getString(record, "thread.name")
	codeFile := getString(record, "code.filepath")
	codeFunc := getString(record, "code.function")
	line := getString(record, "code.line.number")

	return map[string]interface{}{
		"id":   getString(record, "snapshot.id"),
		"time": getString(record, "timestamp"),
		"ruleInfo": map[string]interface{}{
			"id": getString(record, "breakpoint.id"),
		},
		"message": map[string]interface{}{
			"title": snapshotMsg,
			"rookout": map[string]interface{}{
				"frame": map[string]interface{}{
					"filename": codeFile,
					"function": codeFunc,
					"line":     line,
					"locals": map[string]interface{}{
						"names": extractLocalNames(stringCache),
					},
				},
				"tracing": map[string]interface{}{
					"traceId": traceID,
					"spanId":  spanID,
				},
				"threading": map[string]interface{}{
					"thread_name": threadName,
					"thread_id":   getString(record, "thread.id"),
				},
			},
			"traceback": extractTraceback(stringCache),
			"decoded":   decoded,
		},
	}
}

func buildSerializerCompatibleNamespace(record map[string]interface{}, decoded interface{}, stringCache []string) map[string]interface{} {
	localsNames := extractLocalNames(stringCache)
	localDetails := inferLocalDetails(localsNames, stringCache)
	locals := make(map[string]interface{}, len(localsNames))
	for _, name := range localsNames {
		details := localDetails[name]
		originalType, _ := details["originalType"].(string)
		if originalType == "" {
			originalType = "snapshot.inferred"
		}
		value := details["value"]
		if value == nil || value == "" {
			value = "<value unavailable: requires typed snapshot protobuf schema>"
		}

		locals[name] = map[string]interface{}{
			"@common_type":  inferCommonType(originalType, value),
			"@original_type": originalType,
			"@value":        value,
		}
	}

	variables := map[string]interface{}{
		"snapshot.id": map[string]interface{}{
			"@common_type":  "string",
			"@original_type": "java.lang.String",
			"@value":        getString(record, "snapshot.id"),
		},
		"snapshot.message": map[string]interface{}{
			"@common_type":  "string",
			"@original_type": "java.lang.String",
			"@value":        getString(record, "snapshot.message"),
		},
		"trace.id": map[string]interface{}{
			"@common_type":  "string",
			"@original_type": "java.lang.String",
			"@value":        getString(record, "trace.id"),
		},
		"span.id": map[string]interface{}{
			"@common_type":  "string",
			"@original_type": "java.lang.String",
			"@value":        getString(record, "span.id"),
		},
		"thread.name": map[string]interface{}{
			"@common_type":  "string",
			"@original_type": "java.lang.String",
			"@value":        getString(record, "thread.name"),
		},
		"breakpoint.id": map[string]interface{}{
			"@common_type":  "string",
			"@original_type": "java.lang.String",
			"@value":        getString(record, "breakpoint.id"),
		},
	}

	namespace := map[string]interface{}{
		"snapshot": map[string]interface{}{
			"id":      getString(record, "snapshot.id"),
			"message": getString(record, "snapshot.message"),
			"decoded": decoded,
		},
		"trace": map[string]interface{}{
			"id": getString(record, "trace.id"),
		},
		"span": map[string]interface{}{
			"id": getString(record, "span.id"),
		},
		"thread": map[string]interface{}{
			"name": getString(record, "thread.name"),
			"id":   getString(record, "thread.id"),
		},
		"rookout": map[string]interface{}{
			"frame": map[string]interface{}{
				"filename": getString(record, "code.filepath"),
				"function": getString(record, "code.function"),
				"line":     getString(record, "code.line.number"),
				"locals":   locals,
			},
			"variables": variables,
			"tracing": map[string]interface{}{
				"traceId": getString(record, "trace.id"),
				"spanId":  getString(record, "span.id"),
			},
		},
	}

	return namespace
}

func buildNamespaceViewEquivalent(namespace map[string]interface{}) map[string]interface{} {
	rookoutNs, _ := namespace["rookout"].(map[string]interface{})
	frame, _ := rookoutNs["frame"].(map[string]interface{})
	locals, _ := frame["locals"].(map[string]interface{})
	locals = normalizeLocalsHierarchy(locals)
	vars, _ := rookoutNs["variables"].(map[string]interface{})

	otherNs := make(map[string]interface{})
	for k, v := range namespace {
		if k != "rookout" {
			otherNs[k] = v
		}
	}

	variablesNs := make(map[string]interface{})
	for k, v := range otherNs {
		variablesNs[k] = v
	}
	for k, v := range vars {
		variablesNs[k] = v
	}

	viewName := ""
	var view interface{}
	switch {
	case len(locals) > 0 && len(variablesNs) > 0:
		viewName = "LocalsAndVariables"
		view = map[string]interface{}{"locals": locals, "variables": variablesNs}
	case len(locals) > 0:
		viewName = "Locals"
		view = locals
	case len(variablesNs) > 0:
		viewName = "Message"
		view = variablesNs
	default:
		viewName = "Message"
		view = map[string]interface{}{}
	}

	return map[string]interface{}{
		"viewName": viewName,
		"view":     view,
	}
}

func normalizeLocalsHierarchy(locals map[string]interface{}) map[string]interface{} {
	if len(locals) == 0 {
		return locals
	}

	normalized := make(map[string]interface{}, len(locals))
	for k, v := range locals {
		normalized[k] = v
	}

	thisObj := map[string]interface{}{}
	if existingThis, ok := normalized["this"]; ok {
		if existingMap, ok := existingThis.(map[string]interface{}); ok {
			for k, v := range existingMap {
				thisObj[k] = v
			}
		} else {
			thisObj["@value"] = existingThis
		}
	}

	linkedObjName, _ := thisObj["@value"].(string)
	if linkedObjName == "" || linkedObjName == "this" {
		return normalized
	}

	rawLinkedObj, exists := normalized[linkedObjName]
	if !exists {
		return normalized
	}

	linkedObj := map[string]interface{}{}
	if linkedMap, ok := rawLinkedObj.(map[string]interface{}); ok {
		for k, v := range linkedMap {
			linkedObj[k] = v
		}
	} else {
		linkedObj["@value"] = rawLinkedObj
		linkedObj["@common_type"] = "dynamic"
		linkedObj["@original_type"] = "snapshot.inferred"
	}

	if _, ok := linkedObj["@common_type"]; !ok {
		linkedObj["@common_type"] = "object"
	}
	if _, ok := linkedObj["@original_type"]; !ok {
		linkedObj["@original_type"] = "snapshot.inferred.nested_object"
	}
	if _, ok := linkedObj["@value"]; !ok {
		linkedObj["@value"] = "<nested object>"
	}

	thisObj[linkedObjName] = linkedObj
	normalized["this"] = thisObj
	delete(normalized, linkedObjName)

	return normalized
}

func inferLocalDetails(localNames []string, stringCache []string) map[string]map[string]interface{} {
	positions := buildStringPositions(stringCache)
	result := make(map[string]map[string]interface{}, len(localNames))

	for _, name := range localNames {
		detail := map[string]interface{}{}
		indexes := positions[name]
		if len(indexes) == 0 {
			result[name] = detail
			continue
		}

		idx := indexes[0]
		detail["originalType"] = inferTypeAroundIndex(name, idx, stringCache)
		detail["value"] = inferValueAroundIndex(name, idx, stringCache)
		result[name] = detail
	}

	return result
}

func buildStringPositions(stringCache []string) map[string][]int {
	positions := make(map[string][]int, len(stringCache))
	for i, s := range stringCache {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		positions[s] = append(positions[s], i)
	}
	return positions
}

func inferTypeAroundIndex(name string, idx int, stringCache []string) string {
	if name == "this" {
		for i := idx + 1; i < min(idx+12, len(stringCache)); i++ {
			if isLikelyTypeToken(stringCache[i]) {
				return strings.TrimSpace(stringCache[i])
			}
		}
	}

	for i := idx + 1; i < min(idx+8, len(stringCache)); i++ {
		candidate := strings.TrimSpace(stringCache[i])
		if isLikelyTypeToken(candidate) {
			return candidate
		}
	}

	if strings.HasPrefix(name, "is") || strings.HasSuffix(name, "Enabled") || strings.HasSuffix(name, "On") {
		return "java.lang.Boolean"
	}
	if strings.HasSuffix(strings.ToLower(name), "id") {
		return "java.lang.String"
	}
	if strings.Contains(strings.ToLower(name), "timeout") || strings.Contains(strings.ToLower(name), "count") {
		return "java.lang.Integer"
	}

	return "snapshot.inferred"
}

func inferValueAroundIndex(name string, idx int, stringCache []string) interface{} {
	for i := idx + 1; i < min(idx+10, len(stringCache)); i++ {
		candidate := strings.TrimSpace(stringCache[i])
		if candidate == "" || candidate == name {
			continue
		}
		if isLikelyTypeToken(candidate) {
			continue
		}
		if isLikelyValueToken(candidate) {
			if candidate == "true" || candidate == "false" {
				return candidate
			}
			if n, err := strconv.ParseInt(candidate, 10, 64); err == nil {
				return n
			}
			if f, err := strconv.ParseFloat(candidate, 64); err == nil {
				return f
			}
			return candidate
		}
	}

	return "<inferred variable present; value omitted without typed protobuf schema>"
}

func isLikelyTypeToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "java.") || strings.HasPrefix(s, "jdk.") || strings.HasPrefix(s, "org.") || strings.HasPrefix(s, "com.") {
		return true
	}
	if strings.Contains(s, "$") && strings.Contains(s, ".") {
		return true
	}
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "<") {
		return true
	}
	return false
}

func isLikelyValueToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	if strings.Contains(s, ".") && !strings.Contains(s, "-") {
		return false
	}
	if strings.HasSuffix(s, ".java") {
		return false
	}
	if s == "true" || s == "false" || s == "null" {
		return true
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	if strings.ContainsAny(s, "-_") {
		return true
	}
	return len(s) <= 80
}

func inferCommonType(originalType string, value interface{}) string {
	ot := strings.ToLower(originalType)
	if strings.Contains(ot, "bool") {
		return "bool"
	}
	if strings.Contains(ot, "int") || strings.Contains(ot, "long") || strings.Contains(ot, "short") {
		return "int"
	}
	if strings.Contains(ot, "float") || strings.Contains(ot, "double") || strings.Contains(ot, "decimal") {
		return "float"
	}
	if strings.Contains(ot, "map") || strings.Contains(ot, "dict") {
		return "dict"
	}
	if strings.Contains(ot, "list") || strings.Contains(ot, "set") || strings.Contains(ot, "array") {
		return "list"
	}
	if strings.Contains(ot, "string") {
		return "string"
	}

	sv, ok := value.(string)
	if ok {
		if sv == "true" || sv == "false" {
			return "bool"
		}
		if _, err := strconv.ParseInt(sv, 10, 64); err == nil {
			return "int"
		}
		if _, err := strconv.ParseFloat(sv, 64); err == nil {
			return "float"
		}
		if strings.HasPrefix(sv, "<") {
			return "dynamic"
		}
		return "string"
	}

	switch value.(type) {
	case int, int32, int64, uint64:
		return "int"
	case float32, float64:
		return "float"
	case bool:
		return "bool"
	}

	return "dynamic"
}

func extractTraceback(stringsCache []string) []map[string]interface{} {
	frames := make([]map[string]interface{}, 0)
	for i, s := range stringsCache {
		if !strings.HasSuffix(s, ".java") {
			continue
		}
		frame := map[string]interface{}{"file": s}
		if i > 0 {
			prev := stringsCache[i-1]
			if strings.Contains(prev, ".") {
				frame["class"] = prev
			}
		}
		if i+1 < len(stringsCache) {
			next := stringsCache[i+1]
			if !strings.Contains(next, " ") && !strings.Contains(next, ".") {
				frame["function"] = next
			}
		}
		frames = append(frames, frame)
		if len(frames) >= 30 {
			break
		}
	}
	return frames
}

func extractLocalNames(stringsCache []string) []string {
	seen := make(map[string]struct{})
	names := make([]string, 0)
	appendUnique := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	start := -1
	for i, s := range stringsCache {
		if s == "locals" {
			start = i + 1
			break
		}
	}
	if start != -1 {
		stopWords := map[string]struct{}{
			"traceback": {}, "threading": {}, "tracing": {}, "frame": {}, "filename": {}, "line": {}, "module": {}, "function": {},
		}

		for i := start; i < len(stringsCache) && i < start+240; i++ {
			s := strings.TrimSpace(stringsCache[i])
			if s == "" {
				continue
			}
			if _, stop := stopWords[s]; stop {
				break
			}
			if strings.Contains(s, " ") || strings.Contains(s, "(") || strings.Contains(s, ")") || strings.Contains(s, "=") {
				continue
			}
			if strings.Contains(s, ".") && !strings.HasPrefix(s, "this") {
				continue
			}
			appendUnique(s)
			if len(names) >= 120 {
				break
			}
		}
	}

	if len(names) == 0 {
		return nil
	}
	return names
}

func getString(record map[string]interface{}, key string) string {
	v, ok := record[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
