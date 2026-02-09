// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/leseb/openai-responses-gateway/pkg/core/engine"
	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
	"github.com/leseb/openai-responses-gateway/pkg/core/services"
	"github.com/leseb/openai-responses-gateway/pkg/observability/logging"
	"github.com/leseb/openai-responses-gateway/pkg/storage/memory"
)

// Handler implements the HTTP adapter
type Handler struct {
	engine            *engine.Engine
	logger            *logging.Logger
	mux               *http.ServeMux
	modelsService     *services.ModelsService
	promptsStore      *memory.PromptsStore
	filesStore        *memory.FilesStore
	vectorStoresStore *memory.VectorStoresStore
}

// New creates a new HTTP handler
func New(eng *engine.Engine, logger *logging.Logger, modelsService *services.ModelsService, promptsStore *memory.PromptsStore, filesStore *memory.FilesStore, vectorStoresStore *memory.VectorStoresStore) *Handler {
	h := &Handler{
		engine:            eng,
		logger:            logger,
		mux:               http.NewServeMux(),
		modelsService:     modelsService,
		promptsStore:      promptsStore,
		filesStore:        filesStore,
		vectorStoresStore: vectorStoresStore,
	}

	// Register routes
	h.mux.HandleFunc("GET /health", h.handleHealth)
	h.mux.HandleFunc("GET /openapi.json", h.handleOpenAPI)

	// Responses API
	h.mux.HandleFunc("POST /v1/responses", h.handleResponses)
	h.mux.HandleFunc("GET /v1/responses", h.handleListResponses)
	h.mux.HandleFunc("GET /v1/responses/{id}", h.handleGetResponse)
	h.mux.HandleFunc("DELETE /v1/responses/{id}", h.handleDeleteResponse)
	h.mux.HandleFunc("GET /v1/responses/{id}/input_items", h.handleListResponseInputItems)

	// Chat Completions API (direct backend access)
	h.mux.HandleFunc("POST /v1/chat/completions", h.handleChatCompletions)

	// Conversations API
	h.mux.HandleFunc("POST /v1/conversations", h.handleCreateConversation)
	h.mux.HandleFunc("GET /v1/conversations", h.handleListConversations)
	h.mux.HandleFunc("GET /v1/conversations/{id}", h.handleGetConversation)
	h.mux.HandleFunc("DELETE /v1/conversations/{id}", h.handleDeleteConversation)
	h.mux.HandleFunc("POST /v1/conversations/{id}/items", h.handleAddConversationItems)
	h.mux.HandleFunc("GET /v1/conversations/{id}/items", h.handleListConversationItems)

	// Models API
	h.mux.HandleFunc("GET /v1/models", h.handleListModels)
	h.mux.HandleFunc("GET /v1/models/{id}", h.handleGetModel)

	// Prompts API
	h.mux.HandleFunc("POST /v1/prompts", h.handleCreatePrompt)
	h.mux.HandleFunc("GET /v1/prompts", h.handleListPrompts)
	h.mux.HandleFunc("GET /v1/prompts/{id}", h.handleGetPrompt)
	h.mux.HandleFunc("PUT /v1/prompts/{id}", h.handleUpdatePrompt)
	h.mux.HandleFunc("DELETE /v1/prompts/{id}", h.handleDeletePrompt)

	// Files API
	h.mux.HandleFunc("POST /v1/files", h.handleUploadFile)
	h.mux.HandleFunc("GET /v1/files", h.handleListFiles)
	h.mux.HandleFunc("GET /v1/files/{id}", h.handleGetFile)
	h.mux.HandleFunc("GET /v1/files/{id}/content", h.handleGetFileContent)
	h.mux.HandleFunc("DELETE /v1/files/{id}", h.handleDeleteFile)

	// Vector Stores API
	h.mux.HandleFunc("POST /v1/vector_stores", h.handleCreateVectorStore)
	h.mux.HandleFunc("GET /v1/vector_stores", h.handleListVectorStores)
	h.mux.HandleFunc("GET /v1/vector_stores/{id}", h.handleGetVectorStore)
	h.mux.HandleFunc("PUT /v1/vector_stores/{id}", h.handleUpdateVectorStore)
	h.mux.HandleFunc("DELETE /v1/vector_stores/{id}", h.handleDeleteVectorStore)
	h.mux.HandleFunc("POST /v1/vector_stores/{id}/files", h.handleAddVectorStoreFile)
	h.mux.HandleFunc("GET /v1/vector_stores/{id}/files", h.handleListVectorStoreFiles)

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

// generateID generates a unique ID with a prefix
func generateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
