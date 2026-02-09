// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
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
	resp := schema.NewResponse(respID, req.Model)

	// 4. Load conversation history if this is a follow-up
	var conversationHistory []string
	if req.PreviousResponseID != "" {
		// TODO: Load from state store
		// For now, just note it in metadata
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]string)
		}
		resp.Metadata["previous_response_id"] = req.PreviousResponseID
	}

	// 5. Prepare LLM request
	inputText := extractInputText(req.Input)
	llmReq := &api.ChatCompletionRequest{
		Model: req.Model,
		Messages: []api.Message{
			{Role: "user", Content: inputText},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxOutputTokens,
		Stream:      false,
	}

	// Add instructions as system message if provided
	if req.Instructions != "" {
		llmReq.Messages = append([]api.Message{
			{Role: "system", Content: req.Instructions},
		}, llmReq.Messages...)
	}

	// 6. Call LLM
	llmResp, err := e.llm.CreateChatCompletion(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}

	// 7. Build output from LLM response
	outputText := ""
	if len(llmResp.Choices) > 0 {
		outputText = llmResp.Choices[0].Message.Content
	}

	resp.Output = []schema.OutputItem{
		{
			Type: "message",
			ID:   generateID("msg_"),
			Role: "assistant",
			Content: map[string]interface{}{
				"type": "text",
				"text": outputText,
			},
		},
	}

	// 8. Set usage from LLM response
	resp.Usage = &schema.Usage{
		InputTokens:  llmResp.Usage.PromptTokens,
		OutputTokens: llmResp.Usage.CompletionTokens,
		TotalTokens:  llmResp.Usage.TotalTokens,
	}

	// 9. Mark as completed
	resp.MarkCompleted()

	// 10. Save response to state store
	if err := e.sessions.SaveResponse(ctx, &state.Response{
		ID:                 resp.ID,
		PreviousResponseID: req.PreviousResponseID,
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
func (e *Engine) ProcessRequestStream(ctx context.Context, req *schema.ResponseRequest) (<-chan *schema.ResponseStreamEvent, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	events := make(chan *schema.ResponseStreamEvent, 10)

	go func() {
		defer close(events)

		respID := generateID("resp_")
		resp := schema.NewResponse(respID, req.Model)

		// Send response.created event
		events <- &schema.ResponseStreamEvent{
			Type:        "response.created",
			SequenceNum: 0,
			Response:    resp,
		}

		// Prepare LLM request
		inputText := extractInputText(req.Input)
		llmReq := &api.ChatCompletionRequest{
			Model: req.Model,
			Messages: []api.Message{
				{Role: "user", Content: inputText},
			},
			Temperature: req.Temperature,
			MaxTokens:   req.MaxOutputTokens,
			Stream:      true,
		}

		// Add instructions as system message if provided
		if req.Instructions != "" {
			llmReq.Messages = append([]api.Message{
				{Role: "system", Content: req.Instructions},
			}, llmReq.Messages...)
		}

		// Send output_item.added event
		events <- &schema.ResponseStreamEvent{
			Type:        "response.output_item.added",
			SequenceNum: 1,
			OutputIndex: 0,
		}

		// Call LLM streaming
		chunks, err := e.llm.CreateChatCompletionStream(ctx, llmReq)
		if err != nil {
			resp.MarkFailed("llm_error", "stream_error", err.Error())
			events <- &schema.ResponseStreamEvent{
				Type:     "response.failed",
				Response: resp,
			}
			return
		}

		// Stream chunks
		seqNum := 2
		totalOutput := ""
		for chunk := range chunks {
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta.Content
				if delta != "" {
					totalOutput += delta
					events <- &schema.ResponseStreamEvent{
						Type:         "response.output_text.delta",
						SequenceNum:  seqNum,
						Delta:        delta,
						OutputIndex:  0,
						ContentIndex: 0,
					}
					seqNum++
				}
			}
		}

		// Mark completed
		resp.MarkCompleted()
		resp.Output = []schema.OutputItem{
			{
				Type: "message",
				ID:   generateID("msg_"),
				Role: "assistant",
				Content: map[string]interface{}{
					"type": "text",
					"text": totalOutput,
				},
			},
		}
		resp.Usage = &schema.Usage{
			InputTokens:  countTokens(inputText),
			OutputTokens: countTokens(totalOutput),
			TotalTokens:  countTokens(inputText) + countTokens(totalOutput),
		}

		// Send response.completed event
		events <- &schema.ResponseStreamEvent{
			Type:        "response.completed",
			SequenceNum: seqNum,
			Response:    resp,
		}
	}()

	return events, nil
}

// Helper functions

func generateID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

func extractInputText(input interface{}) string {
	switch v := input.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok {
			return text
		}
	case []interface{}:
		// Handle array of messages
		if len(v) > 0 {
			if msg, ok := v[0].(map[string]interface{}); ok {
				if content, ok := msg["content"].(string); ok {
					return content
				}
			}
		}
	}
	return "Hello"
}

func countTokens(text string) int {
	// Rough approximation: ~4 chars per token
	return len(text) / 4
}

func splitWords(text string) []string {
	var words []string
	current := ""
	for _, char := range text {
		if char == ' ' {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

func timePtr(unixPtr *int64) *time.Time {
	if unixPtr == nil {
		return nil
	}
	t := time.Unix(*unixPtr, 0)
	return &t
}
