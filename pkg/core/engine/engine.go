// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/leseb/openai-responses-gateway/pkg/core/config"
	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
	"github.com/leseb/openai-responses-gateway/pkg/core/state"
)

// Engine is the core orchestration engine for the Responses API
type Engine struct {
	config   *config.EngineConfig
	sessions state.SessionStore
}

// New creates a new Engine instance
func New(cfg *config.EngineConfig, store state.SessionStore) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if store == nil {
		return nil, fmt.Errorf("session store is required")
	}

	return &Engine{
		config:   cfg,
		sessions: store,
	}, nil
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
	// TODO: This is a placeholder - will be implemented in later phases
	// For now, just create a simple mock response
	inputText := extractInputText(req.Input)

	// 6. Mock LLM call (will be replaced with real LLM client)
	outputText := fmt.Sprintf("Mock response to: %s", inputText)

	// 7. Build output
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

	// 8. Set usage
	resp.Usage = &schema.Usage{
		InputTokens:  countTokens(inputText),
		OutputTokens: countTokens(outputText),
		TotalTokens:  countTokens(inputText) + countTokens(outputText),
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

		// Mock streaming output
		inputText := extractInputText(req.Input)
		outputText := fmt.Sprintf("Mock streaming response to: %s", inputText)

		// Send output_item.added event
		events <- &schema.ResponseStreamEvent{
			Type:        "response.output_item.added",
			SequenceNum: 1,
			OutputIndex: 0,
		}

		// Stream the output text in chunks
		words := splitWords(outputText)
		for i, word := range words {
			events <- &schema.ResponseStreamEvent{
				Type:         "response.output_text.delta",
				SequenceNum:  2 + i,
				Delta:        word + " ",
				OutputIndex:  0,
				ContentIndex: 0,
			}
			time.Sleep(50 * time.Millisecond) // Simulate streaming delay
		}

		// Mark completed
		resp.MarkCompleted()
		resp.Usage = &schema.Usage{
			InputTokens:  countTokens(inputText),
			OutputTokens: countTokens(outputText),
			TotalTokens:  countTokens(inputText) + countTokens(outputText),
		}

		// Send response.completed event
		events <- &schema.ResponseStreamEvent{
			Type:        "response.completed",
			SequenceNum: 2 + len(words),
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
