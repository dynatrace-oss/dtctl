package output

import (
	"fmt"
	"io"
	"reflect"
	"sort"
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
	indices  []int // Field path for nested/embedded fields
	wideOnly bool
}

// getTableFields extracts field information from struct tags
// Returns fields that should be displayed based on the "table" tag
// Tag format: `table:"HEADER"` or `table:"HEADER,wide"` or `table:"-"` (skip)
func getTableFields(t reflect.Type, wide bool) []tableFieldInfo {
	var fields []tableFieldInfo
	hasTableTags := false

	// First pass: check if any field has a table tag (including embedded structs)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		if tag := field.Tag.Get("table"); tag != "" {
			hasTableTags = true
			break
		}
		// Check embedded structs for table tags
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			for j := 0; j < field.Type.NumField(); j++ {
				if embeddedField := field.Type.Field(j); embeddedField.IsExported() {
					if tag := embeddedField.Tag.Get("table"); tag != "" {
						hasTableTags = true
						break
					}
				}
			}
			if hasTableTags {
				break
			}
		}
	}

	// Second pass: collect fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("table")

		// Handle embedded structs - recursively process their fields
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			embeddedFields := getTableFields(field.Type, wide)
			// Prepend parent field index to create field path
			for _, ef := range embeddedFields {
				indices := append([]int{i}, ef.indices...)
				fields = append(fields, tableFieldInfo{
					name:     ef.name,
					indices:  indices,
					wideOnly: ef.wideOnly,
				})
			}
			continue
		}

		// If no table tags exist, fall back to showing all fields
		if !hasTableTags {
			fields = append(fields, tableFieldInfo{
				name:    field.Name,
				indices: []int{i},
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
			indices:  []int{i},
			wideOnly: wideOnly,
		})
	}

	return fields
}

// getFieldByPath traverses a field path to get the final field value
func getFieldByPath(v reflect.Value, indices []int) reflect.Value {
	for _, idx := range indices {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		v = v.Field(idx)
	}
	return v
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
		value := getFieldByPath(v, f.indices)
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

	// Handle slice of maps (e.g., from DQL results or lookup tables)
	if first.Kind() == reflect.Map || (first.Kind() == reflect.Interface && first.Elem().Kind() == reflect.Map) {
		return p.printMaps(v, table)
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
			value := getFieldByPath(elem, f.indices)
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

// printMaps prints a slice of maps as a table
func (p *TablePrinter) printMaps(v reflect.Value, table *tablewriter.Table) error {
	// Collect all unique keys from all maps to create headers
	keySet := make(map[string]bool)
	var rows []map[string]interface{}

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)

		// Handle interface{} wrapping a map
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}

		if elem.Kind() != reflect.Map {
			continue
		}

		row := make(map[string]interface{})
		iter := elem.MapRange()
		for iter.Next() {
			key := fmt.Sprintf("%v", iter.Key().Interface())
			keySet[key] = true
			row[key] = iter.Value().Interface()
		}
		rows = append(rows, row)
	}

	// Sort keys for consistent column order
	var keys []string
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Convert keys to uppercase for headers (kubectl style)
	var headers []string
	for _, k := range keys {
		headers = append(headers, strings.ToUpper(k))
	}
	table.SetHeader(headers)

	// Add rows
	for _, row := range rows {
		var values []string
		for _, key := range keys {
			val := row[key]
			values = append(values, formatTableMapValue(val))
		}
		table.Append(values)
	}

	table.Render()
	return nil
}

// formatTableMapValue formats a value from a map for table display
func formatTableMapValue(val interface{}) string {
	if val == nil {
		return ""
	}

	v := reflect.ValueOf(val)

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		return formatTableMapValue(v.Elem().Interface())
	}

	// Handle maps and slices
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() || v.Len() == 0 {
			return ""
		}
		return fmt.Sprintf("<%d items>", v.Len())
	case reflect.Slice:
		if v.IsNil() || v.Len() == 0 {
			return ""
		}
		// For slices, try to display items if they're simple types
		if v.Len() <= 3 {
			var items []string
			for i := 0; i < v.Len(); i++ {
				item := v.Index(i).Interface()
				items = append(items, fmt.Sprintf("%v", item))
			}
			return strings.Join(items, ", ")
		}
		return fmt.Sprintf("<%d items>", v.Len())
	default:
		return fmt.Sprintf("%v", val)
	}
}
