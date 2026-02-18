// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Request conversion tests ---

func TestConvertToChatRequest_BasicText(t *testing.T) {
	instructions := "You are a helpful assistant."
	req := &ResponsesAPIRequest{
		Model:        "gpt-4",
		Input:        "Hello, world!",
		Instructions: &instructions,
	}

	chatReq := ConvertToChatRequest(req)

	if chatReq.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", chatReq.Model)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "system" {
		t.Errorf("expected first message role system, got %s", chatReq.Messages[0].Role)
	}
	if chatReq.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("expected system content, got %v", chatReq.Messages[0].Content)
	}
	if chatReq.Messages[1].Role != "user" {
		t.Errorf("expected second message role user, got %s", chatReq.Messages[1].Role)
	}
	if chatReq.Messages[1].Content != "Hello, world!" {
		t.Errorf("expected user content, got %v", chatReq.Messages[1].Content)
	}
}

func TestConvertToChatRequest_NoInstructions(t *testing.T) {
	req := &ResponsesAPIRequest{
		Model: "gpt-4",
		Input: "Hello",
	}

	chatReq := ConvertToChatRequest(req)

	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "user" {
		t.Errorf("expected user role, got %s", chatReq.Messages[0].Role)
	}
}

func TestConvertToChatRequest_SamplingParams(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTokens := 100
	freqPenalty := 0.5
	presPenalty := 0.3
	topLogprobs := 5

	req := &ResponsesAPIRequest{
		Model:            "gpt-4",
		Input:            "test",
		Temperature:      &temp,
		TopP:             &topP,
		MaxOutputTokens:  &maxTokens,
		FrequencyPenalty: &freqPenalty,
		PresencePenalty:  &presPenalty,
		TopLogprobs:      &topLogprobs,
	}

	chatReq := ConvertToChatRequest(req)

	if chatReq.Temperature == nil || *chatReq.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", chatReq.Temperature)
	}
	if chatReq.TopP == nil || *chatReq.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %v", chatReq.TopP)
	}
	if chatReq.MaxTokens == nil || *chatReq.MaxTokens != 100 {
		t.Errorf("expected max_tokens 100, got %v", chatReq.MaxTokens)
	}
	if chatReq.FrequencyPenalty == nil || *chatReq.FrequencyPenalty != 0.5 {
		t.Errorf("expected frequency_penalty 0.5, got %v", chatReq.FrequencyPenalty)
	}
	if chatReq.PresencePenalty == nil || *chatReq.PresencePenalty != 0.3 {
		t.Errorf("expected presence_penalty 0.3, got %v", chatReq.PresencePenalty)
	}
	if chatReq.Logprobs == nil || !*chatReq.Logprobs {
		t.Error("expected logprobs true")
	}
	if chatReq.TopLogprobs == nil || *chatReq.TopLogprobs != 5 {
		t.Errorf("expected top_logprobs 5, got %v", chatReq.TopLogprobs)
	}
}

func TestConvertToChatRequest_Tools(t *testing.T) {
	desc := "Get weather info"
	req := &ResponsesAPIRequest{
		Model: "gpt-4",
		Input: "What's the weather?",
		Tools: []ToolParam{
			{
				Type:        "function",
				Name:        "get_weather",
				Description: &desc,
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
	}

	chatReq := ConvertToChatRequest(req)

	if len(chatReq.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(chatReq.Tools))
	}
	if chatReq.Tools[0].Type != "function" {
		t.Errorf("expected tool type function, got %s", chatReq.Tools[0].Type)
	}
	if chatReq.Tools[0].Function.Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", chatReq.Tools[0].Function.Name)
	}
	if chatReq.Tools[0].Function.Description != "Get weather info" {
		t.Errorf("expected description, got %s", chatReq.Tools[0].Function.Description)
	}
}

func TestConvertToChatRequest_ToolChoice(t *testing.T) {
	tests := []struct {
		name       string
		toolChoice interface{}
		expected   interface{}
	}{
		{"nil", nil, nil},
		{"auto string", "auto", "auto"},
		{"none string", "none", "none"},
		{"required string", "required", "required"},
		{
			"function object",
			map[string]interface{}{"type": "function", "name": "get_weather"},
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToolChoice(tt.toolChoice)
			expectedJSON, _ := json.Marshal(tt.expected)
			resultJSON, _ := json.Marshal(result)
			if string(expectedJSON) != string(resultJSON) {
				t.Errorf("expected %s, got %s", expectedJSON, resultJSON)
			}
		})
	}
}

func TestConvertToChatRequest_ToolChoiceDroppedWhenNoFunctionTools(t *testing.T) {
	// When all tools are non-function (e.g., web_search), they get stripped
	// during conversion. tool_choice should also be dropped to avoid
	// the backend rejecting tool_choice without any tools.
	req := &ResponsesAPIRequest{
		Model: "gpt-4",
		Input: "Hello",
		Tools: []ToolParam{
			{Type: "web_search"},
		},
		ToolChoice: "auto",
	}

	chatReq := ConvertToChatRequest(req)

	if len(chatReq.Tools) != 0 {
		t.Errorf("expected 0 tools (web_search stripped), got %d", len(chatReq.Tools))
	}
	if chatReq.ToolChoice != nil {
		t.Errorf("expected nil tool_choice when no tools, got %v", chatReq.ToolChoice)
	}
}

func TestConvertToChatRequest_ToolChoiceKeptWithFunctionTools(t *testing.T) {
	desc := "A function"
	req := &ResponsesAPIRequest{
		Model: "gpt-4",
		Input: "Hello",
		Tools: []ToolParam{
			{Type: "web_search"},
			{Type: "function", Name: "my_func", Description: &desc},
		},
		ToolChoice: "auto",
	}

	chatReq := ConvertToChatRequest(req)

	if len(chatReq.Tools) != 1 {
		t.Fatalf("expected 1 tool (function only), got %d", len(chatReq.Tools))
	}
	if chatReq.ToolChoice != "auto" {
		t.Errorf("expected tool_choice=auto, got %v", chatReq.ToolChoice)
	}
}

// --- Input conversion tests ---

func TestConvertInputToMessages_String(t *testing.T) {
	msgs := convertInputToMessages("Hello")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("expected user message with Hello, got %+v", msgs[0])
	}
}

func TestConvertInputToMessages_StructuredItems(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"role":    "user",
			"content": "Hello",
		},
		map[string]interface{}{
			"role":    "assistant",
			"content": "Hi there!",
		},
		map[string]interface{}{
			"role":    "user",
			"content": "How are you?",
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected user, got %s", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("expected assistant, got %s", msgs[1].Role)
	}
	if msgs[2].Role != "user" {
		t.Errorf("expected user, got %s", msgs[2].Role)
	}
}

func TestConvertInputToMessages_FunctionCalls(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"role":    "user",
			"content": "What's the weather?",
		},
		map[string]interface{}{
			"type":      "function_call",
			"call_id":   "call_123",
			"name":      "get_weather",
			"arguments": `{"location":"NYC"}`,
		},
		map[string]interface{}{
			"type":    "function_call_output",
			"call_id": "call_123",
			"output":  "Sunny, 72°F",
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// User message
	if msgs[0].Role != "user" {
		t.Errorf("expected user, got %s", msgs[0].Role)
	}

	// Assistant with tool call
	if msgs[1].Role != "assistant" {
		t.Errorf("expected assistant, got %s", msgs[1].Role)
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msgs[1].ToolCalls))
	}
	if msgs[1].ToolCalls[0].ID != "call_123" {
		t.Errorf("expected call_id call_123, got %s", msgs[1].ToolCalls[0].ID)
	}
	if msgs[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", msgs[1].ToolCalls[0].Function.Name)
	}

	// Tool result
	if msgs[2].Role != "tool" {
		t.Errorf("expected tool, got %s", msgs[2].Role)
	}
	if msgs[2].ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id call_123, got %s", msgs[2].ToolCallID)
	}
	if msgs[2].Content != "Sunny, 72°F" {
		t.Errorf("expected content, got %v", msgs[2].Content)
	}
}

func TestConvertInputToMessages_ConsecutiveFunctionCalls(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type":      "function_call",
			"call_id":   "call_1",
			"name":      "get_weather",
			"arguments": `{"location":"NYC"}`,
		},
		map[string]interface{}{
			"type":      "function_call",
			"call_id":   "call_2",
			"name":      "get_time",
			"arguments": `{"timezone":"EST"}`,
		},
		map[string]interface{}{
			"type":    "function_call_output",
			"call_id": "call_1",
			"output":  "Sunny",
		},
		map[string]interface{}{
			"type":    "function_call_output",
			"call_id": "call_2",
			"output":  "3pm",
		},
	}

	msgs := convertInputToMessages(input)

	// Consecutive function_calls should be merged into one assistant message
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (1 assistant + 2 tool), got %d", len(msgs))
	}

	if msgs[0].Role != "assistant" {
		t.Errorf("expected assistant, got %s", msgs[0].Role)
	}
	if len(msgs[0].ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls merged, got %d", len(msgs[0].ToolCalls))
	}

	if msgs[1].Role != "tool" {
		t.Errorf("expected tool, got %s", msgs[1].Role)
	}
	if msgs[2].Role != "tool" {
		t.Errorf("expected tool, got %s", msgs[2].Role)
	}
}

func TestConvertInputToMessages_DeveloperRole(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"role":    "developer",
			"content": "Be concise.",
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	// developer should be mapped to system
	if msgs[0].Role != "system" {
		t.Errorf("expected system (mapped from developer), got %s", msgs[0].Role)
	}
}

func TestConvertInputToMessages_MessageType(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "input_text",
					"text": "Hello there",
				},
			},
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected user, got %s", msgs[0].Role)
	}
	// Text-only multimodal should be flattened to simple string
	if msgs[0].Content != "Hello there" {
		t.Errorf("expected 'Hello there', got %v", msgs[0].Content)
	}
}

func TestConvertInputToMessages_ImageInput(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "input_text",
					"text": "What is in this image?",
				},
				map[string]interface{}{
					"type":      "input_image",
					"image_url": "data:image/png;base64,abc123",
				},
			},
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	parts, ok := msgs[0].Content.([]ChatCompletionContentPart)
	if !ok {
		t.Fatalf("expected []ChatCompletionContentPart, got %T", msgs[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "What is in this image?" {
		t.Errorf("expected text part, got %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("expected image_url part, got %+v", parts[1])
	}
	if parts[1].ImageURL.URL != "data:image/png;base64,abc123" {
		t.Errorf("expected image URL, got %s", parts[1].ImageURL.URL)
	}
}

func TestConvertInputToMessages_FileInput(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "input_text",
					"text": "Summarize this file.",
				},
				map[string]interface{}{
					"type": "input_file",
					"file": map[string]interface{}{
						"file_data": "SGVsbG8sIHdvcmxkIQ==",
						"filename":  "hello.txt",
					},
				},
			},
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	parts, ok := msgs[0].Content.([]ChatCompletionContentPart)
	if !ok {
		t.Fatalf("expected []ChatCompletionContentPart, got %T", msgs[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "Summarize this file." {
		t.Errorf("expected text part, got %+v", parts[0])
	}
	if parts[1].Type != "file" || parts[1].File == nil {
		t.Fatalf("expected file part, got %+v", parts[1])
	}
	if parts[1].File.FileData != "SGVsbG8sIHdvcmxkIQ==" {
		t.Errorf("expected file_data, got %s", parts[1].File.FileData)
	}
	if parts[1].File.Filename != "hello.txt" {
		t.Errorf("expected filename hello.txt, got %s", parts[1].File.Filename)
	}
}

func TestConvertInputToMessages_FileInputByID(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "input_file",
					"file": map[string]interface{}{
						"file_id": "file-abc123",
					},
				},
			},
		},
	}

	msgs := convertInputToMessages(input)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	parts, ok := msgs[0].Content.([]ChatCompletionContentPart)
	if !ok {
		t.Fatalf("expected []ChatCompletionContentPart, got %T", msgs[0].Content)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].File == nil || parts[0].File.FileID != "file-abc123" {
		t.Errorf("expected file_id=file-abc123, got %+v", parts[0].File)
	}
}

// --- Response conversion tests ---

func TestConvertFromChatResponse_TextResponse(t *testing.T) {
	content := "Hello! How can I help?"
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: &content,
				},
			},
		},
		Usage: &ChatCompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected id chatcmpl-123, got %s", resp.ID)
	}
	if resp.Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Status)
	}
	if resp.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", resp.Model)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	if resp.Output[0].Type != "message" {
		t.Errorf("expected type message, got %s", resp.Output[0].Type)
	}
	if resp.Output[0].Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Output[0].Role)
	}
	if len(resp.Output[0].Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(resp.Output[0].Content))
	}
	if resp.Output[0].Content[0].Type != "output_text" {
		t.Errorf("expected content type output_text, got %s", resp.Output[0].Content[0].Type)
	}
	if resp.Output[0].Content[0].Text != "Hello! How can I help?" {
		t.Errorf("expected text, got %s", resp.Output[0].Content[0].Text)
	}

	// Usage
	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens 5, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total_tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestConvertFromChatResponse_ToolCalls(t *testing.T) {
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-456",
		Object:  "chat.completion",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: ChatCompletionChoiceMsg{
					Role: "assistant",
					ToolCalls: []ChatCompletionToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: ChatCompletionToolCallFunction{
								Name:      "get_weather",
								Arguments: `{"location":"NYC"}`,
							},
						},
					},
				},
			},
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if resp.Status != "completed" {
		t.Errorf("expected status completed (tool_calls maps to completed), got %s", resp.Status)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	if resp.Output[0].Type != "function_call" {
		t.Errorf("expected type function_call, got %s", resp.Output[0].Type)
	}
	if resp.Output[0].CallID != "call_abc" {
		t.Errorf("expected call_id call_abc, got %s", resp.Output[0].CallID)
	}
	if resp.Output[0].Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", resp.Output[0].Name)
	}
	if resp.Output[0].Arguments != `{"location":"NYC"}` {
		t.Errorf("expected arguments, got %s", resp.Output[0].Arguments)
	}
}

func TestConvertFromChatResponse_LengthFinish(t *testing.T) {
	content := "partial..."
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-789",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				FinishReason: "length",
				Message: ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: &content,
				},
			},
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if resp.Status != "incomplete" {
		t.Errorf("expected status incomplete for length finish, got %s", resp.Status)
	}
}

func TestConvertFromChatResponse_MultipleToolCalls(t *testing.T) {
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-multi",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: ChatCompletionChoiceMsg{
					Role: "assistant",
					ToolCalls: []ChatCompletionToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: ChatCompletionToolCallFunction{
								Name:      "get_weather",
								Arguments: `{"location":"NYC"}`,
							},
						},
						{
							ID:   "call_2",
							Type: "function",
							Function: ChatCompletionToolCallFunction{
								Name:      "get_time",
								Arguments: `{"tz":"EST"}`,
							},
						},
					},
				},
			},
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if len(resp.Output) != 2 {
		t.Fatalf("expected 2 output items, got %d", len(resp.Output))
	}
	if resp.Output[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", resp.Output[0].Name)
	}
	if resp.Output[1].Name != "get_time" {
		t.Errorf("expected get_time, got %s", resp.Output[1].Name)
	}
}

func TestConvertFromChatResponse_TextAndToolCalls(t *testing.T) {
	content := "Let me check that for you."
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-both",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: &content,
					ToolCalls: []ChatCompletionToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: ChatCompletionToolCallFunction{
								Name:      "search",
								Arguments: `{"q":"test"}`,
							},
						},
					},
				},
			},
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if len(resp.Output) != 2 {
		t.Fatalf("expected 2 output items (message + function_call), got %d", len(resp.Output))
	}
	if resp.Output[0].Type != "message" {
		t.Errorf("expected first item to be message, got %s", resp.Output[0].Type)
	}
	if resp.Output[1].Type != "function_call" {
		t.Errorf("expected second item to be function_call, got %s", resp.Output[1].Type)
	}
}

// --- Non-streaming integration test with httptest ---

func TestCreateResponse_Integration(t *testing.T) {
	content := "I'm doing well, thanks!"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected content-type application/json")
		}

		// Verify the request body
		var chatReq ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if chatReq.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", chatReq.Model)
		}
		if chatReq.Stream {
			t.Error("expected stream false for non-streaming")
		}

		resp := ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Model:   "test-model",
			Created: 1700000000,
			Choices: []ChatCompletionChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: ChatCompletionChoiceMsg{
						Role:    "assistant",
						Content: &content,
					},
				},
			},
			Usage: &ChatCompletionUsage{
				PromptTokens:     5,
				CompletionTokens: 8,
				TotalTokens:      13,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	adapter := NewChatCompletionsAdapter(server.URL+"/v1", "test-key")

	resp, err := adapter.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "How are you?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Status)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output, got %d", len(resp.Output))
	}
	if resp.Output[0].Content[0].Text != "I'm doing well, thanks!" {
		t.Errorf("expected response text, got %s", resp.Output[0].Content[0].Text)
	}
	if resp.Usage.InputTokens != 5 {
		t.Errorf("expected 5 input tokens, got %d", resp.Usage.InputTokens)
	}
}

// --- Streaming integration test ---

func TestCreateResponseStream_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var chatReq ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if !chatReq.Stream {
			t.Error("expected stream true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

		// Send chunks with text content
		chunks := []ChatCompletionChunk{
			{
				ID:      "chatcmpl-stream",
				Object:  "chat.completion.chunk",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: ChatCompletionChunkDelta{
							Role: "assistant",
						},
					},
				},
			},
			{
				ID:      "chatcmpl-stream",
				Object:  "chat.completion.chunk",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: ChatCompletionChunkDelta{
							Content: strPtr("Hello"),
						},
					},
				},
			},
			{
				ID:      "chatcmpl-stream",
				Object:  "chat.completion.chunk",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: ChatCompletionChunkDelta{
							Content: strPtr(", world!"),
						},
					},
				},
			},
			{
				ID:      "chatcmpl-stream",
				Object:  "chat.completion.chunk",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        ChatCompletionChunkDelta{},
						FinishReason: strPtr("stop"),
					},
				},
				Usage: &ChatCompletionUsage{
					PromptTokens:     5,
					CompletionTokens: 3,
					TotalTokens:      8,
				},
			},
		}

		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	adapter := NewChatCompletionsAdapter(server.URL+"/v1", "")

	eventsChan, err := adapter.CreateResponseStream(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "Hi",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []ResponsesStreamEvent
	for evt := range eventsChan {
		events = append(events, evt)
	}

	// Should have text deltas and a completed event
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// Check text deltas
	textDeltaCount := 0
	var completedEvent *ResponsesStreamEvent
	for i, evt := range events {
		if evt.Type == "response.output_text.delta" {
			textDeltaCount++

			// Verify type field is present in data payload (OpenAI SDK reads type from data)
			var dataMap map[string]interface{}
			if err := json.Unmarshal(evt.Data, &dataMap); err != nil {
				t.Fatalf("failed to parse delta data: %v", err)
			}
			if dataMap["type"] != "response.output_text.delta" {
				t.Errorf("expected type field in data payload, got %v", dataMap["type"])
			}
		}
		if evt.Type == "response.completed" {
			completedEvent = &events[i]
		}
	}

	if textDeltaCount != 2 {
		t.Errorf("expected 2 text delta events, got %d", textDeltaCount)
	}

	if completedEvent == nil {
		t.Fatal("expected response.completed event")
	}

	// Verify type field in completed event data
	var completedMap map[string]interface{}
	if err := json.Unmarshal(completedEvent.Data, &completedMap); err != nil {
		t.Fatalf("failed to parse completed data: %v", err)
	}
	if completedMap["type"] != "response.completed" {
		t.Errorf("expected type field in completed data, got %v", completedMap["type"])
	}

	// Parse completed event to verify accumulated text
	var wrapper struct {
		Response ResponsesAPIResponse `json:"response"`
	}
	if err := json.Unmarshal(completedEvent.Data, &wrapper); err != nil {
		t.Fatalf("failed to parse completed event: %v", err)
	}

	if len(wrapper.Response.Output) != 1 {
		t.Fatalf("expected 1 output in completed, got %d", len(wrapper.Response.Output))
	}
	if wrapper.Response.Output[0].Content[0].Text != "Hello, world!" {
		t.Errorf("expected accumulated text 'Hello, world!', got %s", wrapper.Response.Output[0].Content[0].Text)
	}
	if wrapper.Response.Usage == nil {
		t.Fatal("expected usage in completed event")
	}
	if wrapper.Response.Usage.InputTokens != 5 {
		t.Errorf("expected 5 input tokens, got %d", wrapper.Response.Usage.InputTokens)
	}
}

// --- Streaming with tool calls ---

func TestCreateResponseStream_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		idx0 := 0
		chunks := []ChatCompletionChunk{
			{
				ID:      "chatcmpl-tc",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: ChatCompletionChunkDelta{
							Role: "assistant",
							ToolCalls: []ChatCompletionToolCall{
								{
									Index: &idx0,
									ID:    "call_abc",
									Type:  "function",
									Function: ChatCompletionToolCallFunction{
										Name:      "get_weather",
										Arguments: "",
									},
								},
							},
						},
					},
				},
			},
			{
				ID:      "chatcmpl-tc",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: ChatCompletionChunkDelta{
							ToolCalls: []ChatCompletionToolCall{
								{
									Index: &idx0,
									Function: ChatCompletionToolCallFunction{
										Arguments: `{"loc`,
									},
								},
							},
						},
					},
				},
			},
			{
				ID:      "chatcmpl-tc",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: ChatCompletionChunkDelta{
							ToolCalls: []ChatCompletionToolCall{
								{
									Index: &idx0,
									Function: ChatCompletionToolCallFunction{
										Arguments: `ation":"NYC"}`,
									},
								},
							},
						},
					},
				},
			},
			{
				ID:      "chatcmpl-tc",
				Model:   "test-model",
				Created: 1700000000,
				Choices: []ChatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        ChatCompletionChunkDelta{},
						FinishReason: strPtr("tool_calls"),
					},
				},
			},
		}

		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	adapter := NewChatCompletionsAdapter(server.URL+"/v1", "")

	eventsChan, err := adapter.CreateResponseStream(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "What's the weather in NYC?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []ResponsesStreamEvent
	for evt := range eventsChan {
		events = append(events, evt)
	}

	// Check for function_call_arguments.delta events
	argDeltaCount := 0
	for _, evt := range events {
		if evt.Type == "response.function_call_arguments.delta" {
			argDeltaCount++

			// Verify type field is present in data payload
			var dataMap map[string]interface{}
			if err := json.Unmarshal(evt.Data, &dataMap); err != nil {
				t.Fatalf("failed to parse delta data: %v", err)
			}
			if dataMap["type"] != "response.function_call_arguments.delta" {
				t.Errorf("expected type field in data payload, got %v", dataMap["type"])
			}
		}
	}
	if argDeltaCount != 2 {
		t.Errorf("expected 2 function_call_arguments.delta events, got %d", argDeltaCount)
	}

	// Verify completed event has the tool call
	var completedEvt *ResponsesStreamEvent
	for i, evt := range events {
		if evt.Type == "response.completed" {
			completedEvt = &events[i]
			break
		}
	}
	if completedEvt == nil {
		t.Fatal("expected response.completed event")
	}

	var wrapper struct {
		Response ResponsesAPIResponse `json:"response"`
	}
	if err := json.Unmarshal(completedEvt.Data, &wrapper); err != nil {
		t.Fatalf("failed to parse completed event: %v", err)
	}

	if len(wrapper.Response.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(wrapper.Response.Output))
	}
	if wrapper.Response.Output[0].Type != "function_call" {
		t.Errorf("expected function_call, got %s", wrapper.Response.Output[0].Type)
	}
	if wrapper.Response.Output[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", wrapper.Response.Output[0].Name)
	}
	if wrapper.Response.Output[0].Arguments != `{"location":"NYC"}` {
		t.Errorf("expected accumulated arguments, got %s", wrapper.Response.Output[0].Arguments)
	}
}

// --- Error handling ---

func TestCreateResponse_BackendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	adapter := NewChatCompletionsAdapter(server.URL+"/v1", "")

	_, err := adapter.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "test",
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestCreateResponse_AuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret-key" {
			t.Errorf("expected Bearer my-secret-key, got %s", auth)
		}
		content := "ok"
		resp := ChatCompletionResponse{
			ID:    "test",
			Model: "m",
			Choices: []ChatCompletionChoice{{
				FinishReason: "stop",
				Message:      ChatCompletionChoiceMsg{Role: "assistant", Content: &content},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	adapter := NewChatCompletionsAdapter(server.URL+"/v1", "my-secret-key")
	_, err := adapter.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "m",
		Input: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Edge cases ---

func TestConvertFromChatResponse_EmptyContent(t *testing.T) {
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-empty",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: nil,
				},
			},
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if len(resp.Output) != 0 {
		t.Errorf("expected 0 output items for nil content, got %d", len(resp.Output))
	}
}

func TestConvertFromChatResponse_EmptyString(t *testing.T) {
	empty := ""
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-emptystr",
		Model:   "gpt-4",
		Created: 1700000000,
		Choices: []ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: &empty,
				},
			},
		},
	}

	resp := ConvertFromChatResponse(chatResp)

	if len(resp.Output) != 0 {
		t.Errorf("expected 0 output items for empty string, got %d", len(resp.Output))
	}
}

func TestConvertInputToMessages_Nil(t *testing.T) {
	msgs := convertInputToMessages(nil)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for nil input, got %d", len(msgs))
	}
}

func strPtr(s string) *string {
	return &s
}
