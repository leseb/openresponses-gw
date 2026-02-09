// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/leseb/openai-responses-gateway/pkg/core/api"
	"github.com/leseb/openai-responses-gateway/pkg/core/config"
	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
	"github.com/leseb/openai-responses-gateway/pkg/core/state"
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
	resp.ToolChoice = req.ToolChoice
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
	resp.Truncation = req.Truncation
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

	// 8. Set usage from LLM response
	resp.Usage = &schema.UsageField{
		InputTokens:  llmResp.Usage.PromptTokens,
		OutputTokens: llmResp.Usage.CompletionTokens,
		TotalTokens:  llmResp.Usage.TotalTokens,
	}

	// 9. Text field is complex object - skip for now (not a simple string)
	resp.Text = nil

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

		// Echo ALL request parameters (Open Responses spec requires echoing all fields)
		resp.PreviousResponseID = req.PreviousResponseID
		resp.Instructions = req.Instructions
		resp.Tools = convertToolsToResponse(req.Tools)
		resp.ToolChoice = req.ToolChoice
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
		resp.Truncation = req.Truncation
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
			Type:     "response.created",
			Response: *resp,
		}

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
			Type:     "response.in_progress",
			Response: *resp,
		}

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
			Type:        "response.output_item.added",
			ResponseID:  respID,
			OutputIndex: 0,
			Item:        messageItem,
		}

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
				Type:         "response.output_text.delta",
				ResponseID:   respID,
				OutputIndex:  0,
				ContentIndex: contentIndex,
				Delta:        delta,
			}
		}

		// Send text done event
		events <- &schema.ResponseOutputTextDoneStreamingEvent{
			Type:         "response.output_text.done",
			ResponseID:   respID,
			OutputIndex:  0,
			ContentIndex: contentIndex,
			Text:         fullText,
		}

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
			Type:        "response.output_item.done",
			ResponseID:  respID,
			OutputIndex: 0,
			Item:        messageItem,
		}

		// Update response
		resp.Output = []schema.ItemField{messageItem}
		resp.MarkCompleted()
		resp.Text = nil // Text field is complex object - skip for now

		// Send response.completed event
		events <- &schema.ResponseCompletedStreamingEvent{
			Type:     "response.completed",
			Response: *resp,
		}
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
			Type:     t.Type,
			Function: t.Function,
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
