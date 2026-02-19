// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/core/state"
	"github.com/leseb/openresponses-gw/pkg/mcp"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
)

const defaultMaxToolCalls = 10

// ConnectorLookup provides read access to registered connectors.
type ConnectorLookup interface {
	GetConnector(ctx context.Context, connectorID string) (*memory.Connector, error)
}

// VectorSearcher performs vector similarity search.
// Implemented by services.VectorStoreService.
type VectorSearcher interface {
	Search(ctx context.Context, vectorStoreID, query string, topK int) ([]vectorstore.SearchResult, error)
}

// Engine is the core orchestration engine for the Responses API.
// It calls a /v1/responses-compatible backend for inference and adds
// persistence, conversations, MCP tools, file_search, and prompts.
type Engine struct {
	config       *config.EngineConfig
	sessions     state.SessionStore
	llm          api.ResponsesAPIClient
	connectors   ConnectorLookup // nil-safe: nil means no MCP support
	vectorSearch VectorSearcher  // nil-safe: nil means no file_search support
}

// New creates a new Engine instance.
// The connectors and vectorSearch parameters are optional (nil disables the feature).
func New(cfg *config.EngineConfig, store state.SessionStore, connectors ConnectorLookup, vectorSearch VectorSearcher) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if store == nil {
		return nil, fmt.Errorf("session store is required")
	}

	// Create backend API client
	if cfg.ModelEndpoint == "" {
		return nil, fmt.Errorf("model endpoint is required (set OPENAI_API_ENDPOINT)")
	}
	var llm api.ResponsesAPIClient
	if cfg.BackendAPI == "responses" {
		llm = api.NewOpenAIResponsesClient(cfg.ModelEndpoint, cfg.APIKey)
	} else {
		llm = api.NewChatCompletionsAdapter(cfg.ModelEndpoint, cfg.APIKey)
	}

	return &Engine{
		config:       cfg,
		sessions:     store,
		llm:          llm,
		connectors:   connectors,
		vectorSearch: vectorSearch,
	}, nil
}

// Store returns the session store
func (e *Engine) Store() state.SessionStore {
	return e.sessions
}

// BackendAPI returns the configured backend API mode ("chat_completions" or "responses").
func (e *Engine) BackendAPI() string {
	return e.config.BackendAPI
}

// echoRequestParams copies all request parameters to the response (Open Responses spec)
func echoRequestParams(resp *schema.Response, req *schema.ResponseRequest) {
	resp.PreviousResponseID = req.PreviousResponseID
	resp.Conversation = req.Conversation
	resp.Instructions = req.Instructions
	resp.Tools = convertToolsToResponse(req.Tools)
	if req.ToolChoice != nil {
		resp.ToolChoice = req.ToolChoice
	}
	resp.Reasoning = convertReasoningToResponse(req.Reasoning)
	if req.Temperature != nil {
		resp.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		resp.TopP = *req.TopP
	}
	resp.MaxOutputTokens = req.MaxOutputTokens
	resp.MaxToolCalls = req.MaxToolCalls
	if req.FrequencyPenalty != nil {
		resp.FrequencyPenalty = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		resp.PresencePenalty = *req.PresencePenalty
	}
	resp.Metadata = req.Metadata

	// Inference parameters (forwarded to and handled by vLLM)
	if req.Truncation != nil {
		resp.Truncation = *req.Truncation
	}
	if req.ParallelToolCalls != nil {
		resp.ParallelToolCalls = *req.ParallelToolCalls
	}
	if req.Text != nil {
		resp.Text = *req.Text
	}
	if req.TopLogprobs != nil {
		resp.TopLogprobs = *req.TopLogprobs
	}
	// Gateway-managed persistence flag
	if req.Store != nil {
		resp.Store = *req.Store
	}
}

// extractInputMessages parses the Responses API input field into chat messages
func extractInputMessages(input interface{}) []api.Message {
	switch v := input.(type) {
	case string:
		return []api.Message{{Role: "user", Content: v}}
	case []interface{}:
		var messages []api.Message
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			itemType, _ := itemMap["type"].(string)
			switch itemType {
			case "message":
				role, _ := itemMap["role"].(string)
				if role == "" {
					continue
				}
				msg := extractMessageFromItem(itemMap, role)
				if msg != nil {
					messages = append(messages, *msg)
				}
			case "function_call_output":
				callID, _ := itemMap["call_id"].(string)
				output, _ := itemMap["output"].(string)
				if callID != "" {
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    output,
						ToolCallID: callID,
					})
				}
			case "function_call":
				callID, _ := itemMap["call_id"].(string)
				name, _ := itemMap["name"].(string)
				arguments, _ := itemMap["arguments"].(string)
				if name != "" {
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{
							{
								ID:   callID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      name,
									Arguments: arguments,
								},
							},
						},
					})
				}
			default:
				// Try to extract content for unknown types
				if content, ok := itemMap["content"].(string); ok && content != "" {
					role, _ := itemMap["role"].(string)
					if role == "" {
						role = "user"
					}
					messages = append(messages, api.Message{Role: role, Content: content})
				}
			}
		}
		if len(messages) == 0 {
			return []api.Message{{Role: "user", Content: fmt.Sprintf("%v", input)}}
		}
		return messages
	default:
		return []api.Message{{Role: "user", Content: fmt.Sprintf("%v", v)}}
	}
}

// extractMessageFromItem extracts a Message from a message input item,
// handling both text-only and multimodal (image/file) content parts.
func extractMessageFromItem(item map[string]interface{}, role string) *api.Message {
	// Direct content string
	if content, ok := item["content"].(string); ok {
		if content == "" {
			return nil
		}
		return &api.Message{Role: role, Content: content}
	}
	// Content as array of parts
	if contentArr, ok := item["content"].([]interface{}); ok {
		var textParts []string
		var contentParts []api.MessageContentPart
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
				contentParts = append(contentParts, api.MessageContentPart{
					Type: "text",
					Text: text,
				})
			case "input_image":
				hasNonText = true
				cp := api.MessageContentPart{Type: "image_url"}
				// Responses API: image_url can be a string or an object
				switch v := partMap["image_url"].(type) {
				case string:
					cp.ImageURL = &api.MessageImageURL{URL: v}
				case map[string]interface{}:
					url, _ := v["url"].(string)
					detail, _ := v["detail"].(string)
					cp.ImageURL = &api.MessageImageURL{URL: url, Detail: detail}
				}
				if cp.ImageURL == nil {
					// Try direct "url" field
					if url, ok := partMap["url"].(string); ok {
						cp.ImageURL = &api.MessageImageURL{URL: url}
					}
				}
				if detail, ok := partMap["detail"].(string); ok && cp.ImageURL != nil && cp.ImageURL.Detail == "" {
					cp.ImageURL.Detail = detail
				}
				contentParts = append(contentParts, cp)
			case "input_file":
				hasNonText = true
				cp := api.MessageContentPart{Type: "file"}
				file := &api.MessageFile{}
				// Responses API: file info can be nested under "file" or at top level
				if fileMap, ok := partMap["file"].(map[string]interface{}); ok {
					file.FileData, _ = fileMap["file_data"].(string)
					file.FileID, _ = fileMap["file_id"].(string)
					file.Filename, _ = fileMap["filename"].(string)
				} else {
					file.FileData, _ = partMap["file_data"].(string)
					file.FileID, _ = partMap["file_id"].(string)
					file.Filename, _ = partMap["filename"].(string)
				}
				cp.File = file
				contentParts = append(contentParts, cp)
			}
		}

		if len(contentParts) == 0 {
			return nil
		}

		// When all parts are text, use the simple Content string for backward compat
		if !hasNonText {
			text := ""
			for i, t := range textParts {
				if i > 0 {
					text += " "
				}
				text += t
			}
			if text == "" {
				return nil
			}
			return &api.Message{Role: role, Content: text}
		}

		return &api.Message{Role: role, ContentParts: contentParts}
	}
	return nil
}

// convertToToolParams converts Responses API tool params to backend ToolParams.
// Only function tools are forwarded; MCP and file_search are already expanded.
func convertToToolParams(tools []schema.ResponsesToolParam) []api.ToolParam {
	var result []api.ToolParam
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		result = append(result, api.ToolParam{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Strict:      t.Strict,
		})
	}
	return result
}

// outputToItemFields converts a stored Output (which may be []interface{} after
// JSON round-tripping through the database) back into []schema.ItemField.
func outputToItemFields(output interface{}) []schema.ItemField {
	if items, ok := output.([]schema.ItemField); ok {
		return items
	}
	data, err := json.Marshal(output)
	if err != nil {
		return nil
	}
	var items []schema.ItemField
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	return items
}

// buildResponsesAPIRequest constructs a ResponsesAPIRequest from conversation
// messages and the original request parameters.
func buildResponsesAPIRequest(model string, messages []api.Message, req *schema.ResponseRequest, tools []schema.ResponsesToolParam, stream bool) *api.ResponsesAPIRequest {
	// Convert messages to Responses API input, skipping system messages
	// (instructions are passed via the Instructions field)
	input := convertMessagesToResponsesInput(messages)

	// Only include function tools (MCP/file_search already expanded)
	funcTools := convertToToolParams(tools)

	// Gateway owns storage, not the backend
	storeFalse := false

	apiReq := &api.ResponsesAPIRequest{
		Model:  model,
		Input:  input,
		Tools:  funcTools,
		Stream: stream,
		Store:  &storeFalse,
	}

	// Pass instructions from request
	apiReq.Instructions = req.Instructions

	// Sampling parameters
	apiReq.Temperature = req.Temperature
	apiReq.TopP = req.TopP
	apiReq.FrequencyPenalty = req.FrequencyPenalty
	apiReq.PresencePenalty = req.PresencePenalty
	apiReq.MaxOutputTokens = req.MaxOutputTokens

	// Tool choice
	apiReq.ToolChoice = req.ToolChoice

	// Inference parameters forwarded to vLLM
	apiReq.Truncation = req.Truncation
	apiReq.ParallelToolCalls = req.ParallelToolCalls
	if req.Text != nil {
		apiReq.Text = req.Text
	}
	apiReq.Include = req.Include
	apiReq.TopLogprobs = req.TopLogprobs

	// Reasoning
	if req.Reasoning != nil && req.Reasoning.Effort != nil {
		apiReq.Reasoning = &api.ReasoningParam{
			Effort: req.Reasoning.Effort,
		}
	}

	return apiReq
}

// convertMessagesToResponsesInput converts internal Messages to the Responses
// API input format. System messages are skipped (handled by the Instructions field).
//
// vLLM compatibility: vLLM's /v1/responses endpoint only accepts the simple
// {role, content} format for user and assistant text messages. The structured
// Responses API format ({type:"message", role:"user", content:[{type:"input_text",
// text:"..."}]}) causes vLLM to return 400 errors with Pydantic validation
// failures in multi-turn conversations. We therefore use the simple chat-style
// format for plain text messages, and only use the structured format for
// multimodal content (images, files) and tool calls (function_call,
// function_call_output) which require it.
func convertMessagesToResponsesInput(messages []api.Message) []interface{} {
	var input []interface{}
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// Skip — instructions are passed separately via the Instructions field
			continue
		case "user":
			if len(msg.ContentParts) > 0 {
				// Multimodal content — use structured format
				var parts []map[string]interface{}
				for _, cp := range msg.ContentParts {
					switch cp.Type {
					case "text":
						parts = append(parts, map[string]interface{}{
							"type": "input_text",
							"text": cp.Text,
						})
					case "image_url":
						if cp.ImageURL != nil {
							parts = append(parts, map[string]interface{}{
								"type":      "input_image",
								"image_url": cp.ImageURL.URL,
							})
						}
					case "file":
						if cp.File != nil {
							fileMap := map[string]interface{}{}
							if cp.File.FileData != "" {
								fileMap["file_data"] = cp.File.FileData
							}
							if cp.File.FileID != "" {
								fileMap["file_id"] = cp.File.FileID
							}
							if cp.File.Filename != "" {
								fileMap["filename"] = cp.File.Filename
							}
							parts = append(parts, map[string]interface{}{
								"type": "input_file",
								"file": fileMap,
							})
						}
					}
				}
				input = append(input, map[string]interface{}{
					"type":    "message",
					"role":    "user",
					"content": parts,
				})
			} else {
				// Simple format: vLLM expects {role, content} for plain text
				input = append(input, map[string]interface{}{
					"role":    "user",
					"content": msg.Content,
				})
			}
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Each tool call becomes a separate function_call input item
				for _, tc := range msg.ToolCalls {
					input = append(input, map[string]interface{}{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			if msg.Content != "" {
				// Simple format: vLLM expects {role, content} for plain text
				input = append(input, map[string]interface{}{
					"role":    "assistant",
					"content": msg.Content,
				})
			}
		case "tool":
			input = append(input, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})
		case "developer":
			input = append(input, map[string]interface{}{
				"role":    "developer",
				"content": msg.Content,
			})
		}
	}
	return input
}

// toolCallInfo holds extracted function call information from backend output.
type toolCallInfo struct {
	ID        string
	Name      string
	Arguments string
	CallID    string
}

// parseResponsesOutput extracts text content and tool calls from backend output items.
func parseResponsesOutput(output []api.OutputItem) (textContent string, toolCalls []toolCallInfo, hasToolCalls bool) {
	for _, item := range output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" || c.Type == "text" {
					textContent += c.Text
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, toolCallInfo{
				ID:        item.ID,
				Name:      item.Name,
				Arguments: item.Arguments,
				CallID:    item.CallID,
			})
			hasToolCalls = true
		}
	}
	return
}

// convertOutputItemsToSchema converts backend OutputItems to schema ItemFields.
func convertOutputItemsToSchema(items []api.OutputItem) []schema.ItemField {
	var result []schema.ItemField
	for _, item := range items {
		switch item.Type {
		case "message":
			role := item.Role
			status := item.Status
			if status == "" {
				status = "completed"
			}
			var content []schema.ContentPart
			for _, c := range item.Content {
				text := c.Text
				cp := schema.ContentPart{
					Type: c.Type,
					Text: &text,
				}
				if c.Type == "output_text" {
					cp.Annotations = make([]schema.Annotation, 0)
					if len(c.Logprobs) > 0 {
						cp.Logprobs = c.Logprobs
					} else {
						cp.Logprobs = make([]interface{}, 0)
					}
				}
				content = append(content, cp)
			}
			result = append(result, schema.ItemField{
				Type:    "message",
				ID:      item.ID,
				Role:    &role,
				Status:  &status,
				Content: content,
			})
		case "function_call":
			name := item.Name
			args := item.Arguments
			callID := item.CallID
			status := item.Status
			if status == "" {
				status = "completed"
			}
			result = append(result, schema.ItemField{
				Type:      "function_call",
				ID:        item.ID,
				Name:      &name,
				Arguments: &args,
				CallID:    &callID,
				Status:    &status,
			})
		case "function_call_output":
			callID := item.CallID
			output := item.Output
			result = append(result, schema.ItemField{
				Type:   "function_call_output",
				ID:     item.ID,
				CallID: &callID,
				Output: &output,
			})
		}
	}
	return result
}

// patchResponseID replaces the response_id field in a raw JSON event
// with the gateway's own response ID.
func patchResponseID(data json.RawMessage, newResponseID string) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return data
	}
	if _, ok := m["response_id"]; ok {
		quoted, _ := json.Marshal(newResponseID)
		m["response_id"] = quoted
		patched, err := json.Marshal(m)
		if err != nil {
			return data
		}
		return patched
	}
	return data
}

// emitOutputItemAddedIfNeeded emits a response.output_item.added event if
// the given output_index hasn't been announced yet. The OpenAI Python SDK
// expects this event before any delta events for that output index.
func emitOutputItemAddedIfNeeded(
	events chan<- interface{},
	announced map[int]string,
	outputIndex int,
	itemID string,
	itemType string,
	seqNum int,
) int {
	if _, ok := announced[outputIndex]; ok {
		return seqNum
	}
	if itemID == "" {
		if itemType == "function_call" {
			itemID = generateID("fc_")
		} else {
			itemID = generateID("msg_")
		}
	}
	announced[outputIndex] = itemID

	role := "assistant"
	status := "in_progress"
	item := schema.ItemField{
		Type:    itemType,
		ID:      itemID,
		Content: make([]schema.ContentPart, 0),
	}
	if itemType == "message" {
		item.Role = &role
		item.Status = &status
	}

	events <- &schema.ResponseOutputItemAddedStreamingEvent{
		Type:           "response.output_item.added",
		SequenceNumber: seqNum,
		OutputIndex:    outputIndex,
		Item:           item,
	}
	return seqNum + 1
}

// emitContentPartAddedIfNeeded emits a response.content_part.added event if
// the given output_index:content_index pair hasn't been announced yet.
func emitContentPartAddedIfNeeded(
	events chan<- interface{},
	announcedParts map[string]bool,
	announcedOutputs map[int]string,
	outputIndex int,
	contentIndex int,
	seqNum int,
) int {
	key := fmt.Sprintf("%d:%d", outputIndex, contentIndex)
	if announcedParts[key] {
		return seqNum
	}
	announcedParts[key] = true
	itemID := announcedOutputs[outputIndex]
	emptyText := ""

	events <- &schema.ResponseContentPartAddedStreamingEvent{
		Type:           "response.content_part.added",
		SequenceNumber: seqNum,
		ItemID:         itemID,
		OutputIndex:    outputIndex,
		ContentIndex:   contentIndex,
		Part: schema.ContentPart{
			Type:        "output_text",
			Text:        &emptyText,
			Annotations: make([]schema.Annotation, 0),
		},
	}
	return seqNum + 1
}

// buildConversationMessages reconstructs conversation history for multi-turn
func (e *Engine) buildConversationMessages(ctx context.Context, req *schema.ResponseRequest) ([]api.Message, error) {
	var messages []api.Message

	// Load previous conversation if this is a follow-up
	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		prevResp, err := e.sessions.GetResponse(ctx, *req.PreviousResponseID)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous response %s: %w", *req.PreviousResponseID, err)
		}

		// Load stored messages from previous response
		for _, m := range prevResp.Messages {
			msg := api.Message{
				Role:       m.Role,
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			}
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					msg.ToolCalls = append(msg.ToolCalls, api.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: api.ToolCallFunction{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}
			messages = append(messages, msg)
		}

		// NOTE: stored messages already include the assistant response
		// (appended during ProcessRequest before save), so we do NOT
		// re-process prevResp.Output here to avoid duplicates.
	}

	// Add instructions as system message
	if req.Instructions != nil && *req.Instructions != "" {
		// Prepend system message if not already present
		hasSystem := false
		for _, m := range messages {
			if m.Role == "system" {
				hasSystem = true
				break
			}
		}
		if !hasSystem {
			messages = append([]api.Message{
				{Role: "system", Content: *req.Instructions},
			}, messages...)
		}
	}

	// Append current input
	inputMessages := extractInputMessages(req.Input)
	messages = append(messages, inputMessages...)

	return messages, nil
}

// messagesToConversationMessages converts api.Messages to state.ConversationMessages for storage
func messagesToConversationMessages(messages []api.Message) []state.ConversationMessage {
	result := make([]state.ConversationMessage, 0, len(messages))
	for _, m := range messages {
		cm := state.ConversationMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, state.ToolCallRecord{
				ID:        tc.ID,
				Type:      tc.Type,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		result = append(result, cm)
	}
	return result
}

// resolveConversation returns a conversation ID for the request.
// If req.Conversation is set, it validates the conversation exists.
// Otherwise, it auto-creates a new conversation.
func (e *Engine) resolveConversation(ctx context.Context, req *schema.ResponseRequest) (string, error) {
	if req.Conversation != nil && *req.Conversation != "" {
		// Validate existing conversation
		_, err := e.sessions.GetConversation(ctx, *req.Conversation)
		if err != nil {
			return "", fmt.Errorf("conversation %s not found", *req.Conversation)
		}
		return *req.Conversation, nil
	}

	// Auto-create a new conversation
	convID := generateID("conv_")
	conv := &state.Conversation{
		ID:        convID,
		Messages:  []state.Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := e.sessions.CreateConversation(ctx, conv); err != nil {
		return "", fmt.Errorf("failed to create conversation: %w", err)
	}
	return convID, nil
}

// findLatestResponseInConversation finds the most recent response in a conversation.
// Returns nil if no responses exist yet (first message in conversation).
func (e *Engine) findLatestResponseInConversation(ctx context.Context, conversationID string) (*state.Response, error) {
	responses, err := e.sessions.ListResponses(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list responses for conversation %s: %w", conversationID, err)
	}
	if len(responses) == 0 {
		return nil, nil
	}

	// Find the response with the latest CreatedAt
	latest := responses[0]
	for _, r := range responses[1:] {
		if r.CreatedAt.After(latest.CreatedAt) {
			latest = r
		}
	}
	return latest, nil
}

// appendItemsToConversation adds the current turn's input and output messages to the conversation.
func (e *Engine) appendItemsToConversation(ctx context.Context, conversationID string, req *schema.ResponseRequest, output []schema.ItemField) error {
	var items []state.Message

	// Add user input messages
	inputMessages := extractInputMessages(req.Input)
	for _, m := range inputMessages {
		if m.Role == "system" {
			continue // skip system messages
		}
		item := state.Message{
			ID:        generateID("msg_"),
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: time.Now(),
		}
		if m.Role == "tool" {
			item.Metadata = map[string]string{"type": "function_call_output"}
		}
		items = append(items, item)
	}

	// Add assistant output messages
	for _, out := range output {
		switch out.Type {
		case "message":
			role := "assistant"
			if out.Role != nil {
				role = *out.Role
			}
			content := ""
			for _, part := range out.Content {
				if part.Text != nil {
					content += *part.Text
				}
			}
			if content != "" {
				items = append(items, state.Message{
					ID:        generateID("msg_"),
					Role:      role,
					Content:   content,
					CreatedAt: time.Now(),
				})
			}
		case "function_call":
			args := ""
			if out.Arguments != nil {
				args = *out.Arguments
			}
			name := ""
			if out.Name != nil {
				name = *out.Name
			}
			items = append(items, state.Message{
				ID:        generateID("msg_"),
				Role:      "assistant",
				Content:   fmt.Sprintf(`{"name":%q,"arguments":%s}`, name, args),
				Metadata:  map[string]string{"type": "function_call"},
				CreatedAt: time.Now(),
			})
		}
	}

	if len(items) > 0 {
		return e.sessions.AddConversationItems(ctx, conversationID, items)
	}
	return nil
}

// buildConversationMessagesFromConversation builds messages from the latest response in a conversation.
// This reuses the same mechanism as previous_response_id: load stored Messages + Output from the latest response.
func (e *Engine) buildConversationMessagesFromConversation(ctx context.Context, conversationID string, req *schema.ResponseRequest) ([]api.Message, error) {
	var messages []api.Message

	// Find the latest response in the conversation
	latestResp, err := e.findLatestResponseInConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	if latestResp != nil {
		// Load stored messages from the latest response (same as previous_response_id logic)
		for _, m := range latestResp.Messages {
			msg := api.Message{
				Role:       m.Role,
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			}
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					msg.ToolCalls = append(msg.ToolCalls, api.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: api.ToolCallFunction{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}
			messages = append(messages, msg)
		}

		// NOTE: stored messages already include the assistant response
		// (appended during ProcessRequest before save), so we do NOT
		// re-process latestResp.Output here to avoid duplicates.
	}

	// Add instructions as system message
	if req.Instructions != nil && *req.Instructions != "" {
		hasSystem := false
		for _, m := range messages {
			if m.Role == "system" {
				hasSystem = true
				break
			}
		}
		if !hasSystem {
			messages = append([]api.Message{
				{Role: "system", Content: *req.Instructions},
			}, messages...)
		}
	}

	// Append current input
	inputMessages := extractInputMessages(req.Input)
	messages = append(messages, inputMessages...)

	return messages, nil
}

// expandMCPTools discovers tools from MCP servers and replaces MCP tool entries
// with concrete function tool definitions. It returns the expanded tools list
// and a map from tool name to MCP client for server-side execution.
func (e *Engine) expandMCPTools(ctx context.Context, tools []schema.ResponsesToolParam) (
	[]schema.ResponsesToolParam, map[string]*mcp.Client, error,
) {
	if e.connectors == nil {
		// No connector support — pass through all tools unchanged
		return tools, nil, nil
	}

	var expanded []schema.ResponsesToolParam
	mcpToolNames := map[string]*mcp.Client{}

	for _, t := range tools {
		if t.Type != "mcp" {
			expanded = append(expanded, t)
			continue
		}

		// Look up the connector by server_label (which matches connector_id)
		connector, err := e.connectors.GetConnector(ctx, t.ServerLabel)
		if err != nil {
			return nil, nil, fmt.Errorf("mcp connector %q not found: %w", t.ServerLabel, err)
		}

		// Create MCP client, initialize, and list tools
		mcpClient := mcp.NewClient(connector.URL)
		if err := mcpClient.Initialize(ctx); err != nil {
			return nil, nil, fmt.Errorf("mcp server %q initialize: %w", t.ServerLabel, err)
		}

		toolInfos, err := mcpClient.ListTools(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("mcp server %q list tools: %w", t.ServerLabel, err)
		}

		// Convert each MCP ToolInfo to a function tool
		for _, ti := range toolInfos {
			desc := ti.Description
			expanded = append(expanded, schema.ResponsesToolParam{
				Type:        "function",
				Name:        ti.Name,
				Description: &desc,
				Parameters:  ti.InputSchema,
			})
			mcpToolNames[ti.Name] = mcpClient
		}
	}

	return expanded, mcpToolNames, nil
}

// fileSearchConfig holds the configuration for a file_search tool.
type fileSearchConfig struct {
	VectorStoreIDs []string
	MaxNumResults  int
}

// expandFileSearchTools replaces file_search tool entries with a synthetic
// function tool and records the configuration for server-side execution.
// Returns the expanded tools and a map from tool name to config.
func (e *Engine) expandFileSearchTools(tools []schema.ResponsesToolParam) (
	[]schema.ResponsesToolParam, map[string]fileSearchConfig,
) {
	if e.vectorSearch == nil {
		return tools, nil
	}

	var expanded []schema.ResponsesToolParam
	configs := map[string]fileSearchConfig{}

	for _, t := range tools {
		if t.Type != "file_search" {
			expanded = append(expanded, t)
			continue
		}

		// Record config
		maxResults := 10
		if t.MaxNumResults != nil && *t.MaxNumResults > 0 {
			maxResults = *t.MaxNumResults
		}
		configs["file_search"] = fileSearchConfig{
			VectorStoreIDs: t.VectorStoreIDs,
			MaxNumResults:  maxResults,
		}

		// Replace with a synthetic function tool
		desc := "Search files in vector stores for relevant content based on a query."
		expanded = append(expanded, schema.ResponsesToolParam{
			Type:        "function",
			Name:        "file_search",
			Description: &desc,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query to find relevant file content.",
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		})
	}

	if len(configs) == 0 {
		return tools, nil
	}

	return expanded, configs
}

// executeFileSearch runs a file_search tool call against all configured vector stores.
func (e *Engine) executeFileSearch(ctx context.Context, cfg fileSearchConfig, query string) string {
	var allResults []vectorstore.SearchResult
	for _, vsID := range cfg.VectorStoreIDs {
		results, err := e.vectorSearch.Search(ctx, vsID, query, cfg.MaxNumResults)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	if len(allResults) == 0 {
		return "No relevant results found."
	}

	// Format results as text
	var sb strings.Builder
	for i, r := range allResults {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		fmt.Fprintf(&sb, "[File: %s, Score: %.4f]\n%s", r.FileID, r.Score, r.Content)
	}
	return sb.String()
}

// parseJSONArgs parses a JSON string into a map for MCP tool call arguments.
func parseJSONArgs(jsonStr string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &args); err != nil {
		return map[string]any{}
	}
	return args
}

// mcpResultToString converts an MCP tool call result to a string for the LLM.
func mcpResultToString(result *mcp.ToolCallResult) string {
	var parts []string
	for _, block := range result.Content {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ProcessRequest processes a Responses API request (non-streaming).
// It calls the backend's /v1/responses endpoint and adds state management.
func (e *Engine) ProcessRequest(ctx context.Context, req *schema.ResponseRequest) (*schema.Response, error) {
	// 1. Validate request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// 2. Generate response ID
	respID := generateID("resp_")

	// 3. Create response object
	model := ""
	if req.Model != nil {
		model = *req.Model
	}
	resp := schema.NewResponse(respID, model)

	// 4. Resolve conversation (auto-create or validate existing)
	conversationID, err := e.resolveConversation(ctx, req)
	if err != nil {
		resp.MarkFailed("api_error", "conversation_error", fmt.Sprintf("failed to resolve conversation: %v", err))
		return resp, nil
	}

	// 5. Echo ALL request parameters and set conversation
	echoRequestParams(resp, req)
	resp.Conversation = &conversationID

	// 6. Build conversation messages (including multi-turn history)
	var messages []api.Message
	if req.Conversation != nil && *req.Conversation != "" {
		messages, err = e.buildConversationMessagesFromConversation(ctx, conversationID, req)
	} else {
		messages, err = e.buildConversationMessages(ctx, req)
	}
	if err != nil {
		resp.MarkFailed("api_error", "conversation_error", fmt.Sprintf("failed to build conversation: %v", err))
		return resp, nil
	}

	// 7. Expand MCP tools into function tools
	expandedTools := req.Tools
	var mcpToolNames map[string]*mcp.Client
	if len(req.Tools) > 0 {
		var expandErr error
		expandedTools, mcpToolNames, expandErr = e.expandMCPTools(ctx, req.Tools)
		if expandErr != nil {
			resp.MarkFailed("api_error", "mcp_error", fmt.Sprintf("failed to expand MCP tools: %v", expandErr))
			return resp, nil
		}
	}

	// 7b. Expand file_search tools into function tools
	var fileSearchConfigs map[string]fileSearchConfig
	if len(expandedTools) > 0 {
		expandedTools, fileSearchConfigs = e.expandFileSearchTools(expandedTools)
	}

	// 8. Agentic loop
	maxIters := defaultMaxToolCalls
	if req.MaxToolCalls != nil && *req.MaxToolCalls > 0 {
		maxIters = *req.MaxToolCalls
	}

	accumulatedOutputTokens := 0
	var allOutput []schema.ItemField

	for iter := 0; iter < maxIters; iter++ {
		// Build Responses API request
		apiReq := buildResponsesAPIRequest(model, messages, req, expandedTools, false)

		// Adjust token budget if max_output_tokens is set
		if req.MaxOutputTokens != nil {
			remaining := *req.MaxOutputTokens - accumulatedOutputTokens
			if remaining <= 0 {
				resp.MarkIncomplete("max_output_tokens")
				break
			}
			apiReq.MaxOutputTokens = &remaining
		}

		// Call backend
		apiResp, err := e.llm.CreateResponse(ctx, apiReq)
		if err != nil {
			resp.MarkFailed("api_error", "llm_error", fmt.Sprintf("failed to call backend: %v", err))
			return resp, nil
		}

		// Track usage
		if apiResp.Usage != nil {
			accumulatedOutputTokens += apiResp.Usage.OutputTokens
		}

		// Parse output for tool calls
		_, toolCalls, hasToolCalls := parseResponsesOutput(apiResp.Output)

		if hasToolCalls {
			var clientSideCalls []api.ToolCall

			for _, tc := range toolCalls {
				mcpClient, isMCP := mcpToolNames[tc.Name]
				fsCfg, isFileSearch := fileSearchConfigs[tc.Name]

				if isMCP {
					// Execute MCP tool server-side
					args := parseJSONArgs(tc.Arguments)
					result, mcpErr := mcpClient.CallTool(ctx, tc.Name, args)

					completedStatus := "completed"
					callID := tc.CallID
					funcName := tc.Name
					funcArgs := tc.Arguments

					allOutput = append(allOutput, schema.ItemField{
						Type:      "function_call",
						ID:        generateID("fc_"),
						CallID:    &callID,
						Name:      &funcName,
						Arguments: &funcArgs,
						Status:    &completedStatus,
					})

					var outputStr string
					if mcpErr != nil {
						outputStr = fmt.Sprintf("Error calling tool: %v", mcpErr)
					} else {
						outputStr = mcpResultToString(result)
					}
					allOutput = append(allOutput, schema.ItemField{
						Type:   "function_call_output",
						ID:     generateID("fco_"),
						CallID: &callID,
						Output: &outputStr,
					})

					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{{
							ID:   tc.CallID,
							Type: "function",
							Function: api.ToolCallFunction{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						}},
					})
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    outputStr,
						ToolCallID: tc.CallID,
					})
				} else if isFileSearch {
					args := parseJSONArgs(tc.Arguments)
					query, _ := args["query"].(string)
					outputStr := e.executeFileSearch(ctx, fsCfg, query)

					completedStatus := "completed"
					callID := tc.CallID
					funcName := tc.Name
					funcArgs := tc.Arguments

					allOutput = append(allOutput, schema.ItemField{
						Type:      "function_call",
						ID:        generateID("fc_"),
						CallID:    &callID,
						Name:      &funcName,
						Arguments: &funcArgs,
						Status:    &completedStatus,
					})
					allOutput = append(allOutput, schema.ItemField{
						Type:   "function_call_output",
						ID:     generateID("fco_"),
						CallID: &callID,
						Output: &outputStr,
					})

					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{{
							ID:   tc.CallID,
							Type: "function",
							Function: api.ToolCallFunction{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						}},
					})
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    outputStr,
						ToolCallID: tc.CallID,
					})
				} else {
					// Client-side function — collect for break
					completedStatus := "completed"
					callID := tc.CallID
					funcName := tc.Name
					funcArgs := tc.Arguments
					allOutput = append(allOutput, schema.ItemField{
						Type:      "function_call",
						ID:        generateID("fc_"),
						CallID:    &callID,
						Name:      &funcName,
						Arguments: &funcArgs,
						Status:    &completedStatus,
					})
					clientSideCalls = append(clientSideCalls, api.ToolCall{
						ID:   tc.CallID,
						Type: "function",
						Function: api.ToolCallFunction{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}

			if len(clientSideCalls) > 0 {
				messages = append(messages, api.Message{
					Role:      "assistant",
					ToolCalls: clientSideCalls,
				})
				break // client handles execution
			}
			// All calls were server-side — continue loop
			continue
		}

		// Normal response — convert backend output items to schema
		backendOutput := convertOutputItemsToSchema(apiResp.Output)
		allOutput = append(allOutput, backendOutput...)

		// Append assistant message for storage
		textContent, _, _ := parseResponsesOutput(apiResp.Output)
		if textContent != "" {
			messages = append(messages, api.Message{
				Role:    "assistant",
				Content: textContent,
			})
		}

		// Set usage from backend response
		if apiResp.Usage != nil {
			resp.Usage = &schema.UsageField{
				InputTokens:  apiResp.Usage.InputTokens,
				OutputTokens: accumulatedOutputTokens,
				TotalTokens:  apiResp.Usage.InputTokens + accumulatedOutputTokens,
				InputTokensDetails: schema.InputTokensDetails{
					CachedTokens: 0,
				},
				OutputTokensDetails: schema.OutputTokensDetails{
					ReasoningTokens: 0,
				},
			}
		}

		break
	}

	// 9. Set output
	resp.Output = allOutput
	if resp.Output == nil {
		resp.Output = make([]schema.ItemField, 0)
	}

	// 10. Set usage if not already set
	if resp.Usage == nil {
		resp.Usage = &schema.UsageField{
			InputTokensDetails:  schema.InputTokensDetails{},
			OutputTokensDetails: schema.OutputTokensDetails{},
		}
	}

	// 11. Mark as completed if not already marked
	if resp.Status == "in_progress" {
		resp.MarkCompleted()
	}

	// 12. Save response to state store
	prevRespID := ""
	if req.PreviousResponseID != nil {
		prevRespID = *req.PreviousResponseID
	}

	if err := e.sessions.SaveResponse(ctx, &state.Response{
		ID:                 resp.ID,
		ConversationID:     conversationID,
		PreviousResponseID: prevRespID,
		Request:            req,
		Output:             resp.Output,
		Status:             resp.Status,
		Usage:              resp.Usage,
		Messages:           messagesToConversationMessages(messages),
		CreatedAt:          time.Unix(resp.CreatedAt, 0),
		CompletedAt:        timePtr(resp.CompletedAt),
	}); err != nil {
		return nil, fmt.Errorf("failed to save response: %w", err)
	}

	// 13. Append items to conversation for the Conversations API
	if err := e.appendItemsToConversation(ctx, conversationID, req, allOutput); err != nil {
		_ = err
	}

	return resp, nil
}

// ProcessRequestStream processes a streaming Responses API request.
// It streams from the backend's /v1/responses endpoint, forwarding SSE events
// to the client and intercepting tool calls for server-side execution.
func (e *Engine) ProcessRequestStream(ctx context.Context, req *schema.ResponseRequest) (<-chan interface{}, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	events := make(chan interface{}, 10)

	go func() {
		defer close(events)

		respID := generateID("resp_")
		model := ""
		if req.Model != nil {
			model = *req.Model
		}
		resp := schema.NewResponse(respID, model)

		// Track sequence number for events
		seqNum := 0

		// Resolve conversation before emitting response.created
		conversationID, err := e.resolveConversation(ctx, req)
		if err != nil {
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: schema.ErrorField{Type: "api_error", Message: fmt.Sprintf("failed to resolve conversation: %v", err)},
			}
			return
		}

		// Echo ALL request parameters and set conversation
		echoRequestParams(resp, req)
		resp.Conversation = &conversationID

		// Send response.created event
		events <- &schema.ResponseCreatedStreamingEvent{
			Type:           "response.created",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Save response on creation (in_progress)
		prevRespID := ""
		if req.PreviousResponseID != nil {
			prevRespID = *req.PreviousResponseID
		}
		_ = e.sessions.SaveResponse(ctx, &state.Response{
			ID:                 resp.ID,
			ConversationID:     conversationID,
			PreviousResponseID: prevRespID,
			Request:            req,
			Output:             resp.Output,
			Status:             "in_progress",
			CreatedAt:          time.Unix(resp.CreatedAt, 0),
		})

		// Build conversation messages
		var messages []api.Message
		if req.Conversation != nil && *req.Conversation != "" {
			messages, err = e.buildConversationMessagesFromConversation(ctx, conversationID, req)
		} else {
			messages, err = e.buildConversationMessages(ctx, req)
		}
		if err != nil {
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: schema.ErrorField{Type: "api_error", Message: fmt.Sprintf("failed to build conversation: %v", err)},
			}
			return
		}

		// Send response.in_progress event
		resp.Status = "in_progress"
		events <- &schema.ResponseInProgressStreamingEvent{
			Type:           "response.in_progress",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Expand MCP tools
		expandedTools := req.Tools
		var mcpToolNames map[string]*mcp.Client
		if len(req.Tools) > 0 {
			var expandErr error
			expandedTools, mcpToolNames, expandErr = e.expandMCPTools(ctx, req.Tools)
			if expandErr != nil {
				events <- &schema.ErrorStreamingEvent{
					Type:  "error",
					Error: schema.ErrorField{Type: "api_error", Message: fmt.Sprintf("failed to expand MCP tools: %v", expandErr)},
				}
				return
			}
		}

		// Expand file_search tools
		var fileSearchConfigs map[string]fileSearchConfig
		if len(expandedTools) > 0 {
			expandedTools, fileSearchConfigs = e.expandFileSearchTools(expandedTools)
		}

		// Agentic loop
		maxIters := defaultMaxToolCalls
		if req.MaxToolCalls != nil && *req.MaxToolCalls > 0 {
			maxIters = *req.MaxToolCalls
		}

		var allOutput []schema.ItemField

		for iter := 0; iter < maxIters; iter++ {
			// Build Responses API request
			apiReq := buildResponsesAPIRequest(model, messages, req, expandedTools, true)

			// Start streaming from backend
			streamChan, streamErr := e.llm.CreateResponseStream(ctx, apiReq)
			if streamErr != nil {
				events <- &schema.ErrorStreamingEvent{
					Type:  "error",
					Error: schema.ErrorField{Type: "api_error", Message: fmt.Sprintf("failed to start streaming: %v", streamErr)},
				}
				return
			}

			// Track the backend's completed response (from response.completed event)
			var backendOutput []api.OutputItem
			var backendUsage *api.UsageInfo

			// Track announced items for proper SSE event sequencing.
			// The OpenAI SDK expects response.output_item.added and
			// response.content_part.added before any delta events.
			// vLLM uses a per-token content_index (0, 1, 2, ...) instead of
			// the standard content_index=0 for all deltas in one content part.
			// We normalise: emit our own lifecycle events, rewrite delta
			// content_index to 0, and skip vLLM's lifecycle events.
			announcedOutputs := make(map[int]string) // output_index → item_id
			announcedContent := make(map[int]bool)   // output_index → content_part announced
			accumulatedText := make(map[int]string)  // output_index → accumulated text

			// Forward backend events to client, skipping lifecycle events
			for evt := range streamChan {
				switch evt.Type {
				case "response.created", "response.queued", "response.in_progress":
					// Skip — we manage lifecycle events ourselves
					continue

				case "response.completed":
					// Parse to extract final output and usage
					var wrapper struct {
						Response api.ResponsesAPIResponse `json:"response"`
					}
					if err := json.Unmarshal(evt.Data, &wrapper); err == nil {
						backendOutput = wrapper.Response.Output
						backendUsage = wrapper.Response.Usage
					}
					continue

				case "response.failed":
					events <- &schema.RawStreamingEvent{
						EventType: evt.Type,
						RawData:   patchResponseID(evt.Data, respID),
					}
					continue

				case "response.output_item.added",
					"response.output_item.done",
					"response.content_part.added",
					"response.content_part.done",
					"response.output_text.done":
					// Skip — the gateway emits its own normalised versions
					continue

				case "response.output_text.delta":
					var fields struct {
						OutputIndex int    `json:"output_index"`
						ItemID      string `json:"item_id"`
						Delta       string `json:"delta"`
					}
					if err := json.Unmarshal(evt.Data, &fields); err == nil {
						// Emit output_item.added + content_part.added on first delta
						seqNum = emitOutputItemAddedIfNeeded(events, announcedOutputs, fields.OutputIndex, fields.ItemID, "message", seqNum)
						if !announcedContent[fields.OutputIndex] {
							announcedContent[fields.OutputIndex] = true
							seqNum = emitContentPartAddedIfNeeded(events, make(map[string]bool), announcedOutputs, fields.OutputIndex, 0, seqNum)
						}
						accumulatedText[fields.OutputIndex] += fields.Delta
					}

					// Re-emit delta with normalised content_index=0 and correct sequence_number
					var m map[string]json.RawMessage
					if err := json.Unmarshal(evt.Data, &m); err == nil {
						m["content_index"], _ = json.Marshal(0)
						m["sequence_number"], _ = json.Marshal(seqNum)
						seqNum++
						patched, _ := json.Marshal(m)
						events <- &schema.RawStreamingEvent{
							EventType: evt.Type,
							RawData:   patchResponseID(json.RawMessage(patched), respID),
						}
					}

				case "response.function_call_arguments.delta":
					var fields struct {
						OutputIndex int    `json:"output_index"`
						ItemID      string `json:"item_id"`
					}
					if err := json.Unmarshal(evt.Data, &fields); err == nil {
						seqNum = emitOutputItemAddedIfNeeded(events, announcedOutputs, fields.OutputIndex, fields.ItemID, "function_call", seqNum)
					}
					events <- &schema.RawStreamingEvent{
						EventType: evt.Type,
						RawData:   patchResponseID(evt.Data, respID),
					}

				default:
					events <- &schema.RawStreamingEvent{
						EventType: evt.Type,
						RawData:   patchResponseID(evt.Data, respID),
					}
				}
			}

			// Emit done events for text content parts
			for outputIdx, text := range accumulatedText {
				itemID := announcedOutputs[outputIdx]

				events <- &schema.ResponseOutputTextDoneStreamingEvent{
					Type:           "response.output_text.done",
					SequenceNumber: seqNum,
					ItemID:         itemID,
					OutputIndex:    outputIdx,
					ContentIndex:   0,
					Text:           text,
					Logprobs:       make([]interface{}, 0),
				}
				seqNum++

				events <- &schema.ResponseContentPartDoneStreamingEvent{
					Type:           "response.content_part.done",
					SequenceNumber: seqNum,
					ItemID:         itemID,
					OutputIndex:    outputIdx,
					ContentIndex:   0,
					Part: schema.ContentPart{
						Type:        "output_text",
						Text:        &text,
						Annotations: make([]schema.Annotation, 0),
					},
				}
				seqNum++

				completedStatus := "completed"
				role := "assistant"
				t := text
				events <- &schema.ResponseOutputItemDoneStreamingEvent{
					Type:           "response.output_item.done",
					SequenceNumber: seqNum,
					OutputIndex:    outputIdx,
					Item: schema.ItemField{
						Type: "message",
						ID:   itemID,
						Role: &role,
						Content: []schema.ContentPart{{
							Type:        "output_text",
							Text:        &t,
							Annotations: make([]schema.Annotation, 0),
						}},
						Status: &completedStatus,
					},
				}
				seqNum++
			}

			// Check for server-side tool calls in the completed output
			_, toolCalls, hasToolCalls := parseResponsesOutput(backendOutput)

			if hasToolCalls {
				hasServerSide := false
				var clientSideCalls []api.ToolCall

				for _, tc := range toolCalls {
					mcpClient, isMCP := mcpToolNames[tc.Name]
					fsCfg, isFileSearch := fileSearchConfigs[tc.Name]

					if isMCP {
						hasServerSide = true
						args := parseJSONArgs(tc.Arguments)
						result, mcpErr := mcpClient.CallTool(ctx, tc.Name, args)

						completedStatus := "completed"
						callID := tc.CallID
						funcName := tc.Name
						funcArgs := tc.Arguments

						allOutput = append(allOutput, schema.ItemField{
							Type:      "function_call",
							ID:        generateID("fc_"),
							CallID:    &callID,
							Name:      &funcName,
							Arguments: &funcArgs,
							Status:    &completedStatus,
						})

						var outputStr string
						if mcpErr != nil {
							outputStr = fmt.Sprintf("Error calling tool: %v", mcpErr)
						} else {
							outputStr = mcpResultToString(result)
						}

						outputItem := schema.ItemField{
							Type:   "function_call_output",
							ID:     generateID("fco_"),
							CallID: &callID,
							Output: &outputStr,
						}
						allOutput = append(allOutput, outputItem)

						// Emit function_call_output events to client
						events <- &schema.ResponseOutputItemAddedStreamingEvent{
							Type:           "response.output_item.added",
							SequenceNumber: seqNum,
							OutputIndex:    len(allOutput) - 1,
							Item:           outputItem,
						}
						seqNum++
						events <- &schema.ResponseOutputItemDoneStreamingEvent{
							Type:           "response.output_item.done",
							SequenceNumber: seqNum,
							OutputIndex:    len(allOutput) - 1,
							Item:           outputItem,
						}
						seqNum++

						messages = append(messages, api.Message{
							Role: "assistant",
							ToolCalls: []api.ToolCall{{
								ID:   tc.CallID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      tc.Name,
									Arguments: tc.Arguments,
								},
							}},
						})
						messages = append(messages, api.Message{
							Role:       "tool",
							Content:    outputStr,
							ToolCallID: tc.CallID,
						})

					} else if isFileSearch {
						hasServerSide = true
						args := parseJSONArgs(tc.Arguments)
						query, _ := args["query"].(string)
						outputStr := e.executeFileSearch(ctx, fsCfg, query)

						completedStatus := "completed"
						callID := tc.CallID
						funcName := tc.Name
						funcArgs := tc.Arguments

						allOutput = append(allOutput, schema.ItemField{
							Type:      "function_call",
							ID:        generateID("fc_"),
							CallID:    &callID,
							Name:      &funcName,
							Arguments: &funcArgs,
							Status:    &completedStatus,
						})

						outputItem := schema.ItemField{
							Type:   "function_call_output",
							ID:     generateID("fco_"),
							CallID: &callID,
							Output: &outputStr,
						}
						allOutput = append(allOutput, outputItem)

						events <- &schema.ResponseOutputItemAddedStreamingEvent{
							Type:           "response.output_item.added",
							SequenceNumber: seqNum,
							OutputIndex:    len(allOutput) - 1,
							Item:           outputItem,
						}
						seqNum++
						events <- &schema.ResponseOutputItemDoneStreamingEvent{
							Type:           "response.output_item.done",
							SequenceNumber: seqNum,
							OutputIndex:    len(allOutput) - 1,
							Item:           outputItem,
						}
						seqNum++

						messages = append(messages, api.Message{
							Role: "assistant",
							ToolCalls: []api.ToolCall{{
								ID:   tc.CallID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      tc.Name,
									Arguments: tc.Arguments,
								},
							}},
						})
						messages = append(messages, api.Message{
							Role:       "tool",
							Content:    outputStr,
							ToolCallID: tc.CallID,
						})

					} else {
						// Client-side function call — already forwarded via raw events
						completedStatus := "completed"
						callID := tc.CallID
						funcName := tc.Name
						funcArgs := tc.Arguments
						allOutput = append(allOutput, schema.ItemField{
							Type:      "function_call",
							ID:        generateID("fc_"),
							CallID:    &callID,
							Name:      &funcName,
							Arguments: &funcArgs,
							Status:    &completedStatus,
						})
						clientSideCalls = append(clientSideCalls, api.ToolCall{
							ID:   tc.CallID,
							Type: "function",
							Function: api.ToolCallFunction{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						})
					}
				}

				if len(clientSideCalls) > 0 {
					messages = append(messages, api.Message{
						Role:      "assistant",
						ToolCalls: clientSideCalls,
					})
				}

				if hasServerSide && len(clientSideCalls) == 0 {
					// All calls were server-side — continue agentic loop
					continue
				}
				// Client-side calls present — break
				break
			}

			// No tool calls — collect text output from backend
			if len(backendOutput) > 0 {
				backendSchemaOutput := convertOutputItemsToSchema(backendOutput)
				allOutput = append(allOutput, backendSchemaOutput...)

				textContent, _, _ := parseResponsesOutput(backendOutput)
				if textContent != "" {
					messages = append(messages, api.Message{
						Role:    "assistant",
						Content: textContent,
					})
				}
			}

			// Set usage from backend
			if backendUsage != nil {
				resp.Usage = &schema.UsageField{
					InputTokens:  backendUsage.InputTokens,
					OutputTokens: backendUsage.OutputTokens,
					TotalTokens:  backendUsage.TotalTokens,
					InputTokensDetails: schema.InputTokensDetails{
						CachedTokens: 0,
					},
					OutputTokensDetails: schema.OutputTokensDetails{
						ReasoningTokens: 0,
					},
				}
			}

			break
		}

		// Update response
		resp.Output = allOutput
		if resp.Output == nil {
			resp.Output = make([]schema.ItemField, 0)
		}

		resp.MarkCompleted()

		// Set usage if not already set
		if resp.Usage == nil {
			resp.Usage = &schema.UsageField{
				InputTokensDetails:  schema.InputTokensDetails{},
				OutputTokensDetails: schema.OutputTokensDetails{},
			}
		}

		// Send response.completed event
		events <- &schema.ResponseCompletedStreamingEvent{
			Type:           "response.completed",
			SequenceNumber: seqNum,
			Response:       *resp,
		}

		// Final save with complete state
		_ = e.sessions.SaveResponse(ctx, &state.Response{
			ID:                 resp.ID,
			ConversationID:     conversationID,
			PreviousResponseID: prevRespID,
			Request:            req,
			Output:             resp.Output,
			Status:             resp.Status,
			Usage:              resp.Usage,
			Messages:           messagesToConversationMessages(messages),
			CreatedAt:          time.Unix(resp.CreatedAt, 0),
			CompletedAt:        timePtr(resp.CompletedAt),
		})

		// Append items to conversation for the Conversations API
		_ = e.appendItemsToConversation(ctx, conversationID, req, allOutput)
	}()

	return events, nil
}

// Helper functions

func generateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func timePtr(t *int64) *time.Time {
	if t == nil {
		return nil
	}
	tm := time.Unix(*t, 0)
	return &tm
}

// convertToolsToResponse converts request tools to response tools
func convertToolsToResponse(reqTools []schema.ResponsesToolParam) []schema.ResponsesTool {
	if len(reqTools) == 0 {
		return make([]schema.ResponsesTool, 0)
	}
	respTools := make([]schema.ResponsesTool, len(reqTools))
	for i, t := range reqTools {
		respTools[i] = schema.ResponsesTool{
			Type:              t.Type,
			Name:              t.Name,
			Description:       t.Description,
			Parameters:        t.Parameters,
			Strict:            t.Strict,
			ServerLabel:       t.ServerLabel,
			SearchContextSize: t.SearchContextSize,
			UserLocation:      t.UserLocation,
			VectorStoreIDs:    t.VectorStoreIDs,
			MaxNumResults:     t.MaxNumResults,
			RankingOptions:    t.RankingOptions,
			Filters:           t.Filters,
		}
	}
	return respTools
}

// convertReasoningToResponse converts request reasoning to response reasoning
func convertReasoningToResponse(reqReasoning *schema.ReasoningParam) *schema.ReasoningConfig {
	if reqReasoning == nil {
		return nil
	}
	return &schema.ReasoningConfig{
		Type:   reqReasoning.Type,
		Effort: reqReasoning.Effort,
		Budget: reqReasoning.Budget,
	}
}

// GetResponse retrieves a response by ID from the session store
func (e *Engine) GetResponse(ctx context.Context, responseID string) (*schema.Response, error) {
	stateResp, err := e.sessions.GetResponse(ctx, responseID)
	if err != nil {
		return nil, fmt.Errorf("response not found: %w", err)
	}

	// Convert state.Response to schema.Response
	var model string
	if req, ok := stateResp.Request.(*schema.ResponseRequest); ok && req != nil && req.Model != nil {
		model = *req.Model
	}

	schemaResp := schema.NewResponse(stateResp.ID, model)
	schemaResp.Status = stateResp.Status

	// Type assert Output
	if output, ok := stateResp.Output.([]schema.ItemField); ok {
		schemaResp.Output = output
	}

	// Type assert Usage
	if usage, ok := stateResp.Usage.(*schema.UsageField); ok {
		schemaResp.Usage = usage
	}

	schemaResp.CreatedAt = stateResp.CreatedAt.Unix()
	if stateResp.CompletedAt != nil {
		completedAt := stateResp.CompletedAt.Unix()
		schemaResp.CompletedAt = &completedAt
	}

	// Echo request parameters if available
	if req, ok := stateResp.Request.(*schema.ResponseRequest); ok && req != nil {
		schemaResp.PreviousResponseID = req.PreviousResponseID
		schemaResp.Instructions = req.Instructions

		if req.Temperature != nil {
			temp := *req.Temperature
			schemaResp.Temperature = temp
		}
		if req.TopP != nil {
			topP := *req.TopP
			schemaResp.TopP = topP
		}

		schemaResp.MaxOutputTokens = req.MaxOutputTokens
		schemaResp.Metadata = req.Metadata

		if req.Tools != nil {
			schemaResp.Tools = convertToolsToResponse(req.Tools)
		}

		if req.Reasoning != nil {
			schemaResp.Reasoning = convertReasoningToResponse(req.Reasoning)
		}
	}

	// Populate conversation from state
	if stateResp.ConversationID != "" {
		convID := stateResp.ConversationID
		schemaResp.Conversation = &convID
	}

	return schemaResp, nil
}

// ListResponses retrieves a paginated list of responses
func (e *Engine) ListResponses(ctx context.Context, after, before string, limit int, order, model string) ([]*schema.Response, bool, error) {
	stateResponses, hasMore, err := e.sessions.ListResponsesPaginated(ctx, after, before, limit, order, model)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list responses: %w", err)
	}

	// Convert state.Response to schema.Response
	responses := make([]*schema.Response, 0, len(stateResponses))
	for _, stateResp := range stateResponses {
		var modelName string
		if req, ok := stateResp.Request.(*schema.ResponseRequest); ok && req != nil && req.Model != nil {
			modelName = *req.Model
		}

		schemaResp := schema.NewResponse(stateResp.ID, modelName)
		schemaResp.Status = stateResp.Status

		if output, ok := stateResp.Output.([]schema.ItemField); ok {
			schemaResp.Output = output
		}
		if usage, ok := stateResp.Usage.(*schema.UsageField); ok {
			schemaResp.Usage = usage
		}

		schemaResp.CreatedAt = stateResp.CreatedAt.Unix()
		if stateResp.CompletedAt != nil {
			completedAt := stateResp.CompletedAt.Unix()
			schemaResp.CompletedAt = &completedAt
		}

		// Populate conversation from state
		if stateResp.ConversationID != "" {
			convID := stateResp.ConversationID
			schemaResp.Conversation = &convID
		}

		responses = append(responses, schemaResp)
	}

	return responses, hasMore, nil
}

// DeleteResponse deletes a response by ID
func (e *Engine) DeleteResponse(ctx context.Context, responseID string) error {
	if err := e.sessions.DeleteResponse(ctx, responseID); err != nil {
		return fmt.Errorf("failed to delete response: %w", err)
	}
	return nil
}

// GetResponseInputItems retrieves input items for a specific response
func (e *Engine) GetResponseInputItems(ctx context.Context, responseID string) (interface{}, error) {
	items, err := e.sessions.GetResponseInputItems(ctx, responseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get response input items: %w", err)
	}
	return items, nil
}
