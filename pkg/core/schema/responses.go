// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"time"
)

// ResponseRequest represents a request to the /v1/responses endpoint
// Fully compliant with Open Responses specification
type ResponseRequest struct {
	// Model ID used to generate the response
	Model *string `json:"model,omitempty"`

	// Input can be a string or array of items
	Input interface{} `json:"input,omitempty"` // string | []ItemParam

	// Previous response ID for multi-turn conversations
	PreviousResponseID *string `json:"previous_response_id,omitempty"`

	// Controls what to include in the response
	Include []string `json:"include,omitempty"` // IncludeEnum

	// Tools available for the model to use
	Tools []ResponsesToolParam `json:"tools,omitempty"`

	// Controls which tool the model should use
	ToolChoice interface{} `json:"tool_choice,omitempty"` // ToolChoiceParam

	// Metadata key-value pairs (max 16, 512 chars per value)
	Metadata map[string]string `json:"metadata,omitempty"`

	// Reasoning configuration for o-series models
	Reasoning *ReasoningParam `json:"reasoning,omitempty"`

	// Instructions (system message)
	Instructions *string `json:"instructions,omitempty"`

	// Temperature for sampling (0-2)
	Temperature *float64 `json:"temperature,omitempty"`

	// Top P for nucleus sampling
	TopP *float64 `json:"top_p,omitempty"`

	// Maximum output tokens
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`

	// Maximum number of tool calls
	MaxToolCalls *int `json:"max_tool_calls,omitempty"`

	// Allow parallel tool calls
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`

	// Store conversation for later retrieval
	Store *bool `json:"store,omitempty"`

	// Frequency penalty (-2.0 to 2.0)
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`

	// Presence penalty (-2.0 to 2.0)
	PresencePenalty *float64 `json:"presence_penalty,omitempty"`

	// Truncation strategy for context
	Truncation *TruncationStrategyParam `json:"truncation,omitempty"`

	// Number of top log probabilities to return
	TopLogprobs *int `json:"top_logprobs,omitempty"`

	// Service tier for routing (auto, default)
	ServiceTier *string `json:"service_tier,omitempty"`

	// Process in background
	Background *bool `json:"background,omitempty"`

	// Cache key for prompt caching
	PromptCacheKey *string `json:"prompt_cache_key,omitempty"`

	// Safety identifier for content filtering
	SafetyIdentifier *string `json:"safety_identifier,omitempty"`

	// Whether to stream the response (HTTP-specific, not in spec but required for SSE)
	Stream bool `json:"stream,omitempty"`
}

// Response represents a response from the API
// Fully compliant with Open Responses specification
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

	// Status: "queued", "in_progress", "completed", "failed", "incomplete"
	Status string `json:"status"`

	// Output items
	Output []ItemField `json:"output,omitempty"`

	// Token usage
	Usage *UsageField `json:"usage,omitempty"`

	// Error details if status is "failed"
	Error *ErrorField `json:"error,omitempty"`

	// Incomplete details if status is "incomplete"
	IncompleteDetails *IncompleteDetailsField `json:"incomplete_details,omitempty"`

	// Metadata (echoed from request)
	Metadata map[string]string `json:"metadata,omitempty"`

	// Echo request parameters
	PreviousResponseID *string                  `json:"previous_response_id,omitempty"`
	Instructions       *string                  `json:"instructions,omitempty"`
	Tools              []ResponsesTool          `json:"tools,omitempty"`
	ToolChoice         interface{}              `json:"tool_choice,omitempty"`
	Reasoning          *ReasoningConfig         `json:"reasoning,omitempty"`
	Temperature        *float64                 `json:"temperature,omitempty"`
	TopP               *float64                 `json:"top_p,omitempty"`
	MaxOutputTokens    *int                     `json:"max_output_tokens,omitempty"`
	MaxToolCalls       *int                     `json:"max_tool_calls,omitempty"`
	ParallelToolCalls  *bool                    `json:"parallel_tool_calls,omitempty"`
	Store              *bool                    `json:"store,omitempty"`
	FrequencyPenalty   *float64                 `json:"frequency_penalty,omitempty"`
	PresencePenalty    *float64                 `json:"presence_penalty,omitempty"`
	Truncation         *TruncationStrategy      `json:"truncation,omitempty"`
	TopLogprobs        *int                     `json:"top_logprobs,omitempty"`
	ServiceTier        *string                  `json:"service_tier,omitempty"`
	Background         *bool                    `json:"background,omitempty"`
	PromptCacheKey     *string                  `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier   *string                  `json:"safety_identifier,omitempty"`

	// Convenience field: concatenated text from all output items
	Text *string `json:"text,omitempty"`
}

// ItemField represents an output item (discriminated union by type)
type ItemField struct {
	Type string `json:"type"` // "message", "function_call", "function_call_output", "reasoning"
	ID   string `json:"id,omitempty"`

	// Message fields
	Role    *string       `json:"role,omitempty"` // "user", "assistant", "system", "developer"
	Content []ContentPart `json:"content,omitempty"`
	Status  *string       `json:"status,omitempty"` // "in_progress", "completed"

	// Function call fields
	Name      *string `json:"name,omitempty"`
	CallID    *string `json:"call_id,omitempty"`
	Arguments *string `json:"arguments,omitempty"`

	// Function output fields
	Output *string `json:"output,omitempty"`

	// Reasoning fields
	Summary *string `json:"summary,omitempty"`
}

// ContentPart represents a part of message content
type ContentPart struct {
	Type string `json:"type"` // "text", "image", "file", "video", "refusal", "output_text_annotation"

	// Text content
	Text *string `json:"text,omitempty"`

	// Image content
	ImageURL *ImageURL `json:"image_url,omitempty"`

	// File content
	FileID *string `json:"file_id,omitempty"`

	// Video content
	VideoURL *VideoURL `json:"video_url,omitempty"`

	// Annotation fields
	StartIndex *int    `json:"start_index,omitempty"`
	EndIndex   *int    `json:"end_index,omitempty"`
	FileID2    *string `json:"file_id,omitempty"` // For annotations
}

// ImageURL represents an image URL
type ImageURL struct {
	URL    string  `json:"url"`
	Detail *string `json:"detail,omitempty"` // "auto", "low", "high"
}

// VideoURL represents a video URL
type VideoURL struct {
	URL string `json:"url"`
}

// UsageField represents token usage
type UsageField struct {
	InputTokens          int                   `json:"input_tokens"`
	OutputTokens         int                   `json:"output_tokens"`
	TotalTokens          int                   `json:"total_tokens"`
	InputTokensDetails   *InputTokensDetails   `json:"input_tokens_details,omitempty"`
	OutputTokensDetails  *OutputTokensDetails  `json:"output_tokens_details,omitempty"`
}

// InputTokensDetails provides breakdown of input tokens
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	TextTokens   int `json:"text_tokens,omitempty"`
	ImageTokens  int `json:"image_tokens,omitempty"`
}

// OutputTokensDetails provides breakdown of output tokens
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	AudioTokens     int `json:"audio_tokens,omitempty"`
	TextTokens      int `json:"text_tokens,omitempty"`
}

// ErrorField represents error information
type ErrorField struct {
	Type    string  `json:"type"`
	Code    *string `json:"code,omitempty"`
	Message string  `json:"message"`
	Param   *string `json:"param,omitempty"`
}

// IncompleteDetailsField represents why response is incomplete
type IncompleteDetailsField struct {
	Reason string `json:"reason"` // "max_output_tokens", "content_filter"
}

// ResponsesToolParam represents a tool definition (request)
type ResponsesToolParam struct {
	Type     string                 `json:"type"` // "function", "file_search", "web_search"
	Function *FunctionDefinition    `json:"function,omitempty"`
	// Additional tool-specific fields can be added
}

// ResponsesTool represents a tool (response echo)
type ResponsesTool struct {
	Type     string              `json:"type"`
	Function *FunctionDefinition `json:"function,omitempty"`
}

// FunctionDefinition represents a function tool
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"` // JSON Schema
	Strict      *bool                  `json:"strict,omitempty"`
}

// ReasoningParam represents reasoning configuration (request)
type ReasoningParam struct {
	Type   string           `json:"type"`   // "default", "extended"
	Effort *string          `json:"effort,omitempty"` // "low", "medium", "high"
	Budget *ReasoningBudget `json:"budget,omitempty"`
}

// ReasoningConfig represents reasoning configuration (response)
type ReasoningConfig struct {
	Type   string           `json:"type"`
	Effort *string          `json:"effort,omitempty"`
	Budget *ReasoningBudget `json:"budget,omitempty"`
}

// ReasoningBudget represents reasoning token budget
type ReasoningBudget struct {
	TokenBudget *int `json:"token_budget,omitempty"`
}

// TruncationStrategyParam represents truncation configuration (request)
type TruncationStrategyParam struct {
	Type         string `json:"type"`          // "auto", "last_messages"
	LastMessages *int   `json:"last_messages,omitempty"`
}

// TruncationStrategy represents truncation configuration (response)
type TruncationStrategy struct {
	Type         string `json:"type"`
	LastMessages *int   `json:"last_messages,omitempty"`
}

// Streaming Event Types (24 event types per Open Responses spec)

// BaseStreamingEvent contains common fields for all events
type BaseStreamingEvent struct {
	Type string `json:"type"`
}

// ResponseCreatedStreamingEvent - response.created
type ResponseCreatedStreamingEvent struct {
	Type     string   `json:"type"` // "response.created"
	Response Response `json:"response"`
}

// ResponseQueuedStreamingEvent - response.queued
type ResponseQueuedStreamingEvent struct {
	Type     string   `json:"type"` // "response.queued"
	Response Response `json:"response"`
}

// ResponseInProgressStreamingEvent - response.in_progress
type ResponseInProgressStreamingEvent struct {
	Type     string   `json:"type"` // "response.in_progress"
	Response Response `json:"response"`
}

// ResponseCompletedStreamingEvent - response.completed
type ResponseCompletedStreamingEvent struct {
	Type     string   `json:"type"` // "response.completed"
	Response Response `json:"response"`
}

// ResponseFailedStreamingEvent - response.failed
type ResponseFailedStreamingEvent struct {
	Type     string   `json:"type"` // "response.failed"
	Response Response `json:"response"`
}

// ResponseIncompleteStreamingEvent - response.incomplete
type ResponseIncompleteStreamingEvent struct {
	Type     string   `json:"type"` // "response.incomplete"
	Response Response `json:"response"`
}

// ResponseOutputItemAddedStreamingEvent - response.output_item.added
type ResponseOutputItemAddedStreamingEvent struct {
	Type        string    `json:"type"` // "response.output_item.added"
	ResponseID  string    `json:"response_id"`
	OutputIndex int       `json:"output_index"`
	Item        ItemField `json:"item"`
}

// ResponseOutputItemDoneStreamingEvent - response.output_item.done
type ResponseOutputItemDoneStreamingEvent struct {
	Type        string    `json:"type"` // "response.output_item.done"
	ResponseID  string    `json:"response_id"`
	OutputIndex int       `json:"output_index"`
	Item        ItemField `json:"item"`
}

// ResponseContentPartAddedStreamingEvent - response.content_part.added
type ResponseContentPartAddedStreamingEvent struct {
	Type         string      `json:"type"` // "response.content_part.added"
	ResponseID   string      `json:"response_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Part         ContentPart `json:"part"`
}

// ResponseContentPartDoneStreamingEvent - response.content_part.done
type ResponseContentPartDoneStreamingEvent struct {
	Type         string      `json:"type"` // "response.content_part.done"
	ResponseID   string      `json:"response_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Part         ContentPart `json:"part"`
}

// ResponseOutputTextDeltaStreamingEvent - response.output_text.delta
type ResponseOutputTextDeltaStreamingEvent struct {
	Type         string `json:"type"` // "response.output_text.delta"
	ResponseID   string `json:"response_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ResponseOutputTextDoneStreamingEvent - response.output_text.done
type ResponseOutputTextDoneStreamingEvent struct {
	Type         string `json:"type"` // "response.output_text.done"
	ResponseID   string `json:"response_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

// ResponseRefusalDeltaStreamingEvent - response.refusal.delta
type ResponseRefusalDeltaStreamingEvent struct {
	Type         string `json:"type"` // "response.refusal.delta"
	ResponseID   string `json:"response_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ResponseRefusalDoneStreamingEvent - response.refusal.done
type ResponseRefusalDoneStreamingEvent struct {
	Type         string `json:"type"` // "response.refusal.done"
	ResponseID   string `json:"response_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Refusal      string `json:"refusal"`
}

// ResponseReasoningDeltaStreamingEvent - response.reasoning.delta
type ResponseReasoningDeltaStreamingEvent struct {
	Type        string `json:"type"` // "response.reasoning.delta"
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

// ResponseReasoningDoneStreamingEvent - response.reasoning.done
type ResponseReasoningDoneStreamingEvent struct {
	Type        string `json:"type"` // "response.reasoning.done"
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Reasoning   string `json:"reasoning"`
}

// ResponseReasoningSummaryDeltaStreamingEvent - response.reasoning_summary.delta
type ResponseReasoningSummaryDeltaStreamingEvent struct {
	Type        string `json:"type"` // "response.reasoning_summary.delta"
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

// ResponseReasoningSummaryDoneStreamingEvent - response.reasoning_summary.done
type ResponseReasoningSummaryDoneStreamingEvent struct {
	Type        string `json:"type"` // "response.reasoning_summary.done"
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Summary     string `json:"summary"`
}

// ResponseReasoningSummaryPartAddedStreamingEvent - response.reasoning_summary_part.added
type ResponseReasoningSummaryPartAddedStreamingEvent struct {
	Type         string      `json:"type"` // "response.reasoning_summary_part.added"
	ResponseID   string      `json:"response_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Part         ContentPart `json:"part"`
}

// ResponseReasoningSummaryPartDoneStreamingEvent - response.reasoning_summary_part.done
type ResponseReasoningSummaryPartDoneStreamingEvent struct {
	Type         string      `json:"type"` // "response.reasoning_summary_part.done"
	ResponseID   string      `json:"response_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Part         ContentPart `json:"part"`
}

// ResponseOutputTextAnnotationAddedStreamingEvent - response.output_text_annotation.added
type ResponseOutputTextAnnotationAddedStreamingEvent struct {
	Type         string      `json:"type"` // "response.output_text_annotation.added"
	ResponseID   string      `json:"response_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Annotation   ContentPart `json:"annotation"`
}

// ResponseFunctionCallArgumentsDeltaStreamingEvent - response.function_call_arguments.delta
type ResponseFunctionCallArgumentsDeltaStreamingEvent struct {
	Type        string `json:"type"` // "response.function_call_arguments.delta"
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

// ResponseFunctionCallArgumentsDoneStreamingEvent - response.function_call_arguments.done
type ResponseFunctionCallArgumentsDoneStreamingEvent struct {
	Type        string `json:"type"` // "response.function_call_arguments.done"
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Arguments   string `json:"arguments"`
}

// ErrorStreamingEvent - error
type ErrorStreamingEvent struct {
	Type  string     `json:"type"` // "error"
	Error ErrorField `json:"error"`
}

// Validate validates the request
func (r *ResponseRequest) Validate() error {
	if r.Model == nil || *r.Model == "" {
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
		Output:    make([]ItemField, 0),
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
	r.Error = &ErrorField{
		Type:    errType,
		Code:    &code,
		Message: message,
	}
}

// MarkIncomplete marks the response as incomplete
func (r *Response) MarkIncomplete(reason string) {
	r.Status = "incomplete"
	r.IncompleteDetails = &IncompleteDetailsField{
		Reason: reason,
	}
}
