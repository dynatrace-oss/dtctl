package cmd

import (
	"reflect"
	"testing"
)

func TestCreateDocumentLabelFlag(t *testing.T) {
	if createDocumentCmd.Flags().Lookup("label") == nil {
		t.Fatal("create document is missing the --label flag")
	}
}

func TestExtractDocumentLabels_Cmd(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]interface{}
		want []string
	}{
		{"absent", map[string]interface{}{}, nil},
		{"strings", map[string]interface{}{"labels": []interface{}{"a", "b"}}, []string{"a", "b"}},
		{"skips blanks and non-strings", map[string]interface{}{"labels": []interface{}{"a", "", 3, "b"}}, []string{"a", "b"}},
		{"wrong type", map[string]interface{}{"labels": "a,b"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDocumentLabels(tt.doc); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractDocumentLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}
