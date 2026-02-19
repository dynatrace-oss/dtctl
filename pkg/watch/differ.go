package watch

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type Differ struct {
	previous map[string]interface{}
}

func NewDiffer() *Differ {
	return &Differ{
		previous: make(map[string]interface{}),
	}
}

func (d *Differ) Detect(current []interface{}) []Change {
	changes := []Change{}

	currentMap := make(map[string]interface{})
	for _, item := range current {
		id := extractID(item)
		if id != "" {
			currentMap[id] = item
		}
	}

	// Detect additions and modifications - only return actual changes
	for id, item := range currentMap {
		if prev, exists := d.previous[id]; !exists {
			changes = append(changes, Change{
				Type:     ChangeTypeAdded,
				Resource: item,
			})
		} else if !deepEqual(prev, item) {
			field, oldVal, newVal := detectChangedField(prev, item)
			changes = append(changes, Change{
				Type:     ChangeTypeModified,
				Resource: item,
				Field:    field,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	// Detect deletions
	for id, item := range d.previous {
		if _, exists := currentMap[id]; !exists {
			changes = append(changes, Change{
				Type:     ChangeTypeDeleted,
				Resource: item,
			})
		}
	}

	d.previous = currentMap
	return changes
}

func (d *Differ) Reset() {
	d.previous = make(map[string]interface{})
}

func extractID(item interface{}) string {
	if item == nil {
		return ""
	}

	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() == reflect.Map {
		for _, key := range v.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			if keyStr == "id" || keyStr == "ID" || keyStr == "Id" {
				idVal := v.MapIndex(key)
				if idVal.IsValid() {
					return fmt.Sprintf("%v", idVal.Interface())
				}
			}
		}
		for _, key := range v.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			if keyStr == "name" || keyStr == "Name" {
				nameVal := v.MapIndex(key)
				if nameVal.IsValid() {
					return fmt.Sprintf("%v", nameVal.Interface())
				}
			}
		}
		return ""
	}

	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			fieldName := field.Name
			if fieldName == "ID" || fieldName == "Id" {
				fieldVal := v.Field(i)
				if fieldVal.IsValid() && fieldVal.CanInterface() {
					return fmt.Sprintf("%v", fieldVal.Interface())
				}
			}
		}
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			fieldName := field.Name
			if fieldName == "Name" {
				fieldVal := v.Field(i)
				if fieldVal.IsValid() && fieldVal.CanInterface() {
					return fmt.Sprintf("%v", fieldVal.Interface())
				}
			}
		}
	}

	return ""
}

func deepEqual(a, b interface{}) bool {
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

func detectChangedField(prev, current interface{}) (string, interface{}, interface{}) {
	prevVal := reflect.ValueOf(prev)
	currVal := reflect.ValueOf(current)

	if prevVal.Kind() == reflect.Ptr {
		prevVal = prevVal.Elem()
	}
	if currVal.Kind() == reflect.Ptr {
		currVal = currVal.Elem()
	}

	if prevVal.Kind() == reflect.Map && currVal.Kind() == reflect.Map {
		for _, key := range currVal.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			prevFieldVal := prevVal.MapIndex(key)
			currFieldVal := currVal.MapIndex(key)

			if !prevFieldVal.IsValid() || !currFieldVal.IsValid() {
				continue
			}

			if !deepEqual(prevFieldVal.Interface(), currFieldVal.Interface()) {
				return keyStr, prevFieldVal.Interface(), currFieldVal.Interface()
			}
		}
	}

	if prevVal.Kind() == reflect.Struct && currVal.Kind() == reflect.Struct {
		for i := 0; i < currVal.NumField(); i++ {
			field := currVal.Type().Field(i)
			if !field.IsExported() {
				continue
			}

			prevFieldVal := prevVal.Field(i)
			currFieldVal := currVal.Field(i)

			if !prevFieldVal.CanInterface() || !currFieldVal.CanInterface() {
				continue
			}

			if !deepEqual(prevFieldVal.Interface(), currFieldVal.Interface()) {
				return field.Name, prevFieldVal.Interface(), currFieldVal.Interface()
			}
		}
	}

	return "", nil, nil
}
