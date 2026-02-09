// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/leseb/openai-responses-gateway/pkg/core/api"
)

// handleChatCompletions handles POST /v1/chat/completions
// This is a direct pass-through to the LLM backend (OpenAI-compatible)
func (h *Handler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req api.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse chat completion request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	h.logger.Info("Processing chat completion request",
		"model", req.Model,
		"messages", len(req.Messages),
		"stream", req.Stream)

	// Handle streaming vs non-streaming
	if req.Stream {
		h.handleChatCompletionStream(w, r, &req)
		return
	}

	// Non-streaming chat completion
	// Get the LLM client directly from engine
	llmClient := h.engine.LLMClient()

	resp, err := llmClient.CreateChatCompletion(r.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create chat completion", "error", err)
		h.writeError(w, http.StatusInternalServerError, "completion_error", err.Error())
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	h.logger.Info("Chat completion sent",
		"completion_id", resp.ID,
		"model", resp.Model,
		"usage_tokens", resp.Usage.TotalTokens)
}

// handleChatCompletionStream handles streaming chat completions
func (h *Handler) handleChatCompletionStream(w http.ResponseWriter, r *http.Request, req *api.ChatCompletionRequest) {
	// Check if streaming is supported
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "streaming_not_supported", "Streaming not supported")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Get the LLM client directly from engine
	llmClient := h.engine.LLMClient()

	// Get chunk stream
	chunks, err := llmClient.CreateChatCompletionStream(r.Context(), req)
	if err != nil {
		h.logger.Error("Failed to start chat completion stream", "error", err)
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Stream chunks
	for chunk := range chunks {
		data, err := json.Marshal(chunk)
		if err != nil {
			h.logger.Error("Failed to marshal chunk", "error", err)
			continue
		}

		// Write SSE event (OpenAI format doesn't use event: line, just data:)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Check if this is the last chunk
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != nil {
			break
		}
	}

	// Send [DONE] message
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	h.logger.Info("Chat completion streaming completed")
}
