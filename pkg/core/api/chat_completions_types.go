// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

// ChatCompletionRequest represents a request to the /v1/chat/completions endpoint.
type ChatCompletionRequest struct {
	Model             string               `json:"model"`
	Messages          []ChatCompletionMsg  `json:"messages"`
	Tools             []ChatCompletionTool `json:"tools,omitempty"`
	ToolChoice        interface{}          `json:"tool_choice,omitempty"`
	Stream            bool                 `json:"stream,omitempty"`
	Temperature       *float64             `json:"temperature,omitempty"`
	TopP              *float64             `json:"top_p,omitempty"`
	MaxTokens         *int                 `json:"max_tokens,omitempty"`
	FrequencyPenalty  *float64             `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64             `json:"presence_penalty,omitempty"`
	ParallelToolCalls *bool                `json:"parallel_tool_calls,omitempty"`
	TopLogprobs       *int                 `json:"top_logprobs,omitempty"`
	Logprobs          *bool                `json:"logprobs,omitempty"`
	StreamOptions     *ChatStreamOptions   `json:"stream_options,omitempty"`
}

// ChatCompletionMsg represents a message in the Chat Completions API.
type ChatCompletionMsg struct {
	Role       string                   `json:"role"`
	Content    interface{}              `json:"content,omitempty"` // string or []ChatCompletionContentPart
	ToolCalls  []ChatCompletionToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
}

// ChatCompletionContentPart represents a content part in a multimodal message.
type ChatCompletionContentPart struct {
	Type     string                  `json:"type"` // "text", "image_url", "file"
	Text     string                  `json:"text,omitempty"`
	ImageURL *ChatCompletionImageURL `json:"image_url,omitempty"`
	File     *ChatCompletionFile     `json:"file,omitempty"`
}

// ChatCompletionImageURL represents an image URL in a content part.
type ChatCompletionImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ChatCompletionFile represents a file in a content part.
type ChatCompletionFile struct {
	FileData string `json:"file_data,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// ChatCompletionTool represents a tool definition for Chat Completions.
type ChatCompletionTool struct {
	Type     string                     `json:"type"` // "function"
	Function ChatCompletionToolFunction `json:"function"`
}

// ChatCompletionToolFunction is the function definition inside a ChatCompletionTool.
type ChatCompletionToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"`
}

// ChatCompletionToolCall represents a tool call in a Chat Completions response.
type ChatCompletionToolCall struct {
	Index    *int                           `json:"index,omitempty"`
	ID       string                         `json:"id,omitempty"`
	Type     string                         `json:"type,omitempty"` // "function"
	Function ChatCompletionToolCallFunction `json:"function"`
}

// ChatCompletionToolCallFunction contains the function name and arguments for a tool call.
type ChatCompletionToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ChatCompletionResponse represents a non-streaming response from /v1/chat/completions.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Model   string                 `json:"model"`
	Created int64                  `json:"created"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *ChatCompletionUsage   `json:"usage,omitempty"`
}

// ChatCompletionChoice represents a choice in a non-streaming response.
type ChatCompletionChoice struct {
	Index        int                     `json:"index"`
	Message      ChatCompletionChoiceMsg `json:"message"`
	FinishReason string                  `json:"finish_reason"`
}

// ChatCompletionChoiceMsg is the message inside a non-streaming choice.
type ChatCompletionChoiceMsg struct {
	Role      string                   `json:"role"`
	Content   *string                  `json:"content,omitempty"`
	ToolCalls []ChatCompletionToolCall `json:"tool_calls,omitempty"`
}

// ChatCompletionChunk represents a streaming chunk from /v1/chat/completions.
type ChatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Model   string                      `json:"model"`
	Created int64                       `json:"created"`
	Choices []ChatCompletionChunkChoice `json:"choices"`
	Usage   *ChatCompletionUsage        `json:"usage,omitempty"`
}

// ChatCompletionChunkChoice represents a choice in a streaming chunk.
type ChatCompletionChunkChoice struct {
	Index        int                      `json:"index"`
	Delta        ChatCompletionChunkDelta `json:"delta"`
	FinishReason *string                  `json:"finish_reason,omitempty"`
}

// ChatCompletionChunkDelta represents the delta content in a streaming chunk.
type ChatCompletionChunkDelta struct {
	Role      string                   `json:"role,omitempty"`
	Content   *string                  `json:"content,omitempty"`
	ToolCalls []ChatCompletionToolCall `json:"tool_calls,omitempty"`
}

// ChatCompletionUsage represents token usage in a Chat Completions response.
type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatStreamOptions controls streaming behavior.
type ChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}
