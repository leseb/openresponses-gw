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

// Engine is the core orchestration engine for the Responses API
type Engine struct {
	config       *config.EngineConfig
	sessions     state.SessionStore
	llm          api.ChatCompletionClient
	connectors   ConnectorLookup  // nil-safe: nil means no MCP support
	vectorSearch VectorSearcher   // nil-safe: nil means no file_search support
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

	// Create chat completion client – a real inference backend is required
	if cfg.ModelEndpoint == "" {
		return nil, fmt.Errorf("model endpoint is required (set OPENAI_API_ENDPOINT)")
	}
	llm := api.NewOpenAIClient(cfg.ModelEndpoint, cfg.APIKey)

	return &Engine{
		config:       cfg,
		sessions:     store,
		llm:          llm,
		connectors:   connectors,
		vectorSearch: vectorSearch,
	}, nil
}

// LLMClient returns the chat completion client
func (e *Engine) LLMClient() api.ChatCompletionClient {
	return e.llm
}

// Store returns the session store
func (e *Engine) Store() state.SessionStore {
	return e.sessions
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
	if req.ParallelToolCalls != nil {
		resp.ParallelToolCalls = *req.ParallelToolCalls
	}
	if req.Store != nil {
		resp.Store = *req.Store
	}
	if req.FrequencyPenalty != nil {
		resp.FrequencyPenalty = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		resp.PresencePenalty = *req.PresencePenalty
	}
	if req.Truncation != nil {
		if req.Truncation.Type == "last_messages" {
			resp.Truncation = "disabled"
		} else {
			resp.Truncation = "auto"
		}
	} else {
		resp.Truncation = "auto"
	}
	if req.TopLogprobs != nil {
		resp.TopLogprobs = *req.TopLogprobs
	}
	if req.ServiceTier != nil {
		resp.ServiceTier = *req.ServiceTier
	}
	if req.Background != nil {
		resp.Background = *req.Background
	}
	resp.PromptCacheKey = req.PromptCacheKey
	resp.SafetyIdentifier = req.SafetyIdentifier
	resp.Metadata = req.Metadata
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

// convertToolsForLLM converts Responses API tools to chat completion tools
func convertToolsForLLM(tools []schema.ResponsesToolParam) []api.Tool {
	var result []api.Tool
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		tool := api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:       t.Name,
				Parameters: t.Parameters,
			},
		}
		if t.Description != nil {
			tool.Function.Description = *t.Description
		}
		if t.Strict != nil {
			tool.Function.Strict = t.Strict
		}
		result = append(result, tool)
	}
	return result
}

// buildLLMRequest constructs a ChatCompletionRequest from a ResponseRequest
func buildLLMRequest(model string, messages []api.Message, req *schema.ResponseRequest, stream bool) *api.ChatCompletionRequest {
	llmReq := &api.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	}

	// Sampling parameters
	llmReq.Temperature = req.Temperature
	llmReq.TopP = req.TopP
	if req.FrequencyPenalty != nil {
		llmReq.FrequencyPenalty = req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		llmReq.PresencePenalty = req.PresencePenalty
	}

	// Token limits
	if req.MaxOutputTokens != nil {
		llmReq.MaxCompletionTokens = req.MaxOutputTokens
	}

	// Tools
	tools := convertToolsForLLM(req.Tools)
	if len(tools) > 0 {
		llmReq.Tools = tools
	}

	// ToolChoice
	if req.ToolChoice != nil {
		llmReq.ToolChoice = req.ToolChoice
	}

	// ParallelToolCalls
	llmReq.ParallelToolCalls = req.ParallelToolCalls

	// Reasoning effort
	if req.Reasoning != nil && req.Reasoning.Effort != nil {
		llmReq.ReasoningEffort = req.Reasoning.Effort
	}

	// PromptCacheKey
	llmReq.PromptCacheKey = req.PromptCacheKey

	// SafetyIdentifier
	llmReq.SafetyIdentifier = req.SafetyIdentifier

	return llmReq
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

		// Append previous response output as context
		if output, ok := prevResp.Output.([]schema.ItemField); ok {
			for _, item := range output {
				switch item.Type {
				case "message":
					role := "assistant"
					if item.Role != nil {
						role = *item.Role
					}
					content := ""
					for _, part := range item.Content {
						if part.Text != nil {
							content += *part.Text
						}
					}
					if content != "" {
						messages = append(messages, api.Message{Role: role, Content: content})
					}
				case "function_call":
					name := ""
					if item.Name != nil {
						name = *item.Name
					}
					args := ""
					if item.Arguments != nil {
						args = *item.Arguments
					}
					callID := ""
					if item.CallID != nil {
						callID = *item.CallID
					}
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{
							{
								ID:   callID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      name,
									Arguments: args,
								},
							},
						},
					})
				case "function_call_output":
					callID := ""
					if item.CallID != nil {
						callID = *item.CallID
					}
					output := ""
					if item.Output != nil {
						output = *item.Output
					}
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    output,
						ToolCallID: callID,
					})
				}
			}
		}
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

		// Append the latest response's output as context
		if output, ok := latestResp.Output.([]schema.ItemField); ok {
			for _, item := range output {
				switch item.Type {
				case "message":
					role := "assistant"
					if item.Role != nil {
						role = *item.Role
					}
					content := ""
					for _, part := range item.Content {
						if part.Text != nil {
							content += *part.Text
						}
					}
					if content != "" {
						messages = append(messages, api.Message{Role: role, Content: content})
					}
				case "function_call":
					name := ""
					if item.Name != nil {
						name = *item.Name
					}
					args := ""
					if item.Arguments != nil {
						args = *item.Arguments
					}
					callID := ""
					if item.CallID != nil {
						callID = *item.CallID
					}
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{
							{
								ID:   callID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      name,
									Arguments: args,
								},
							},
						},
					})
				case "function_call_output":
					callID := ""
					if item.CallID != nil {
						callID = *item.CallID
					}
					output := ""
					if item.Output != nil {
						output = *item.Output
					}
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    output,
						ToolCallID: callID,
					})
				}
			}
		}
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

// ProcessRequest processes a Responses API request (non-streaming)
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
		// Use conversation-based history
		messages, err = e.buildConversationMessagesFromConversation(ctx, conversationID, req)
	} else {
		// Use previous_response_id-based history (existing behavior)
		messages, err = e.buildConversationMessages(ctx, req)
	}
	if err != nil {
		resp.MarkFailed("api_error", "conversation_error", fmt.Sprintf("failed to build conversation: %v", err))
		return resp, nil
	}

	// 6. Expand MCP tools into function tools
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

	// 6b. Expand file_search tools into function tools
	var fileSearchConfigs map[string]fileSearchConfig
	if len(expandedTools) > 0 {
		expandedTools, fileSearchConfigs = e.expandFileSearchTools(expandedTools)
	}

	// Build a modified request with expanded tools for the LLM
	expandedReq := *req
	expandedReq.Tools = expandedTools

	// 7. Agentic loop
	maxIters := defaultMaxToolCalls
	if req.MaxToolCalls != nil && *req.MaxToolCalls > 0 {
		maxIters = *req.MaxToolCalls
	}

	accumulatedOutputTokens := 0
	var allOutput []schema.ItemField

	for iter := 0; iter < maxIters; iter++ {
		// Build LLM request
		llmReq := buildLLMRequest(model, messages, &expandedReq, false)

		// Adjust token budget if max_output_tokens is set
		if req.MaxOutputTokens != nil {
			remaining := *req.MaxOutputTokens - accumulatedOutputTokens
			if remaining <= 0 {
				resp.MarkIncomplete("max_output_tokens")
				break
			}
			llmReq.MaxCompletionTokens = &remaining
		}

		// Call LLM
		llmResp, err := e.llm.CreateChatCompletion(ctx, llmReq)
		if err != nil {
			resp.MarkFailed("api_error", "llm_error", fmt.Sprintf("failed to call LLM: %v", err))
			return resp, nil
		}

		accumulatedOutputTokens += llmResp.Usage.CompletionTokens

		if len(llmResp.Choices) == 0 {
			resp.MarkFailed("api_error", "no_choices", "LLM returned no choices")
			return resp, nil
		}

		choice := llmResp.Choices[0]

		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			var clientSideCalls []api.ToolCall

			for _, tc := range choice.Message.ToolCalls {
				mcpClient, isMCP := mcpToolNames[tc.Function.Name]
				fsCfg, isFileSearch := fileSearchConfigs[tc.Function.Name]

				if isMCP {
					// Execute MCP tool server-side
					args := parseJSONArgs(tc.Function.Arguments)
					result, mcpErr := mcpClient.CallTool(ctx, tc.Function.Name, args)

					completedStatus := "completed"
					callID := tc.ID
					funcName := tc.Function.Name
					funcArgs := tc.Function.Arguments

					// Emit function_call item
					allOutput = append(allOutput, schema.ItemField{
						Type:      "function_call",
						ID:        generateID("fc_"),
						CallID:    &callID,
						Name:      &funcName,
						Arguments: &funcArgs,
						Status:    &completedStatus,
					})

					// Emit function_call_output item
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

					// Append tool call + result to messages for LLM context
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{tc},
					})
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    outputStr,
						ToolCallID: tc.ID,
					})
				} else if isFileSearch {
					// Execute file_search server-side
					args := parseJSONArgs(tc.Function.Arguments)
					query, _ := args["query"].(string)
					outputStr := e.executeFileSearch(ctx, fsCfg, query)

					completedStatus := "completed"
					callID := tc.ID
					funcName := tc.Function.Name
					funcArgs := tc.Function.Arguments

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
						Role:      "assistant",
						ToolCalls: []api.ToolCall{tc},
					})
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    outputStr,
						ToolCallID: tc.ID,
					})
				} else {
					// Client-side function — collect for break
					completedStatus := "completed"
					callID := tc.ID
					funcName := tc.Function.Name
					funcArgs := tc.Function.Arguments
					allOutput = append(allOutput, schema.ItemField{
						Type:      "function_call",
						ID:        generateID("fc_"),
						CallID:    &callID,
						Name:      &funcName,
						Arguments: &funcArgs,
						Status:    &completedStatus,
					})
					clientSideCalls = append(clientSideCalls, tc)
				}
			}

			if len(clientSideCalls) > 0 {
				// Append assistant message with client-side calls and break
				messages = append(messages, api.Message{
					Role:      "assistant",
					ToolCalls: clientSideCalls,
				})
				break // client handles execution
			}
			// All calls were MCP — continue loop so LLM can reason with results
			continue
		}

		// Normal text response
		outputText := choice.Message.Content
		assistantRole := "assistant"
		completedStatus := "completed"
		allOutput = append(allOutput, schema.ItemField{
			Type:   "message",
			ID:     generateID("msg_"),
			Role:   &assistantRole,
			Status: &completedStatus,
			Content: []schema.ContentPart{
				{
					Type: "output_text",
					Text: &outputText,
				},
			},
		})

		// Append assistant message for storage
		messages = append(messages, api.Message{
			Role:    "assistant",
			Content: outputText,
		})

		// Set usage from LLM response
		resp.Usage = &schema.UsageField{
			InputTokens:  llmResp.Usage.PromptTokens,
			OutputTokens: accumulatedOutputTokens,
			TotalTokens:  llmResp.Usage.PromptTokens + accumulatedOutputTokens,
			InputTokensDetails: schema.InputTokensDetails{
				CachedTokens: 0,
			},
			OutputTokensDetails: schema.OutputTokensDetails{
				ReasoningTokens: 0,
			},
		}

		break
	}

	// 7. Set output
	resp.Output = allOutput
	if resp.Output == nil {
		resp.Output = make([]schema.ItemField, 0)
	}

	// 8. Set usage if not already set
	if resp.Usage == nil {
		resp.Usage = &schema.UsageField{
			InputTokensDetails:  schema.InputTokensDetails{},
			OutputTokensDetails: schema.OutputTokensDetails{},
		}
	}

	// 9. Mark as completed if not already marked
	if resp.Status == "in_progress" {
		resp.MarkCompleted()
	}

	// 10. Save response to state store
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

	// 11. Append items to conversation for the Conversations API
	if err := e.appendItemsToConversation(ctx, conversationID, req, allOutput); err != nil {
		// Log but don't fail the response
		_ = err
	}

	return resp, nil
}

// ProcessRequestStream processes a streaming Responses API request
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
			errorField := schema.ErrorField{
				Type:    "api_error",
				Message: fmt.Sprintf("failed to resolve conversation: %v", err),
			}
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: errorField,
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

		// Build conversation messages (including multi-turn history)
		var messages []api.Message
		if req.Conversation != nil && *req.Conversation != "" {
			messages, err = e.buildConversationMessagesFromConversation(ctx, conversationID, req)
		} else {
			messages, err = e.buildConversationMessages(ctx, req)
		}
		if err != nil {
			errorField := schema.ErrorField{
				Type:    "api_error",
				Message: fmt.Sprintf("failed to build conversation: %v", err),
			}
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: errorField,
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

		// Expand MCP tools into function tools
		expandedTools := req.Tools
		var mcpToolNames map[string]*mcp.Client
		if len(req.Tools) > 0 {
			var expandErr error
			expandedTools, mcpToolNames, expandErr = e.expandMCPTools(ctx, req.Tools)
			if expandErr != nil {
				errorField := schema.ErrorField{
					Type:    "api_error",
					Message: fmt.Sprintf("failed to expand MCP tools: %v", expandErr),
				}
				events <- &schema.ErrorStreamingEvent{
					Type:  "error",
					Error: errorField,
				}
				return
			}
		}

		// Expand file_search tools into function tools
		var fileSearchConfigs map[string]fileSearchConfig
		if len(expandedTools) > 0 {
			expandedTools, fileSearchConfigs = e.expandFileSearchTools(expandedTools)
		}

		// Build a modified request with expanded tools for the LLM
		expandedReq := *req
		expandedReq.Tools = expandedTools

		// Build LLM request
		llmReq := buildLLMRequest(model, messages, &expandedReq, true)

		// Get streaming response from LLM
		streamChan, err := e.llm.CreateChatCompletionStream(ctx, llmReq)
		if err != nil {
			errorField := schema.ErrorField{
				Type:    "api_error",
				Message: fmt.Sprintf("failed to start streaming: %v", err),
			}
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: errorField,
			}
			return
		}

		// Track accumulated tool calls from the stream
		type accumulatedToolCall struct {
			ID        string
			Name      string
			Arguments string
		}
		var toolCallAccum []accumulatedToolCall
		var finishReason string

		fullText := ""
		contentIndex := 0
		outputIndex := 0
		messageItemID := ""
		messageItemEmitted := false

		// Stream deltas
		for chunk := range streamChan {
			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0]

			// Capture finish reason
			if delta.FinishReason != nil {
				finishReason = *delta.FinishReason
			}

			// Handle tool call deltas
			if len(delta.Delta.ToolCalls) > 0 {
				for _, tcDelta := range delta.Delta.ToolCalls {
					idx := tcDelta.Index
					// Extend accumulator as needed
					for len(toolCallAccum) <= idx {
						toolCallAccum = append(toolCallAccum, accumulatedToolCall{})
					}
					if tcDelta.ID != "" {
						toolCallAccum[idx].ID = tcDelta.ID
					}
					if tcDelta.Function.Name != "" {
						toolCallAccum[idx].Name = tcDelta.Function.Name
					}
					if tcDelta.Function.Arguments != "" {
						toolCallAccum[idx].Arguments += tcDelta.Function.Arguments

						// Emit function_call_arguments.delta event
						events <- &schema.ResponseFunctionCallArgumentsDeltaStreamingEvent{
							Type:        "response.function_call_arguments.delta",
							ResponseID:  respID,
							OutputIndex: idx,
							Delta:       tcDelta.Function.Arguments,
						}
						seqNum++
					}
				}
				continue
			}

			// Handle text content
			textDelta := delta.Delta.Content
			if textDelta == "" {
				continue
			}

			// Emit output_item.added on first text
			if !messageItemEmitted {
				messageItemID = generateID("msg_")
				assistantRole := "assistant"
				inProgressStatus := "in_progress"
				messageItem := schema.ItemField{
					Type:    "message",
					ID:      messageItemID,
					Role:    &assistantRole,
					Status:  &inProgressStatus,
					Content: []schema.ContentPart{},
				}

				events <- &schema.ResponseOutputItemAddedStreamingEvent{
					Type:           "response.output_item.added",
					SequenceNumber: seqNum,
					OutputIndex:    outputIndex,
					Item:           messageItem,
				}
				seqNum++

				// Emit content_part.added so the SDK knows about the content slot
				emptyText := ""
				events <- &schema.ResponseContentPartAddedStreamingEvent{
					Type:           "response.content_part.added",
					SequenceNumber: seqNum,
					ItemID:         messageItemID,
					OutputIndex:    outputIndex,
					ContentIndex:   contentIndex,
					Part: schema.ContentPart{
						Type: "output_text",
						Text: &emptyText,
					},
				}
				seqNum++

				messageItemEmitted = true
			}

			fullText += textDelta

			// Send text delta event
			events <- &schema.ResponseOutputTextDeltaStreamingEvent{
				Type:           "response.output_text.delta",
				SequenceNumber: seqNum,
				ItemID:         messageItemID,
				OutputIndex:    outputIndex,
				ContentIndex:   contentIndex,
				Delta:          textDelta,
				Logprobs:       make([]interface{}, 0),
			}
			seqNum++
		}

		// Determine output based on finish reason
		var allOutput []schema.ItemField

		if finishReason == "tool_calls" && len(toolCallAccum) > 0 {
			// Emit tool call output items
			outputIdx := 0
			for _, tc := range toolCallAccum {
				mcpClient, isMCP := mcpToolNames[tc.Name]
				fsCfg, isFileSearch := fileSearchConfigs[tc.Name]

				completedStatus := "completed"
				callID := tc.ID
				funcName := tc.Name
				funcArgs := tc.Arguments
				toolItem := schema.ItemField{
					Type:      "function_call",
					ID:        generateID("fc_"),
					CallID:    &callID,
					Name:      &funcName,
					Arguments: &funcArgs,
					Status:    &completedStatus,
				}

				events <- &schema.ResponseOutputItemAddedStreamingEvent{
					Type:           "response.output_item.added",
					SequenceNumber: seqNum,
					OutputIndex:    outputIdx,
					Item:           toolItem,
				}
				seqNum++

				// Emit function_call_arguments.done
				events <- &schema.ResponseFunctionCallArgumentsDoneStreamingEvent{
					Type:        "response.function_call_arguments.done",
					ResponseID:  respID,
					OutputIndex: outputIdx,
					Arguments:   funcArgs,
				}
				seqNum++

				// Emit output_item.done
				events <- &schema.ResponseOutputItemDoneStreamingEvent{
					Type:           "response.output_item.done",
					SequenceNumber: seqNum,
					OutputIndex:    outputIdx,
					Item:           toolItem,
				}
				seqNum++

				allOutput = append(allOutput, toolItem)
				outputIdx++

				// Determine if this is a server-side tool call
				var serverSideOutput string
				isServerSide := false

				if isMCP {
					isServerSide = true
					args := parseJSONArgs(tc.Arguments)
					result, mcpErr := mcpClient.CallTool(ctx, tc.Name, args)
					if mcpErr != nil {
						serverSideOutput = fmt.Sprintf("Error calling tool: %v", mcpErr)
					} else {
						serverSideOutput = mcpResultToString(result)
					}
				} else if isFileSearch {
					isServerSide = true
					args := parseJSONArgs(tc.Arguments)
					query, _ := args["query"].(string)
					serverSideOutput = e.executeFileSearch(ctx, fsCfg, query)
				}

				if isServerSide {
					// Emit function_call_output item
					outputItem := schema.ItemField{
						Type:   "function_call_output",
						ID:     generateID("fco_"),
						CallID: &callID,
						Output: &serverSideOutput,
					}

					events <- &schema.ResponseOutputItemAddedStreamingEvent{
						Type:           "response.output_item.added",
						SequenceNumber: seqNum,
						OutputIndex:    outputIdx,
						Item:           outputItem,
					}
					seqNum++

					events <- &schema.ResponseOutputItemDoneStreamingEvent{
						Type:           "response.output_item.done",
						SequenceNumber: seqNum,
						OutputIndex:    outputIdx,
						Item:           outputItem,
					}
					seqNum++

					allOutput = append(allOutput, outputItem)
					outputIdx++

					// Append to messages for LLM context
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{{
							ID:   tc.ID,
							Type: "function",
							Function: api.ToolCallFunction{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						}},
					})
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    serverSideOutput,
						ToolCallID: tc.ID,
					})
				}
			}

			// Check if any tool calls were client-side (not MCP and not file_search)
			hasClientSide := false
			var clientSideTCs []api.ToolCall
			for _, tc := range toolCallAccum {
				_, isMCP := mcpToolNames[tc.Name]
				_, isFS := fileSearchConfigs[tc.Name]
				if !isMCP && !isFS {
					hasClientSide = true
					clientSideTCs = append(clientSideTCs, api.ToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: api.ToolCallFunction{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}

			if hasClientSide {
				// Append assistant message with client-side calls for storage
				messages = append(messages, api.Message{
					Role:      "assistant",
					ToolCalls: clientSideTCs,
				})
			}

			// If all calls were server-side (MCP or file_search), the agentic loop
			// in streaming doesn't re-iterate here, but we include the results in
			// the output.

		} else {
			// Normal text response
			if messageItemEmitted {
				// Send text done event
				events <- &schema.ResponseOutputTextDoneStreamingEvent{
					Type:           "response.output_text.done",
					SequenceNumber: seqNum,
					ItemID:         messageItemID,
					OutputIndex:    outputIndex,
					ContentIndex:   contentIndex,
					Text:           fullText,
					Logprobs:       make([]interface{}, 0),
				}
				seqNum++

				// Emit content_part.done
				events <- &schema.ResponseContentPartDoneStreamingEvent{
					Type:           "response.content_part.done",
					SequenceNumber: seqNum,
					ItemID:         messageItemID,
					OutputIndex:    outputIndex,
					ContentIndex:   contentIndex,
					Part: schema.ContentPart{
						Type: "output_text",
						Text: &fullText,
					},
				}
				seqNum++

				// Complete the message item
				completedStatus := "completed"
				assistantRole := "assistant"
				messageItem := schema.ItemField{
					Type:   "message",
					ID:     messageItemID,
					Role:   &assistantRole,
					Status: &completedStatus,
					Content: []schema.ContentPart{
						{
							Type: "output_text",
							Text: &fullText,
						},
					},
				}

				// Send output_item.done event
				events <- &schema.ResponseOutputItemDoneStreamingEvent{
					Type:           "response.output_item.done",
					SequenceNumber: seqNum,
					OutputIndex:    outputIndex,
					Item:           messageItem,
				}
				seqNum++

				allOutput = append(allOutput, messageItem)

				// Append assistant message for storage
				messages = append(messages, api.Message{
					Role:    "assistant",
					Content: fullText,
				})
			}
		}

		// Update response
		resp.Output = allOutput
		if resp.Output == nil {
			resp.Output = make([]schema.ItemField, 0)
		}

		resp.MarkCompleted()

		// Set usage stats
		inputLen := 0
		for _, m := range messages {
			inputLen += len(m.Content)
		}
		outputLen := len(fullText)
		for _, tc := range toolCallAccum {
			outputLen += len(tc.Arguments)
		}
		resp.Usage = &schema.UsageField{
			InputTokens:  inputLen / 4,
			OutputTokens: outputLen / 4,
			TotalTokens:  (inputLen + outputLen) / 4,
			InputTokensDetails: schema.InputTokensDetails{
				CachedTokens: 0,
			},
			OutputTokensDetails: schema.OutputTokensDetails{
				ReasoningTokens: 0,
			},
		}

		// Send response.completed event
		events <- &schema.ResponseCompletedStreamingEvent{
			Type:           "response.completed",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

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

// convertTruncationToResponse converts request truncation to response truncation
func convertTruncationToResponse(reqTruncation *schema.TruncationStrategyParam) *schema.TruncationStrategy {
	if reqTruncation == nil {
		return nil
	}
	return &schema.TruncationStrategy{
		Type:         reqTruncation.Type,
		LastMessages: reqTruncation.LastMessages,
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
		if req.Store != nil {
			schemaResp.Store = *req.Store
		}
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
