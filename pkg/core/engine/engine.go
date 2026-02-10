// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/core/state"
)

const defaultMaxToolCalls = 10

// Engine is the core orchestration engine for the Responses API
type Engine struct {
	config   *config.EngineConfig
	sessions state.SessionStore
	llm      api.ChatCompletionClient
}

// New creates a new Engine instance
func New(cfg *config.EngineConfig, store state.SessionStore) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if store == nil {
		return nil, fmt.Errorf("session store is required")
	}

	// Create chat completion client
	var llm api.ChatCompletionClient
	if cfg.APIKey != "" && cfg.ModelEndpoint != "" {
		// Use real OpenAI-compatible client
		llm = api.NewOpenAIClient(cfg.ModelEndpoint, cfg.APIKey)
	} else {
		// Use mock client for testing
		llm = api.NewMockChatCompletionClient()
	}

	return &Engine{
		config:   cfg,
		sessions: store,
		llm:      llm,
	}, nil
}

// LLMClient returns the chat completion client
func (e *Engine) LLMClient() api.ChatCompletionClient {
	return e.llm
}

// Store returns the session store
func (e *Engine) Store() state.SessionStore {
	return e.sessions
}

// echoRequestParams copies all request parameters to the response (Open Responses spec)
func echoRequestParams(resp *schema.Response, req *schema.ResponseRequest) {
	resp.PreviousResponseID = req.PreviousResponseID
	resp.Instructions = req.Instructions
	resp.Tools = convertToolsToResponse(req.Tools)
	if req.ToolChoice != nil {
		resp.ToolChoice = req.ToolChoice
	}
	resp.Reasoning = convertReasoningToResponse(req.Reasoning)
	if req.Temperature != nil {
		resp.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		resp.TopP = *req.TopP
	}
	resp.MaxOutputTokens = req.MaxOutputTokens
	resp.MaxToolCalls = req.MaxToolCalls
	if req.ParallelToolCalls != nil {
		resp.ParallelToolCalls = *req.ParallelToolCalls
	}
	if req.Store != nil {
		resp.Store = *req.Store
	}
	if req.FrequencyPenalty != nil {
		resp.FrequencyPenalty = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		resp.PresencePenalty = *req.PresencePenalty
	}
	if req.Truncation != nil {
		if req.Truncation.Type == "last_messages" {
			resp.Truncation = "disabled"
		} else {
			resp.Truncation = "auto"
		}
	} else {
		resp.Truncation = "auto"
	}
	if req.TopLogprobs != nil {
		resp.TopLogprobs = *req.TopLogprobs
	}
	if req.ServiceTier != nil {
		resp.ServiceTier = *req.ServiceTier
	}
	if req.Background != nil {
		resp.Background = *req.Background
	}
	resp.PromptCacheKey = req.PromptCacheKey
	resp.SafetyIdentifier = req.SafetyIdentifier
	resp.Metadata = req.Metadata
}

// extractInputMessages parses the Responses API input field into chat messages
func extractInputMessages(input interface{}) []api.Message {
	switch v := input.(type) {
	case string:
		return []api.Message{{Role: "user", Content: v}}
	case []interface{}:
		var messages []api.Message
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			itemType, _ := itemMap["type"].(string)
			switch itemType {
			case "message":
				role, _ := itemMap["role"].(string)
				content := extractContentFromItem(itemMap)
				if role != "" && content != "" {
					messages = append(messages, api.Message{Role: role, Content: content})
				}
			case "function_call_output":
				callID, _ := itemMap["call_id"].(string)
				output, _ := itemMap["output"].(string)
				if callID != "" {
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    output,
						ToolCallID: callID,
					})
				}
			case "function_call":
				callID, _ := itemMap["call_id"].(string)
				name, _ := itemMap["name"].(string)
				arguments, _ := itemMap["arguments"].(string)
				if name != "" {
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{
							{
								ID:   callID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      name,
									Arguments: arguments,
								},
							},
						},
					})
				}
			default:
				// Try to extract content for unknown types
				if content, ok := itemMap["content"].(string); ok && content != "" {
					role, _ := itemMap["role"].(string)
					if role == "" {
						role = "user"
					}
					messages = append(messages, api.Message{Role: role, Content: content})
				}
			}
		}
		if len(messages) == 0 {
			return []api.Message{{Role: "user", Content: fmt.Sprintf("%v", input)}}
		}
		return messages
	default:
		return []api.Message{{Role: "user", Content: fmt.Sprintf("%v", v)}}
	}
}

// extractContentFromItem extracts text content from a message input item
func extractContentFromItem(item map[string]interface{}) string {
	// Direct content string
	if content, ok := item["content"].(string); ok {
		return content
	}
	// Content as array of parts
	if contentArr, ok := item["content"].([]interface{}); ok {
		text := ""
		for _, part := range contentArr {
			if partMap, ok := part.(map[string]interface{}); ok {
				if partText, ok := partMap["text"].(string); ok {
					if text != "" {
						text += " "
					}
					text += partText
				}
			}
		}
		return text
	}
	return ""
}

// convertToolsForLLM converts Responses API tools to chat completion tools
func convertToolsForLLM(tools []schema.ResponsesToolParam) []api.Tool {
	var result []api.Tool
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		tool := api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:       t.Name,
				Parameters: t.Parameters,
			},
		}
		if t.Description != nil {
			tool.Function.Description = *t.Description
		}
		if t.Strict != nil {
			tool.Function.Strict = t.Strict
		}
		result = append(result, tool)
	}
	return result
}

// buildLLMRequest constructs a ChatCompletionRequest from a ResponseRequest
func buildLLMRequest(model string, messages []api.Message, req *schema.ResponseRequest, stream bool) *api.ChatCompletionRequest {
	llmReq := &api.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	}

	// Sampling parameters
	llmReq.Temperature = req.Temperature
	llmReq.TopP = req.TopP
	if req.FrequencyPenalty != nil {
		llmReq.FrequencyPenalty = req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		llmReq.PresencePenalty = req.PresencePenalty
	}

	// Token limits
	if req.MaxOutputTokens != nil {
		llmReq.MaxCompletionTokens = req.MaxOutputTokens
	}

	// Tools
	tools := convertToolsForLLM(req.Tools)
	if len(tools) > 0 {
		llmReq.Tools = tools
	}

	// ToolChoice
	if req.ToolChoice != nil {
		llmReq.ToolChoice = req.ToolChoice
	}

	// ParallelToolCalls
	llmReq.ParallelToolCalls = req.ParallelToolCalls

	// Reasoning effort
	if req.Reasoning != nil && req.Reasoning.Effort != nil {
		llmReq.ReasoningEffort = req.Reasoning.Effort
	}

	// PromptCacheKey
	llmReq.PromptCacheKey = req.PromptCacheKey

	// SafetyIdentifier
	llmReq.SafetyIdentifier = req.SafetyIdentifier

	return llmReq
}

// buildConversationMessages reconstructs conversation history for multi-turn
func (e *Engine) buildConversationMessages(ctx context.Context, req *schema.ResponseRequest) ([]api.Message, error) {
	var messages []api.Message

	// Load previous conversation if this is a follow-up
	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		prevResp, err := e.sessions.GetResponse(ctx, *req.PreviousResponseID)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous response %s: %w", *req.PreviousResponseID, err)
		}

		// Load stored messages from previous response
		for _, m := range prevResp.Messages {
			msg := api.Message{
				Role:       m.Role,
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			}
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					msg.ToolCalls = append(msg.ToolCalls, api.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: api.ToolCallFunction{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}
			messages = append(messages, msg)
		}

		// Append previous response output as context
		if output, ok := prevResp.Output.([]schema.ItemField); ok {
			for _, item := range output {
				switch item.Type {
				case "message":
					role := "assistant"
					if item.Role != nil {
						role = *item.Role
					}
					content := ""
					for _, part := range item.Content {
						if part.Text != nil {
							content += *part.Text
						}
					}
					if content != "" {
						messages = append(messages, api.Message{Role: role, Content: content})
					}
				case "function_call":
					name := ""
					if item.Name != nil {
						name = *item.Name
					}
					args := ""
					if item.Arguments != nil {
						args = *item.Arguments
					}
					callID := ""
					if item.CallID != nil {
						callID = *item.CallID
					}
					messages = append(messages, api.Message{
						Role: "assistant",
						ToolCalls: []api.ToolCall{
							{
								ID:   callID,
								Type: "function",
								Function: api.ToolCallFunction{
									Name:      name,
									Arguments: args,
								},
							},
						},
					})
				case "function_call_output":
					callID := ""
					if item.CallID != nil {
						callID = *item.CallID
					}
					output := ""
					if item.Output != nil {
						output = *item.Output
					}
					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    output,
						ToolCallID: callID,
					})
				}
			}
		}
	}

	// Add instructions as system message
	if req.Instructions != nil && *req.Instructions != "" {
		// Prepend system message if not already present
		hasSystem := false
		for _, m := range messages {
			if m.Role == "system" {
				hasSystem = true
				break
			}
		}
		if !hasSystem {
			messages = append([]api.Message{
				{Role: "system", Content: *req.Instructions},
			}, messages...)
		}
	}

	// Append current input
	inputMessages := extractInputMessages(req.Input)
	messages = append(messages, inputMessages...)

	return messages, nil
}

// messagesToConversationMessages converts api.Messages to state.ConversationMessages for storage
func messagesToConversationMessages(messages []api.Message) []state.ConversationMessage {
	result := make([]state.ConversationMessage, 0, len(messages))
	for _, m := range messages {
		cm := state.ConversationMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, state.ToolCallRecord{
				ID:        tc.ID,
				Type:      tc.Type,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		result = append(result, cm)
	}
	return result
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
	model := ""
	if req.Model != nil {
		model = *req.Model
	}
	resp := schema.NewResponse(respID, model)

	// 4. Echo ALL request parameters
	echoRequestParams(resp, req)

	// 5. Build conversation messages (including multi-turn history)
	messages, err := e.buildConversationMessages(ctx, req)
	if err != nil {
		resp.MarkFailed("api_error", "conversation_error", fmt.Sprintf("failed to build conversation: %v", err))
		return resp, nil
	}

	// 6. Agentic loop
	maxIters := defaultMaxToolCalls
	if req.MaxToolCalls != nil && *req.MaxToolCalls > 0 {
		maxIters = *req.MaxToolCalls
	}

	accumulatedOutputTokens := 0
	var allOutput []schema.ItemField

	for iter := 0; iter < maxIters; iter++ {
		// Build LLM request
		llmReq := buildLLMRequest(model, messages, req, false)

		// Adjust token budget if max_output_tokens is set
		if req.MaxOutputTokens != nil {
			remaining := *req.MaxOutputTokens - accumulatedOutputTokens
			if remaining <= 0 {
				resp.MarkIncomplete("max_output_tokens")
				break
			}
			llmReq.MaxCompletionTokens = &remaining
		}

		// Call LLM
		llmResp, err := e.llm.CreateChatCompletion(ctx, llmReq)
		if err != nil {
			resp.MarkFailed("api_error", "llm_error", fmt.Sprintf("failed to call LLM: %v", err))
			return resp, nil
		}

		accumulatedOutputTokens += llmResp.Usage.CompletionTokens

		if len(llmResp.Choices) == 0 {
			resp.MarkFailed("api_error", "no_choices", "LLM returned no choices")
			return resp, nil
		}

		choice := llmResp.Choices[0]

		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			// Emit function_call output items
			for _, tc := range choice.Message.ToolCalls {
				completedStatus := "completed"
				callID := tc.ID
				funcName := tc.Function.Name
				funcArgs := tc.Function.Arguments
				allOutput = append(allOutput, schema.ItemField{
					Type:      "function_call",
					ID:        generateID("fc_"),
					CallID:    &callID,
					Name:      &funcName,
					Arguments: &funcArgs,
					Status:    &completedStatus,
				})
			}

			// Append assistant message with tool calls to messages for storage
			messages = append(messages, api.Message{
				Role:      "assistant",
				ToolCalls: choice.Message.ToolCalls,
			})

			// Function tools are client-side - break to let client execute
			break
		}

		// Normal text response
		outputText := choice.Message.Content
		assistantRole := "assistant"
		completedStatus := "completed"
		allOutput = append(allOutput, schema.ItemField{
			Type:   "message",
			ID:     generateID("msg_"),
			Role:   &assistantRole,
			Status: &completedStatus,
			Content: []schema.ContentPart{
				{
					Type: "output_text",
					Text: &outputText,
				},
			},
		})

		// Append assistant message for storage
		messages = append(messages, api.Message{
			Role:    "assistant",
			Content: outputText,
		})

		// Set usage from LLM response
		resp.Usage = &schema.UsageField{
			InputTokens:  llmResp.Usage.PromptTokens,
			OutputTokens: accumulatedOutputTokens,
			TotalTokens:  llmResp.Usage.PromptTokens + accumulatedOutputTokens,
			InputTokensDetails: schema.InputTokensDetails{
				CachedTokens: 0,
			},
			OutputTokensDetails: schema.OutputTokensDetails{
				ReasoningTokens: 0,
			},
		}

		break
	}

	// 7. Set output
	resp.Output = allOutput
	if resp.Output == nil {
		resp.Output = make([]schema.ItemField, 0)
	}

	// 8. Set usage if not already set
	if resp.Usage == nil {
		resp.Usage = &schema.UsageField{
			InputTokensDetails:  schema.InputTokensDetails{},
			OutputTokensDetails: schema.OutputTokensDetails{},
		}
	}

	// 9. Mark as completed if not already marked
	if resp.Status == "in_progress" {
		resp.MarkCompleted()
	}

	// 10. Save response to state store
	prevRespID := ""
	if req.PreviousResponseID != nil {
		prevRespID = *req.PreviousResponseID
	}

	if err := e.sessions.SaveResponse(ctx, &state.Response{
		ID:                 resp.ID,
		PreviousResponseID: prevRespID,
		Request:            req,
		Output:             resp.Output,
		Status:             resp.Status,
		Usage:              resp.Usage,
		Messages:           messagesToConversationMessages(messages),
		CreatedAt:          time.Unix(resp.CreatedAt, 0),
		CompletedAt:        timePtr(resp.CompletedAt),
	}); err != nil {
		return nil, fmt.Errorf("failed to save response: %w", err)
	}

	return resp, nil
}

// ProcessRequestStream processes a streaming Responses API request
func (e *Engine) ProcessRequestStream(ctx context.Context, req *schema.ResponseRequest) (<-chan interface{}, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	events := make(chan interface{}, 10)

	go func() {
		defer close(events)

		respID := generateID("resp_")
		model := ""
		if req.Model != nil {
			model = *req.Model
		}
		resp := schema.NewResponse(respID, model)

		// Track sequence number for events
		seqNum := 0

		// Echo ALL request parameters
		echoRequestParams(resp, req)

		// Send response.created event
		events <- &schema.ResponseCreatedStreamingEvent{
			Type:           "response.created",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Save response on creation (in_progress)
		prevRespID := ""
		if req.PreviousResponseID != nil {
			prevRespID = *req.PreviousResponseID
		}
		_ = e.sessions.SaveResponse(ctx, &state.Response{
			ID:                 resp.ID,
			PreviousResponseID: prevRespID,
			Request:            req,
			Output:             resp.Output,
			Status:             "in_progress",
			CreatedAt:          time.Unix(resp.CreatedAt, 0),
		})

		// Build conversation messages (including multi-turn history)
		messages, err := e.buildConversationMessages(ctx, req)
		if err != nil {
			errorField := schema.ErrorField{
				Type:    "api_error",
				Message: fmt.Sprintf("failed to build conversation: %v", err),
			}
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: errorField,
			}
			return
		}

		// Send response.in_progress event
		resp.Status = "in_progress"
		events <- &schema.ResponseInProgressStreamingEvent{
			Type:           "response.in_progress",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Build LLM request
		llmReq := buildLLMRequest(model, messages, req, true)

		// Get streaming response from LLM
		streamChan, err := e.llm.CreateChatCompletionStream(ctx, llmReq)
		if err != nil {
			errorField := schema.ErrorField{
				Type:    "api_error",
				Message: fmt.Sprintf("failed to start streaming: %v", err),
			}
			events <- &schema.ErrorStreamingEvent{
				Type:  "error",
				Error: errorField,
			}
			return
		}

		// Track accumulated tool calls from the stream
		type accumulatedToolCall struct {
			ID        string
			Name      string
			Arguments string
		}
		var toolCallAccum []accumulatedToolCall
		var finishReason string

		fullText := ""
		contentIndex := 0
		outputIndex := 0
		messageItemID := ""
		messageItemEmitted := false

		// Stream deltas
		for chunk := range streamChan {
			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0]

			// Capture finish reason
			if delta.FinishReason != nil {
				finishReason = *delta.FinishReason
			}

			// Handle tool call deltas
			if len(delta.Delta.ToolCalls) > 0 {
				for _, tcDelta := range delta.Delta.ToolCalls {
					idx := tcDelta.Index
					// Extend accumulator as needed
					for len(toolCallAccum) <= idx {
						toolCallAccum = append(toolCallAccum, accumulatedToolCall{})
					}
					if tcDelta.ID != "" {
						toolCallAccum[idx].ID = tcDelta.ID
					}
					if tcDelta.Function.Name != "" {
						toolCallAccum[idx].Name = tcDelta.Function.Name
					}
					if tcDelta.Function.Arguments != "" {
						toolCallAccum[idx].Arguments += tcDelta.Function.Arguments

						// Emit function_call_arguments.delta event
						events <- &schema.ResponseFunctionCallArgumentsDeltaStreamingEvent{
							Type:        "response.function_call_arguments.delta",
							ResponseID:  respID,
							OutputIndex: idx,
							Delta:       tcDelta.Function.Arguments,
						}
						seqNum++
					}
				}
				continue
			}

			// Handle text content
			textDelta := delta.Delta.Content
			if textDelta == "" {
				continue
			}

			// Emit output_item.added on first text
			if !messageItemEmitted {
				messageItemID = generateID("msg_")
				assistantRole := "assistant"
				inProgressStatus := "in_progress"
				messageItem := schema.ItemField{
					Type:    "message",
					ID:      messageItemID,
					Role:    &assistantRole,
					Status:  &inProgressStatus,
					Content: []schema.ContentPart{},
				}

				events <- &schema.ResponseOutputItemAddedStreamingEvent{
					Type:           "response.output_item.added",
					SequenceNumber: seqNum,
					OutputIndex:    outputIndex,
					Item:           messageItem,
				}
				seqNum++

				// Emit content_part.added so the SDK knows about the content slot
				emptyText := ""
				events <- &schema.ResponseContentPartAddedStreamingEvent{
					Type:           "response.content_part.added",
					SequenceNumber: seqNum,
					ItemID:         messageItemID,
					OutputIndex:    outputIndex,
					ContentIndex:   contentIndex,
					Part: schema.ContentPart{
						Type: "output_text",
						Text: &emptyText,
					},
				}
				seqNum++

				messageItemEmitted = true
			}

			fullText += textDelta

			// Send text delta event
			events <- &schema.ResponseOutputTextDeltaStreamingEvent{
				Type:           "response.output_text.delta",
				SequenceNumber: seqNum,
				ItemID:         messageItemID,
				OutputIndex:    outputIndex,
				ContentIndex:   contentIndex,
				Delta:          textDelta,
				Logprobs:       make([]interface{}, 0),
			}
			seqNum++
		}

		// Determine output based on finish reason
		var allOutput []schema.ItemField

		if finishReason == "tool_calls" && len(toolCallAccum) > 0 {
			// Emit tool call output items
			for i, tc := range toolCallAccum {
				completedStatus := "completed"
				callID := tc.ID
				funcName := tc.Name
				funcArgs := tc.Arguments
				toolItem := schema.ItemField{
					Type:      "function_call",
					ID:        generateID("fc_"),
					CallID:    &callID,
					Name:      &funcName,
					Arguments: &funcArgs,
					Status:    &completedStatus,
				}

				events <- &schema.ResponseOutputItemAddedStreamingEvent{
					Type:           "response.output_item.added",
					SequenceNumber: seqNum,
					OutputIndex:    i,
					Item:           toolItem,
				}
				seqNum++

				// Emit function_call_arguments.done
				events <- &schema.ResponseFunctionCallArgumentsDoneStreamingEvent{
					Type:        "response.function_call_arguments.done",
					ResponseID:  respID,
					OutputIndex: i,
					Arguments:   funcArgs,
				}
				seqNum++

				// Emit output_item.done
				events <- &schema.ResponseOutputItemDoneStreamingEvent{
					Type:           "response.output_item.done",
					SequenceNumber: seqNum,
					OutputIndex:    i,
					Item:           toolItem,
				}
				seqNum++

				allOutput = append(allOutput, toolItem)
			}

			// Append assistant message with tool calls to messages for storage
			var tcForMsg []api.ToolCall
			for _, tc := range toolCallAccum {
				tcForMsg = append(tcForMsg, api.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: api.ToolCallFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
			messages = append(messages, api.Message{
				Role:      "assistant",
				ToolCalls: tcForMsg,
			})

		} else {
			// Normal text response
			if messageItemEmitted {
				// Send text done event
				events <- &schema.ResponseOutputTextDoneStreamingEvent{
					Type:           "response.output_text.done",
					SequenceNumber: seqNum,
					ItemID:         messageItemID,
					OutputIndex:    outputIndex,
					ContentIndex:   contentIndex,
					Text:           fullText,
					Logprobs:       make([]interface{}, 0),
				}
				seqNum++

				// Emit content_part.done
				events <- &schema.ResponseContentPartDoneStreamingEvent{
					Type:           "response.content_part.done",
					SequenceNumber: seqNum,
					ItemID:         messageItemID,
					OutputIndex:    outputIndex,
					ContentIndex:   contentIndex,
					Part: schema.ContentPart{
						Type: "output_text",
						Text: &fullText,
					},
				}
				seqNum++

				// Complete the message item
				completedStatus := "completed"
				assistantRole := "assistant"
				messageItem := schema.ItemField{
					Type:   "message",
					ID:     messageItemID,
					Role:   &assistantRole,
					Status: &completedStatus,
					Content: []schema.ContentPart{
						{
							Type: "output_text",
							Text: &fullText,
						},
					},
				}

				// Send output_item.done event
				events <- &schema.ResponseOutputItemDoneStreamingEvent{
					Type:           "response.output_item.done",
					SequenceNumber: seqNum,
					OutputIndex:    outputIndex,
					Item:           messageItem,
				}
				seqNum++

				allOutput = append(allOutput, messageItem)

				// Append assistant message for storage
				messages = append(messages, api.Message{
					Role:    "assistant",
					Content: fullText,
				})
			}
		}

		// Update response
		resp.Output = allOutput
		if resp.Output == nil {
			resp.Output = make([]schema.ItemField, 0)
		}

		resp.MarkCompleted()

		// Set usage stats
		inputLen := 0
		for _, m := range messages {
			inputLen += len(m.Content)
		}
		outputLen := len(fullText)
		for _, tc := range toolCallAccum {
			outputLen += len(tc.Arguments)
		}
		resp.Usage = &schema.UsageField{
			InputTokens:  inputLen / 4,
			OutputTokens: outputLen / 4,
			TotalTokens:  (inputLen + outputLen) / 4,
			InputTokensDetails: schema.InputTokensDetails{
				CachedTokens: 0,
			},
			OutputTokensDetails: schema.OutputTokensDetails{
				ReasoningTokens: 0,
			},
		}

		// Send response.completed event
		events <- &schema.ResponseCompletedStreamingEvent{
			Type:           "response.completed",
			SequenceNumber: seqNum,
			Response:       *resp,
		}
		seqNum++

		// Final save with complete state
		_ = e.sessions.SaveResponse(ctx, &state.Response{
			ID:                 resp.ID,
			PreviousResponseID: prevRespID,
			Request:            req,
			Output:             resp.Output,
			Status:             resp.Status,
			Usage:              resp.Usage,
			Messages:           messagesToConversationMessages(messages),
			CreatedAt:          time.Unix(resp.CreatedAt, 0),
			CompletedAt:        timePtr(resp.CompletedAt),
		})
	}()

	return events, nil
}

// Helper functions

func generateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func timePtr(t *int64) *time.Time {
	if t == nil {
		return nil
	}
	tm := time.Unix(*t, 0)
	return &tm
}

// convertToolsToResponse converts request tools to response tools
func convertToolsToResponse(reqTools []schema.ResponsesToolParam) []schema.ResponsesTool {
	if len(reqTools) == 0 {
		return make([]schema.ResponsesTool, 0)
	}
	respTools := make([]schema.ResponsesTool, len(reqTools))
	for i, t := range reqTools {
		respTools[i] = schema.ResponsesTool{
			Type:        t.Type,
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Strict:      t.Strict,
		}
	}
	return respTools
}

// convertReasoningToResponse converts request reasoning to response reasoning
func convertReasoningToResponse(reqReasoning *schema.ReasoningParam) *schema.ReasoningConfig {
	if reqReasoning == nil {
		return nil
	}
	return &schema.ReasoningConfig{
		Type:   reqReasoning.Type,
		Effort: reqReasoning.Effort,
		Budget: reqReasoning.Budget,
	}
}

// convertTruncationToResponse converts request truncation to response truncation
func convertTruncationToResponse(reqTruncation *schema.TruncationStrategyParam) *schema.TruncationStrategy {
	if reqTruncation == nil {
		return nil
	}
	return &schema.TruncationStrategy{
		Type:         reqTruncation.Type,
		LastMessages: reqTruncation.LastMessages,
	}
}

// GetResponse retrieves a response by ID from the session store
func (e *Engine) GetResponse(ctx context.Context, responseID string) (*schema.Response, error) {
	stateResp, err := e.sessions.GetResponse(ctx, responseID)
	if err != nil {
		return nil, fmt.Errorf("response not found: %w", err)
	}

	// Convert state.Response to schema.Response
	var model string
	if req, ok := stateResp.Request.(*schema.ResponseRequest); ok && req != nil && req.Model != nil {
		model = *req.Model
	}

	schemaResp := schema.NewResponse(stateResp.ID, model)
	schemaResp.Status = stateResp.Status

	// Type assert Output
	if output, ok := stateResp.Output.([]schema.ItemField); ok {
		schemaResp.Output = output
	}

	// Type assert Usage
	if usage, ok := stateResp.Usage.(*schema.UsageField); ok {
		schemaResp.Usage = usage
	}

	schemaResp.CreatedAt = stateResp.CreatedAt.Unix()
	if stateResp.CompletedAt != nil {
		completedAt := stateResp.CompletedAt.Unix()
		schemaResp.CompletedAt = &completedAt
	}

	// Echo request parameters if available
	if req, ok := stateResp.Request.(*schema.ResponseRequest); ok && req != nil {
		schemaResp.PreviousResponseID = req.PreviousResponseID
		schemaResp.Instructions = req.Instructions

		if req.Temperature != nil {
			temp := *req.Temperature
			schemaResp.Temperature = temp
		}
		if req.TopP != nil {
			topP := *req.TopP
			schemaResp.TopP = topP
		}

		schemaResp.MaxOutputTokens = req.MaxOutputTokens
		if req.Store != nil {
			schemaResp.Store = *req.Store
		}
		schemaResp.Metadata = req.Metadata

		if req.Tools != nil {
			schemaResp.Tools = convertToolsToResponse(req.Tools)
		}

		if req.Reasoning != nil {
			schemaResp.Reasoning = convertReasoningToResponse(req.Reasoning)
		}
	}

	return schemaResp, nil
}

// ListResponses retrieves a paginated list of responses
func (e *Engine) ListResponses(ctx context.Context, after, before string, limit int, order, model string) ([]*schema.Response, bool, error) {
	stateResponses, hasMore, err := e.sessions.ListResponsesPaginated(ctx, after, before, limit, order, model)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list responses: %w", err)
	}

	// Convert state.Response to schema.Response
	responses := make([]*schema.Response, 0, len(stateResponses))
	for _, stateResp := range stateResponses {
		var modelName string
		if req, ok := stateResp.Request.(*schema.ResponseRequest); ok && req != nil && req.Model != nil {
			modelName = *req.Model
		}

		schemaResp := schema.NewResponse(stateResp.ID, modelName)
		schemaResp.Status = stateResp.Status

		if output, ok := stateResp.Output.([]schema.ItemField); ok {
			schemaResp.Output = output
		}
		if usage, ok := stateResp.Usage.(*schema.UsageField); ok {
			schemaResp.Usage = usage
		}

		schemaResp.CreatedAt = stateResp.CreatedAt.Unix()
		if stateResp.CompletedAt != nil {
			completedAt := stateResp.CompletedAt.Unix()
			schemaResp.CompletedAt = &completedAt
		}

		responses = append(responses, schemaResp)
	}

	return responses, hasMore, nil
}

// DeleteResponse deletes a response by ID
func (e *Engine) DeleteResponse(ctx context.Context, responseID string) error {
	if err := e.sessions.DeleteResponse(ctx, responseID); err != nil {
		return fmt.Errorf("failed to delete response: %w", err)
	}
	return nil
}

// GetResponseInputItems retrieves input items for a specific response
func (e *Engine) GetResponseInputItems(ctx context.Context, responseID string) (interface{}, error) {
	items, err := e.sessions.GetResponseInputItems(ctx, responseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get response input items: %w", err)
	}
	return items, nil
}
