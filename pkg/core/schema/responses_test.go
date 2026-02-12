// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"testing"
)

func TestResponsesToolParam_UnmarshalJSON_FlatFileSearch(t *testing.T) {
	input := `{"type":"file_search","vector_store_ids":["vs_1","vs_2"],"max_num_results":5}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if tool.Type != "file_search" {
		t.Errorf("Type = %q, want file_search", tool.Type)
	}
	if len(tool.VectorStoreIDs) != 2 || tool.VectorStoreIDs[0] != "vs_1" {
		t.Errorf("VectorStoreIDs = %v, want [vs_1 vs_2]", tool.VectorStoreIDs)
	}
	if tool.MaxNumResults == nil || *tool.MaxNumResults != 5 {
		t.Errorf("MaxNumResults = %v, want 5", tool.MaxNumResults)
	}
}

func TestResponsesToolParam_UnmarshalJSON_NestedFileSearch(t *testing.T) {
	// This is the format the OpenAI Python SDK sends.
	input := `{"type":"file_search","file_search":{"vector_store_ids":["vs_abc"],"max_num_results":3}}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if tool.Type != "file_search" {
		t.Errorf("Type = %q, want file_search", tool.Type)
	}
	if len(tool.VectorStoreIDs) != 1 || tool.VectorStoreIDs[0] != "vs_abc" {
		t.Errorf("VectorStoreIDs = %v, want [vs_abc]", tool.VectorStoreIDs)
	}
	if tool.MaxNumResults == nil || *tool.MaxNumResults != 3 {
		t.Errorf("MaxNumResults = %v, want 3", tool.MaxNumResults)
	}
}

func TestResponsesToolParam_UnmarshalJSON_NestedFileSearchWithRankingAndFilters(t *testing.T) {
	input := `{
		"type": "file_search",
		"file_search": {
			"vector_store_ids": ["vs_1"],
			"ranking_options": {"ranker": "auto", "score_threshold": 0.5},
			"filters": {"type": "eq", "key": "source", "value": "paper"}
		}
	}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(tool.VectorStoreIDs) != 1 {
		t.Errorf("VectorStoreIDs = %v, want [vs_1]", tool.VectorStoreIDs)
	}
	if tool.RankingOptions == nil {
		t.Fatal("RankingOptions is nil, want non-nil")
	}
	if tool.RankingOptions["ranker"] != "auto" {
		t.Errorf("RankingOptions[ranker] = %v, want auto", tool.RankingOptions["ranker"])
	}
	if tool.Filters == nil {
		t.Fatal("Filters is nil, want non-nil")
	}
}

func TestResponsesToolParam_UnmarshalJSON_FlatWebSearch(t *testing.T) {
	input := `{"type":"web_search","search_context_size":"medium","user_location":{"city":"NYC"}}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if tool.Type != "web_search" {
		t.Errorf("Type = %q, want web_search", tool.Type)
	}
	if tool.SearchContextSize == nil || *tool.SearchContextSize != "medium" {
		t.Errorf("SearchContextSize = %v, want medium", tool.SearchContextSize)
	}
	if tool.UserLocation["city"] != "NYC" {
		t.Errorf("UserLocation[city] = %v, want NYC", tool.UserLocation["city"])
	}
}

func TestResponsesToolParam_UnmarshalJSON_NestedWebSearch(t *testing.T) {
	input := `{"type":"web_search","web_search":{"search_context_size":"high","user_location":{"city":"SF"}}}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if tool.SearchContextSize == nil || *tool.SearchContextSize != "high" {
		t.Errorf("SearchContextSize = %v, want high", tool.SearchContextSize)
	}
	if tool.UserLocation["city"] != "SF" {
		t.Errorf("UserLocation[city] = %v, want SF", tool.UserLocation["city"])
	}
}

func TestResponsesToolParam_UnmarshalJSON_FunctionTool(t *testing.T) {
	input := `{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object"}}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if tool.Type != "function" {
		t.Errorf("Type = %q, want function", tool.Type)
	}
	if tool.Name != "get_weather" {
		t.Errorf("Name = %q, want get_weather", tool.Name)
	}
}

func TestResponsesToolParam_UnmarshalJSON_FlatTakesPrecedence(t *testing.T) {
	// If both flat and nested are present, flat wins (already populated).
	input := `{"type":"file_search","vector_store_ids":["vs_flat"],"file_search":{"vector_store_ids":["vs_nested"]}}`

	var tool ResponsesToolParam
	if err := json.Unmarshal([]byte(input), &tool); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(tool.VectorStoreIDs) != 1 || tool.VectorStoreIDs[0] != "vs_flat" {
		t.Errorf("VectorStoreIDs = %v, want [vs_flat] (flat should take precedence)", tool.VectorStoreIDs)
	}
}
