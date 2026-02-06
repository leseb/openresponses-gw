// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMClient interface for calling LLM backends
type LLMClient interface {
	// CreateChatCompletion calls the LLM with a chat completion request
	CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)

	// CreateChatCompletionStream calls the LLM with streaming
	CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamChunk, error)
}

// ChatCompletionRequest represents a request to the LLM
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents a response from the LLM
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a streaming chunk
type StreamChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []StreamDelta `json:"choices"`
}

// StreamDelta represents a delta in streaming
type StreamDelta struct {
	Index        int           `json:"index"`
	Delta        MessageDelta  `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

// MessageDelta represents a message delta
type MessageDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// OpenAIClient implements LLMClient for OpenAI-compatible APIs
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAIClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// CreateChatCompletion implements LLMClient.CreateChatCompletion
func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Ensure stream is false
	req.Stream = false

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Send request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var completion ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &completion, nil
}

// CreateChatCompletionStream implements LLMClient.CreateChatCompletionStream
func (c *OpenAIClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamChunk, error) {
	// Ensure stream is true
	req.Stream = true

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Send request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Create channel for chunks
	chunks := make(chan StreamChunk, 10)

	// Start goroutine to read stream
	go func() {
		defer close(chunks)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Check for SSE data line
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			// Extract data
			data := strings.TrimPrefix(line, "data: ")

			// Check for [DONE] message
			if data == "[DONE]" {
				break
			}

			// Parse chunk
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// Log error but continue
				continue
			}

			// Send chunk
			select {
			case chunks <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return chunks, nil
}

// MockLLMClient is a mock implementation for testing
type MockLLMClient struct{}

// NewMockLLMClient creates a new mock client
func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{}
}

// CreateChatCompletion implements LLMClient.CreateChatCompletion
func (m *MockLLMClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Extract user message
	userMessage := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			userMessage = msg.Content
			break
		}
	}

	return &ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: fmt.Sprintf("Mock response to: %s", userMessage),
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     len(userMessage) / 4,
			CompletionTokens: 10,
			TotalTokens:      len(userMessage)/4 + 10,
		},
	}, nil
}

// CreateChatCompletionStream implements LLMClient.CreateChatCompletionStream
func (m *MockLLMClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamChunk, error) {
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

		words := strings.Fields(fmt.Sprintf("Mock streaming response to: %s", userMessage))

		for i, word := range words {
			chunk := StreamChunk{
				ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
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

			// Last chunk
			if i == len(words)-1 {
				finishReason := "stop"
				chunk.Choices[0].FinishReason = &finishReason
			}

			select {
			case chunks <- chunk:
			case <-ctx.Done():
				return
			}

			time.Sleep(50 * time.Millisecond)
		}
	}()

	return chunks, nil
}
