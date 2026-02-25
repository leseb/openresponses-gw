// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"
)

func TestParseFilter_Comparison(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		wantErr bool
		wantKey string
	}{
		{
			name:    "eq filter",
			input:   map[string]interface{}{"type": "eq", "key": "category", "value": "docs"},
			wantKey: "category",
		},
		{
			name:    "ne filter",
			input:   map[string]interface{}{"type": "ne", "key": "status", "value": "draft"},
			wantKey: "status",
		},
		{
			name:    "gt filter",
			input:   map[string]interface{}{"type": "gt", "key": "size", "value": float64(100)},
			wantKey: "size",
		},
		{
			name:    "missing key",
			input:   map[string]interface{}{"type": "eq", "value": "x"},
			wantErr: true,
		},
		{
			name:    "missing value",
			input:   map[string]interface{}{"type": "eq", "key": "k"},
			wantErr: true,
		},
		{
			name:    "unknown type",
			input:   map[string]interface{}{"type": "unknown", "key": "k", "value": "v"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFilter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			cf, ok := f.(ComparisonFilter)
			if !ok {
				t.Fatalf("expected ComparisonFilter, got %T", f)
			}
			if cf.Key != tt.wantKey {
				t.Errorf("expected key %q, got %q", tt.wantKey, cf.Key)
			}
		})
	}
}

func TestParseFilter_Compound(t *testing.T) {
	input := map[string]interface{}{
		"type": "and",
		"filters": []interface{}{
			map[string]interface{}{"type": "eq", "key": "category", "value": "docs"},
			map[string]interface{}{"type": "gt", "key": "size", "value": float64(10)},
		},
	}

	f, err := ParseFilter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf, ok := f.(CompoundFilter)
	if !ok {
		t.Fatalf("expected CompoundFilter, got %T", f)
	}
	if cf.Type != "and" {
		t.Errorf("expected type=and, got %q", cf.Type)
	}
	if len(cf.Filters) != 2 {
		t.Fatalf("expected 2 sub-filters, got %d", len(cf.Filters))
	}
}

func TestParseFilter_NestedCompound(t *testing.T) {
	input := map[string]interface{}{
		"type": "or",
		"filters": []interface{}{
			map[string]interface{}{
				"type": "and",
				"filters": []interface{}{
					map[string]interface{}{"type": "eq", "key": "a", "value": "1"},
					map[string]interface{}{"type": "eq", "key": "b", "value": "2"},
				},
			},
			map[string]interface{}{"type": "eq", "key": "c", "value": "3"},
		},
	}

	f, err := ParseFilter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf := f.(CompoundFilter)
	if len(cf.Filters) != 2 {
		t.Fatalf("expected 2 sub-filters, got %d", len(cf.Filters))
	}
	nested, ok := cf.Filters[0].(CompoundFilter)
	if !ok {
		t.Fatal("expected first sub-filter to be CompoundFilter")
	}
	if nested.Type != "and" {
		t.Errorf("expected nested type=and, got %q", nested.Type)
	}
}

func TestEvaluateFilter_Comparison(t *testing.T) {
	attrs := map[string]interface{}{
		"category": "docs",
		"size":     float64(50),
		"active":   true,
	}

	tests := []struct {
		name   string
		filter ComparisonFilter
		want   bool
	}{
		{"eq match", ComparisonFilter{Type: "eq", Key: "category", Value: "docs"}, true},
		{"eq no match", ComparisonFilter{Type: "eq", Key: "category", Value: "images"}, false},
		{"ne match", ComparisonFilter{Type: "ne", Key: "category", Value: "images"}, true},
		{"ne no match", ComparisonFilter{Type: "ne", Key: "category", Value: "docs"}, false},
		{"gt match", ComparisonFilter{Type: "gt", Key: "size", Value: float64(30)}, true},
		{"gt no match", ComparisonFilter{Type: "gt", Key: "size", Value: float64(50)}, false},
		{"gte match equal", ComparisonFilter{Type: "gte", Key: "size", Value: float64(50)}, true},
		{"lt match", ComparisonFilter{Type: "lt", Key: "size", Value: float64(100)}, true},
		{"lt no match", ComparisonFilter{Type: "lt", Key: "size", Value: float64(50)}, false},
		{"lte match equal", ComparisonFilter{Type: "lte", Key: "size", Value: float64(50)}, true},
		{"missing key", ComparisonFilter{Type: "eq", Key: "missing", Value: "x"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateFilter(tt.filter, attrs)
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestEvaluateFilter_Compound(t *testing.T) {
	attrs := map[string]interface{}{
		"category": "docs",
		"size":     float64(50),
	}

	// AND: both true
	andFilter := CompoundFilter{
		Type: "and",
		Filters: []Filter{
			ComparisonFilter{Type: "eq", Key: "category", Value: "docs"},
			ComparisonFilter{Type: "gt", Key: "size", Value: float64(30)},
		},
	}
	if !EvaluateFilter(andFilter, attrs) {
		t.Error("AND filter should match")
	}

	// AND: one false
	andFilter2 := CompoundFilter{
		Type: "and",
		Filters: []Filter{
			ComparisonFilter{Type: "eq", Key: "category", Value: "docs"},
			ComparisonFilter{Type: "gt", Key: "size", Value: float64(100)},
		},
	}
	if EvaluateFilter(andFilter2, attrs) {
		t.Error("AND filter should not match")
	}

	// OR: one true
	orFilter := CompoundFilter{
		Type: "or",
		Filters: []Filter{
			ComparisonFilter{Type: "eq", Key: "category", Value: "images"},
			ComparisonFilter{Type: "gt", Key: "size", Value: float64(30)},
		},
	}
	if !EvaluateFilter(orFilter, attrs) {
		t.Error("OR filter should match")
	}

	// OR: none true
	orFilter2 := CompoundFilter{
		Type: "or",
		Filters: []Filter{
			ComparisonFilter{Type: "eq", Key: "category", Value: "images"},
			ComparisonFilter{Type: "gt", Key: "size", Value: float64(100)},
		},
	}
	if EvaluateFilter(orFilter2, attrs) {
		t.Error("OR filter should not match")
	}
}

func TestBuildMilvusExpr(t *testing.T) {
	tests := []struct {
		name    string
		fileIDs []string
		want    string
	}{
		{
			name:    "empty",
			fileIDs: nil,
			want:    "",
		},
		{
			name:    "single",
			fileIDs: []string{"file_1"},
			want:    `file_id in ["file_1"]`,
		},
		{
			name:    "multiple",
			fileIDs: []string{"file_1", "file_2", "file_3"},
			want:    `file_id in ["file_1", "file_2", "file_3"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMilvusExpr(tt.fileIDs)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
