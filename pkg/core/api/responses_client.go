// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
)

// ResponsesAPIClient calls a backend's /v1/responses endpoint.
type ResponsesAPIClient interface {
	// CreateResponse sends a non-streaming request and returns the full response.
	CreateResponse(ctx context.Context, req *ResponsesAPIRequest) (*ResponsesAPIResponse, error)

	// CreateResponseStream sends a streaming request and returns a channel of SSE events.
	CreateResponseStream(ctx context.Context, req *ResponsesAPIRequest) (<-chan ResponsesStreamEvent, error)
}

// ResponsesAPIRequest represents a request sent to the backend's /v1/responses endpoint.
type ResponsesAPIRequest struct {
	Model             string          `json:"model"`
	Input             interface{}     `json:"input"`
	Instructions      *string         `json:"instructions,omitempty"`
	Tools             []ToolParam     `json:"tools,omitempty"`
	ToolChoice        interface{}     `json:"tool_choice,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	MaxOutputTokens   *int            `json:"max_output_tokens,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Stream            bool            `json:"stream,omitempty"`
	Reasoning         *ReasoningParam `json:"reasoning,omitempty"`
	FrequencyPenalty  *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64        `json:"presence_penalty,omitempty"`
	Truncation        *string         `json:"truncation,omitempty"`
	Text              interface{}     `json:"text,omitempty"`
	Store             *bool           `json:"store,omitempty"`
}

// ToolParam defines a function tool sent to the backend.
type ToolParam struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"`
}

// ReasoningParam configures reasoning behavior for the backend.
type ReasoningParam struct {
	Effort *string `json:"effort,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

// ResponsesAPIResponse is the response from the backend's /v1/responses endpoint.
type ResponsesAPIResponse struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"`
	Status            string       `json:"status"`
	Output            []OutputItem `json:"output"`
	Usage             *UsageInfo   `json:"usage,omitempty"`
	Model             string       `json:"model"`
	CreatedAt         float64      `json:"created_at"`
	Error             interface{}  `json:"error,omitempty"`
	IncompleteDetails interface{}  `json:"incomplete_details,omitempty"`
}

// OutputItem represents an item in the backend response output.
type OutputItem struct {
	Type      string        `json:"type"`
	ID        string        `json:"id"`
	Role      string        `json:"role,omitempty"`
	Content   []ContentItem `json:"content,omitempty"`
	Name      string        `json:"name,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
	Status    string        `json:"status,omitempty"`
	Output    string        `json:"output,omitempty"`
}

// ContentItem represents a content element within an output item.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// UsageInfo represents token usage from the backend.
type UsageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponsesStreamEvent represents a single SSE event from the backend.
// Data is kept as raw JSON so events can be forwarded without parsing.
type ResponsesStreamEvent struct {
	Type string          // SSE event type, e.g. "response.output_text.delta"
	Data json.RawMessage // raw JSON payload
}
