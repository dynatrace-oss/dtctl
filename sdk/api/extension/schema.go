package extension

// FluffKeys are schema fields removed by StripSchemaFluff: they add human-readable context
// but bulk up the schema when only the structural definition is needed.
var FluffKeys = map[string]bool{
	"documentation": true,
	"customMessage": true,
	"displayName":   true,
}

// StripSchemaFluff recursively removes FluffKeys from a parsed JSON Schema object.
func StripSchemaFluff(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		for k := range val {
			if FluffKeys[k] {
				delete(val, k)
			} else {
				val[k] = StripSchemaFluff(val[k])
			}
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = StripSchemaFluff(item)
		}
		return val
	default:
		return v
	}
}
