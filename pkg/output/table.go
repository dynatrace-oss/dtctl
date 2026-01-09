package output

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

// TablePrinter prints output as a table
type TablePrinter struct {
	writer io.Writer
	wide   bool
}

// tableFieldInfo holds metadata about a field for table display
type tableFieldInfo struct {
	name     string
	index    int
	wideOnly bool
}

// getTableFields extracts field information from struct tags
// Returns fields that should be displayed based on the "table" tag
// Tag format: `table:"HEADER"` or `table:"HEADER,wide"` or `table:"-"` (skip)
func getTableFields(t reflect.Type, wide bool) []tableFieldInfo {
	var fields []tableFieldInfo
	hasTableTags := false

	// First pass: check if any field has a table tag
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		if tag := field.Tag.Get("table"); tag != "" {
			hasTableTags = true
			break
		}
	}

	// Second pass: collect fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("table")

		// If no table tags exist, fall back to showing all fields
		if !hasTableTags {
			fields = append(fields, tableFieldInfo{
				name:  field.Name,
				index: i,
			})
			continue
		}

		// Skip fields marked with "-"
		if tag == "-" {
			continue
		}

		// Skip fields without table tag
		if tag == "" {
			continue
		}

		// Parse tag: "HEADER" or "HEADER,wide"
		parts := strings.Split(tag, ",")
		header := parts[0]
		wideOnly := len(parts) > 1 && parts[1] == "wide"

		// Skip wide-only fields if not in wide mode
		if wideOnly && !wide {
			continue
		}

		fields = append(fields, tableFieldInfo{
			name:     header,
			index:    i,
			wideOnly: wideOnly,
		})
	}

	return fields
}

// configureKubectlStyle configures the tablewriter to match kubectl's output style
func configureKubectlStyle(table *tablewriter.Table) {
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("   ") // Three spaces between columns like kubectl
	table.SetNoWhiteSpace(true)
}

// Print prints a single object as a table
func (p *TablePrinter) Print(obj interface{}) error {
	table := tablewriter.NewWriter(p.writer)
	configureKubectlStyle(table)

	// Use reflection to get field names and values
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		// For non-struct types, just print the value
		fmt.Fprintln(p.writer, obj)
		return nil
	}

	t := v.Type()
	fields := getTableFields(t, p.wide)

	// Create header and data rows
	var headers []string
	var values []string

	for _, f := range fields {
		headers = append(headers, f.name)
		value := v.Field(f.index)
		values = append(values, formatValue(value))
	}

	table.SetHeader(headers)
	table.Append(values)
	table.Render()

	return nil
}

// PrintList prints a list of objects as a table
func (p *TablePrinter) PrintList(obj interface{}) error {
	table := tablewriter.NewWriter(p.writer)
	configureKubectlStyle(table)

	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %s", v.Kind())
	}

	if v.Len() == 0 {
		fmt.Fprintln(p.writer, "No resources found.")
		return nil
	}

	// Get headers from first element
	first := v.Index(0)
	if first.Kind() == reflect.Ptr {
		first = first.Elem()
	}

	if first.Kind() != reflect.Struct {
		// For non-struct elements, print a simple list
		for i := 0; i < v.Len(); i++ {
			fmt.Fprintln(p.writer, v.Index(i).Interface())
		}
		return nil
	}

	t := first.Type()
	fields := getTableFields(t, p.wide)

	var headers []string
	for _, f := range fields {
		headers = append(headers, f.name)
	}

	table.SetHeader(headers)

	// Add rows
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		var row []string
		for _, f := range fields {
			value := elem.Field(f.index)
			row = append(row, formatValue(value))
		}
		table.Append(row)
	}

	table.Render()
	return nil
}

// formatValue formats a reflect.Value for table display
func formatValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	// Handle pointer types
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	// Handle time.Time specially
	if v.Type() == reflect.TypeOf(time.Time{}) {
		t := v.Interface().(time.Time)
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02 15:04:05")
	}

	// Format based on type
	switch v.Kind() {
	case reflect.Map, reflect.Slice:
		if v.IsNil() || v.Len() == 0 {
			return ""
		}
		return fmt.Sprintf("<%d items>", v.Len())
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}
