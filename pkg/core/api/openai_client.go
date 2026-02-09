// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"io"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// OpenAIClient implements ChatCompletionClient using the official OpenAI Go SDK
// Supports OpenAI, Ollama, vLLM, and other OpenAI-compatible backends
type OpenAIClient struct {
	client openai.Client
}

// NewOpenAIClient creates a new OpenAI-compatible client using the official SDK
// The baseURL parameter allows connecting to OpenAI-compatible backends like Ollama and vLLM
func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	opts := []option.RequestOption{}

	// Set custom base URL if provided (for Ollama, vLLM, etc.)
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	// Set API key if provided (optional for local backends like Ollama)
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	} else {
		// Use a dummy key for local backends that don't require authentication
		opts = append(opts, option.WithAPIKey("dummy"))
	}

	return &OpenAIClient{
		client: openai.NewClient(opts...),
	}
}

// CreateChatCompletion implements ChatCompletionClient.CreateChatCompletion
func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Convert our request to OpenAI SDK format
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(msg.Content))
		case "user":
			messages = append(messages, openai.UserMessage(msg.Content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(msg.Content))
		default:
			return nil, fmt.Errorf("unsupported message role: %s", msg.Role)
		}
	}

	// Build chat completion request
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}

	// Add optional parameters
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if req.MaxTokens != nil {
		params.MaxTokens = openai.Int(int64(*req.MaxTokens))
	}

	// Call OpenAI API
	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Convert response to our format
	choices := make([]Choice, 0, len(completion.Choices))
	for _, choice := range completion.Choices {
		choices = append(choices, Choice{
			Index: int(choice.Index),
			Message: Message{
				Role:    string(choice.Message.Role),
				Content: choice.Message.Content,
			},
			FinishReason: string(choice.FinishReason),
		})
	}

	return &ChatCompletionResponse{
		ID:      completion.ID,
		Object:  string(completion.Object),
		Created: completion.Created,
		Model:   completion.Model,
		Choices: choices,
		Usage: Usage{
			PromptTokens:     int(completion.Usage.PromptTokens),
			CompletionTokens: int(completion.Usage.CompletionTokens),
			TotalTokens:      int(completion.Usage.TotalTokens),
		},
	}, nil
}

// CreateChatCompletionStream implements ChatCompletionClient.CreateChatCompletionStream
func (c *OpenAIClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamChunk, error) {
	// Convert our request to OpenAI SDK format
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(msg.Content))
		case "user":
			messages = append(messages, openai.UserMessage(msg.Content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(msg.Content))
		default:
			return nil, fmt.Errorf("unsupported message role: %s", msg.Role)
		}
	}

	// Build streaming chat completion request
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}

	// Add optional parameters
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if req.MaxTokens != nil {
		params.MaxTokens = openai.Int(int64(*req.MaxTokens))
	}

	// Create streaming completion
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)

	// Create channel for chunks
	chunks := make(chan StreamChunk, 10)

	// Start goroutine to read stream
	go func() {
		defer close(chunks)
		defer stream.Close()

		// Read chunks from stream
		for stream.Next() {
			chunk := stream.Current()

			// Convert to our format
			deltas := make([]StreamDelta, 0, len(chunk.Choices))
			for _, choice := range chunk.Choices {
				delta := StreamDelta{
					Index: int(choice.Index),
					Delta: MessageDelta{
						Role:    string(choice.Delta.Role),
						Content: choice.Delta.Content,
					},
				}

				// Add finish reason if present
				if choice.FinishReason != "" {
					finishReason := string(choice.FinishReason)
					delta.FinishReason = &finishReason
				}

				deltas = append(deltas, delta)
			}

			// Send chunk
			select {
			case chunks <- StreamChunk{
				ID:      chunk.ID,
				Object:  string(chunk.Object),
				Created: chunk.Created,
				Model:   chunk.Model,
				Choices: deltas,
			}:
			case <-ctx.Done():
				return
			}
		}

		// Check for errors
		if err := stream.Err(); err != nil && err != io.EOF {
			// Log error but can't return it through channel
			// In production, consider using a separate error channel or logging
			_ = err
		}
	}()

	return chunks, nil
}
