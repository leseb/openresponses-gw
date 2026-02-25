// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"strings"
)

// Filter is a marker interface for ComparisonFilter and CompoundFilter.
type Filter interface {
	isFilter()
}

// ComparisonFilter represents a single comparison operation on file attributes.
type ComparisonFilter struct {
	Type  string      `json:"type"`  // eq, ne, gt, gte, lt, lte
	Key   string      `json:"key"`
	Value interface{} `json:"value"` // string, number, or bool
}

func (ComparisonFilter) isFilter() {}

// CompoundFilter combines multiple filters with a logical operator.
type CompoundFilter struct {
	Type    string   `json:"type"`    // and, or
	Filters []Filter `json:"filters"` // ComparisonFilter or CompoundFilter
}

func (CompoundFilter) isFilter() {}

// ParseFilter parses a generic map (from JSON) into a typed Filter.
func ParseFilter(raw interface{}) (Filter, error) {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("filter must be an object, got %T", raw)
	}

	typ, _ := m["type"].(string)
	switch typ {
	case "eq", "ne", "gt", "gte", "lt", "lte":
		key, _ := m["key"].(string)
		if key == "" {
			return nil, fmt.Errorf("comparison filter requires 'key'")
		}
		value, ok := m["value"]
		if !ok {
			return nil, fmt.Errorf("comparison filter requires 'value'")
		}
		return ComparisonFilter{Type: typ, Key: key, Value: value}, nil

	case "and", "or":
		rawFilters, ok := m["filters"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("compound filter requires 'filters' array")
		}
		var filters []Filter
		for i, rf := range rawFilters {
			f, err := ParseFilter(rf)
			if err != nil {
				return nil, fmt.Errorf("filters[%d]: %w", i, err)
			}
			filters = append(filters, f)
		}
		return CompoundFilter{Type: typ, Filters: filters}, nil

	default:
		return nil, fmt.Errorf("unknown filter type: %q", typ)
	}
}

// EvaluateFilter evaluates a filter against file attributes.
// Returns true if the attributes match the filter.
func EvaluateFilter(filter Filter, attributes map[string]interface{}) bool {
	switch f := filter.(type) {
	case ComparisonFilter:
		return evaluateComparison(f, attributes)
	case CompoundFilter:
		return evaluateCompound(f, attributes)
	default:
		return false
	}
}

func evaluateComparison(f ComparisonFilter, attrs map[string]interface{}) bool {
	attrVal, exists := attrs[f.Key]
	if !exists {
		return false
	}

	cmp := compareValues(attrVal, f.Value)

	switch f.Type {
	case "eq":
		return cmp == 0
	case "ne":
		return cmp != 0
	case "gt":
		return cmp > 0
	case "gte":
		return cmp >= 0
	case "lt":
		return cmp < 0
	case "lte":
		return cmp <= 0
	default:
		return false
	}
}

// compareValues compares two values, returning -1, 0, or 1.
// Returns 0 for incomparable types (treated as equal for simplicity).
func compareValues(a, b interface{}) int {
	// Try numeric comparison first
	aNum, aOK := toFloat64(a)
	bNum, bOK := toFloat64(b)
	if aOK && bOK {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// String comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}

func evaluateCompound(f CompoundFilter, attrs map[string]interface{}) bool {
	switch f.Type {
	case "and":
		for _, sub := range f.Filters {
			if !EvaluateFilter(sub, attrs) {
				return false
			}
		}
		return true
	case "or":
		for _, sub := range f.Filters {
			if EvaluateFilter(sub, attrs) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// BuildMilvusExpr builds a Milvus filter expression to restrict search to specific file IDs.
// Returns an empty string if fileIDs is empty (no filtering).
func BuildMilvusExpr(fileIDs []string) string {
	if len(fileIDs) == 0 {
		return ""
	}
	quoted := make([]string, len(fileIDs))
	for i, id := range fileIDs {
		quoted[i] = `"` + strings.ReplaceAll(id, `"`, `\"`) + `"`
	}
	return "file_id in [" + strings.Join(quoted, ", ") + "]"
}
