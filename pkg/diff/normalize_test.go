package diff

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name           string
		data           interface{}
		ignoreMetadata bool
		ignoreOrder    bool
		want           interface{}
	}{
		{
			name: "no normalization",
			data: map[string]interface{}{
				"key": "value",
			},
			ignoreMetadata: false,
			ignoreOrder:    false,
			want: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "remove metadata fields",
			data: map[string]interface{}{
				"key": "value",
				"metadata": map[string]interface{}{
					"createdAt": "2024-01-01",
					"version":   1,
				},
			},
			ignoreMetadata: true,
			ignoreOrder:    false,
			want: map[string]interface{}{
				"key":      "value",
				"metadata": map[string]interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.data, tt.ignoreMetadata, tt.ignoreOrder)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemovePath(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
		path string
		want interface{}
	}{
		{
			name: "remove top-level field",
			data: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			path: "key2",
			want: map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			name: "remove nested field",
			data: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
					"keep":  "this",
				},
			},
			path: "outer.inner",
			want: map[string]interface{}{
				"outer": map[string]interface{}{
					"keep": "this",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removePath(tt.data, tt.path)
			if !reflect.DeepEqual(tt.data, tt.want) {
				t.Errorf("removePath() result = %v, want %v", tt.data, tt.want)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "simple path",
			path: "key",
			want: []string{"key"},
		},
		{
			name: "nested path",
			path: "outer.inner.deep",
			want: []string{"outer", "inner", "deep"},
		},
		{
			name: "empty path",
			path: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPath(tt.path)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasStableKey(t *testing.T) {
	tests := []struct {
		name string
		arr  []interface{}
		want bool
	}{
		{
			name: "empty array",
			arr:  []interface{}{},
			want: false,
		},
		{
			name: "array with id keys",
			arr: []interface{}{
				map[string]interface{}{"id": "1", "name": "first"},
				map[string]interface{}{"id": "2", "name": "second"},
			},
			want: true,
		},
		{
			name: "array with name keys",
			arr: []interface{}{
				map[string]interface{}{"name": "first", "value": "a"},
				map[string]interface{}{"name": "second", "value": "b"},
			},
			want: true,
		},
		{
			name: "array without stable keys",
			arr: []interface{}{
				map[string]interface{}{"value": "a"},
				map[string]interface{}{"value": "b"},
			},
			want: false,
		},
		{
			name: "array with non-map items",
			arr: []interface{}{
				"string1",
				"string2",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasStableKey(tt.arr)
			if got != tt.want {
				t.Errorf("hasStableKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{
			name: "string comparison",
			a:    "apple",
			b:    "banana",
			want: true,
		},
		{
			name: "number comparison",
			a:    1,
			b:    2,
			want: true,
		},
		{
			name: "float comparison",
			a:    1.5,
			b:    2.5,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareValues(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       interface{}
		wantVal float64
		wantOk  bool
	}{
		{
			name:    "float64",
			v:       float64(42.5),
			wantVal: 42.5,
			wantOk:  true,
		},
		{
			name:    "float32",
			v:       float32(10.5),
			wantVal: 10.5,
			wantOk:  true,
		},
		{
			name:    "int",
			v:       int(100),
			wantVal: 100.0,
			wantOk:  true,
		},
		{
			name:    "int64",
			v:       int64(200),
			wantVal: 200.0,
			wantOk:  true,
		},
		{
			name:    "int32",
			v:       int32(50),
			wantVal: 50.0,
			wantOk:  true,
		},
		{
			name:    "string - not convertible",
			v:       "not a number",
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "bool - not convertible",
			v:       true,
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "nil - not convertible",
			v:       nil,
			wantVal: 0,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotVal, gotOk := toFloat64(tt.v)
			if gotOk != tt.wantOk {
				t.Errorf("toFloat64() ok = %v, want %v", gotOk, tt.wantOk)
			}
			if gotOk && gotVal != tt.wantVal {
				t.Errorf("toFloat64() val = %v, want %v", gotVal, tt.wantVal)
			}
		})
	}
}

func TestDeepCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data interface{}
		want interface{}
	}{
		{
			name: "simple map",
			data: map[string]interface{}{
				"key": "value",
			},
			want: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "nested map",
			data: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
			want: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name: "array",
			data: []interface{}{"a", "b", "c"},
			want: []interface{}{"a", "b", "c"},
		},
		{
			name: "numbers",
			data: map[string]interface{}{
				"int":   float64(42),
				"float": 3.14,
			},
			want: map[string]interface{}{
				"int":   float64(42),
				"float": 3.14,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := deepCopy(tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deepCopy() = %v, want %v", got, tt.want)
			}
			// Ensure it's actually a copy, not the same reference
			if gotMap, ok := got.(map[string]interface{}); ok {
				if origMap, ok := tt.data.(map[string]interface{}); ok {
					// Modify the copy
					gotMap["test"] = "modified"
					// Original should be unchanged
					if _, exists := origMap["test"]; exists {
						t.Error("deepCopy() did not create independent copy")
					}
				}
			}
		})
	}
}

func TestRemovePath_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data interface{}
		path string
		want interface{}
	}{
		{
			name: "empty path",
			data: map[string]interface{}{
				"key": "value",
			},
			path: "",
			want: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "path with no matching key",
			data: map[string]interface{}{
				"key": "value",
			},
			path: "nonexistent",
			want: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "nested path with missing intermediate key",
			data: map[string]interface{}{
				"key": "value",
			},
			path: "missing.nested.path",
			want: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "data is not a map",
			data: "string value",
			path: "key",
			want: "string value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Make a copy to avoid modifying test data
			dataCopy := deepCopy(tt.data)
			removePath(dataCopy, tt.path)
			if !reflect.DeepEqual(dataCopy, tt.want) {
				t.Errorf("removePath() result = %v, want %v", dataCopy, tt.want)
			}
		})
	}
}

func TestRemovePathRecursive_NonMapData(t *testing.T) {
	t.Parallel()

	// Test with non-map data
	data := []interface{}{"a", "b", "c"}
	removePathRecursive(data, []string{"key"})
	// Should not panic and data should be unchanged
	want := []interface{}{"a", "b", "c"}
	if !reflect.DeepEqual(data, want) {
		t.Errorf("removePathRecursive() with non-map data modified it: %v", data)
	}
}

func TestSortByKey_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arr  []interface{}
		want []interface{}
	}{
		{
			name: "sort by id",
			arr: []interface{}{
				map[string]interface{}{"id": "b", "value": 2},
				map[string]interface{}{"id": "a", "value": 1},
			},
			want: []interface{}{
				map[string]interface{}{"id": "a", "value": 1},
				map[string]interface{}{"id": "b", "value": 2},
			},
		},
		{
			name: "sort by name when id missing",
			arr: []interface{}{
				map[string]interface{}{"name": "zebra"},
				map[string]interface{}{"name": "apple"},
			},
			want: []interface{}{
				map[string]interface{}{"name": "apple"},
				map[string]interface{}{"name": "zebra"},
			},
		},
		{
			name: "sort by numeric key",
			arr: []interface{}{
				map[string]interface{}{"key": 30},
				map[string]interface{}{"key": 10},
				map[string]interface{}{"key": 20},
			},
			want: []interface{}{
				map[string]interface{}{"key": 10},
				map[string]interface{}{"key": 20},
				map[string]interface{}{"key": 30},
			},
		},
		{
			name: "array with non-map elements",
			arr: []interface{}{
				"string",
				map[string]interface{}{"id": "a"},
			},
			want: []interface{}{
				"string",
				map[string]interface{}{"id": "a"},
			},
		},
		{
			name: "maps without stable keys",
			arr: []interface{}{
				map[string]interface{}{"value": "b"},
				map[string]interface{}{"value": "a"},
			},
			want: []interface{}{
				map[string]interface{}{"value": "b"},
				map[string]interface{}{"value": "a"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Make a copy
			arr := make([]interface{}, len(tt.arr))
			copy(arr, tt.arr)

			sortByKey(arr)
			if !reflect.DeepEqual(arr, tt.want) {
				t.Errorf("sortByKey() = %v, want %v", arr, tt.want)
			}
		})
	}
}

func TestCompareValues_MixedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    interface{}
		b    interface{}
	}{
		{
			name: "string vs number - falls back to reflect",
			a:    "text",
			b:    42,
		},
		{
			name: "map vs string - falls back to reflect",
			a:    map[string]interface{}{"key": "val"},
			b:    "text",
		},
		{
			name: "different types",
			a:    true,
			b:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Just ensure it doesn't panic
			_ = compareValues(tt.a, tt.b)
		})
	}
}
