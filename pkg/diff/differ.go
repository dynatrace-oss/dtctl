package diff

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
)

type Differ struct {
	options DiffOptions
}

type DiffOptions struct {
	Format         DiffFormat
	IgnoreMetadata bool
	IgnoreOrder    bool
	ContextLines   int
	Colorize       bool
	Semantic       bool
}

type DiffFormat string

const (
	DiffFormatUnified    DiffFormat = "unified"
	DiffFormatSideBySide DiffFormat = "side-by-side"
	DiffFormatJSONPatch  DiffFormat = "json-patch"
	DiffFormatSemantic   DiffFormat = "semantic"
)

type DiffResult struct {
	HasChanges bool
	Changes    []Change
	Summary    DiffSummary
	Patch      string
	LeftLabel  string
	RightLabel string
}

type Change struct {
	Path      string
	Operation ChangeOperation
	OldValue  interface{}
	NewValue  interface{}
	Context   []string
}

type ChangeOperation string

const (
	ChangeOpAdd     ChangeOperation = "add"
	ChangeOpRemove  ChangeOperation = "remove"
	ChangeOpReplace ChangeOperation = "replace"
)

type DiffSummary struct {
	Added    int
	Removed  int
	Modified int
	Impact   ImpactLevel
}

type ImpactLevel string

const (
	ImpactLow      ImpactLevel = "low"
	ImpactMedium   ImpactLevel = "medium"
	ImpactHigh     ImpactLevel = "high"
	ImpactCritical ImpactLevel = "critical"
)

func NewDiffer(opts DiffOptions) *Differ {
	return &Differ{options: opts}
}

func (d *Differ) Compare(left, right interface{}, leftLabel, rightLabel string) (*DiffResult, error) {
	leftNorm := normalize(left, d.options.IgnoreMetadata, d.options.IgnoreOrder)
	rightNorm := normalize(right, d.options.IgnoreMetadata, d.options.IgnoreOrder)

	changes := computeDiff("", leftNorm, rightNorm)

	result := &DiffResult{
		HasChanges: len(changes) > 0,
		Changes:    changes,
		Summary:    computeSummary(changes),
		LeftLabel:  leftLabel,
		RightLabel: rightLabel,
	}

	formatter := d.getFormatter()
	patch, err := formatter.Format(result)
	if err != nil {
		return nil, fmt.Errorf("failed to format diff: %w", err)
	}
	result.Patch = patch

	return result, nil
}

func (d *Differ) CompareFiles(leftPath, rightPath string) (*DiffResult, error) {
	left, err := parseFile(leftPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse left file: %w", err)
	}

	right, err := parseFile(rightPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse right file: %w", err)
	}

	return d.Compare(left, right, leftPath, rightPath)
}

func (d *Differ) getFormatter() Formatter {
	if d.options.Semantic {
		return &SemanticFormatter{}
	}

	switch d.options.Format {
	case DiffFormatSideBySide:
		return &SideBySideFormatter{
			width:    120,
			colorize: d.options.Colorize,
		}
	case DiffFormatJSONPatch:
		return &JSONPatchFormatter{}
	case DiffFormatSemantic:
		return &SemanticFormatter{}
	default:
		return &UnifiedFormatter{
			contextLines: d.options.ContextLines,
			colorize:     d.options.Colorize,
		}
	}
}

func parseFile(path string) (interface{}, error) {
	var data []byte
	var err error

	if path == "-" {
		return nil, fmt.Errorf("stdin not yet implemented")
	}

	data, err = readFile(path)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := yaml.Unmarshal(data, &result); err == nil {
		return result, nil
	}

	if err := json.Unmarshal(data, &result); err == nil {
		return result, nil
	}

	return nil, fmt.Errorf("file is not valid YAML or JSON")
}

func computeDiff(path string, left, right interface{}) []Change {
	var changes []Change

	if reflect.DeepEqual(left, right) {
		return changes
	}

	leftMap, leftIsMap := left.(map[string]interface{})
	rightMap, rightIsMap := right.(map[string]interface{})

	if leftIsMap && rightIsMap {
		allKeys := make(map[string]bool)
		for k := range leftMap {
			allKeys[k] = true
		}
		for k := range rightMap {
			allKeys[k] = true
		}

		for k := range allKeys {
			newPath := k
			if path != "" {
				newPath = path + "." + k
			}

			leftVal, leftExists := leftMap[k]
			rightVal, rightExists := rightMap[k]

			if !leftExists {
				changes = append(changes, Change{
					Path:      newPath,
					Operation: ChangeOpAdd,
					NewValue:  rightVal,
				})
			} else if !rightExists {
				changes = append(changes, Change{
					Path:      newPath,
					Operation: ChangeOpRemove,
					OldValue:  leftVal,
				})
			} else {
				changes = append(changes, computeDiff(newPath, leftVal, rightVal)...)
			}
		}
		return changes
	}

	leftSlice, leftIsSlice := left.([]interface{})
	rightSlice, rightIsSlice := right.([]interface{})

	if leftIsSlice && rightIsSlice {
		maxLen := len(leftSlice)
		if len(rightSlice) > maxLen {
			maxLen = len(rightSlice)
		}

		for i := 0; i < maxLen; i++ {
			newPath := fmt.Sprintf("%s[%d]", path, i)

			if i >= len(leftSlice) {
				changes = append(changes, Change{
					Path:      newPath,
					Operation: ChangeOpAdd,
					NewValue:  rightSlice[i],
				})
			} else if i >= len(rightSlice) {
				changes = append(changes, Change{
					Path:      newPath,
					Operation: ChangeOpRemove,
					OldValue:  leftSlice[i],
				})
			} else {
				changes = append(changes, computeDiff(newPath, leftSlice[i], rightSlice[i])...)
			}
		}
		return changes
	}

	if !reflect.DeepEqual(left, right) {
		changes = append(changes, Change{
			Path:      path,
			Operation: ChangeOpReplace,
			OldValue:  left,
			NewValue:  right,
		})
	}

	return changes
}

func computeSummary(changes []Change) DiffSummary {
	summary := DiffSummary{}

	for _, change := range changes {
		switch change.Operation {
		case ChangeOpAdd:
			summary.Added++
		case ChangeOpRemove:
			summary.Removed++
		case ChangeOpReplace:
			summary.Modified++
		}
	}

	summary.Impact = calculateImpact(summary)
	return summary
}

func calculateImpact(summary DiffSummary) ImpactLevel {
	total := summary.Added + summary.Removed + summary.Modified

	if total == 0 {
		return ImpactLow
	}

	if summary.Removed > 5 || total > 20 {
		return ImpactHigh
	}

	if summary.Removed > 0 || total > 10 {
		return ImpactMedium
	}

	return ImpactLow
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
