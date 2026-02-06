// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/leseb/openai-responses-gateway/pkg/core/engine"
	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
	"github.com/leseb/openai-responses-gateway/pkg/observability/logging"
)

// Handler implements the HTTP adapter
type Handler struct {
	engine *engine.Engine
	logger *logging.Logger
	mux    *http.ServeMux
}

// New creates a new HTTP handler
func New(eng *engine.Engine, logger *logging.Logger) *Handler {
	h := &Handler{
		engine: eng,
		logger: logger,
		mux:    http.NewServeMux(),
	}

	// Register routes
	h.mux.HandleFunc("GET /health", h.handleHealth)
	h.mux.HandleFunc("POST /v1/responses", h.handleResponses)

	return h
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Log request
	h.logger.Info("Request",
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr)

	// Serve
	h.mux.ServeHTTP(w, r)
}

// handleHealth handles health check requests
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleResponses handles /v1/responses requests
func (h *Handler) handleResponses(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req schema.ResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Log request
	h.logger.Info("Processing response request",
		"model", req.Model,
		"stream", req.Stream)

	// Handle streaming vs non-streaming
	if req.Stream {
		h.handleStreamingResponse(w, r, &req)
		return
	}

	// Non-streaming response
	resp, err := h.engine.ProcessRequest(r.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to process request", "error", err)
		h.writeError(w, http.StatusInternalServerError, "processing_error", err.Error())
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	h.logger.Info("Response sent",
		"response_id", resp.ID,
		"status", resp.Status)
}

// handleStreamingResponse handles SSE streaming
func (h *Handler) handleStreamingResponse(w http.ResponseWriter, r *http.Request, req *schema.ResponseRequest) {
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

	// Get event stream
	events, err := h.engine.ProcessRequestStream(r.Context(), req)
	if err != nil {
		h.logger.Error("Failed to start streaming", "error", err)
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Stream events
	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			h.logger.Error("Failed to marshal event", "error", err)
			continue
		}

		// Write SSE event
		fmt.Fprintf(w, "event: %s\n", event.Type)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	h.logger.Info("Streaming completed")
}

// writeError writes an error response
func (h *Handler) writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
}
