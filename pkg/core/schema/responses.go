// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"time"
)

// ResponseRequest represents a request to the /v1/responses endpoint
type ResponseRequest struct {
	// Model ID used to generate the response
	Model string `json:"model"`

	// Input can be a string or array of messages
	Input interface{} `json:"input,omitempty"`

	// Whether to stream the response
	Stream bool `json:"stream,omitempty"`

	// Instructions (system message)
	Instructions string `json:"instructions,omitempty"`

	// Previous response ID for multi-turn conversations
	PreviousResponseID string `json:"previous_response_id,omitempty"`

	// Tools available for the model to use
	Tools []Tool `json:"tools,omitempty"`

	// Temperature for sampling
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP for sampling
	TopP *float64 `json:"top_p,omitempty"`

	// MaxOutputTokens limits the response length
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`

	// Metadata for the request
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Response represents a response from the API
type Response struct {
	// Unique identifier
	ID string `json:"id"`

	// Object type, always "response"
	Object string `json:"object"`

	// Creation timestamp
	CreatedAt int64 `json:"created_at"`

	// Completion timestamp
	CompletedAt *int64 `json:"completed_at,omitempty"`

	// Model used
	Model string `json:"model"`

	// Status: "in_progress", "completed", "failed", "cancelled"
	Status string `json:"status"`

	// Output items
	Output []OutputItem `json:"output,omitempty"`

	// Error details if status is "failed"
	Error *ErrorDetails `json:"error,omitempty"`

	// Token usage
	Usage *Usage `json:"usage,omitempty"`

	// Metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// OutputItem represents an item in the response output
type OutputItem struct {
	Type    string      `json:"type"` // "message", "tool_call", "reasoning"
	ID      string      `json:"id,omitempty"`
	Role    string      `json:"role,omitempty"`
	Content interface{} `json:"content,omitempty"`
}

// Tool represents a tool that can be used by the model
type Tool struct {
	Type string                 `json:"type"` // "function", "file_search", "web_search", etc.
	Name string                 `json:"name,omitempty"`
	Spec map[string]interface{} `json:"spec,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ErrorDetails represents error information
type ErrorDetails struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// ResponseStreamEvent represents a streaming event
type ResponseStreamEvent struct {
	Type         string    `json:"type"`
	SequenceNum  int       `json:"sequence_number,omitempty"`
	Response     *Response `json:"response,omitempty"`
	Delta        string    `json:"delta,omitempty"`
	OutputIndex  int       `json:"output_index,omitempty"`
	ContentIndex int       `json:"content_index,omitempty"`
}

// Validate validates the request
func (r *ResponseRequest) Validate() error {
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if r.Input == nil {
		return fmt.Errorf("input is required")
	}
	return nil
}

// NewResponse creates a new Response with defaults
func NewResponse(id, model string) *Response {
	now := time.Now().Unix()
	return &Response{
		ID:        id,
		Object:    "response",
		CreatedAt: now,
		Model:     model,
		Status:    "in_progress",
		Output:    make([]OutputItem, 0),
	}
}

// MarkCompleted marks the response as completed
func (r *Response) MarkCompleted() {
	r.Status = "completed"
	now := time.Now().Unix()
	r.CompletedAt = &now
}

// MarkFailed marks the response as failed with an error
func (r *Response) MarkFailed(errType, code, message string) {
	r.Status = "failed"
	r.Error = &ErrorDetails{
		Type:    errType,
		Code:    code,
		Message: message,
	}
}
