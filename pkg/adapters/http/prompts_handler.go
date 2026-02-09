// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
	"github.com/leseb/openai-responses-gateway/pkg/storage/memory"
)

// handleCreatePrompt handles POST /v1/prompts
func (h *Handler) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req schema.CreatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse prompt request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Name is required")
		return
	}
	if req.Template == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Template is required")
		return
	}

	// Create prompt
	promptID := generateID("prompt_")
	now := time.Now()

	prompt := &memory.Prompt{
		ID:          promptID,
		Name:        req.Name,
		Description: req.Description,
		Template:    req.Template,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    convertMetadata(req.Metadata),
	}

	err := h.promptsStore.CreatePrompt(r.Context(), prompt)
	if err != nil {
		h.logger.Error("Failed to create prompt", "error", err)
		h.writeError(w, http.StatusInternalServerError, "creation_error", err.Error())
		return
	}

	h.logger.Info("Prompt created", "prompt_id", promptID)

	// Return prompt
	schemaPrompt := schema.Prompt{
		ID:          prompt.ID,
		Object:      "prompt",
		Name:        prompt.Name,
		Description: prompt.Description,
		Template:    prompt.Template,
		Variables:   prompt.Variables,
		CreatedAt:   prompt.CreatedAt.Unix(),
		UpdatedAt:   prompt.UpdatedAt.Unix(),
		Metadata:    convertMetadataToInterface(prompt.Metadata),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaPrompt)
}

// handleListPrompts handles GET /v1/prompts
func (h *Handler) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
	order := query.Get("order")
	if order == "" {
		order = "desc"
	}

	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	h.logger.Info("Listing prompts", "after", after, "limit", limit, "order", order)

	// Get prompts from storage
	prompts, hasMore, err := h.promptsStore.ListPromptsPaginated(
		r.Context(), after, before, limit, order,
	)
	if err != nil {
		h.logger.Error("Failed to list prompts", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	schemaPrompts := make([]schema.Prompt, 0, len(prompts))
	for _, prompt := range prompts {
		p := schema.Prompt{
			ID:          prompt.ID,
			Object:      "prompt",
			Name:        prompt.Name,
			Description: prompt.Description,
			Template:    prompt.Template,
			Variables:   prompt.Variables,
			CreatedAt:   prompt.CreatedAt.Unix(),
			UpdatedAt:   prompt.UpdatedAt.Unix(),
			Metadata:    convertMetadataToInterface(prompt.Metadata),
		}
		schemaPrompts = append(schemaPrompts, p)
	}

	// Build response
	listResp := schema.ListPromptsResponse{
		Object:  "list",
		Data:    schemaPrompts,
		HasMore: hasMore,
	}

	if len(schemaPrompts) > 0 {
		listResp.FirstID = schemaPrompts[0].ID
		listResp.LastID = schemaPrompts[len(schemaPrompts)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleGetPrompt handles GET /v1/prompts/{id}
func (h *Handler) handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	// Extract prompt ID from path
	promptID := r.PathValue("id")
	if promptID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Prompt ID is required")
		return
	}

	h.logger.Info("Getting prompt", "prompt_id", promptID)

	// Get prompt from storage
	prompt, err := h.promptsStore.GetPrompt(r.Context(), promptID)
	if err != nil {
		h.logger.Error("Failed to get prompt", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		return
	}

	// Convert to schema
	schemaPrompt := schema.Prompt{
		ID:          prompt.ID,
		Object:      "prompt",
		Name:        prompt.Name,
		Description: prompt.Description,
		Template:    prompt.Template,
		Variables:   prompt.Variables,
		CreatedAt:   prompt.CreatedAt.Unix(),
		UpdatedAt:   prompt.UpdatedAt.Unix(),
		Metadata:    convertMetadataToInterface(prompt.Metadata),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaPrompt)
}

// handleUpdatePrompt handles PUT /v1/prompts/{id}
func (h *Handler) handleUpdatePrompt(w http.ResponseWriter, r *http.Request) {
	// Extract prompt ID from path
	promptID := r.PathValue("id")
	if promptID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Prompt ID is required")
		return
	}

	// Parse request body
	var req schema.UpdatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse update request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	h.logger.Info("Updating prompt", "prompt_id", promptID)

	// Get existing prompt
	prompt, err := h.promptsStore.GetPrompt(r.Context(), promptID)
	if err != nil {
		h.logger.Error("Failed to get prompt", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		return
	}

	// Update fields if provided
	if req.Name != nil {
		prompt.Name = *req.Name
	}
	if req.Description != nil {
		prompt.Description = *req.Description
	}
	if req.Template != nil {
		prompt.Template = *req.Template
	}
	if req.Metadata != nil {
		prompt.Metadata = convertMetadata(req.Metadata)
	}

	// Update in storage
	err = h.promptsStore.UpdatePrompt(r.Context(), prompt)
	if err != nil {
		h.logger.Error("Failed to update prompt", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusInternalServerError, "update_error", err.Error())
		return
	}

	// Convert to schema
	schemaPrompt := schema.Prompt{
		ID:          prompt.ID,
		Object:      "prompt",
		Name:        prompt.Name,
		Description: prompt.Description,
		Template:    prompt.Template,
		Variables:   prompt.Variables,
		CreatedAt:   prompt.CreatedAt.Unix(),
		UpdatedAt:   prompt.UpdatedAt.Unix(),
		Metadata:    convertMetadataToInterface(prompt.Metadata),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaPrompt)
}

// handleDeletePrompt handles DELETE /v1/prompts/{id}
func (h *Handler) handleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	// Extract prompt ID from path
	promptID := r.PathValue("id")
	if promptID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Prompt ID is required")
		return
	}

	h.logger.Info("Deleting prompt", "prompt_id", promptID)

	// Delete prompt from storage
	err := h.promptsStore.DeletePrompt(r.Context(), promptID)
	if err != nil {
		h.logger.Error("Failed to delete prompt", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		return
	}

	// Return deletion confirmation
	deleteResp := schema.DeletePromptResponse{
		ID:      promptID,
		Object:  "prompt.deleted",
		Deleted: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}
