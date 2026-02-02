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
