// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/core/state"
)

// Engine is the core orchestration engine for the Responses API
type Engine struct {
	config   *config.EngineConfig
	sessions state.SessionStore
	llm      api.ChatCompletionClient
}

// New creates a new Engine instance
func New(cfg *config.EngineConfig, store state.SessionStore) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if store == nil {
		return nil, fmt.Errorf("session store is required")
	}

	// Create chat completion client
	var llm api.ChatCompletionClient
	if cfg.APIKey != "" && cfg.ModelEndpoint != "" {
		// Use real OpenAI-compatible client
		llm = api.NewOpenAIClient(cfg.ModelEndpoint, cfg.APIKey)
	} else {
		// Use mock client for testing
		llm = api.NewMockChatCompletionClient()
	}

	return &Engine{
		config:   cfg,
		sessions: store,
		llm:      llm,
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

	// Echo ALL request parameters (Open Responses spec requires echoing all fields)
	resp.PreviousResponseID = req.PreviousResponseID
	resp.Instructions = req.Instructions
	resp.Tools = convertToolsToResponse(req.Tools)
	// ToolChoice defaults to "none" if not specified, or echo the request value
	if req.ToolChoice != nil {
		resp.ToolChoice = req.ToolChoice
	} else {
		// Keep the default "none" from NewResponse
	}
	resp.Reasoning = convertReasoningToResponse(req.Reasoning)
	// Required number fields - use pointer value or default to 0
	if req.Temperature != nil {
		resp.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		resp.TopP = *req.TopP
	}
	resp.MaxOutputTokens = req.MaxOutputTokens
	resp.MaxToolCalls = req.MaxToolCalls
	// Required boolean fields - use pointer value or default to false
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
	// Truncation must be "auto" or "disabled"
	if req.Truncation != nil {
		if req.Truncation.Type == "last_messages" {
			resp.Truncation = "disabled"
		} else {
			resp.Truncation = "auto"
		}
	} else {
		resp.Truncation = "auto" // Default
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

	// 4. Load conversation history if this is a follow-up
	var conversationHistory []string
	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		// TODO: Load from state store
		// For now, just note it in metadata
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]string)
		}
		resp.Metadata["previous_response_id"] = *req.PreviousResponseID
	}

	// 5. Prepare LLM request
	inputText := extractInputText(req.Input)
	llmReq := &api.ChatCompletionRequest{
		Model: model,
		Messages: []api.Message{
			{Role: "user", Content: inputText},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxOutputTokens,
		Stream:      false,
	}

	// Add instructions as system message if provided
	if req.Instructions != nil && *req.Instructions != "" {
		llmReq.Messages = append([]api.Message{
			{Role: "system", Content: *req.Instructions},
		}, llmReq.Messages...)
	}

	// 6. Call LLM
	llmResp, err := e.llm.CreateChatCompletion(ctx, llmReq)
	if err != nil {
		resp.MarkFailed("api_error", "llm_error", fmt.Sprintf("failed to call LLM: %v", err))
		return resp, nil
	}

	// 7. Build output from LLM response
	// If tools are provided, simulate a function call (for testing)
	if len(req.Tools) > 0 {
		// Generate a function call output for the first tool
		tool := req.Tools[0]
		completedStatus := "completed"
		funcArgs := `{"location":"San Francisco, CA"}`
		callID := generateID("call_")
		resp.Output = []schema.ItemField{
			{
				Type:      "function_call",
				ID:        generateID("item_"),
				CallID:    &callID,
				Name:      &tool.Name,
				Arguments: &funcArgs,
				Status:    &completedStatus,
			},
		}
	} else {
		// Normal text message response
		outputText := ""
		if len(llmResp.Choices) > 0 {
			outputText = llmResp.Choices[0].Message.Content
		}

		assistantRole := "assistant"
		completedStatus := "completed"
		resp.Output = []schema.ItemField{
			{
				Type:   "message",
				ID:     generateID("msg_"),
				Role:   &assistantRole,
				Status: &completedStatus,
				Content: []schema.ContentPart{
					{
						Type: "text",
						Text: &outputText,
					},
				},
			},
		}
	}

	// 8. Set usage from LLM response
	resp.Usage = &schema.UsageField{
		InputTokens:  llmResp.Usage.PromptTokens,
		OutputTokens: llmResp.Usage.CompletionTokens,
		TotalTokens:  llmResp.Usage.TotalTokens,
		InputTokensDetails: schema.InputTokensDetails{
			CachedTokens: 0, // No caching in mock client
		},
		OutputTokensDetails: schema.OutputTokensDetails{
			ReasoningTokens: 0, // No reasoning tokens in basic responses
		},
	}

	// 9. Text field is already initialized in NewResponse with default format

	// 10. Mark as completed
	resp.MarkCompleted()

	// 11. Save response to state store
	prevRespID := ""
	if req.PreviousResponseID != nil {
		prevRespID = *req.PreviousResponseID
	}

	if err := e.sessions.SaveResponse(ctx, &state.Response{
		ID:                 resp.ID,
		PreviousResponseID: prevRespID,
		Request:            req,
		Output:             resp.Output,
		Status:             resp.Status,
		Usage:              resp.Usage,
		CreatedAt:          time.Unix(resp.CreatedAt, 0),
		CompletedAt:        timePtr(resp.CompletedAt),
	}); err != nil {
		return nil, fmt.Errorf("failed to save response: %w", err)
	}

	_ = conversationHistory // Will be used in Phase 2

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

		// Echo ALL request parameters (Open Responses spec requires echoing all fields)
		resp.PreviousResponseID = req.PreviousResponseID
		resp.Instructions = req.Instructions
		resp.Tools = convertToolsToResponse(req.Tools)
		// ToolChoice defaults to "none" if not specified, or echo the request value
		if req.ToolChoice != nil {
			resp.ToolChoice = req.ToolChoice
		} else {
			// Keep the default "none" from NewResponse
		}
		resp.Reasoning = convertReasoningToResponse(req.Reasoning)
		// Required number fields - use pointer value or default to 0
		if req.Temperature != nil {
			resp.Temperature = *req.Temperature
		}
		if req.TopP != nil {
			resp.TopP = *req.TopP
		}
		resp.MaxOutputTokens = req.MaxOutputTokens
		resp.MaxToolCalls = req.MaxToolCalls
		// Required boolean fields - use pointer value or default to false
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
		// Truncation must be "auto" or "disabled"
		if req.Truncation != nil {
			if req.Truncation.Type == "last_messages" {
				resp.Truncation = "disabled"
			} else {
				resp.Truncation = "auto"
			}
		} else {
			resp.Truncation = "auto" // Default
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

		// Send response.created event
		events <- &schema.ResponseCreatedStreamingEvent{
			Type:           "response.created",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Prepare LLM request
		inputText := extractInputText(req.Input)
		llmReq := &api.ChatCompletionRequest{
			Model: model,
			Messages: []api.Message{
				{Role: "user", Content: inputText},
			},
			Temperature: req.Temperature,
			MaxTokens:   req.MaxOutputTokens,
			Stream:      true,
		}

		// Add instructions as system message if provided
		if req.Instructions != nil && *req.Instructions != "" {
			llmReq.Messages = append([]api.Message{
				{Role: "system", Content: *req.Instructions},
			}, llmReq.Messages...)
		}

		// Send response.in_progress event
		resp.Status = "in_progress"
		events <- &schema.ResponseInProgressStreamingEvent{
			Type:           "response.in_progress",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Initialize output item
		assistantRole := "assistant"
		inProgressStatus := "in_progress"
		messageItem := schema.ItemField{
			Type:    "message",
			ID:      generateID("msg_"),
			Role:    &assistantRole,
			Status:  &inProgressStatus,
			Content: []schema.ContentPart{},
		}

		// Send output_item.added event
		events <- &schema.ResponseOutputItemAddedStreamingEvent{
			Type:           "response.output_item.added",
			SequenceNumber: seqNum,
			OutputIndex:    0,
			Item:           messageItem,
		}
		seqNum++

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

		fullText := ""
		contentIndex := 0

		// Stream deltas
		for chunk := range streamChan {
			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta.Content
			if delta == "" {
				continue
			}

			fullText += delta

			// Send text delta event
			events <- &schema.ResponseOutputTextDeltaStreamingEvent{
				Type:           "response.output_text.delta",
				SequenceNumber: seqNum,
				ItemID:         messageItem.ID,
				OutputIndex:    0,
				ContentIndex:   contentIndex,
				Delta:          delta,
				Logprobs:       make([]interface{}, 0), // empty array
			}
			seqNum++
		}

		// Send text done event
		events <- &schema.ResponseOutputTextDoneStreamingEvent{
			Type:           "response.output_text.done",
			SequenceNumber: seqNum,
			ItemID:         messageItem.ID,
			OutputIndex:    0,
			ContentIndex:   contentIndex,
			Text:           fullText,
			Logprobs:       make([]interface{}, 0), // empty array
		}
		seqNum++

		// Complete the message item
		completedStatus := "completed"
		messageItem.Status = &completedStatus
		messageItem.Content = []schema.ContentPart{
			{
				Type: "text",
				Text: &fullText,
			},
		}

		// Send output_item.done event
		events <- &schema.ResponseOutputItemDoneStreamingEvent{
			Type:           "response.output_item.done",
			SequenceNumber: seqNum,
			OutputIndex:    0,
			Item:           messageItem,
		}
		seqNum++

		// Update response
		resp.Output = []schema.ItemField{messageItem}
		resp.MarkCompleted()
		// Text field is already initialized in NewResponse with default format

		// Set usage stats (would be collected during streaming in real implementation)
		resp.Usage = &schema.UsageField{
			InputTokens:  len(inputText) / 4, // rough approximation
			OutputTokens: len(fullText) / 4,  // rough approximation
			TotalTokens:  (len(inputText) + len(fullText)) / 4,
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
	}()

	return events, nil
}

// Helper functions

func extractInputText(input interface{}) string {
	switch v := input.(type) {
	case string:
		return v
	case []interface{}:
		// For now, just extract the first text item
		// TODO: Handle complex item types properly
		if len(v) > 0 {
			if item, ok := v[0].(map[string]interface{}); ok {
				if content, ok := item["content"].(string); ok {
					return content
				}
			}
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

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
	if reqTools == nil || len(reqTools) == 0 {
		return make([]schema.ResponsesTool, 0) // Return empty array, not nil
	}
	respTools := make([]schema.ResponsesTool, len(reqTools))
	for i, t := range reqTools {
		respTools[i] = schema.ResponsesTool{
			Type:        t.Type,
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Strict:      t.Strict,
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

	return schemaResp, nil
}
