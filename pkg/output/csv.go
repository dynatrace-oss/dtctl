package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"reflect"
	"sort"
)

// CSVPrinter prints output as CSV
type CSVPrinter struct {
	writer io.Writer
}

// Print prints a single object as CSV
func (p *CSVPrinter) Print(obj interface{}) error {
	// For single objects, convert to slice and use PrintList
	return p.PrintList([]interface{}{obj})
}

// PrintList prints a list of objects as CSV
func (p *CSVPrinter) PrintList(obj interface{}) error {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %s", v.Kind())
	}

	if v.Len() == 0 {
		// Write empty CSV (just headers would be nice, but we don't know the structure)
		return nil
	}

	writer := csv.NewWriter(p.writer)
	defer writer.Flush()

	// Handle slice of maps (most common for DQL results)
	if v.Index(0).Kind() == reflect.Map || (v.Index(0).Kind() == reflect.Interface && v.Index(0).Elem().Kind() == reflect.Map) {
		return p.printMaps(v, writer)
	}

	// Handle slice of structs
	first := v.Index(0)
	if first.Kind() == reflect.Ptr {
		first = first.Elem()
	}

	if first.Kind() == reflect.Struct {
		return p.printStructs(v, writer)
	}

	// For other types, print as single column
	if err := writer.Write([]string{"Value"}); err != nil {
		return err
	}

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if err := writer.Write([]string{fmt.Sprintf("%v", elem.Interface())}); err != nil {
			return err
		}
	}

	return nil
}

// printMaps prints a slice of maps as CSV
func (p *CSVPrinter) printMaps(v reflect.Value, writer *csv.Writer) error {
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

	// Write header
	if err := writer.Write(keys); err != nil {
		return err
	}

	// Write rows
	for _, row := range rows {
		var values []string
		for _, key := range keys {
			val := row[key]
			values = append(values, formatCSVValue(val))
		}
		if err := writer.Write(values); err != nil {
			return err
		}
	}

	return nil
}

// printStructs prints a slice of structs as CSV
func (p *CSVPrinter) printStructs(v reflect.Value, writer *csv.Writer) error {
	first := v.Index(0)
	if first.Kind() == reflect.Ptr {
		first = first.Elem()
	}

	t := first.Type()
	fields := getTableFields(t, true) // Use wide mode to get all fields

	// Write header
	var headers []string
	for _, f := range fields {
		headers = append(headers, f.name)
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// Write rows
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		var row []string
		for _, f := range fields {
			value := getFieldByPath(elem, f.indices)
			row = append(row, formatCSVValue(value.Interface()))
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// formatCSVValue formats a value for CSV output
func formatCSVValue(val interface{}) string {
	if val == nil {
		return ""
	}

	v := reflect.ValueOf(val)

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		return formatCSVValue(v.Elem().Interface())
	}

	// Handle maps and slices as JSON-like strings
	switch v.Kind() {
	case reflect.Map, reflect.Slice:
		if v.IsNil() || v.Len() == 0 {
			return ""
		}
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
