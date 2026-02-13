// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
)

// --- Test helpers ---

func stringPtr(s string) *string    { return &s }
func float64Ptr(f float64) *float64 { return &f }
func intPtr(i int) *int             { return &i }
func boolPtr(b bool) *bool          { return &b }

// dummyVectorSearcher implements VectorSearcher for testing.
type dummyVectorSearcher struct {
	results []vectorstore.SearchResult
	err     error
}

func (d *dummyVectorSearcher) Search(_ context.Context, _, _ string, _ int) ([]vectorstore.SearchResult, error) {
	return d.results, d.err
}

// --- extractInputMessages tests ---

func TestExtractInputMessages_StringInput(t *testing.T) {
	msgs := extractInputMessages("hello world")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected role=user, got %q", msgs[0].Role)
	}
	if msgs[0].Content != "hello world" {
		t.Errorf("expected content %q, got %q", "hello world", msgs[0].Content)
	}
}

func TestExtractInputMessages_MessageItems(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		wantLen  int
		wantRole string
	}{
		{
			name: "user message",
			input: []interface{}{
				map[string]interface{}{
					"type":    "message",
					"role":    "user",
					"content": "hello",
				},
			},
			wantLen:  1,
			wantRole: "user",
		},
		{
			name: "assistant message",
			input: []interface{}{
				map[string]interface{}{
					"type":    "message",
					"role":    "assistant",
					"content": "hi there",
				},
			},
			wantLen:  1,
			wantRole: "assistant",
		},
		{
			name: "function_call item",
			input: []interface{}{
				map[string]interface{}{
					"type":      "function_call",
					"call_id":   "call-1",
					"name":      "get_weather",
					"arguments": `{"city":"NYC"}`,
				},
			},
			wantLen:  1,
			wantRole: "assistant",
		},
		{
			name: "function_call_output item",
			input: []interface{}{
				map[string]interface{}{
					"type":    "function_call_output",
					"call_id": "call-1",
					"output":  "sunny",
				},
			},
			wantLen:  1,
			wantRole: "tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := extractInputMessages(tt.input)
			if len(msgs) != tt.wantLen {
				t.Fatalf("expected %d messages, got %d", tt.wantLen, len(msgs))
			}
			if msgs[0].Role != tt.wantRole {
				t.Errorf("expected role %q, got %q", tt.wantRole, msgs[0].Role)
			}
		})
	}
}

func TestExtractInputMessages_MultimodalContent(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "input_text",
					"text": "describe this",
				},
				map[string]interface{}{
					"type":      "input_image",
					"image_url": "https://example.com/img.png",
				},
			},
		},
	}

	msgs := extractInputMessages(input)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].ContentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msgs[0].ContentParts))
	}
	if msgs[0].ContentParts[0].Type != "text" {
		t.Errorf("expected first part type=text, got %q", msgs[0].ContentParts[0].Type)
	}
	if msgs[0].ContentParts[1].Type != "image_url" {
		t.Errorf("expected second part type=image_url, got %q", msgs[0].ContentParts[1].Type)
	}
	if msgs[0].ContentParts[1].ImageURL == nil {
		t.Fatal("expected ImageURL to be non-nil")
	}
	if msgs[0].ContentParts[1].ImageURL.URL != "https://example.com/img.png" {
		t.Errorf("expected URL %q, got %q", "https://example.com/img.png", msgs[0].ContentParts[1].ImageURL.URL)
	}
}

func TestExtractInputMessages_NonStringNonArray(t *testing.T) {
	msgs := extractInputMessages(42)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected role=user, got %q", msgs[0].Role)
	}
	if msgs[0].Content != "42" {
		t.Errorf("expected content %q, got %q", "42", msgs[0].Content)
	}
}

// --- convertMessagesToResponsesInput tests ---

func TestConvertMessagesToResponsesInput_UserMessage(t *testing.T) {
	messages := []api.Message{
		{Role: "user", Content: "hello"},
	}
	input := convertMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}

	item, ok := input[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", input[0])
	}
	if item["type"] != "message" {
		t.Errorf("expected type=message, got %v", item["type"])
	}
	if item["role"] != "user" {
		t.Errorf("expected role=user, got %v", item["role"])
	}

	content, ok := item["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content as []map, got %T", item["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(content))
	}
	if content[0]["type"] != "input_text" {
		t.Errorf("expected content type=input_text, got %v", content[0]["type"])
	}
	if content[0]["text"] != "hello" {
		t.Errorf("expected text=hello, got %v", content[0]["text"])
	}
}

func TestConvertMessagesToResponsesInput_SystemSkipped(t *testing.T) {
	messages := []api.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "hello"},
	}
	input := convertMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input item (system skipped), got %d", len(input))
	}
	item := input[0].(map[string]interface{})
	if item["role"] != "user" {
		t.Errorf("expected remaining item role=user, got %v", item["role"])
	}
}

func TestConvertMessagesToResponsesInput_AssistantWithToolCalls(t *testing.T) {
	messages := []api.Message{
		{
			Role: "assistant",
			ToolCalls: []api.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: api.ToolCallFunction{
						Name:      "get_weather",
						Arguments: `{"city":"NYC"}`,
					},
				},
			},
		},
	}
	input := convertMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	item := input[0].(map[string]interface{})
	if item["type"] != "function_call" {
		t.Errorf("expected type=function_call, got %v", item["type"])
	}
	if item["call_id"] != "call-1" {
		t.Errorf("expected call_id=call-1, got %v", item["call_id"])
	}
	if item["name"] != "get_weather" {
		t.Errorf("expected name=get_weather, got %v", item["name"])
	}
}

func TestConvertMessagesToResponsesInput_ToolMessage(t *testing.T) {
	messages := []api.Message{
		{Role: "tool", Content: "sunny", ToolCallID: "call-1"},
	}
	input := convertMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	item := input[0].(map[string]interface{})
	if item["type"] != "function_call_output" {
		t.Errorf("expected type=function_call_output, got %v", item["type"])
	}
	if item["call_id"] != "call-1" {
		t.Errorf("expected call_id=call-1, got %v", item["call_id"])
	}
	if item["output"] != "sunny" {
		t.Errorf("expected output=sunny, got %v", item["output"])
	}
}

func TestConvertMessagesToResponsesInput_MultimodalUser(t *testing.T) {
	messages := []api.Message{
		{
			Role: "user",
			ContentParts: []api.MessageContentPart{
				{Type: "text", Text: "look at this"},
				{Type: "image_url", ImageURL: &api.MessageImageURL{URL: "https://img.example.com/1.png"}},
				{Type: "file", File: &api.MessageFile{FileID: "file-123", Filename: "doc.pdf"}},
			},
		},
	}
	input := convertMessagesToResponsesInput(messages)
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	item := input[0].(map[string]interface{})
	parts := item["content"].([]map[string]interface{})
	if len(parts) != 3 {
		t.Fatalf("expected 3 content parts, got %d", len(parts))
	}
	if parts[0]["type"] != "input_text" {
		t.Errorf("expected type=input_text, got %v", parts[0]["type"])
	}
	if parts[1]["type"] != "input_image" {
		t.Errorf("expected type=input_image, got %v", parts[1]["type"])
	}
	if parts[2]["type"] != "input_file" {
		t.Errorf("expected type=input_file, got %v", parts[2]["type"])
	}
	fileMap := parts[2]["file"].(map[string]interface{})
	if fileMap["file_id"] != "file-123" {
		t.Errorf("expected file_id=file-123, got %v", fileMap["file_id"])
	}
}

func TestConvertMessagesToResponsesInput_MixedMessages(t *testing.T) {
	messages := []api.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "let me check", ToolCalls: []api.ToolCall{
			{ID: "c1", Type: "function", Function: api.ToolCallFunction{Name: "search", Arguments: `{"q":"hi"}`}},
		}},
		{Role: "tool", Content: "result", ToolCallID: "c1"},
	}
	input := convertMessagesToResponsesInput(messages)
	// user message + function_call + assistant message + function_call_output = 4
	if len(input) != 4 {
		t.Fatalf("expected 4 input items, got %d", len(input))
	}

	// Verify ordering: user, function_call, assistant message, function_call_output
	types := make([]string, len(input))
	for i, item := range input {
		m := item.(map[string]interface{})
		types[i] = m["type"].(string)
	}
	expected := []string{"message", "function_call", "message", "function_call_output"}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("item[%d] type: expected %q, got %q", i, want, types[i])
		}
	}
}

// --- parseResponsesOutput tests ---

func TestParseResponsesOutput_TextOnly(t *testing.T) {
	output := []api.OutputItem{
		{
			Type: "message",
			Content: []api.ContentItem{
				{Type: "output_text", Text: "hello world"},
			},
		},
	}
	text, toolCalls, hasToolCalls := parseResponsesOutput(output)
	if text != "hello world" {
		t.Errorf("expected text %q, got %q", "hello world", text)
	}
	if len(toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(toolCalls))
	}
	if hasToolCalls {
		t.Error("expected hasToolCalls=false")
	}
}

func TestParseResponsesOutput_FunctionCallOnly(t *testing.T) {
	output := []api.OutputItem{
		{
			Type:      "function_call",
			ID:        "fc-1",
			Name:      "get_weather",
			Arguments: `{"city":"NYC"}`,
			CallID:    "call-1",
		},
	}
	text, toolCalls, hasToolCalls := parseResponsesOutput(output)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if !hasToolCalls {
		t.Error("expected hasToolCalls=true")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("expected name=get_weather, got %q", toolCalls[0].Name)
	}
	if toolCalls[0].CallID != "call-1" {
		t.Errorf("expected callID=call-1, got %q", toolCalls[0].CallID)
	}
}

func TestParseResponsesOutput_MixedOutput(t *testing.T) {
	output := []api.OutputItem{
		{
			Type: "message",
			Content: []api.ContentItem{
				{Type: "output_text", Text: "result: "},
			},
		},
		{
			Type:      "function_call",
			ID:        "fc-1",
			Name:      "search",
			Arguments: `{"q":"test"}`,
			CallID:    "call-1",
		},
	}
	text, toolCalls, hasToolCalls := parseResponsesOutput(output)
	if text != "result: " {
		t.Errorf("expected text %q, got %q", "result: ", text)
	}
	if !hasToolCalls {
		t.Error("expected hasToolCalls=true")
	}
	if len(toolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(toolCalls))
	}
}

func TestParseResponsesOutput_EmptyOutput(t *testing.T) {
	text, toolCalls, hasToolCalls := parseResponsesOutput(nil)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(toolCalls))
	}
	if hasToolCalls {
		t.Error("expected hasToolCalls=false")
	}
}

// --- convertOutputItemsToSchema tests ---

func TestConvertOutputItemsToSchema_Message(t *testing.T) {
	items := []api.OutputItem{
		{
			Type: "message",
			ID:   "msg-1",
			Role: "assistant",
			Content: []api.ContentItem{
				{Type: "output_text", Text: "hello"},
			},
		},
	}
	result := convertOutputItemsToSchema(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Type != "message" {
		t.Errorf("expected type=message, got %q", result[0].Type)
	}
	if result[0].ID != "msg-1" {
		t.Errorf("expected ID=msg-1, got %q", result[0].ID)
	}
	if *result[0].Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", *result[0].Role)
	}
	if *result[0].Status != "completed" {
		t.Errorf("expected default status=completed, got %q", *result[0].Status)
	}
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(result[0].Content))
	}
	if *result[0].Content[0].Text != "hello" {
		t.Errorf("expected text=hello, got %q", *result[0].Content[0].Text)
	}
}

func TestConvertOutputItemsToSchema_FunctionCall(t *testing.T) {
	items := []api.OutputItem{
		{
			Type:      "function_call",
			ID:        "fc-1",
			Name:      "search",
			Arguments: `{"q":"test"}`,
			CallID:    "call-1",
			Status:    "completed",
		},
	}
	result := convertOutputItemsToSchema(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Type != "function_call" {
		t.Errorf("expected type=function_call, got %q", result[0].Type)
	}
	if *result[0].Name != "search" {
		t.Errorf("expected name=search, got %q", *result[0].Name)
	}
	if *result[0].Arguments != `{"q":"test"}` {
		t.Errorf("expected arguments, got %q", *result[0].Arguments)
	}
	if *result[0].CallID != "call-1" {
		t.Errorf("expected callID=call-1, got %q", *result[0].CallID)
	}
}

func TestConvertOutputItemsToSchema_FunctionCallOutput(t *testing.T) {
	items := []api.OutputItem{
		{
			Type:   "function_call_output",
			ID:     "fco-1",
			CallID: "call-1",
			Output: "result data",
		},
	}
	result := convertOutputItemsToSchema(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Type != "function_call_output" {
		t.Errorf("expected type=function_call_output, got %q", result[0].Type)
	}
	if *result[0].CallID != "call-1" {
		t.Errorf("expected callID=call-1, got %q", *result[0].CallID)
	}
	if *result[0].Output != "result data" {
		t.Errorf("expected output=%q, got %q", "result data", *result[0].Output)
	}
}

// --- convertToToolParams tests ---

func TestConvertToToolParams_FunctionToolsOnly(t *testing.T) {
	tools := []schema.ResponsesToolParam{
		{Type: "function", Name: "search", Description: stringPtr("search things")},
		{Type: "mcp", Name: "mcp-tool", ServerLabel: "server1"},
		{Type: "file_search", VectorStoreIDs: []string{"vs-1"}},
		{Type: "function", Name: "calc", Description: stringPtr("calculate")},
	}
	result := convertToToolParams(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 function tools, got %d", len(result))
	}
	if result[0].Name != "search" {
		t.Errorf("expected first tool name=search, got %q", result[0].Name)
	}
	if result[1].Name != "calc" {
		t.Errorf("expected second tool name=calc, got %q", result[1].Name)
	}
}

// --- expandFileSearchTools tests ---

func TestExpandFileSearchTools_NoVectorSearch(t *testing.T) {
	e := &Engine{vectorSearch: nil}
	tools := []schema.ResponsesToolParam{
		{Type: "file_search", VectorStoreIDs: []string{"vs-1"}},
		{Type: "function", Name: "search"},
	}
	expanded, configs := e.expandFileSearchTools(tools)
	if configs != nil {
		t.Error("expected nil configs when vectorSearch is nil")
	}
	// Tools should pass through unchanged
	if len(expanded) != 2 {
		t.Errorf("expected 2 tools unchanged, got %d", len(expanded))
	}
}

func TestExpandFileSearchTools_WithFileSearchTool(t *testing.T) {
	e := &Engine{vectorSearch: &dummyVectorSearcher{}}
	tools := []schema.ResponsesToolParam{
		{Type: "file_search", VectorStoreIDs: []string{"vs-1"}, MaxNumResults: intPtr(5)},
	}
	expanded, configs := e.expandFileSearchTools(tools)
	if len(expanded) != 1 {
		t.Fatalf("expected 1 expanded tool, got %d", len(expanded))
	}
	if expanded[0].Type != "function" {
		t.Errorf("expected type=function, got %q", expanded[0].Type)
	}
	if expanded[0].Name != "file_search" {
		t.Errorf("expected name=file_search, got %q", expanded[0].Name)
	}
	if configs == nil {
		t.Fatal("expected non-nil configs")
	}
	cfg, ok := configs["file_search"]
	if !ok {
		t.Fatal("expected file_search config")
	}
	if cfg.MaxNumResults != 5 {
		t.Errorf("expected MaxNumResults=5, got %d", cfg.MaxNumResults)
	}
	if len(cfg.VectorStoreIDs) != 1 || cfg.VectorStoreIDs[0] != "vs-1" {
		t.Errorf("expected VectorStoreIDs=[vs-1], got %v", cfg.VectorStoreIDs)
	}
}

func TestExpandFileSearchTools_MixedTools(t *testing.T) {
	e := &Engine{vectorSearch: &dummyVectorSearcher{}}
	tools := []schema.ResponsesToolParam{
		{Type: "function", Name: "calc"},
		{Type: "file_search", VectorStoreIDs: []string{"vs-1"}},
		{Type: "function", Name: "search"},
	}
	expanded, configs := e.expandFileSearchTools(tools)
	if len(expanded) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(expanded))
	}
	// calc stays, file_search replaced, search stays
	if expanded[0].Type != "function" || expanded[0].Name != "calc" {
		t.Errorf("expected first tool to be calc function, got %s/%s", expanded[0].Type, expanded[0].Name)
	}
	if expanded[1].Type != "function" || expanded[1].Name != "file_search" {
		t.Errorf("expected second tool to be file_search function, got %s/%s", expanded[1].Type, expanded[1].Name)
	}
	if expanded[2].Type != "function" || expanded[2].Name != "search" {
		t.Errorf("expected third tool to be search function, got %s/%s", expanded[2].Type, expanded[2].Name)
	}
	if configs == nil {
		t.Fatal("expected configs to be non-nil")
	}
}

func TestExpandFileSearchTools_DefaultMaxResults(t *testing.T) {
	e := &Engine{vectorSearch: &dummyVectorSearcher{}}
	tools := []schema.ResponsesToolParam{
		{Type: "file_search", VectorStoreIDs: []string{"vs-1"}},
	}
	_, configs := e.expandFileSearchTools(tools)
	if configs["file_search"].MaxNumResults != 10 {
		t.Errorf("expected default MaxNumResults=10, got %d", configs["file_search"].MaxNumResults)
	}
}

// --- generateID tests ---

func TestGenerateID_Format(t *testing.T) {
	prefixes := []string{"resp_", "conv_", "msg_", "fc_"}
	for _, prefix := range prefixes {
		id := generateID(prefix)
		if !strings.HasPrefix(id, prefix) {
			t.Errorf("expected prefix %q, got %q", prefix, id)
		}
		// hex suffix should be 32 chars (16 bytes * 2)
		suffix := strings.TrimPrefix(id, prefix)
		if len(suffix) != 32 {
			t.Errorf("expected 32 hex chars suffix, got %d: %q", len(suffix), suffix)
		}
	}

	// Uniqueness
	id1 := generateID("test_")
	id2 := generateID("test_")
	if id1 == id2 {
		t.Error("expected unique IDs, got duplicates")
	}
}

// --- patchResponseID tests ---

func TestPatchResponseID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		newID   string
		wantID  string
		changed bool
	}{
		{
			name:    "patches existing ID",
			input:   `{"response_id":"old-id","text":"hello"}`,
			newID:   "new-id",
			wantID:  "new-id",
			changed: true,
		},
		{
			name:    "no-op without response_id",
			input:   `{"text":"hello"}`,
			newID:   "new-id",
			changed: false,
		},
		{
			name:    "handles invalid JSON",
			input:   `not json`,
			newID:   "new-id",
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := patchResponseID(json.RawMessage(tt.input), tt.newID)

			if tt.changed {
				var m map[string]interface{}
				if err := json.Unmarshal(result, &m); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				if m["response_id"] != tt.wantID {
					t.Errorf("expected response_id=%q, got %v", tt.wantID, m["response_id"])
				}
			} else {
				if string(result) != tt.input {
					t.Errorf("expected unchanged input, got %q", string(result))
				}
			}
		})
	}
}

// --- parseJSONArgs tests ---

func TestParseJSONArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(map[string]any) bool
	}{
		{
			name:  "valid JSON",
			input: `{"key":"value","num":42}`,
			check: func(m map[string]any) bool {
				return m["key"] == "value" && m["num"] == float64(42)
			},
		},
		{
			name:  "empty string",
			input: "",
			check: func(m map[string]any) bool {
				return len(m) == 0
			},
		},
		{
			name:  "invalid JSON",
			input: "not json",
			check: func(m map[string]any) bool {
				return len(m) == 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseJSONArgs(tt.input)
			if !tt.check(result) {
				t.Errorf("unexpected result: %v", result)
			}
		})
	}
}

// --- messagesToConversationMessages tests ---

func TestMessagesToConversationMessages(t *testing.T) {
	messages := []api.Message{
		{Role: "user", Content: "hello"},
		{
			Role: "assistant",
			ToolCalls: []api.ToolCall{
				{
					ID:   "tc-1",
					Type: "function",
					Function: api.ToolCallFunction{
						Name:      "search",
						Arguments: `{"q":"test"}`,
					},
				},
			},
		},
		{Role: "tool", Content: "result", ToolCallID: "tc-1"},
	}

	result := messagesToConversationMessages(messages)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// Check user message
	if result[0].Role != "user" || result[0].Content != "hello" {
		t.Errorf("unexpected user message: %+v", result[0])
	}

	// Check assistant with tool calls
	if result[1].Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", result[1].Role)
	}
	if len(result[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[1].ToolCalls))
	}
	if result[1].ToolCalls[0].Name != "search" {
		t.Errorf("expected tool call name=search, got %q", result[1].ToolCalls[0].Name)
	}
	if result[1].ToolCalls[0].ID != "tc-1" {
		t.Errorf("expected tool call ID=tc-1, got %q", result[1].ToolCalls[0].ID)
	}

	// Check tool message
	if result[2].Role != "tool" || result[2].ToolCallID != "tc-1" {
		t.Errorf("unexpected tool message: %+v", result[2])
	}
}

// --- echoRequestParams tests ---

func TestEchoRequestParams(t *testing.T) {
	req := &schema.ResponseRequest{
		PreviousResponseID: stringPtr("prev-1"),
		Conversation:       stringPtr("conv-1"),
		Instructions:       stringPtr("be helpful"),
		Temperature:        float64Ptr(0.7),
		TopP:               float64Ptr(0.9),
		MaxOutputTokens:    intPtr(100),
		MaxToolCalls:       intPtr(5),
		FrequencyPenalty:   float64Ptr(0.5),
		PresencePenalty:    float64Ptr(0.3),
		Metadata:           map[string]string{"k": "v"},
		Tools: []schema.ResponsesToolParam{
			{Type: "function", Name: "search"},
		},
		Reasoning: &schema.ReasoningParam{
			Type:   "default",
			Effort: stringPtr("medium"),
		},
	}

	resp := schema.NewResponse("resp-test", "test-model")
	echoRequestParams(resp, req)

	if *resp.PreviousResponseID != "prev-1" {
		t.Errorf("PreviousResponseID: expected %q, got %v", "prev-1", resp.PreviousResponseID)
	}
	if *resp.Conversation != "conv-1" {
		t.Errorf("Conversation: expected %q, got %v", "conv-1", resp.Conversation)
	}
	if *resp.Instructions != "be helpful" {
		t.Errorf("Instructions: expected %q, got %v", "be helpful", resp.Instructions)
	}
	if resp.Temperature != 0.7 {
		t.Errorf("Temperature: expected 0.7, got %v", resp.Temperature)
	}
	if resp.TopP != 0.9 {
		t.Errorf("TopP: expected 0.9, got %v", resp.TopP)
	}
	if *resp.MaxOutputTokens != 100 {
		t.Errorf("MaxOutputTokens: expected 100, got %v", resp.MaxOutputTokens)
	}
	if *resp.MaxToolCalls != 5 {
		t.Errorf("MaxToolCalls: expected 5, got %v", resp.MaxToolCalls)
	}
	if resp.FrequencyPenalty != 0.5 {
		t.Errorf("FrequencyPenalty: expected 0.5, got %v", resp.FrequencyPenalty)
	}
	if resp.PresencePenalty != 0.3 {
		t.Errorf("PresencePenalty: expected 0.3, got %v", resp.PresencePenalty)
	}
	if resp.Metadata["k"] != "v" {
		t.Errorf("Metadata: expected k=v, got %v", resp.Metadata)
	}
	if len(resp.Tools) != 1 || resp.Tools[0].Name != "search" {
		t.Errorf("Tools: expected 1 tool named search, got %v", resp.Tools)
	}
	if resp.Reasoning == nil || *resp.Reasoning.Effort != "medium" {
		t.Errorf("Reasoning: expected effort=medium, got %v", resp.Reasoning)
	}
}

func TestEchoRequestParams_InferenceAndStoreFields(t *testing.T) {
	req := &schema.ResponseRequest{
		Truncation:        stringPtr("auto"),
		ParallelToolCalls: boolPtr(false),
		Text:              &schema.TextField{Format: schema.TextFormat{Type: "json_object"}},
		TopLogprobs:       intPtr(5),
		Store:             boolPtr(false),
	}

	resp := schema.NewResponse("resp-test", "test-model")
	echoRequestParams(resp, req)

	if resp.Truncation != "auto" {
		t.Errorf("Truncation: expected %q, got %q", "auto", resp.Truncation)
	}
	if resp.ParallelToolCalls != false {
		t.Errorf("ParallelToolCalls: expected false, got %v", resp.ParallelToolCalls)
	}
	if resp.Text.Format.Type != "json_object" {
		t.Errorf("Text.Format.Type: expected %q, got %q", "json_object", resp.Text.Format.Type)
	}
	if resp.TopLogprobs != 5 {
		t.Errorf("TopLogprobs: expected 5, got %d", resp.TopLogprobs)
	}
	if resp.Store != false {
		t.Errorf("Store: expected false, got %v", resp.Store)
	}
}

func TestEchoRequestParams_InferenceAndStoreDefaults(t *testing.T) {
	req := &schema.ResponseRequest{}
	resp := schema.NewResponse("test", "model")
	echoRequestParams(resp, req)

	// Defaults from NewResponse should remain when request fields are nil
	if resp.Truncation != "disabled" {
		t.Errorf("expected Truncation=%q, got %q", "disabled", resp.Truncation)
	}
	if resp.ParallelToolCalls != true {
		t.Errorf("expected ParallelToolCalls=true, got %v", resp.ParallelToolCalls)
	}
	if resp.Text.Format.Type != "text" {
		t.Errorf("expected Text.Format.Type=%q, got %q", "text", resp.Text.Format.Type)
	}
	if resp.TopLogprobs != 0 {
		t.Errorf("expected TopLogprobs=0, got %d", resp.TopLogprobs)
	}
	if resp.Store != true {
		t.Errorf("expected Store=true, got %v", resp.Store)
	}
}

func TestConvertOutputItemsToSchema_OutputTextAnnotationsAndLogprobs(t *testing.T) {
	items := []api.OutputItem{
		{
			Type: "message",
			ID:   "msg-1",
			Role: "assistant",
			Content: []api.ContentItem{
				{Type: "output_text", Text: "hello"},
			},
		},
	}
	result := convertOutputItemsToSchema(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(result[0].Content))
	}
	cp := result[0].Content[0]
	if cp.Annotations == nil {
		t.Fatal("expected non-nil Annotations on output_text")
	}
	if len(cp.Annotations) != 0 {
		t.Errorf("expected empty Annotations, got %d", len(cp.Annotations))
	}
	if cp.Logprobs == nil {
		t.Fatal("expected non-nil Logprobs on output_text")
	}
	if len(cp.Logprobs) != 0 {
		t.Errorf("expected empty Logprobs, got %d", len(cp.Logprobs))
	}
}

func TestConvertOutputItemsToSchema_NonOutputTextNoAnnotations(t *testing.T) {
	items := []api.OutputItem{
		{
			Type: "message",
			ID:   "msg-1",
			Role: "assistant",
			Content: []api.ContentItem{
				{Type: "text", Text: "hello"},
			},
		},
	}
	result := convertOutputItemsToSchema(items)
	cp := result[0].Content[0]
	if cp.Annotations != nil {
		t.Errorf("expected nil Annotations on non-output_text, got %v", cp.Annotations)
	}
	if cp.Logprobs != nil {
		t.Errorf("expected nil Logprobs on non-output_text, got %v", cp.Logprobs)
	}
}

func TestOutputToItemFields_FromTypedSlice(t *testing.T) {
	role := "assistant"
	text := "hello"
	input := []schema.ItemField{
		{Type: "message", ID: "msg-1", Role: &role, Content: []schema.ContentPart{{Type: "output_text", Text: &text}}},
	}
	result := outputToItemFields(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].Type != "message" {
		t.Errorf("expected type=message, got %q", result[0].Type)
	}
}

func TestOutputToItemFields_FromDeserializedJSON(t *testing.T) {
	// Simulate what happens after JSON round-trip through the database:
	// []schema.ItemField → json.Marshal → json.Unmarshal into interface{} → []interface{}
	role := "assistant"
	text := "hello"
	original := []schema.ItemField{
		{Type: "message", ID: "msg-1", Role: &role, Content: []schema.ContentPart{{Type: "output_text", Text: &text}}},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var deserialized interface{}
	if err := json.Unmarshal(data, &deserialized); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// deserialized is now []interface{}, not []schema.ItemField
	if _, ok := deserialized.([]schema.ItemField); ok {
		t.Fatal("expected deserialized NOT to be []schema.ItemField")
	}

	result := outputToItemFields(deserialized)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].Type != "message" {
		t.Errorf("expected type=message, got %q", result[0].Type)
	}
	if *result[0].Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", *result[0].Role)
	}
	if len(result[0].Content) != 1 || *result[0].Content[0].Text != "hello" {
		t.Errorf("expected content text=hello, got %v", result[0].Content)
	}
}

func TestOutputToItemFields_NilInput(t *testing.T) {
	result := outputToItemFields(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestEchoRequestParams_NilOptionals(t *testing.T) {
	req := &schema.ResponseRequest{}
	resp := schema.NewResponse("test", "model")
	echoRequestParams(resp, req)

	// Temperature and TopP should remain at defaults (0)
	if resp.Temperature != 0 {
		t.Errorf("expected Temperature=0, got %v", resp.Temperature)
	}
	if resp.TopP != 0 {
		t.Errorf("expected TopP=0, got %v", resp.TopP)
	}
}
