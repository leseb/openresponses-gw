// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatCompletionsAdapter implements ResponsesAPIClient by calling /v1/chat/completions
// and translating between Responses API types and Chat Completions types.
type ChatCompletionsAdapter struct {
	baseURL    string // e.g. "http://localhost:8000/v1"
	apiKey     string
	httpClient *http.Client
}

// NewChatCompletionsAdapter creates a new Chat Completions adapter.
// baseURL should include the /v1 prefix (e.g. "http://localhost:8000/v1").
func NewChatCompletionsAdapter(baseURL, apiKey string) *ChatCompletionsAdapter {
	return &ChatCompletionsAdapter{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// CreateResponse sends a non-streaming request to /v1/chat/completions
// and converts the response back to ResponsesAPIResponse.
func (a *ChatCompletionsAdapter) CreateResponse(ctx context.Context, req *ResponsesAPIRequest) (*ResponsesAPIResponse, error) {
	chatReq := ConvertToChatRequest(req)
	chatReq.Stream = false

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	a.setHeaders(httpReq)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request to backend failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chat response: %w", err)
	}

	return ConvertFromChatResponse(&chatResp), nil
}

// CreateResponseStream sends a streaming request to /v1/chat/completions
// and converts the SSE stream into ResponsesStreamEvent channel.
func (a *ChatCompletionsAdapter) CreateResponseStream(ctx context.Context, req *ResponsesAPIRequest) (<-chan ResponsesStreamEvent, error) {
	chatReq := ConvertToChatRequest(req)
	chatReq.Stream = true
	chatReq.StreamOptions = &ChatStreamOptions{IncludeUsage: true}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	a.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request to backend failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(respBody))
	}

	events := make(chan ResponsesStreamEvent, 10)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		a.processSSEStream(ctx, resp.Body, req.Model, events)
	}()

	return events, nil
}

// processSSEStream reads the SSE stream from Chat Completions and emits
// ResponsesStreamEvent events that the engine expects.
func (a *ChatCompletionsAdapter) processSSEStream(
	ctx context.Context,
	body io.Reader,
	model string,
	events chan<- ResponsesStreamEvent,
) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Accumulators for building the final response
	var responseID string
	var responseModel string
	var responseCreated int64
	accumulatedText := make(map[int]string)                    // output_index → text
	accumulatedToolCalls := make(map[int]*accumulatedToolCall) // tool_call index → accumulated data
	var usage *ChatCompletionUsage
	var finishReason string

	// Track which output items we've assigned IDs to
	messageItemID := ""
	toolCallItemIDs := make(map[int]string) // tool_call index → item ID

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Capture response metadata
		if responseID == "" {
			responseID = chunk.ID
		}
		if responseModel == "" {
			responseModel = chunk.Model
		}
		if responseCreated == 0 {
			responseCreated = chunk.Created
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Process text content delta
		if delta.Content != nil && *delta.Content != "" {
			outputIndex := 0 // text is always output_index 0

			if messageItemID == "" {
				messageItemID = adapterGenerateID("msg_")
			}

			accumulatedText[outputIndex] += *delta.Content

			// Emit response.output_text.delta
			deltaEvt := map[string]interface{}{
				"type":          "response.output_text.delta",
				"output_index":  outputIndex,
				"content_index": 0,
				"item_id":       messageItemID,
				"delta":         *delta.Content,
				"response_id":   responseID,
			}
			deltaData, _ := json.Marshal(deltaEvt)

			select {
			case events <- ResponsesStreamEvent{
				Type: "response.output_text.delta",
				Data: deltaData,
			}:
			case <-ctx.Done():
				return
			}
		}

		// Process tool call deltas
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}

			acc, exists := accumulatedToolCalls[idx]
			if !exists {
				acc = &accumulatedToolCall{}
				accumulatedToolCalls[idx] = acc
			}

			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.arguments += tc.Function.Arguments

			// Assign an item ID for this tool call
			if _, ok := toolCallItemIDs[idx]; !ok {
				toolCallItemIDs[idx] = adapterGenerateID("fc_")
			}

			// Emit response.function_call_arguments.delta
			if tc.Function.Arguments != "" {
				outputIndex := idx + 1 // tool calls start after the message at index 0
				if messageItemID == "" && len(accumulatedText) == 0 {
					// No text output, tool calls start at 0
					outputIndex = idx
				}

				deltaEvt := map[string]interface{}{
					"type":         "response.function_call_arguments.delta",
					"output_index": outputIndex,
					"item_id":      toolCallItemIDs[idx],
					"delta":        tc.Function.Arguments,
					"response_id":  responseID,
				}
				deltaData, _ := json.Marshal(deltaEvt)

				select {
				case events <- ResponsesStreamEvent{
					Type: "response.function_call_arguments.delta",
					Data: deltaData,
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		// Track finish reason
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	// Build the final ResponsesAPIResponse for response.completed
	finalResp := buildFinalResponse(
		responseID, responseModel, responseCreated,
		messageItemID, accumulatedText,
		toolCallItemIDs, accumulatedToolCalls,
		usage, finishReason,
	)

	completedEvt := map[string]interface{}{
		"type":     "response.completed",
		"response": finalResp,
	}
	completedData, _ := json.Marshal(completedEvt)

	select {
	case events <- ResponsesStreamEvent{
		Type: "response.completed",
		Data: completedData,
	}:
	case <-ctx.Done():
	}
}

type accumulatedToolCall struct {
	id        string
	name      string
	arguments string
}

func (a *ChatCompletionsAdapter) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
}

// ConvertToChatRequest converts a ResponsesAPIRequest to a ChatCompletionRequest.
func ConvertToChatRequest(req *ResponsesAPIRequest) *ChatCompletionRequest {
	chatReq := &ChatCompletionRequest{
		Model:             req.Model,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxTokens:         req.MaxOutputTokens,
		FrequencyPenalty:  req.FrequencyPenalty,
		PresencePenalty:   req.PresencePenalty,
		ParallelToolCalls: req.ParallelToolCalls,
		Seed:              req.Seed,
		Stop:              req.Stop,
	}

	// Handle logprobs
	if req.TopLogprobs != nil {
		logprobsTrue := true
		chatReq.Logprobs = &logprobsTrue
		chatReq.TopLogprobs = req.TopLogprobs
	}

	// Convert instructions to system message
	var messages []ChatCompletionMsg
	if req.Instructions != nil && *req.Instructions != "" {
		messages = append(messages, ChatCompletionMsg{
			Role:    "system",
			Content: *req.Instructions,
		})
	}

	// Convert input to messages
	messages = append(messages, convertInputToMessages(req.Input)...)
	chatReq.Messages = messages

	// Convert tools
	chatReq.Tools = convertToolsToChatTools(req.Tools)

	// Convert tool choice — only if there are tools to send.
	// Non-function tools (web_search, file_search) are stripped by convertToolsToChatTools,
	// so tool_choice would be orphaned and rejected by the backend.
	if len(chatReq.Tools) > 0 {
		chatReq.ToolChoice = convertToolChoice(req.ToolChoice)
	}

	return chatReq
}

// convertInputToMessages converts Responses API input to Chat Completions messages.
// Input can be a string, or []interface{} of structured items.
func convertInputToMessages(input interface{}) []ChatCompletionMsg {
	if input == nil {
		return nil
	}

	switch v := input.(type) {
	case string:
		return []ChatCompletionMsg{{Role: "user", Content: v}}
	case []interface{}:
		return convertInputItemsToMessages(v)
	default:
		return []ChatCompletionMsg{{Role: "user", Content: fmt.Sprintf("%v", v)}}
	}
}

// convertInputItemsToMessages converts Responses API input items to Chat Completions messages.
// It handles merging consecutive function_call items into a single assistant message.
func convertInputItemsToMessages(items []interface{}) []ChatCompletionMsg {
	var messages []ChatCompletionMsg
	var pendingToolCalls []ChatCompletionToolCall

	flushToolCalls := func() {
		if len(pendingToolCalls) > 0 {
			messages = append(messages, ChatCompletionMsg{
				Role:      "assistant",
				ToolCalls: pendingToolCalls,
			})
			pendingToolCalls = nil
		}
	}

	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		itemType, _ := itemMap["type"].(string)
		role, _ := itemMap["role"].(string)

		switch {
		case itemType == "function_call":
			// Accumulate consecutive function calls
			callID, _ := itemMap["call_id"].(string)
			name, _ := itemMap["name"].(string)
			arguments, _ := itemMap["arguments"].(string)
			pendingToolCalls = append(pendingToolCalls, ChatCompletionToolCall{
				ID:   callID,
				Type: "function",
				Function: ChatCompletionToolCallFunction{
					Name:      name,
					Arguments: arguments,
				},
			})

		case itemType == "function_call_output":
			// Flush any pending tool calls before tool result
			flushToolCalls()
			callID, _ := itemMap["call_id"].(string)
			output, _ := itemMap["output"].(string)
			messages = append(messages, ChatCompletionMsg{
				Role:       "tool",
				Content:    output,
				ToolCallID: callID,
			})

		case itemType == "message" || (itemType == "" && role != ""):
			// Flush any pending tool calls before a new message
			flushToolCalls()
			msg := convertItemToMessage(itemMap, role)
			if msg != nil {
				messages = append(messages, *msg)
			}

		default:
			// Try simple {role, content} format
			flushToolCalls()
			if content, ok := itemMap["content"].(string); ok && content != "" {
				if role == "" {
					role = "user"
				}
				if role == "developer" {
					role = "system"
				}
				messages = append(messages, ChatCompletionMsg{
					Role:    role,
					Content: content,
				})
			}
		}
	}

	flushToolCalls()
	return messages
}

// convertItemToMessage converts a single Responses API input item to a Chat Completions message.
func convertItemToMessage(item map[string]interface{}, role string) *ChatCompletionMsg {
	if role == "" {
		role, _ = item["role"].(string)
	}
	if role == "" {
		return nil
	}

	// Map "developer" to "system" for Chat Completions compatibility
	if role == "developer" {
		role = "system"
	}

	// Direct content string
	if content, ok := item["content"].(string); ok {
		if content == "" {
			return nil
		}
		return &ChatCompletionMsg{Role: role, Content: content}
	}

	// Content as array of parts (multimodal)
	contentArr, ok := item["content"].([]interface{})
	if !ok || len(contentArr) == 0 {
		return nil
	}

	var textParts []string
	var contentParts []ChatCompletionContentPart
	hasNonText := false

	for _, part := range contentArr {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		partType, _ := partMap["type"].(string)
		switch partType {
		case "input_text", "text":
			text, _ := partMap["text"].(string)
			textParts = append(textParts, text)
			contentParts = append(contentParts, ChatCompletionContentPart{
				Type: "text",
				Text: text,
			})
		case "input_image":
			hasNonText = true
			var imgURL string
			switch v := partMap["image_url"].(type) {
			case string:
				imgURL = v
			case map[string]interface{}:
				imgURL, _ = v["url"].(string)
			}
			if imgURL == "" {
				imgURL, _ = partMap["url"].(string)
			}
			if imgURL != "" {
				detail, _ := partMap["detail"].(string)
				contentParts = append(contentParts, ChatCompletionContentPart{
					Type: "image_url",
					ImageURL: &ChatCompletionImageURL{
						URL:    imgURL,
						Detail: detail,
					},
				})
			}
		case "input_file":
			hasNonText = true
			file := &ChatCompletionFile{}
			if fileMap, ok := partMap["file"].(map[string]interface{}); ok {
				file.FileData, _ = fileMap["file_data"].(string)
				file.FileID, _ = fileMap["file_id"].(string)
				file.Filename, _ = fileMap["filename"].(string)
			} else {
				file.FileData, _ = partMap["file_data"].(string)
				file.FileID, _ = partMap["file_id"].(string)
				file.Filename, _ = partMap["filename"].(string)
			}
			if file.FileData != "" || file.FileID != "" {
				contentParts = append(contentParts, ChatCompletionContentPart{
					Type: "file",
					File: file,
				})
			}
		}
	}

	if len(contentParts) == 0 {
		return nil
	}

	// When all parts are text, use simple string content
	if !hasNonText {
		text := strings.Join(textParts, " ")
		if text == "" {
			return nil
		}
		return &ChatCompletionMsg{Role: role, Content: text}
	}

	return &ChatCompletionMsg{Role: role, Content: contentParts}
}

// convertToolsToChatTools converts Responses API ToolParams to Chat Completions tools.
func convertToolsToChatTools(tools []ToolParam) []ChatCompletionTool {
	if len(tools) == 0 {
		return nil
	}

	var chatTools []ChatCompletionTool
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		desc := ""
		if t.Description != nil {
			desc = *t.Description
		}
		chatTools = append(chatTools, ChatCompletionTool{
			Type: "function",
			Function: ChatCompletionToolFunction{
				Name:        t.Name,
				Description: desc,
				Parameters:  t.Parameters,
				Strict:      t.Strict,
			},
		})
	}
	return chatTools
}

// convertToolChoice converts Responses API tool_choice to Chat Completions format.
func convertToolChoice(toolChoice interface{}) interface{} {
	if toolChoice == nil {
		return nil
	}

	// String values: "auto", "none", "required" pass through directly
	if s, ok := toolChoice.(string); ok {
		return s
	}

	// Object format: {type: "function", name: "foo"} →
	// {type: "function", function: {name: "foo"}}
	if m, ok := toolChoice.(map[string]interface{}); ok {
		if t, _ := m["type"].(string); t == "function" {
			name, _ := m["name"].(string)
			return map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": name,
				},
			}
		}
	}

	return toolChoice
}

// ConvertFromChatResponse converts a ChatCompletionResponse to ResponsesAPIResponse.
func ConvertFromChatResponse(chatResp *ChatCompletionResponse) *ResponsesAPIResponse {
	resp := &ResponsesAPIResponse{
		ID:        chatResp.ID,
		Object:    "response",
		Model:     chatResp.Model,
		CreatedAt: float64(chatResp.Created),
		Status:    "completed",
	}

	var output []OutputItem

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]

		// Map finish_reason to status
		switch choice.FinishReason {
		case "length":
			resp.Status = "incomplete"
		default:
			resp.Status = "completed"
		}

		// Convert text content
		if choice.Message.Content != nil && *choice.Message.Content != "" {
			output = append(output, OutputItem{
				Type:   "message",
				ID:     adapterGenerateID("msg_"),
				Role:   "assistant",
				Status: "completed",
				Content: []ContentItem{{
					Type: "output_text",
					Text: *choice.Message.Content,
				}},
			})
		}

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			output = append(output, OutputItem{
				Type:      "function_call",
				ID:        adapterGenerateID("fc_"),
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
				Status:    "completed",
			})
		}
	}

	resp.Output = output

	// Convert usage
	if chatResp.Usage != nil {
		resp.Usage = &UsageInfo{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		}
	}

	return resp
}

// buildFinalResponse constructs the final ResponsesAPIResponse from accumulated stream data.
func buildFinalResponse(
	responseID, model string, created int64,
	messageItemID string,
	accumulatedText map[int]string,
	toolCallItemIDs map[int]string,
	accumulatedToolCalls map[int]*accumulatedToolCall,
	usage *ChatCompletionUsage,
	finishReason string,
) *ResponsesAPIResponse {
	resp := &ResponsesAPIResponse{
		ID:        responseID,
		Object:    "response",
		Model:     model,
		CreatedAt: float64(created),
		Status:    "completed",
	}

	// Map finish_reason to status
	if finishReason == "length" {
		resp.Status = "incomplete"
	}

	var output []OutputItem

	// Add text output
	if text, ok := accumulatedText[0]; ok && text != "" {
		if messageItemID == "" {
			messageItemID = adapterGenerateID("msg_")
		}
		output = append(output, OutputItem{
			Type:   "message",
			ID:     messageItemID,
			Role:   "assistant",
			Status: "completed",
			Content: []ContentItem{{
				Type: "output_text",
				Text: text,
			}},
		})
	}

	// Add tool call outputs
	for idx := 0; idx < len(accumulatedToolCalls); idx++ {
		tc, ok := accumulatedToolCalls[idx]
		if !ok {
			continue
		}
		itemID := toolCallItemIDs[idx]
		if itemID == "" {
			itemID = adapterGenerateID("fc_")
		}
		output = append(output, OutputItem{
			Type:      "function_call",
			ID:        itemID,
			CallID:    tc.id,
			Name:      tc.name,
			Arguments: tc.arguments,
			Status:    "completed",
		})
	}

	resp.Output = output

	// Convert usage
	if usage != nil {
		resp.Usage = &UsageInfo{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			TotalTokens:  usage.TotalTokens,
		}
	}

	if resp.CreatedAt == 0 {
		resp.CreatedAt = float64(time.Now().Unix())
	}

	return resp
}

// adapterGenerateID generates a random ID with the given prefix.
func adapterGenerateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
