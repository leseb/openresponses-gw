// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
)

// toSchemaPrompt converts a memory.Prompt to a schema.Prompt
func toSchemaPrompt(p *memory.Prompt) schema.Prompt {
	return schema.Prompt{
		ID:          p.ID,
		Object:      "prompt",
		Name:        p.Name,
		Description: p.Description,
		Template:    p.Template,
		Variables:   p.Variables,
		Version:     p.Version,
		IsDefault:   p.IsDefault,
		CreatedAt:   p.CreatedAt.Unix(),
		UpdatedAt:   p.UpdatedAt.Unix(),
		Metadata:    convertMetadataToInterface(p.Metadata),
	}
}

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toSchemaPrompt(prompt))
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
		schemaPrompts = append(schemaPrompts, toSchemaPrompt(prompt))
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

	// Check for optional ?version=N query param
	var prompt *memory.Prompt
	var err error

	if versionStr := r.URL.Query().Get("version"); versionStr != "" {
		version, parseErr := strconv.Atoi(versionStr)
		if parseErr != nil || version < 1 {
			h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid version number")
			return
		}
		prompt, err = h.promptsStore.GetPromptVersion(r.Context(), promptID, version)
	} else {
		prompt, err = h.promptsStore.GetPrompt(r.Context(), promptID)
	}

	if err != nil {
		h.logger.Error("Failed to get prompt", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toSchemaPrompt(prompt))
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

	// Validate version field is provided
	if req.Version == 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "version is required")
		return
	}

	h.logger.Info("Updating prompt", "prompt_id", promptID, "version", req.Version)

	// Build updates
	updates := &memory.Prompt{}
	if req.Name != nil {
		updates.Name = *req.Name
	}
	if req.Description != nil {
		updates.Description = *req.Description
	}
	if req.Template != nil {
		updates.Template = *req.Template
	}
	if req.Metadata != nil {
		updates.Metadata = convertMetadata(req.Metadata)
	}

	// Create new version in storage
	newPrompt, err := h.promptsStore.UpdatePrompt(r.Context(), promptID, req.Version, updates, req.SetAsDefault)
	if err != nil {
		h.logger.Error("Failed to update prompt", "error", err, "prompt_id", promptID)
		// Return 409 for version mismatch, 404 for not found
		if isVersionMismatch(err) {
			h.writeError(w, http.StatusConflict, "version_conflict", err.Error())
		} else {
			h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toSchemaPrompt(newPrompt))
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

// handleListPromptVersions handles GET /v1/prompts/{id}/versions
func (h *Handler) handleListPromptVersions(w http.ResponseWriter, r *http.Request) {
	promptID := r.PathValue("id")
	if promptID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Prompt ID is required")
		return
	}

	h.logger.Info("Listing prompt versions", "prompt_id", promptID)

	versions, err := h.promptsStore.ListPromptVersions(r.Context(), promptID)
	if err != nil {
		h.logger.Error("Failed to list prompt versions", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		return
	}

	schemaPrompts := make([]schema.Prompt, 0, len(versions))
	for _, prompt := range versions {
		schemaPrompts = append(schemaPrompts, toSchemaPrompt(prompt))
	}

	listResp := schema.ListPromptsResponse{
		Object:  "list",
		Data:    schemaPrompts,
		HasMore: false,
	}

	if len(schemaPrompts) > 0 {
		listResp.FirstID = schemaPrompts[0].ID
		listResp.LastID = schemaPrompts[len(schemaPrompts)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleSetDefaultVersion handles POST /v1/prompts/{id}/default_version
func (h *Handler) handleSetDefaultVersion(w http.ResponseWriter, r *http.Request) {
	promptID := r.PathValue("id")
	if promptID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Prompt ID is required")
		return
	}

	var req schema.SetDefaultVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse set default version request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	if req.Version < 1 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "version must be >= 1")
		return
	}

	h.logger.Info("Setting default version", "prompt_id", promptID, "version", req.Version)

	prompt, err := h.promptsStore.SetDefaultVersion(r.Context(), promptID, req.Version)
	if err != nil {
		h.logger.Error("Failed to set default version", "error", err, "prompt_id", promptID)
		h.writeError(w, http.StatusNotFound, "prompt_not_found", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toSchemaPrompt(prompt))
}

// isVersionMismatch checks if an error is a version mismatch error
func isVersionMismatch(err error) bool {
	return err != nil && len(err.Error()) > 17 && err.Error()[:17] == "version mismatch:"
}
