// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MockChatCompletionClient is a mock implementation for testing
// It generates predictable responses based on the input
type MockChatCompletionClient struct{}

// NewMockChatCompletionClient creates a new mock client
func NewMockChatCompletionClient() *MockChatCompletionClient {
	return &MockChatCompletionClient{}
}

// CreateChatCompletion implements ChatCompletionClient.CreateChatCompletion
func (m *MockChatCompletionClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Extract user message
	userMessage := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			userMessage = msg.Content
			break
		}
	}

	// Generate mock response
	mockContent := fmt.Sprintf("Mock response to: %s", userMessage)

	return &ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-mock-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: mockContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     estimateTokens(userMessage),
			CompletionTokens: estimateTokens(mockContent),
			TotalTokens:      estimateTokens(userMessage) + estimateTokens(mockContent),
		},
	}, nil
}

// CreateChatCompletionStream implements ChatCompletionClient.CreateChatCompletionStream
func (m *MockChatCompletionClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamChunk, error) {
	chunks := make(chan StreamChunk, 10)

	go func() {
		defer close(chunks)

		// Extract user message
		userMessage := ""
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				userMessage = msg.Content
				break
			}
		}

		// Generate mock streaming response
		mockContent := fmt.Sprintf("Mock streaming response to: %s", userMessage)
		words := strings.Fields(mockContent)

		for i, word := range words {
			chunk := StreamChunk{
				ID:      fmt.Sprintf("chatcmpl-mock-%d", time.Now().Unix()),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []StreamDelta{
					{
						Index: 0,
						Delta: MessageDelta{
							Content: word + " ",
						},
					},
				},
			}

			// Add finish reason to last chunk
			if i == len(words)-1 {
				finishReason := "stop"
				chunk.Choices[0].FinishReason = &finishReason
			}

			// Send chunk
			select {
			case chunks <- chunk:
			case <-ctx.Done():
				return
			}

			// Simulate delay between words
			time.Sleep(50 * time.Millisecond)
		}
	}()

	return chunks, nil
}

// estimateTokens provides a rough token count estimate
// Using ~4 characters per token as a simple heuristic
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 4
}
