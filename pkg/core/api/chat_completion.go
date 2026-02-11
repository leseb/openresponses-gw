// Copyright Open Responses Gateway Authors
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
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	MaxTokens           *int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          interface{}     `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	Seed                *int64          `json:"seed,omitempty"`
	TopLogprobs         *int            `json:"top_logprobs,omitempty"`
	Logprobs            *bool           `json:"logprobs,omitempty"`
	ReasoningEffort     *string         `json:"reasoning_effort,omitempty"`
	PromptCacheKey      *string         `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier    *string         `json:"safety_identifier,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
}

// Tool represents a tool available to the model
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function tool
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"`
}

// ResponseFormat specifies the output format for the model
type ResponseFormat struct {
	Type       string      `json:"type"` // "text", "json_object", "json_schema"
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema defines a JSON schema for structured output
type JSONSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role         string               `json:"role"`                    // "system", "user", "assistant", "tool", "developer"
	Content      string               `json:"content"`                 // Message text content
	ContentParts []MessageContentPart `json:"content_parts,omitempty"` // Multimodal content parts (takes precedence over Content when non-empty)
	ToolCalls    []ToolCall           `json:"tool_calls,omitempty"`    // Tool calls (assistant messages)
	ToolCallID   string               `json:"tool_call_id,omitempty"`  // Tool call ID (tool messages)
}

// MessageContentPart represents a content part in a multimodal message
type MessageContentPart struct {
	Type     string           `json:"type"`                // "text", "image_url", "file"
	Text     string           `json:"text,omitempty"`      // Text content (when Type="text")
	ImageURL *MessageImageURL `json:"image_url,omitempty"` // Image URL (when Type="image_url")
	File     *MessageFile     `json:"file,omitempty"`      // File content (when Type="file")
}

// MessageImageURL represents an image URL in a content part
type MessageImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// MessageFile represents a file in a content part
type MessageFile struct {
	FileData string `json:"file_data,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// ToolCall represents a tool call made by the assistant
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the function name and arguments for a tool call
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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
	FinishReason string  `json:"finish_reason"` // "stop", "length", "tool_calls", "content_filter", etc.
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
	Role      string          `json:"role,omitempty"`       // Role (only in first chunk)
	Content   string          `json:"content,omitempty"`    // Incremental content
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"` // Incremental tool call updates
}

// ToolCallDelta represents an incremental tool call update in streaming
type ToolCallDelta struct {
	Index    int                   `json:"index"`
	ID       string                `json:"id,omitempty"`
	Type     string                `json:"type,omitempty"` // "function"
	Function ToolCallFunctionDelta `json:"function,omitempty"`
}

// ToolCallFunctionDelta represents incremental function call data in streaming
type ToolCallFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
