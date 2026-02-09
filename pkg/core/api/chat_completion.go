// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// ChatCompletionClient interface for calling chat completion backends
type ChatCompletionClient interface {
	// CreateChatCompletion calls the backend with a chat completion request
	CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)

	// CreateChatCompletionStream calls the backend with streaming
	CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamChunk, error)
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Message text content
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	ID      string   `json:"id"`      // Unique completion ID
	Object  string   `json:"object"`  // "chat.completion"
	Created int64    `json:"created"` // Unix timestamp
	Model   string   `json:"model"`   // Model used
	Choices []Choice `json:"choices"` // Generated completions
	Usage   Usage    `json:"usage"`   // Token usage statistics
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`         // Choice index (usually 0)
	Message      Message `json:"message"`       // Generated message
	FinishReason string  `json:"finish_reason"` // "stop", "length", "content_filter", etc.
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`     // Tokens in prompt
	CompletionTokens int `json:"completion_tokens"` // Tokens in completion
	TotalTokens      int `json:"total_tokens"`      // Total tokens used
}

// StreamChunk represents a streaming chunk in Server-Sent Events format
type StreamChunk struct {
	ID      string        `json:"id"`      // Unique completion ID
	Object  string        `json:"object"`  // "chat.completion.chunk"
	Created int64         `json:"created"` // Unix timestamp
	Model   string        `json:"model"`   // Model used
	Choices []StreamDelta `json:"choices"` // Incremental deltas
}

// StreamDelta represents an incremental delta in streaming
type StreamDelta struct {
	Index        int          `json:"index"`                   // Choice index
	Delta        MessageDelta `json:"delta"`                   // Content delta
	FinishReason *string      `json:"finish_reason,omitempty"` // Set on final chunk
}

// MessageDelta represents an incremental message update
type MessageDelta struct {
	Role    string `json:"role,omitempty"`    // Role (only in first chunk)
	Content string `json:"content,omitempty"` // Incremental content
}
