// Copyright Open Responses Gateway Authors
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

// convertMessages converts our Message types to OpenAI SDK message params
func convertMessages(messages []Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))
		case "user":
			if len(msg.ContentParts) > 0 {
				parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.ContentParts))
				for _, cp := range msg.ContentParts {
					switch cp.Type {
					case "text":
						parts = append(parts, openai.TextContentPart(cp.Text))
					case "image_url":
						if cp.ImageURL != nil {
							imgParam := openai.ChatCompletionContentPartImageImageURLParam{
								URL: cp.ImageURL.URL,
							}
							if cp.ImageURL.Detail != "" {
								imgParam.Detail = cp.ImageURL.Detail
							}
							parts = append(parts, openai.ImageContentPart(imgParam))
						}
					case "file":
						if cp.File != nil {
							fileParam := openai.ChatCompletionContentPartFileFileParam{}
							if cp.File.FileData != "" {
								fileParam.FileData = openai.String(cp.File.FileData)
							}
							if cp.File.FileID != "" {
								fileParam.FileID = openai.String(cp.File.FileID)
							}
							if cp.File.Filename != "" {
								fileParam.Filename = openai.String(cp.File.Filename)
							}
							parts = append(parts, openai.FileContentPart(fileParam))
						}
					}
				}
				result = append(result, openai.UserMessage(parts))
			} else {
				result = append(result, openai.UserMessage(msg.Content))
			}
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				toolCalls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
				assistantMsg := &openai.ChatCompletionAssistantMessageParam{
					ToolCalls: toolCalls,
				}
				if msg.Content != "" {
					assistantMsg.Content.OfString = openai.String(msg.Content)
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfAssistant: assistantMsg,
				})
			} else {
				result = append(result, openai.AssistantMessage(msg.Content))
			}
		case "tool":
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		case "developer":
			result = append(result, openai.DeveloperMessage(msg.Content))
		default:
			return nil, fmt.Errorf("unsupported message role: %s", msg.Role)
		}
	}
	return result, nil
}

// buildParams constructs OpenAI SDK ChatCompletionNewParams from our ChatCompletionRequest
func buildParams(req *ChatCompletionRequest, messages []openai.ChatCompletionMessageParamUnion) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}

	// Sampling parameters
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if req.TopP != nil {
		params.TopP = openai.Float(*req.TopP)
	}
	if req.FrequencyPenalty != nil {
		params.FrequencyPenalty = openai.Float(*req.FrequencyPenalty)
	}
	if req.PresencePenalty != nil {
		params.PresencePenalty = openai.Float(*req.PresencePenalty)
	}

	// Token limits: prefer MaxCompletionTokens, fall back to MaxTokens
	if req.MaxCompletionTokens != nil {
		params.MaxCompletionTokens = openai.Int(int64(*req.MaxCompletionTokens))
	} else if req.MaxTokens != nil {
		params.MaxTokens = openai.Int(int64(*req.MaxTokens))
	}

	// Tools
	if len(req.Tools) > 0 {
		tools := make([]openai.ChatCompletionToolParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			funcDef := shared.FunctionDefinitionParam{
				Name: t.Function.Name,
			}
			if t.Function.Description != "" {
				funcDef.Description = openai.String(t.Function.Description)
			}
			if t.Function.Parameters != nil {
				funcDef.Parameters = shared.FunctionParameters(t.Function.Parameters)
			}
			if t.Function.Strict != nil {
				funcDef.Strict = openai.Bool(*t.Function.Strict)
			}
			tools = append(tools, openai.ChatCompletionToolParam{
				Function: funcDef,
			})
		}
		params.Tools = tools
	}

	// ToolChoice
	if req.ToolChoice != nil {
		switch tc := req.ToolChoice.(type) {
		case string:
			// "none", "auto", "required"
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: openai.String(tc),
			}
		case map[string]interface{}:
			// Named tool choice: {"type":"function","function":{"name":"..."}}
			if fnMap, ok := tc["function"].(map[string]interface{}); ok {
				if name, ok := fnMap["name"].(string); ok {
					params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
						OfChatCompletionNamedToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
							Function: openai.ChatCompletionNamedToolChoiceFunctionParam{
								Name: name,
							},
						},
					}
				}
			}
		}
	}

	// ParallelToolCalls
	if req.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.Bool(*req.ParallelToolCalls)
	}

	// ResponseFormat
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case "json_object":
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			}
		case "json_schema":
			if req.ResponseFormat.JSONSchema != nil {
				js := req.ResponseFormat.JSONSchema
				schemaParam := shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name: js.Name,
				}
				if js.Description != "" {
					schemaParam.Description = openai.String(js.Description)
				}
				if js.Schema != nil {
					schemaParam.Schema = js.Schema
				}
				if js.Strict != nil {
					schemaParam.Strict = openai.Bool(*js.Strict)
				}
				params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
					OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
						JSONSchema: schemaParam,
					},
				}
			}
		case "text":
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfText: &shared.ResponseFormatTextParam{},
			}
		}
	}

	// Seed
	if req.Seed != nil {
		params.Seed = openai.Int(*req.Seed)
	}

	// Logprobs
	if req.Logprobs != nil {
		params.Logprobs = openai.Bool(*req.Logprobs)
	}
	if req.TopLogprobs != nil {
		params.TopLogprobs = openai.Int(int64(*req.TopLogprobs))
	}

	// ReasoningEffort
	if req.ReasoningEffort != nil {
		params.ReasoningEffort = shared.ReasoningEffort(*req.ReasoningEffort)
	}

	// PromptCacheKey
	if req.PromptCacheKey != nil {
		params.PromptCacheKey = openai.String(*req.PromptCacheKey)
	}

	// SafetyIdentifier
	if req.SafetyIdentifier != nil {
		params.SafetyIdentifier = openai.String(*req.SafetyIdentifier)
	}

	return params
}

// extractToolCalls converts SDK tool calls to our ToolCall types
func extractToolCalls(sdkToolCalls []openai.ChatCompletionMessageToolCall) []ToolCall {
	if len(sdkToolCalls) == 0 {
		return nil
	}
	result := make([]ToolCall, 0, len(sdkToolCalls))
	for _, tc := range sdkToolCalls {
		result = append(result, ToolCall{
			ID:   tc.ID,
			Type: string(tc.Type),
			Function: ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}

// CreateChatCompletion implements ChatCompletionClient.CreateChatCompletion
func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	messages, err := convertMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	params := buildParams(req, messages)

	// Call OpenAI API
	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Convert response to our format
	choices := make([]Choice, 0, len(completion.Choices))
	for _, choice := range completion.Choices {
		msg := Message{
			Role:      string(choice.Message.Role),
			Content:   choice.Message.Content,
			ToolCalls: extractToolCalls(choice.Message.ToolCalls),
		}
		choices = append(choices, Choice{
			Index:        int(choice.Index),
			Message:      msg,
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
	messages, err := convertMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	params := buildParams(req, messages)

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

				// Convert tool call deltas
				if len(choice.Delta.ToolCalls) > 0 {
					toolCallDeltas := make([]ToolCallDelta, 0, len(choice.Delta.ToolCalls))
					for _, tc := range choice.Delta.ToolCalls {
						toolCallDeltas = append(toolCallDeltas, ToolCallDelta{
							Index: int(tc.Index),
							ID:    tc.ID,
							Type:  string(tc.Type),
							Function: ToolCallFunctionDelta{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						})
					}
					delta.Delta.ToolCalls = toolCallDeltas
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
			_ = err
		}
	}()

	return chunks, nil
}
