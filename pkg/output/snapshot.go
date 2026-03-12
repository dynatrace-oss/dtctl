package output

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/dynatrace-oss/dtctl/pkg/proto/livedebugger"
)

// SnapshotPrinter prints snapshot query records with decoded snapshot.data payload.
type SnapshotPrinter struct {
	writer io.Writer
}

const maxSnapshotStringMapIndex = 100_000

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
			indexToString = nil
		} else {
			indexToString = parsedStrings
		}
	}

	decoded, err := decodeSnapshotDataToGeneric(data, indexToString)
	if err != nil {
		out["snapshot.decode_error"] = err.Error()
		return out
	}

	out["parsed_snapshot"] = decoded
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
		if maxIndex > maxSnapshotStringMapIndex {
			return nil, fmt.Errorf("string map index too large: %d", maxIndex)
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
		if maxIndex > maxSnapshotStringMapIndex {
			return nil, fmt.Errorf("string map index too large: %d", maxIndex)
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

	rawAugReport := new(livedebugger.AugReportMessage)
	err = proto.Unmarshal(decodedData, rawAugReport)
	if err != nil {
		return nil, fmt.Errorf("failed to decode snapshot.data protobuf: %w", err)
	}

	if rawAugReport.GetArguments2() == nil {
		return nil, fmt.Errorf("got aug report without arguments2")
	}

	if len(rawAugReport.GetStringsCache()) == 0 && len(stringCache) == 0 {
		return nil, fmt.Errorf("got aug report without strings cache")
	}

	if len(stringCache) > 0 {
		rawAugReport.StringsCache = toStringCacheEntries(stringCache)
	}

	caches := newVariant2CachesFromAugReport(rawAugReport)
	decoded := variant2ToDict(rawAugReport.GetArguments2(), caches, rawAugReport.GetReverseListOrder())
	return normalizeSnapshotFieldNames(decoded), nil
}

func normalizeSnapshotFieldNames(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			switch key {
			case "@CT", "@OS":
				continue
			case "@OT":
				out["type"] = normalizeSnapshotFieldNames(item)
			case "@value":
				out["value"] = normalizeSnapshotFieldNames(item)
			default:
				out[key] = normalizeSnapshotFieldNames(item)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i := range typed {
			out[i] = normalizeSnapshotFieldNames(typed[i])
		}
		return out
	default:
		return value
	}
}

type variant2Caches struct {
	stringCaches map[int]string
	buffersCache map[int][]byte
}

func newVariant2CachesFromAugReport(augReport *livedebugger.AugReportMessage) *variant2Caches {
	stringCaches := make(map[int]string)
	buffersCache := make(map[int][]byte)

	for _, entry := range augReport.GetStringsCache() {
		stringCaches[int(entry.GetValue())] = entry.GetKey()
	}

	for idx, value := range augReport.GetBufferCacheIndexes() {
		if idx < len(augReport.GetBufferCacheBuffers()) {
			buffersCache[int(value)] = augReport.GetBufferCacheBuffers()[idx]
		}
	}

	return &variant2Caches{stringCaches: stringCaches, buffersCache: buffersCache}
}

func (c *variant2Caches) getStringFromCache(index int) string {
	if value, ok := c.stringCaches[index]; ok {
		return value
	}
	return ""
}

func (c *variant2Caches) getBufferFromCache(index int) []byte {
	if value, ok := c.buffersCache[index]; ok {
		return value
	}
	return nil
}

func toStringCacheEntries(stringCache []string) []*livedebugger.StringCacheEntry {
	entries := make([]*livedebugger.StringCacheEntry, 0, len(stringCache))
	for idx, key := range stringCache {
		entries = append(entries, &livedebugger.StringCacheEntry{Key: key, Value: uint32(idx)})
	}
	return entries
}

func variant2ToDict(v *livedebugger.Variant2, caches *variant2Caches, reverseLists bool) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{"@CT": nullType, "@value": nil}
	}

	dict := map[string]interface{}{
		"@OT": caches.getStringFromCache(int(v.GetOriginalTypeIndexInCache())),
	}
	if (v.GetVariantTypeMaxDepth() & 1) == 1 {
		dict["@max_depth"] = true
	}

	variantType := livedebugger.Variant_Type(v.GetVariantTypeMaxDepth() >> 1)
	originalType := strings.ToLower(caches.getStringFromCache(int(v.GetOriginalTypeIndexInCache())))

	switch variantType {
	case livedebugger.Variant_VARIANT_NONE, livedebugger.Variant_VARIANT_UNDEFINED:
		dict["@CT"] = nullType
		dict["@value"] = nil
	case livedebugger.Variant_VARIANT_INT, livedebugger.Variant_VARIANT_LONG:
		if strings.HasPrefix(originalType, "bool") {
			dict["@CT"] = boolType
			dict["@value"] = v.GetLongValue() != 0
		} else {
			dict["@CT"] = intType
			dict["@value"] = int64ToSafeJSNumber(v.GetLongValue())
		}
	case livedebugger.Variant_VARIANT_DOUBLE:
		dict["@CT"] = floatType
		doubleValue := v.GetDoubleValue()
		switch {
		case math.IsNaN(doubleValue):
			dict["@value"] = "NaN"
		case math.IsInf(doubleValue, 1):
			dict["@value"] = "+Inf"
		case math.IsInf(doubleValue, -1):
			dict["@value"] = "-Inf"
		default:
			dict["@value"] = doubleValue
		}
	case livedebugger.Variant_VARIANT_COMPLEX:
		dict["@CT"] = complexType
		complexValue := v.GetComplexValue()
		if complexValue == nil {
			dict["@value"] = map[string]interface{}{"real": 0.0, "imaginary": 0.0}
			break
		}
		realValue := complexValue.GetReal()
		imaginaryValue := complexValue.GetImaginary()
		dict["@value"] = map[string]interface{}{
			"real":      realValue,
			"imaginary": imaginaryValue,
		}
	case livedebugger.Variant_VARIANT_STRING, livedebugger.Variant_VARIANT_MASKED:
		dict["@CT"] = stringType
		dict["@value"] = caches.getStringFromCache(int(v.GetBytesIndexInCache()))
		dict["@OS"] = v.GetOriginalSize()
	case livedebugger.Variant_VARIANT_LARGE_INT:
		dict["@CT"] = intType
		dict["@value"] = caches.getStringFromCache(int(v.GetBytesIndexInCache()))
	case livedebugger.Variant_VARIANT_BINARY:
		dict["@CT"] = binaryType
		dict["@value"] = caches.getBufferFromCache(int(v.GetBytesIndexInCache()))
		dict["@OS"] = v.GetOriginalSize()
	case livedebugger.Variant_VARIANT_TIME:
		dict["@CT"] = datetimeType
		dict["@value"] = formatRookoutTimestamp(v.GetTimeValue())
	case livedebugger.Variant_VARIANT_ENUM:
		dict["@CT"] = enumType
		dict["@value"] = map[string]interface{}{
			"@ordinal_value": int32(v.GetLongValue()),
			"@type_name":     caches.getStringFromCache(int(v.GetOriginalTypeIndexInCache())),
			"@value":         caches.getStringFromCache(int(v.GetBytesIndexInCache())),
		}
		addAttributesToDict(dict, v, caches, reverseLists)
	case livedebugger.Variant_VARIANT_LIST, livedebugger.Variant_VARIANT_SET:
		if variantType == livedebugger.Variant_VARIANT_SET || originalType == "set" {
			dict["@CT"] = setType
		} else {
			dict["@CT"] = listType
		}
		listValues := make([]interface{}, len(v.GetCollectionValues()))
		for i, value := range v.GetCollectionValues() {
			listValues[i] = variant2ToDict(value, caches, reverseLists)
		}
		if reverseLists {
			reverseInterfaces(listValues)
		}
		dict["@value"] = listValues
		dict["@OS"] = v.GetOriginalSize()
		addAttributesToDict(dict, v, caches, reverseLists)
	case livedebugger.Variant_VARIANT_MAP:
		dict["@CT"] = dictType
		mapEntries := make([][]interface{}, len(v.GetCollectionKeys()))
		for i, key := range v.GetCollectionKeys() {
			mapEntries[i] = []interface{}{
				variant2ToDict(key, caches, reverseLists),
				variant2ToDict(v.GetCollectionValues()[i], caches, reverseLists),
			}
		}
		dict["@OS"] = v.GetOriginalSize()
		dict["@value"] = mapEntries
		addAttributesToDict(dict, v, caches, reverseLists)
	case livedebugger.Variant_VARIANT_OBJECT:
		dict["@CT"] = userObjectType
		addAttributesToDict(dict, v, caches, reverseLists)
	case livedebugger.Variant_VARIANT_NAMESPACE:
		namespaceDict := make(map[string]interface{})
		for i, attrNameIndex := range v.GetAttributeNamesInCache() {
			if i < len(v.GetAttributeValues()) {
				attrName := caches.getStringFromCache(int(attrNameIndex))
				namespaceDict[attrName] = variant2ToDict(v.GetAttributeValues()[i], caches, reverseLists)
			}
		}
		return namespaceDict
	case livedebugger.Variant_VARIANT_UKNOWN_OBJECT:
		dict["@CT"] = unknownObjectType
		addAttributesToDict(dict, v, caches, reverseLists)
	case livedebugger.Variant_VARIANT_ERROR:
		errorValue := v.GetErrorValue()
		if errorValue == nil {
			dict["@OT"] = "Error"
			dict["@CT"] = stringType
			dict["@value"] = "<Error>"
			break
		}
		return map[string]interface{}{
			"@CT": namespaceType,
			"@value": map[string]interface{}{
				"message":    errorValue.GetMessage(),
				"parameters": variant2ToDict(errorValue.GetParameters(), caches, reverseLists),
				"exc":        variant2ToDict(errorValue.GetExc(), caches, reverseLists),
			},
		}
	case livedebugger.Variant_VARIANT_TRACEBACK:
		dict["@CT"] = dictType
		stackTrace := make([]map[string]map[string]interface{}, len(v.GetCodeValues()))
		for i, codeValue := range v.GetCodeValues() {
			idx := i
			if reverseLists {
				idx = len(v.GetCodeValues()) - i - 1
			}
			stackTrace[idx] = map[string]map[string]interface{}{
				"filename": {"@value": caches.getStringFromCache(int(codeValue.GetFilenameIndexInCache()))},
				"module":   {"@value": caches.getStringFromCache(int(codeValue.GetModuleIndexInCache()))},
				"line":     {"@value": codeValue.GetLineno()},
				"function": {"@value": caches.getStringFromCache(int(codeValue.GetNameIndexInCache()))},
			}
		}
		dict["@value"] = stackTrace
		addAttributesToDict(dict, v, caches, reverseLists)
	case livedebugger.Variant_VARIANT_DYNAMIC:
		return map[string]interface{}{"@CT": dynamicType}
	case livedebugger.Variant_VARIANT_FORMATTED_MESSAGE:
		return map[string]interface{}{"@CT": stringType, "@value": caches.getStringFromCache(int(v.GetBytesIndexInCache()))}
	case livedebugger.Variant_VARIANT_MAX_DEPTH:
		return map[string]interface{}{"@CT": namespaceType}
	case livedebugger.Variant_VARIANT_LIVETAIL:
		return map[string]interface{}{"@CT": nullType, "@value": nil}
	default:
		dict["@CT"] = nullType
		dict["@value"] = nil
	}

	return dict
}

func addAttributesToDict(dict map[string]interface{}, v *livedebugger.Variant2, caches *variant2Caches, reverseLists bool) {
	if len(v.GetAttributeNamesInCache()) == 0 {
		return
	}

	attrs := make(map[string]interface{})
	for i, attrNameIndex := range v.GetAttributeNamesInCache() {
		if i < len(v.GetAttributeValues()) {
			attrName := caches.getStringFromCache(int(attrNameIndex))
			attrs[attrName] = variant2ToDict(v.GetAttributeValues()[i], caches, reverseLists)
		}
	}
	dict["@attributes"] = attrs
}

func reverseInterfaces(values []interface{}) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func formatRookoutTimestamp(ts *livedebugger.Timestamp) string {
	if ts == nil {
		return ""
	}
	return time.Unix(ts.GetSeconds(), int64(ts.GetNanos())).UTC().Format("2006-01-02T15:04:05.000000Z")
}

func int64ToSafeJSNumber(num int64) interface{} {
	const jsMaxSafe = int64(1 << 53)
	if num < jsMaxSafe && num > -1*jsMaxSafe {
		return num
	}
	return fmt.Sprintf("%d", num)
}

const (
	stringType        = 1
	intType           = 2
	floatType         = 3
	nullType          = 5
	namespaceType     = 6
	boolType          = 7
	binaryType        = 8
	datetimeType      = 9
	setType           = 10
	listType          = 11
	dictType          = 12
	userObjectType    = 13
	unknownObjectType = 14
	enumType          = 15
	dynamicType       = 16
	complexType       = 17
)
