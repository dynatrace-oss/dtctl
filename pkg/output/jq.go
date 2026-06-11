package output

import (
	"fmt"
	"reflect"

	"github.com/itchyny/gojq"
)

// JQPrinter applies a jq filter before delegating rendering to the wrapped printer.
// This allows --jq to work consistently across all output formats.
type JQPrinter struct {
	base   Printer
	filter string
}

// NewJQPrinter wraps base with jq filtering. If filter is empty, base is returned unchanged.
func NewJQPrinter(base Printer, filter string) Printer {
	if filter == "" {
		return base
	}
	return &JQPrinter{base: base, filter: filter}
}

// Print applies the jq filter and renders the result.
func (p *JQPrinter) Print(obj interface{}) error {
	filtered, err := applyJQ(p.filter, obj)
	if err != nil {
		return err
	}
	return printAuto(p.base, filtered)
}

// PrintList applies the jq filter and renders the result.
func (p *JQPrinter) PrintList(obj interface{}) error {
	filtered, err := applyJQ(p.filter, obj)
	if err != nil {
		return err
	}
	return printAuto(p.base, filtered)
}

func printAuto(p Printer, data interface{}) error {
	if isSliceLike(data) {
		return p.PrintList(data)
	}
	return p.Print(data)
}

func isSliceLike(v interface{}) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
}

func applyJQ(filter string, input interface{}) (interface{}, error) {
	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq filter: %w", err)
	}

	generic, err := toGeneric(input)
	if err != nil {
		return nil, fmt.Errorf("failed to apply --jq filter: %w", err)
	}

	iter := code.Run(generic)
	results := make([]interface{}, 0, 1)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if runErr, ok := v.(error); ok {
			return nil, fmt.Errorf("failed to apply --jq filter: %w", runErr)
		}
		results = append(results, v)
	}

	switch len(results) {
	case 0:
		return nil, nil
	case 1:
		return results[0], nil
	default:
		return results, nil
	}
}
